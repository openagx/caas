/**
 * Two-Layer Authorization Middleware
 * 
 * Layer 1: SpiceDB (ReBAC) - determines relationship permission
 * Layer 2: OPA Sidecar (ABAC + Behavioral + Drift) - contextual allow/deny
 * 
 * Final decision: ALLOW only if SpiceDB = ALLOW AND OPA = ALLOW
 */

import type { FastifyRequest, FastifyReply } from "fastify";
import type { AuthzClient } from "../grpc-client.js";
import { buildOPAAuthZInput, type OPAAuthZInput, type OPADecision } from "./input-builder.js";

export interface AuthZMiddlewareOptions {
  authzClient: AuthzClient;
  opaUrl: string;
  timeoutMs?: number;
  enabled?: boolean;
}

/**
 * Creates the two-layer authZ middleware for Fastify
 */
export function createTwoLayerAuthZMiddleware(options: AuthZMiddlewareOptions) {
  const { authzClient, opaUrl, timeoutMs = 2000, enabled = true } = options;

  /**
   * Two-layer authorization middleware
   * 
   * Flow: request → SpiceDB → OPA → decision
   */
  return async function authZMiddleware(
    request: FastifyRequest,
    reply: FastifyReply
  ): Promise<void> {
    // Skip if disabled or public path
    if (!enabled) return;

    const publicPaths = ["/health", "/ready", "/metrics", "/docs", "/.well-known", "/auth/"];
    const isPublic = publicPaths.some((p) => request.url.startsWith(p));
    if (isPublic) return;

    // Get user from request (set by auth middleware)
    const user = (request as any).logtoUser;
    if (!user) {
      reply.code(401).send({
        error: "Unauthorized",
        message: "Authentication required for authorization check",
      });
      return;
    }

    const entityId = user.custom_data?.entity_id || user.sub;
    if (!entityId) {
      reply.code(403).send({
        error: "Forbidden",
        message: "No entity ID associated with user",
      });
      return;
    }

    // Extract resource info from request
    const { resourceType, resourceId } = extractResourceFromRequest(request);
    if (!resourceType || !resourceId) {
      // Not a resource request, allow through
      return;
    }

    // Extract session/context info
    const sessionContext = extractSessionContext(request);

    try {
      // === LAYER 1: SpiceDB Check ===
      const startTime = Date.now();

      const spiceDBResult = await Promise.race([
        authzClient.check({
          subject: {
            entityType: "user",
            entityId,
          },
          resource: {
            resourceType,
            resourceId,
          },
          permission: "access",
        }),
        new Promise<never>((_, reject) =>
          setTimeout(() => reject(new Error("SpiceDB timeout")), timeoutMs)
        ),
      ]);

      const spiceDBLatency = Date.now() - startTime;
      const traceId = crypto.randomUUID();

      request.log.info({
        layer: "spicedb",
        traceId,
       _allowed: spiceDBResult.allowed,
        resourceType,
        resourceId,
        latency_ms: spiceDBLatency,
      });

      // === LAYER 2: OPA Sidecar Check ===
      const opaStartTime = Date.now();

      // Build OPA input from SpiceDB result and session context
      const opaInput: OPAAuthZInput = await buildOPAAuthZInput({
        spiceDBAllowed: spiceDBResult.allowed,
        spiceDBRelationships: [], // Could be populated if needed
        entityId,
        resourceType,
        resourceId,
        sessionContext,
      });

      const opaDecision = await evaluateOPAPolicy(opaInput, opaUrl, timeoutMs);

      const opaLatency = Date.now() - opaStartTime;
      const totalLatency = Date.now() - startTime;

      // Log the decision
      request.log.info({
        layer: "opa",
        traceId,
        _allow: opaDecision.allow,
        risk_score: opaDecision.risk_score,
        recommended_action: opaDecision.recommended_action,
        reasons: opaDecision.reason,
        latency_ms: opaLatency,
      });

      // === Final Decision ===
      const finalAllow = spiceDBResult.allowed && opaDecision.allow;

      if (!finalAllow) {
        reply.code(403).send({
          error: "Forbidden",
          message: "Authorization denied",
          reasons: opaDecision.reason,
          recommended_action: opaDecision.recommended_action,
          risk_score: opaDecision.risk_score,
          trace_id: traceId,
          spicedb_allowed: spiceDBResult.allowed,
          opa_allowed: opaDecision.allow,
        });
        return;
      }

      // Add headers for downstream services
      reply.header("x-authz-trace-id", traceId);
      reply.header("x-authz-spicedb-allowed", String(spiceDBResult.allowed));
      reply.header("x-authz-opa-allowed", String(opaDecision.allow));
      reply.header("x-authz-risk-score", String(opaDecision.risk_score));

      // Attach decision to request for downstream use
      (request as any).authzDecision = opaDecision;
      (request as any).authzTraceId = traceId;
    } catch (error) {
      const errMsg = error instanceof Error ? error.message : "Unknown error";
      
      request.log.error({ err: error }, "Two-layer authZ failed");
      
      // Fail closed - deny on any error
      reply.code(500).send({
        error: "Authorization error",
        message: "Authorization check failed",
        detail: errMsg,
      });
    }
  };
}

/**
 * Evaluate the OPA policy with the given input
 */
async function evaluateOPAPolicy(
  input: OPAAuthZInput,
  opaUrl: string,
  timeoutMs: number
): Promise<OPADecision> {
  const controller = new AbortController();
  const timeout = setTimeout(() => controller.abort(), timeoutMs);

  try {
    const response = await fetch(`${opaUrl}/v1/data/authz/decision`, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      body: JSON.stringify({ input }),
      signal: controller.signal,
    });

    if (!response.ok) {
      throw new Error(`OPA error: ${response.status}`);
    }

    const data = await response.json() as { result: OPADecision };
    return data.result;
  } finally {
    clearTimeout(timeout);
  }
}

/**
 * Extract resource type and ID from request URL
 */
function extractResourceFromRequest(request: FastifyRequest): {
  resourceType?: string;
  resourceId?: string;
} {
  const url = request.url;

  // Match patterns like /v1/entity/:id or /v1/relationships/:type/:id
  const entityMatch = url.match(/^\/v1\/entity\/([^/]+)/);
  if (entityMatch) {
    return { resourceType: "entity", resourceId: entityMatch[1] };
  }

  const relationshipMatch = url.match(/^\/v1\/relationships\/([^/]+)/);
  if (relationshipMatch) {
    return { resourceType: "relationship", resourceId: relationshipMatch[1] };
  }

  const didMatch = url.match(/^\/v1\/did\//);
  if (didMatch) {
    return { resourceType: "did", resourceId: "default" };
  }

  const vcMatch = url.match(/^\/v1\/vc\//);
  if (vcMatch) {
    return { resourceType: "credential", resourceId: "default" };
  }

  const trustMatch = url.match(/^\/v1\/trust\//);
  if (trustMatch) {
    return { resourceType: "trust", resourceId: "default" };
  }

  // No resource found
  return {};
}

/**
 * Extract session/context info from request
 */
function extractSessionContext(request: FastifyRequest): OPAAuthZInput["context"] {
  const ip = request.ip || request.headers["x-forwarded-for"] as string || "unknown";
  const userAgent = request.headers["user-agent"] as string || "unknown";

  return {
    ip,
    userAgent,
    deviceId: (request.headers["x-device-id"] as string) || "",
    deviceTrust: parseFloat((request.headers["x-device-trust"] as string) || "0"),
    ipReputation: parseFloat((request.headers["x-ip-reputation"] as string) || "0.5"),
    sessionId: (request.headers["x-session-id"] as string) || "",
  };
}