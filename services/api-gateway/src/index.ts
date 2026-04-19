import Fastify from "fastify";
import cors from "@fastify/cors";
import swagger from "@fastify/swagger";
import swaggerUi from "@fastify/swagger-ui";
import { createEntityClient, createAuthzClient, createTrustClient, createDecisionClient, createFederationClient, createSurplusClient, createDIDClient } from "./grpc-client.js";
import { loadConfig } from "./config.js";
import { incrementFraudCounter, metricsContentType, recordRequestDuration, renderPrometheusMetrics } from "./observability.js";

const config = loadConfig();

const rateLimitBuckets = new Map<string, { count: number; windowStart: number }>();

const app = Fastify({
  logger: {
    level: config.nodeEnv === "production" ? "info" : "debug",
    redact: ["req.headers.authorization", "headers.authorization"],
  },
});

// gRPC clients
const entityClient = createEntityClient(config.entityEndpoint);
const authzClient = createAuthzClient(config.authzEndpoint);
const trustClient = createTrustClient(config.trustEndpoint);
const decisionClient = createDecisionClient(config.decisionEndpoint);
const federationClient = createFederationClient(config.federationEndpoint);
const surplusClient = createSurplusClient(config.surplusEndpoint);
const didClient = createDIDClient(config.didEndpoint);

async function fetchWithRetry(url: string, init: RequestInit, retries: number): Promise<Response> {
  let attempt = 0;
  let lastError: unknown;

  while (attempt <= retries) {
    const controller = new AbortController();
    const timeout = setTimeout(() => controller.abort(), config.requestTimeoutMs);

    try {
      const response = await fetch(url, { ...init, signal: controller.signal });
      clearTimeout(timeout);
      if (response.status >= 500 && attempt < retries) {
        attempt += 1;
        continue;
      }
      return response;
    } catch (error) {
      clearTimeout(timeout);
      lastError = error;
      if (attempt >= retries) break;
      attempt += 1;
    }
  }

  throw lastError instanceof Error ? lastError : new Error("Fraud service request failed");
}

// HTTP proxy helper for fraud-pipeline (REST-based service)
async function fraudFetch<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetchWithRetry(`${config.fraudUrl}${path}`, {
    ...init,
    headers: { "Content-Type": "application/json", ...init?.headers },
  }, config.fraudRetryAttempts);

  if (!res.ok) {
    incrementFraudCounter("error");
    const text = await res.text();
    throw new Error(`Fraud service error ${res.status}: ${text}`);
  }
  incrementFraudCounter("ok");
  return res.json() as Promise<T>;
}

// --- Plugins ---

await app.register(cors, { origin: true });

await app.register(swagger, {
  openapi: {
    info: {
      title: "CAAS API",
      description: "Continuous Autonomous Authorization System — REST Gateway",
      version: "0.1.0",
    },
    servers: [{ url: `http://localhost:${config.port}` }],
    tags: [
      { name: "entities", description: "Entity lifecycle management" },
      { name: "authorization", description: "Permission checks and lookups" },
      { name: "relationships", description: "Relationship tuple management" },
      { name: "trust", description: "Trust scoring, endorsements, and graph" },
      { name: "fraud", description: "Fraud detection, incidents, and simulation" },
      { name: "decisions", description: "Human decision integrity, blind review, M-of-N approval" },
      { name: "federation", description: "Sovereign nodes, treaties, federated queries" },
      { name: "credentials", description: "Verifiable credentials issuance and verification" },
      { name: "authority", description: "Authority override hierarchy (5 tiers)" },
      { name: "surplus", description: "Surplus/need listings, matching, and quality verification" },
      { name: "auth", description: "Authentication (Logto OIDC)" },
      { name: "did", description: "Decentralized Identifier management and resolution" },
      { name: "vc", description: "Verifiable Credentials (W3C compliant)" },
    ],
  },
});

await app.register(swaggerUi, { routePrefix: "/docs" });

app.addHook("onRequest", async (request, reply) => {
  reply.header("x-content-type-options", "nosniff");
  reply.header("x-frame-options", "DENY");
  reply.header("referrer-policy", "no-referrer");

  const key = request.ip;
  const now = Date.now();
  const existing = rateLimitBuckets.get(key);
  if (!existing || now - existing.windowStart >= config.rateLimitWindowMs) {
    rateLimitBuckets.set(key, { count: 1, windowStart: now });
    return;
  }

  existing.count += 1;
  if (existing.count > config.rateLimitMax) {
    reply.code(429).send({
      error: "rate_limited",
      message: "Too many requests",
    });
  }
});

app.addHook("onResponse", async (request, reply) => {
  const route = request.routeOptions.url || request.url;
  const durationMs = reply.elapsedTime;
  recordRequestDuration(request.method, route, String(reply.statusCode), durationMs);
});

app.setErrorHandler((error, request, reply) => {
  request.log.error({ err: error }, "request failed");

  if (reply.sent) {
    return;
  }

  const statusCode = (error as { statusCode?: number }).statusCode ?? 500;
  reply.code(statusCode >= 400 && statusCode < 600 ? statusCode : 500).send({
    error: "internal_error",
    message: statusCode >= 500 ? "Internal server error" : (error as Error).message,
  });
});

// --- Health ---

app.get("/health", async () => ({ status: "ok", service: "api-gateway" }));

app.get("/ready", async (_req, reply) => {
  try {
    await entityClient.listEntities({ pageSize: 1, pageToken: "", entityType: 0, lifecycleState: 0 });
    return { status: "ready" };
  } catch (error) {
    reply.code(503);
    return { status: "not_ready", reason: (error as Error).message };
  }
});

app.get("/metrics", async (_req, reply) => {
  reply.header("content-type", metricsContentType);
  return renderPrometheusMetrics();
});

// --- Logto OIDC Auth ---

interface LogtoUserInfo {
  sub: string;
  email?: string;
  name?: string;
  picture?: string;
  custom_data?: {
    entity_id?: string;
    did?: string;
  };
}

async function verifyLogtoToken(authHeader: string | undefined): Promise<LogtoUserInfo | null> {
  if (!authHeader?.startsWith("Bearer ")) return null;
  const token = authHeader.slice(7);

  try {
    // Validate by calling Logto's userinfo endpoint with the access token
    const res = await fetch(`${config.logtoEndpoint}/oidc/me`, {
      headers: { Authorization: `Bearer ${token}` },
    });
    if (!res.ok) return null;
    return res.json() as Promise<LogtoUserInfo>;
  } catch {
    return null;
  }
}

// Decorate request with user info
app.decorateRequest("logtoUser", null);

// Auth middleware — skips public routes
app.addHook("onRequest", async (req, reply) => {
  if (!config.authEnabled) return;

  const publicPaths = ["/health", "/ready", "/metrics", "/docs", "/.well-known", "/auth/"];
  const isPublic = publicPaths.some((p) => req.url.startsWith(p));
  if (isPublic) return;

  const user = await verifyLogtoToken(req.headers.authorization);
  if (!user) {
    reply.code(401).send({ error: "Unauthorized", message: "Valid Logto access token required" });
    return;
  }

  (req as any).logtoUser = user;
});

// --- Auth Routes (public) ---

app.get("/auth/session", {
  schema: { tags: ["auth"] },
}, async (req) => {
  const user = await verifyLogtoToken(req.headers.authorization);
  if (!user) return { authenticated: false };
  return {
    authenticated: true,
    sub: user.sub,
    email: user.email,
    name: user.name,
    entity_id: user.custom_data?.entity_id,
    did: user.custom_data?.did,
  };
});

app.get("/auth/config", {
  schema: { tags: ["auth"] },
}, async () => ({
  logto_endpoint: config.logtoEndpoint,
  resource: config.logtoResource,
}));

// --- Entity Routes ---

app.post<{ Body: { entity_type: number; display_name: string; metadata?: Record<string, unknown> } }>(
  "/v1/entities",
  {
    schema: {
      tags: ["entities"],
      body: {
        type: "object",
        required: ["entity_type", "display_name"],
        properties: {
          entity_type: { type: "integer", minimum: 1, maximum: 6 },
          display_name: { type: "string" },
          metadata: { type: "object", additionalProperties: true },
        },
      },
    },
  },
  async (req, reply) => {
    const { entity_type, display_name, metadata } = req.body;
    const result = await entityClient.createEntity({
      entityType: entity_type,
      displayName: display_name,
      metadata: metadata || undefined,
    });
    reply.code(201).send(result);
  }
);

app.get<{ Params: { id: string } }>(
  "/v1/entities/:id",
  {
    schema: {
      tags: ["entities"],
      params: {
        type: "object",
        properties: { id: { type: "string" } },
      },
    },
  },
  async (req) => {
    return entityClient.getEntity({ id: req.params.id });
  }
);

app.patch<{ Params: { id: string }; Body: { display_name?: string; metadata?: Record<string, unknown> } }>(
  "/v1/entities/:id",
  {
    schema: {
      tags: ["entities"],
      params: {
        type: "object",
        properties: { id: { type: "string" } },
      },
      body: {
        type: "object",
        properties: {
          display_name: { type: "string" },
          metadata: { type: "object", additionalProperties: true },
        },
      },
    },
  },
  async (req) => {
    return entityClient.updateEntity({
      id: req.params.id,
      displayName: req.body.display_name,
      metadata: req.body.metadata || undefined,
    });
  }
);

app.get<{ Querystring: { entity_type?: number; lifecycle_state?: number; page_size?: number; page_token?: string } }>(
  "/v1/entities",
  {
    schema: {
      tags: ["entities"],
      querystring: {
        type: "object",
        properties: {
          entity_type: { type: "integer" },
          lifecycle_state: { type: "integer" },
          page_size: { type: "integer", default: 50 },
          page_token: { type: "string" },
        },
      },
    },
  },
  async (req) => {
    return entityClient.listEntities({
      entityType: req.query.entity_type || 0,
      lifecycleState: req.query.lifecycle_state || 0,
      pageSize: req.query.page_size || 50,
      pageToken: req.query.page_token || "",
    });
  }
);

app.post<{ Params: { id: string } }>(
  "/v1/entities/:id/activate",
  {
    schema: {
      tags: ["entities"],
      params: {
        type: "object",
        properties: { id: { type: "string" } },
      },
    },
  },
  async (req) => {
    return entityClient.activateEntity({ id: req.params.id });
  }
);

app.post<{ Params: { id: string }; Body: { reason?: string } }>(
  "/v1/entities/:id/suspend",
  {
    schema: {
      tags: ["entities"],
      params: {
        type: "object",
        properties: { id: { type: "string" } },
      },
      body: {
        type: "object",
        properties: { reason: { type: "string" } },
      },
    },
  },
  async (req) => {
    return entityClient.suspendEntity({
      id: req.params.id,
      reason: req.body?.reason || "",
    });
  }
);

app.post<{ Params: { id: string }; Body: { reason?: string } }>(
  "/v1/entities/:id/revoke",
  {
    schema: {
      tags: ["entities"],
      params: {
        type: "object",
        properties: { id: { type: "string" } },
      },
      body: {
        type: "object",
        properties: { reason: { type: "string" } },
      },
    },
  },
  async (req) => {
    return entityClient.revokeEntity({
      id: req.params.id,
      reason: req.body?.reason || "",
    });
  }
);

// --- Authorization Routes ---

app.post<{
  Body: {
    subject: { type: string; id: string };
    permission: string;
    resource: { type: string; id: string };
    context?: Record<string, unknown>;
  };
}>(
  "/v1/authz/check",
  {
    schema: {
      tags: ["authorization"],
      body: {
        type: "object",
        required: ["subject", "permission", "resource"],
        properties: {
          subject: {
            type: "object",
            required: ["type", "id"],
            properties: {
              type: { type: "string" },
              id: { type: "string" },
            },
          },
          permission: { type: "string" },
          resource: {
            type: "object",
            required: ["type", "id"],
            properties: {
              type: { type: "string" },
              id: { type: "string" },
            },
          },
          context: { type: "object", additionalProperties: true },
        },
      },
    },
  },
  async (req) => {
    return authzClient.check({
      subject: req.body.subject,
      permission: req.body.permission,
      resource: req.body.resource,
      context: req.body.context || undefined,
    });
  }
);

app.post<{
  Body: {
    resource_type: string;
    permission: string;
    subject: { type: string; id: string };
  };
}>(
  "/v1/authz/lookup-subjects",
  {
    schema: {
      tags: ["authorization"],
      body: {
        type: "object",
        required: ["resource_type", "permission", "subject"],
        properties: {
          resource_type: { type: "string" },
          permission: { type: "string" },
          subject: {
            type: "object",
            required: ["type", "id"],
            properties: {
              type: { type: "string" },
              id: { type: "string" },
            },
          },
        },
      },
    },
  },
  async (req) => {
    // Note: LookupSubjects is a streaming RPC — the gRPC client here
    // uses the unary promisified wrapper which won't work for streaming.
    // For MVP, this returns an error; streaming support comes later.
    return { error: "streaming RPC not yet supported via REST gateway" };
  }
);

app.post<{
  Body: {
    resource_type: string;
    permission: string;
    subject: { type: string; id: string };
  };
}>(
  "/v1/authz/lookup-resources",
  {
    schema: {
      tags: ["authorization"],
      body: {
        type: "object",
        required: ["resource_type", "permission", "subject"],
        properties: {
          resource_type: { type: "string" },
          permission: { type: "string" },
          subject: {
            type: "object",
            required: ["type", "id"],
            properties: {
              type: { type: "string" },
              id: { type: "string" },
            },
          },
        },
      },
    },
  },
  async (req) => {
    return { error: "streaming RPC not yet supported via REST gateway" };
  }
);

// --- Relationship Routes ---

app.post<{
  Body: {
    relationships: Array<{
      resource: { type: string; id: string };
      relation: string;
      subject: { type: string; id: string };
    }>;
  };
}>(
  "/v1/relationships",
  {
    schema: {
      tags: ["relationships"],
      body: {
        type: "object",
        required: ["relationships"],
        properties: {
          relationships: {
            type: "array",
            items: {
              type: "object",
              required: ["resource", "relation", "subject"],
              properties: {
                resource: {
                  type: "object",
                  required: ["type", "id"],
                  properties: {
                    type: { type: "string" },
                    id: { type: "string" },
                  },
                },
                relation: { type: "string" },
                subject: {
                  type: "object",
                  required: ["type", "id"],
                  properties: {
                    type: { type: "string" },
                    id: { type: "string" },
                  },
                },
              },
            },
          },
        },
      },
    },
  },
  async (req, reply) => {
    const result = await authzClient.writeRelationships({
      relationships: req.body.relationships,
    });
    reply.code(201).send(result);
  }
);

app.get<{
  Querystring: {
    resource_type?: string;
    resource_id?: string;
    relation?: string;
    subject_type?: string;
    subject_id?: string;
  };
}>(
  "/v1/relationships",
  {
    schema: {
      tags: ["relationships"],
      querystring: {
        type: "object",
        properties: {
          resource_type: { type: "string" },
          resource_id: { type: "string" },
          relation: { type: "string" },
          subject_type: { type: "string" },
          subject_id: { type: "string" },
        },
      },
    },
  },
  async (req) => {
    return authzClient.readRelationships({
      resourceType: req.query.resource_type || "",
      resourceId: req.query.resource_id || "",
      relation: req.query.relation || "",
      subjectType: req.query.subject_type || "",
      subjectId: req.query.subject_id || "",
    });
  }
);

// --- Trust Routes ---

app.get<{ Params: { entityId: string } }>(
  "/v1/trust/:entityId",
  {
    schema: {
      tags: ["trust"],
      params: {
        type: "object",
        properties: { entityId: { type: "string" } },
      },
    },
  },
  async (req) => {
    return trustClient.getTrustScore({ entityId: req.params.entityId });
  }
);

app.get<{ Params: { entityId: string }; Querystring: { limit?: number } }>(
  "/v1/trust/:entityId/history",
  {
    schema: {
      tags: ["trust"],
      params: {
        type: "object",
        properties: { entityId: { type: "string" } },
      },
      querystring: {
        type: "object",
        properties: { limit: { type: "integer", default: 50 } },
      },
    },
  },
  async (req) => {
    return trustClient.getTrustHistory({
      entityId: req.params.entityId,
      limit: req.query.limit || 50,
    });
  }
);

app.get<{ Params: { entityId: string }; Querystring: { depth?: number } }>(
  "/v1/trust/:entityId/graph",
  {
    schema: {
      tags: ["trust"],
      params: {
        type: "object",
        properties: { entityId: { type: "string" } },
      },
      querystring: {
        type: "object",
        properties: { depth: { type: "integer", default: 2 } },
      },
    },
  },
  async (req) => {
    return trustClient.getTrustGraph({
      entityId: req.params.entityId,
      depth: req.query.depth || 2,
    });
  }
);

app.post<{
  Body: {
    endorser_id: string;
    endorsed_id: string;
    endorsement_type: string;
    weight?: number;
    evidence_hash?: string;
  };
}>(
  "/v1/trust/endorsements",
  {
    schema: {
      tags: ["trust"],
      body: {
        type: "object",
        required: ["endorser_id", "endorsed_id", "endorsement_type"],
        properties: {
          endorser_id: { type: "string" },
          endorsed_id: { type: "string" },
          endorsement_type: { type: "string" },
          weight: { type: "number", default: 1.0 },
          evidence_hash: { type: "string" },
        },
      },
    },
  },
  async (req, reply) => {
    const result = await trustClient.createEndorsement({
      endorserId: req.body.endorser_id,
      endorsedId: req.body.endorsed_id,
      endorsementType: req.body.endorsement_type,
      weight: req.body.weight || 1.0,
      evidenceHash: req.body.evidence_hash || "",
    });
    reply.code(201).send(result);
  }
);

app.delete<{ Params: { id: string } }>(
  "/v1/trust/endorsements/:id",
  {
    schema: {
      tags: ["trust"],
      params: {
        type: "object",
        properties: { id: { type: "string" } },
      },
    },
  },
  async (req) => {
    return trustClient.revokeEndorsement({ endorsementId: req.params.id });
  }
);

app.post<{
  Body: {
    entity_id: string;
    minimum_score?: number;
    minimum_dimensions?: Record<string, number>;
  };
}>(
  "/v1/trust/verify",
  {
    schema: {
      tags: ["trust"],
      body: {
        type: "object",
        required: ["entity_id"],
        properties: {
          entity_id: { type: "string" },
          minimum_score: { type: "integer" },
          minimum_dimensions: {
            type: "object",
            additionalProperties: { type: "integer" },
          },
        },
      },
    },
  },
  async (req) => {
    return trustClient.verifyTrust({
      entityId: req.body.entity_id,
      minimumScore: req.body.minimum_score || 0,
      minimumDimensions: req.body.minimum_dimensions || {},
    });
  }
);

app.post<{
  Body: { entity_id: string; threshold: number };
}>(
  "/v1/trust/attest",
  {
    schema: {
      tags: ["trust"],
      body: {
        type: "object",
        required: ["entity_id", "threshold"],
        properties: {
          entity_id: { type: "string" },
          threshold: { type: "integer", minimum: 1, maximum: 1000 },
        },
      },
    },
  },
  async (req) => {
    return trustClient.attestTrust({
      entityId: req.body.entity_id,
      threshold: req.body.threshold,
    });
  }
);

// --- Fraud Routes (proxy to fraud-pipeline REST API) ---

app.get<{
  Querystring: {
    status?: string;
    min_severity?: string;
    affected_entity_id?: string;
    page_size?: number;
    page_token?: string;
  };
}>(
  "/v1/fraud/incidents",
  {
    schema: {
      tags: ["fraud"],
      querystring: {
        type: "object",
        properties: {
          status: { type: "string" },
          min_severity: { type: "string" },
          affected_entity_id: { type: "string" },
          page_size: { type: "integer", default: 50 },
          page_token: { type: "string" },
        },
      },
    },
  },
  async (req) => {
    const qs = new URLSearchParams();
    if (req.query.status) qs.set("status", req.query.status);
    if (req.query.min_severity) qs.set("min_severity", req.query.min_severity);
    if (req.query.affected_entity_id) qs.set("affected_entity_id", req.query.affected_entity_id);
    if (req.query.page_size) qs.set("page_size", String(req.query.page_size));
    if (req.query.page_token) qs.set("page_token", req.query.page_token);
    const query = qs.toString();
    return fraudFetch(`/v1/fraud/incidents${query ? `?${query}` : ""}`);
  }
);

app.get<{ Params: { id: string } }>(
  "/v1/fraud/incidents/:id",
  {
    schema: {
      tags: ["fraud"],
      params: {
        type: "object",
        properties: { id: { type: "string" } },
      },
    },
  },
  async (req) => {
    return fraudFetch(`/v1/fraud/incidents/${req.params.id}`);
  }
);

app.post<{
  Body: { attack_type: string; target_entity_id: string };
}>(
  "/v1/fraud/simulate",
  {
    schema: {
      tags: ["fraud"],
      body: {
        type: "object",
        required: ["attack_type", "target_entity_id"],
        properties: {
          attack_type: {
            type: "string",
            enum: ["account_takeover", "privilege_escalation", "sybil", "exfiltration", "impersonation"],
          },
          target_entity_id: { type: "string" },
        },
      },
    },
  },
  async (req) => {
    return fraudFetch("/v1/fraud/simulate", {
      method: "POST",
      body: JSON.stringify(req.body),
    });
  }
);

app.get(
  "/v1/fraud/metrics",
  {
    schema: { tags: ["fraud"] },
  },
  async () => {
    return fraudFetch("/v1/fraud/metrics");
  }
);

// --- Decision Routes ---

app.post<{
  Body: {
    workflow_type: string;
    subject_entity_id: string;
    case_data?: Record<string, unknown>;
    required_approvals?: number;
    total_reviewers?: number;
    blind_review?: boolean;
    cool_off_hours?: number;
  };
}>(
  "/v1/decisions/workflows",
  {
    schema: {
      tags: ["decisions"],
      body: {
        type: "object",
        required: ["workflow_type", "subject_entity_id"],
        properties: {
          workflow_type: { type: "string" },
          subject_entity_id: { type: "string" },
          case_data: { type: "object", additionalProperties: true },
          required_approvals: { type: "integer", default: 2 },
          total_reviewers: { type: "integer", default: 3 },
          blind_review: { type: "boolean", default: true },
          cool_off_hours: { type: "integer", default: 168 },
        },
      },
    },
  },
  async (req, reply) => {
    const result = await decisionClient.createWorkflow({
      workflowType: req.body.workflow_type,
      subjectEntityId: req.body.subject_entity_id,
      caseData: req.body.case_data || undefined,
      requiredApprovals: req.body.required_approvals || 2,
      totalReviewers: req.body.total_reviewers || 3,
      blindReview: req.body.blind_review !== false,
      coolOffHours: req.body.cool_off_hours || 168,
    });
    reply.code(201).send(result);
  }
);

app.get<{ Params: { id: string } }>(
  "/v1/decisions/workflows/:id",
  {
    schema: {
      tags: ["decisions"],
      params: {
        type: "object",
        properties: { id: { type: "string" } },
      },
    },
  },
  async (req) => {
    return decisionClient.getWorkflow({ id: req.params.id });
  }
);

app.get<{
  Querystring: { status?: number; subject_entity_id?: string; page_size?: number; page_token?: string };
}>(
  "/v1/decisions/workflows",
  {
    schema: {
      tags: ["decisions"],
      querystring: {
        type: "object",
        properties: {
          status: { type: "integer" },
          subject_entity_id: { type: "string" },
          page_size: { type: "integer", default: 50 },
          page_token: { type: "string" },
        },
      },
    },
  },
  async (req) => {
    return decisionClient.listWorkflows({
      status: req.query.status || 0,
      subjectEntityId: req.query.subject_entity_id || "",
      pageSize: req.query.page_size || 50,
      pageToken: req.query.page_token || "",
    });
  }
);

app.post<{
  Body: {
    workflow_id: string;
    reviewer_id: string;
    vote: number;
    reasoning?: string;
    confidence?: number;
    time_spent_seconds?: number;
  };
}>(
  "/v1/decisions/votes",
  {
    schema: {
      tags: ["decisions"],
      body: {
        type: "object",
        required: ["workflow_id", "reviewer_id", "vote"],
        properties: {
          workflow_id: { type: "string" },
          reviewer_id: { type: "string" },
          vote: { type: "integer", minimum: 1, maximum: 4 },
          reasoning: { type: "string" },
          confidence: { type: "number" },
          time_spent_seconds: { type: "integer" },
        },
      },
    },
  },
  async (req) => {
    return decisionClient.submitVote({
      workflowId: req.body.workflow_id,
      reviewerId: req.body.reviewer_id,
      vote: req.body.vote,
      reasoning: req.body.reasoning || "",
      confidence: req.body.confidence || 0,
      timeSpentSeconds: req.body.time_spent_seconds || 0,
    });
  }
);

app.get<{ Params: { workflowId: string }; Querystring: { reviewer_id: string } }>(
  "/v1/decisions/workflows/:workflowId/blind-case",
  {
    schema: {
      tags: ["decisions"],
      params: {
        type: "object",
        properties: { workflowId: { type: "string" } },
      },
      querystring: {
        type: "object",
        required: ["reviewer_id"],
        properties: { reviewer_id: { type: "string" } },
      },
    },
  },
  async (req) => {
    return decisionClient.getBlindCase({
      workflowId: req.params.workflowId,
      reviewerId: req.query.reviewer_id,
    });
  }
);

app.get<{ Params: { reviewerId: string } }>(
  "/v1/decisions/reviewers/:reviewerId/integrity",
  {
    schema: {
      tags: ["decisions"],
      params: {
        type: "object",
        properties: { reviewerId: { type: "string" } },
      },
    },
  },
  async (req) => {
    return decisionClient.getReviewerIntegrity({ reviewerId: req.params.reviewerId });
  }
);

// --- Federation Routes ---

app.post<{
  Body: {
    name: string;
    did?: string;
    jurisdiction: string;
    endpoint: string;
    public_key?: string;
    metadata?: Record<string, unknown>;
  };
}>(
  "/v1/federation/nodes",
  {
    schema: {
      tags: ["federation"],
      body: {
        type: "object",
        required: ["name", "jurisdiction", "endpoint"],
        properties: {
          name: { type: "string" },
          did: { type: "string" },
          jurisdiction: { type: "string" },
          endpoint: { type: "string" },
          public_key: { type: "string" },
          metadata: { type: "object", additionalProperties: true },
        },
      },
    },
  },
  async (req, reply) => {
    const result = await federationClient.registerNode({
      name: req.body.name,
      did: req.body.did || "",
      jurisdiction: req.body.jurisdiction,
      endpoint: req.body.endpoint,
      publicKey: req.body.public_key || "",
      metadata: req.body.metadata || undefined,
    });
    reply.code(201).send(result);
  }
);

app.get<{ Params: { id: string } }>(
  "/v1/federation/nodes/:id",
  {
    schema: {
      tags: ["federation"],
      params: {
        type: "object",
        properties: { id: { type: "string" } },
      },
    },
  },
  async (req) => {
    return federationClient.getNode({ nodeId: req.params.id });
  }
);

app.get<{ Querystring: { status?: number; page_size?: number; page?: number } }>(
  "/v1/federation/nodes",
  {
    schema: {
      tags: ["federation"],
      querystring: {
        type: "object",
        properties: {
          status: { type: "integer" },
          page_size: { type: "integer", default: 20 },
          page: { type: "integer", default: 1 },
        },
      },
    },
  },
  async (req) => {
    return federationClient.listNodes({
      statusFilter: req.query.status || 0,
      pageSize: req.query.page_size || 20,
      page: req.query.page || 1,
    });
  }
);

app.post<{
  Body: {
    target_node_id: string;
    trust_weight?: number;
    allowed_operations?: string[];
    valid_until?: string;
  };
}>(
  "/v1/federation/treaties",
  {
    schema: {
      tags: ["federation"],
      body: {
        type: "object",
        required: ["target_node_id"],
        properties: {
          target_node_id: { type: "string" },
          trust_weight: { type: "number", default: 0.5 },
          allowed_operations: { type: "array", items: { type: "string" } },
          valid_until: { type: "string" },
        },
      },
    },
  },
  async (req, reply) => {
    const result = await federationClient.proposeTreaty({
      targetNodeId: req.body.target_node_id,
      trustWeight: req.body.trust_weight || 0.5,
      allowedOperations: req.body.allowed_operations || [],
      validUntil: req.body.valid_until ? { seconds: Math.floor(new Date(req.body.valid_until).getTime() / 1000), nanos: 0 } : undefined,
    });
    reply.code(201).send(result);
  }
);

app.post<{ Params: { id: string } }>(
  "/v1/federation/treaties/:id/accept",
  {
    schema: {
      tags: ["federation"],
      params: {
        type: "object",
        properties: { id: { type: "string" } },
      },
    },
  },
  async (req) => {
    return federationClient.acceptTreaty({ treatyId: req.params.id });
  }
);

app.get<{ Querystring: { node_id?: string; status?: number; page_size?: number; page?: number } }>(
  "/v1/federation/treaties",
  {
    schema: {
      tags: ["federation"],
      querystring: {
        type: "object",
        properties: {
          node_id: { type: "string" },
          status: { type: "integer" },
          page_size: { type: "integer", default: 20 },
          page: { type: "integer", default: 1 },
        },
      },
    },
  },
  async (req) => {
    return federationClient.listTreaties({
      nodeId: req.query.node_id || "",
      statusFilter: req.query.status || 0,
      pageSize: req.query.page_size || 20,
      page: req.query.page || 1,
    });
  }
);

app.delete<{ Params: { id: string }; Body: { reason?: string } }>(
  "/v1/federation/treaties/:id",
  {
    schema: {
      tags: ["federation"],
      params: {
        type: "object",
        properties: { id: { type: "string" } },
      },
      body: {
        type: "object",
        properties: { reason: { type: "string" } },
      },
    },
  },
  async (req) => {
    return federationClient.terminateTreaty({
      treatyId: req.params.id,
      reason: req.body?.reason || "",
    });
  }
);

app.post<{
  Body: { entity_id: string; requesting_node_id: string };
}>(
  "/v1/federation/trust-query",
  {
    schema: {
      tags: ["federation"],
      body: {
        type: "object",
        required: ["entity_id", "requesting_node_id"],
        properties: {
          entity_id: { type: "string" },
          requesting_node_id: { type: "string" },
        },
      },
    },
  },
  async (req) => {
    return federationClient.federatedTrustQuery({
      entityId: req.body.entity_id,
      requestingNodeId: req.body.requesting_node_id,
    });
  }
);

// --- Authority Override Routes ---

app.post<{
  Body: {
    workflow_id: string;
    authority_entity_id: string;
    tier: number;
    justification: string;
    legal_reference?: string;
    outcome: string;
    expires_at?: string;
  };
}>(
  "/v1/authority/overrides",
  {
    schema: {
      tags: ["authority"],
      body: {
        type: "object",
        required: ["workflow_id", "authority_entity_id", "tier", "justification", "outcome"],
        properties: {
          workflow_id: { type: "string" },
          authority_entity_id: { type: "string" },
          tier: { type: "integer", minimum: 1, maximum: 5 },
          justification: { type: "string" },
          legal_reference: { type: "string" },
          outcome: { type: "string", enum: ["approved", "rejected"] },
          expires_at: { type: "string" },
        },
      },
    },
  },
  async (req, reply) => {
    const result = await federationClient.createAuthorityOverride({
      workflowId: req.body.workflow_id,
      authorityEntityId: req.body.authority_entity_id,
      tier: req.body.tier,
      justification: req.body.justification,
      legalReference: req.body.legal_reference || "",
      outcome: req.body.outcome,
      expiresAt: req.body.expires_at ? { seconds: Math.floor(new Date(req.body.expires_at).getTime() / 1000), nanos: 0 } : undefined,
    });
    reply.code(201).send(result);
  }
);

app.get<{ Querystring: { workflow_id?: string; page_size?: number; page?: number } }>(
  "/v1/authority/overrides",
  {
    schema: {
      tags: ["authority"],
      querystring: {
        type: "object",
        properties: {
          workflow_id: { type: "string" },
          page_size: { type: "integer", default: 20 },
          page: { type: "integer", default: 1 },
        },
      },
    },
  },
  async (req) => {
    return federationClient.listAuthorityOverrides({
      workflowId: req.query.workflow_id || "",
      pageSize: req.query.page_size || 20,
      page: req.query.page || 1,
    });
  }
);

// --- Verifiable Credential Routes ---

app.post<{
  Body: {
    entity_id: string;
    credential_type: string;
    claims?: Record<string, unknown>;
    expires_at?: string;
  };
}>(
  "/v1/credentials",
  {
    schema: {
      tags: ["credentials"],
      body: {
        type: "object",
        required: ["entity_id", "credential_type"],
        properties: {
          entity_id: { type: "string" },
          credential_type: { type: "string" },
          claims: { type: "object", additionalProperties: true },
          expires_at: { type: "string" },
        },
      },
    },
  },
  async (req, reply) => {
    const result = await federationClient.issueCredential({
      entityId: req.body.entity_id,
      credentialType: req.body.credential_type,
      claims: req.body.claims || undefined,
      expiresAt: req.body.expires_at ? { seconds: Math.floor(new Date(req.body.expires_at).getTime() / 1000), nanos: 0 } : undefined,
    });
    reply.code(201).send(result);
  }
);

app.get<{ Params: { id: string } }>(
  "/v1/credentials/:id/verify",
  {
    schema: {
      tags: ["credentials"],
      params: {
        type: "object",
        properties: { id: { type: "string" } },
      },
    },
  },
  async (req) => {
    return federationClient.verifyCredential({ credentialId: req.params.id });
  }
);

app.post<{ Params: { id: string }; Body: { reason?: string } }>(
  "/v1/credentials/:id/revoke",
  {
    schema: {
      tags: ["credentials"],
      params: {
        type: "object",
        properties: { id: { type: "string" } },
      },
      body: {
        type: "object",
        properties: { reason: { type: "string" } },
      },
    },
  },
  async (req) => {
    return federationClient.revokeCredential({
      credentialId: req.params.id,
      reason: req.body?.reason || "",
    });
  }
);

app.get<{ Querystring: { entity_id?: string; credential_type?: string; page_size?: number; page?: number } }>(
  "/v1/credentials",
  {
    schema: {
      tags: ["credentials"],
      querystring: {
        type: "object",
        properties: {
          entity_id: { type: "string" },
          credential_type: { type: "string" },
          page_size: { type: "integer", default: 20 },
          page: { type: "integer", default: 1 },
        },
      },
    },
  },
  async (req) => {
    return federationClient.listCredentials({
      entityId: req.query.entity_id || "",
      credentialType: req.query.credential_type || "",
      pageSize: req.query.page_size || 20,
      page: req.query.page || 1,
    });
  }
);

// --- Surplus Routes ---

app.post<{
  Body: {
    entity_id: string;
    type: number;
    category: number;
    title: string;
    description?: string;
    quantity?: number;
    unit?: string;
    urgency?: number;
    location?: { latitude: number; longitude: number; address?: string; region?: string; country?: string };
    max_distance_km?: number;
    metadata?: Record<string, unknown>;
    expires_at?: string;
  };
}>(
  "/v1/surplus/listings",
  {
    schema: {
      tags: ["surplus"],
      body: {
        type: "object",
        required: ["entity_id", "type", "category", "title"],
        properties: {
          entity_id: { type: "string" },
          type: { type: "integer", minimum: 1, maximum: 2 },
          category: { type: "integer", minimum: 1, maximum: 10 },
          title: { type: "string" },
          description: { type: "string" },
          quantity: { type: "integer", default: 1 },
          unit: { type: "string", default: "units" },
          urgency: { type: "integer", minimum: 0, maximum: 4 },
          location: {
            type: "object",
            properties: {
              latitude: { type: "number" },
              longitude: { type: "number" },
              address: { type: "string" },
              region: { type: "string" },
              country: { type: "string" },
            },
          },
          max_distance_km: { type: "number", default: 100 },
          metadata: { type: "object", additionalProperties: true },
          expires_at: { type: "string" },
        },
      },
    },
  },
  async (req, reply) => {
    const result = await surplusClient.createListing({
      entityId: req.body.entity_id,
      type: req.body.type,
      category: req.body.category,
      title: req.body.title,
      description: req.body.description || "",
      quantity: req.body.quantity || 1,
      unit: req.body.unit || "units",
      urgency: req.body.urgency || 0,
      location: req.body.location || undefined,
      maxDistanceKm: req.body.max_distance_km || 100,
      metadata: req.body.metadata || undefined,
      expiresAt: req.body.expires_at ? { seconds: Math.floor(new Date(req.body.expires_at).getTime() / 1000), nanos: 0 } : undefined,
    });
    reply.code(201).send(result);
  }
);

app.get<{ Params: { id: string } }>(
  "/v1/surplus/listings/:id",
  {
    schema: {
      tags: ["surplus"],
      params: {
        type: "object",
        properties: { id: { type: "string" } },
      },
    },
  },
  async (req) => {
    return surplusClient.getListing({ listingId: req.params.id });
  }
);

app.get<{
  Querystring: {
    type_filter?: number;
    category_filter?: number;
    status_filter?: number;
    entity_id?: string;
    page_size?: number;
    page?: number;
  };
}>(
  "/v1/surplus/listings",
  {
    schema: {
      tags: ["surplus"],
      querystring: {
        type: "object",
        properties: {
          type_filter: { type: "integer" },
          category_filter: { type: "integer" },
          status_filter: { type: "integer" },
          entity_id: { type: "string" },
          page_size: { type: "integer", default: 20 },
          page: { type: "integer", default: 1 },
        },
      },
    },
  },
  async (req) => {
    return surplusClient.listListings({
      typeFilter: req.query.type_filter || 0,
      categoryFilter: req.query.category_filter || 0,
      statusFilter: req.query.status_filter || 0,
      entityId: req.query.entity_id || "",
      pageSize: req.query.page_size || 20,
      page: req.query.page || 1,
    });
  }
);

app.patch<{
  Params: { id: string };
  Body: { quantity?: number; urgency?: number; status?: number };
}>(
  "/v1/surplus/listings/:id",
  {
    schema: {
      tags: ["surplus"],
      params: {
        type: "object",
        properties: { id: { type: "string" } },
      },
      body: {
        type: "object",
        properties: {
          quantity: { type: "integer" },
          urgency: { type: "integer" },
          status: { type: "integer" },
        },
      },
    },
  },
  async (req) => {
    return surplusClient.updateListing({
      listingId: req.params.id,
      quantity: req.body.quantity || 0,
      urgency: req.body.urgency || 0,
      status: req.body.status || 0,
    });
  }
);

app.post<{ Params: { id: string }; Body: { reason?: string } }>(
  "/v1/surplus/listings/:id/cancel",
  {
    schema: {
      tags: ["surplus"],
      params: {
        type: "object",
        properties: { id: { type: "string" } },
      },
      body: {
        type: "object",
        properties: { reason: { type: "string" } },
      },
    },
  },
  async (req) => {
    return surplusClient.cancelListing({
      listingId: req.params.id,
      reason: req.body?.reason || "",
    });
  }
);

app.post<{
  Body: { listing_id: string; max_results?: number };
}>(
  "/v1/surplus/matches/find",
  {
    schema: {
      tags: ["surplus"],
      body: {
        type: "object",
        required: ["listing_id"],
        properties: {
          listing_id: { type: "string" },
          max_results: { type: "integer", default: 10 },
        },
      },
    },
  },
  async (req) => {
    return surplusClient.findMatches({
      listingId: req.body.listing_id,
      maxResults: req.body.max_results || 10,
    });
  }
);

app.post<{ Params: { id: string } }>(
  "/v1/surplus/matches/:id/accept",
  {
    schema: {
      tags: ["surplus"],
      params: {
        type: "object",
        properties: { id: { type: "string" } },
      },
    },
  },
  async (req) => {
    return surplusClient.acceptMatch({ matchId: req.params.id });
  }
);

app.patch<{
  Params: { id: string };
  Body: { status: number };
}>(
  "/v1/surplus/matches/:id/status",
  {
    schema: {
      tags: ["surplus"],
      params: {
        type: "object",
        properties: { id: { type: "string" } },
      },
      body: {
        type: "object",
        required: ["status"],
        properties: {
          status: { type: "integer", minimum: 1, maximum: 7 },
        },
      },
    },
  },
  async (req) => {
    return surplusClient.updateMatchStatus({
      matchId: req.params.id,
      status: req.body.status,
    });
  }
);

app.get<{ Params: { id: string } }>(
  "/v1/surplus/matches/:id",
  {
    schema: {
      tags: ["surplus"],
      params: {
        type: "object",
        properties: { id: { type: "string" } },
      },
    },
  },
  async (req) => {
    return surplusClient.getMatch({ matchId: req.params.id });
  }
);

app.get<{
  Querystring: { entity_id?: string; status_filter?: number; page_size?: number; page?: number };
}>(
  "/v1/surplus/matches",
  {
    schema: {
      tags: ["surplus"],
      querystring: {
        type: "object",
        properties: {
          entity_id: { type: "string" },
          status_filter: { type: "integer" },
          page_size: { type: "integer", default: 20 },
          page: { type: "integer", default: 1 },
        },
      },
    },
  },
  async (req) => {
    return surplusClient.listMatches({
      entityId: req.query.entity_id || "",
      statusFilter: req.query.status_filter || 0,
      pageSize: req.query.page_size || 20,
      page: req.query.page || 1,
    });
  }
);

app.post<{
  Body: {
    match_id: string;
    verifier_entity_id: string;
    quality_score: number;
    meets_requirements: boolean;
    notes?: string;
  };
}>(
  "/v1/surplus/quality",
  {
    schema: {
      tags: ["surplus"],
      body: {
        type: "object",
        required: ["match_id", "verifier_entity_id", "quality_score", "meets_requirements"],
        properties: {
          match_id: { type: "string" },
          verifier_entity_id: { type: "string" },
          quality_score: { type: "integer", minimum: 0, maximum: 100 },
          meets_requirements: { type: "boolean" },
          notes: { type: "string" },
        },
      },
    },
  },
  async (req, reply) => {
    const result = await surplusClient.verifyQuality({
      matchId: req.body.match_id,
      verifierEntityId: req.body.verifier_entity_id,
      qualityScore: req.body.quality_score,
      meetsRequirements: req.body.meets_requirements,
      notes: req.body.notes || "",
    });
    reply.code(201).send(result);
  }
);

app.get(
  "/v1/surplus/metrics",
  {
    schema: { tags: ["surplus"] },
  },
  async () => {
    return surplusClient.getSurplusMetrics({});
  }
);

// --- DID Routes ---

app.post<{
  Body: { entity_id: string; method: number; key_type?: number; domain?: string };
}>(
  "/v1/did/create",
  {
    schema: {
      tags: ["did"],
      body: {
        type: "object",
        required: ["entity_id", "method"],
        properties: {
          entity_id: { type: "string" },
          method: { type: "integer", minimum: 1, maximum: 2 },
          key_type: { type: "integer", minimum: 1, maximum: 2 },
          domain: { type: "string" },
        },
      },
    },
  },
  async (req, reply) => {
    const result = await didClient.createDID({
      entityId: req.body.entity_id,
      method: req.body.method,
      keyType: req.body.key_type || 1,
      domain: req.body.domain || "",
    });
    reply.code(201).send(result);
  }
);

app.post<{ Body: { did: string } }>(
  "/v1/did/resolve",
  {
    schema: {
      tags: ["did"],
      body: {
        type: "object",
        required: ["did"],
        properties: { did: { type: "string" } },
      },
    },
  },
  async (req) => {
    return didClient.resolveDID({ did: req.body.did });
  }
);

app.get<{
  Querystring: { entity_id?: string; method_filter?: number; status_filter?: number; page_size?: number; page?: number };
}>(
  "/v1/did",
  {
    schema: {
      tags: ["did"],
      querystring: {
        type: "object",
        properties: {
          entity_id: { type: "string" },
          method_filter: { type: "integer" },
          status_filter: { type: "integer" },
          page_size: { type: "integer", default: 20 },
          page: { type: "integer", default: 1 },
        },
      },
    },
  },
  async (req) => {
    return didClient.listDIDs({
      entityId: req.query.entity_id || "",
      methodFilter: req.query.method_filter || 0,
      statusFilter: req.query.status_filter || 0,
      pageSize: req.query.page_size || 20,
      page: req.query.page || 1,
    });
  }
);

app.post<{ Body: { did: string; reason?: string } }>(
  "/v1/did/deactivate",
  {
    schema: {
      tags: ["did"],
      body: {
        type: "object",
        required: ["did"],
        properties: {
          did: { type: "string" },
          reason: { type: "string" },
        },
      },
    },
  },
  async (req) => {
    return didClient.deactivateDID({
      did: req.body.did,
      reason: req.body.reason || "",
    });
  }
);

app.post<{
  Body: { did: string; key_type?: number; purpose?: number };
}>(
  "/v1/did/keys",
  {
    schema: {
      tags: ["did"],
      body: {
        type: "object",
        required: ["did"],
        properties: {
          did: { type: "string" },
          key_type: { type: "integer", minimum: 1, maximum: 2 },
          purpose: { type: "integer", minimum: 1, maximum: 4 },
        },
      },
    },
  },
  async (req, reply) => {
    const result = await didClient.generateKeyPair({
      did: req.body.did,
      keyType: req.body.key_type || 1,
      purpose: req.body.purpose || 2,
    });
    reply.code(201).send(result);
  }
);

app.post<{ Body: { key_id: string; new_key_type?: number } }>(
  "/v1/did/keys/rotate",
  {
    schema: {
      tags: ["did"],
      body: {
        type: "object",
        required: ["key_id"],
        properties: {
          key_id: { type: "string" },
          new_key_type: { type: "integer" },
        },
      },
    },
  },
  async (req) => {
    return didClient.rotateKey({
      keyId: req.body.key_id,
      newKeyType: req.body.new_key_type || 0,
    });
  }
);

app.get<{ Querystring: { did: string } }>(
  "/v1/did/keys",
  {
    schema: {
      tags: ["did"],
      querystring: {
        type: "object",
        required: ["did"],
        properties: { did: { type: "string" } },
      },
    },
  },
  async (req) => {
    return didClient.listKeys({ did: req.query.did });
  }
);

app.get<{ Querystring: { key_id: string; format?: string } }>(
  "/v1/did/keys/export",
  {
    schema: {
      tags: ["did"],
      querystring: {
        type: "object",
        required: ["key_id"],
        properties: {
          key_id: { type: "string" },
          format: { type: "string", enum: ["multibase", "jwk"], default: "multibase" },
        },
      },
    },
  },
  async (req) => {
    return didClient.exportPublicKey({
      keyId: req.query.key_id,
      format: req.query.format || "multibase",
    });
  }
);

// --- Verifiable Credential Routes ---

app.post<{
  Body: {
    issuer_did: string;
    subject_did?: string;
    entity_id?: string;
    credential_type: string;
    credential_subject?: Record<string, unknown>;
    signing_key_id?: string;
    expiration_date?: string;
  };
}>(
  "/v1/vc",
  {
    schema: {
      tags: ["vc"],
      body: {
        type: "object",
        required: ["issuer_did", "credential_type"],
        properties: {
          issuer_did: { type: "string" },
          subject_did: { type: "string" },
          entity_id: { type: "string" },
          credential_type: { type: "string" },
          credential_subject: { type: "object", additionalProperties: true },
          signing_key_id: { type: "string" },
          expiration_date: { type: "string" },
        },
      },
    },
  },
  async (req, reply) => {
    const result = await didClient.issueVerifiableCredential({
      issuerDid: req.body.issuer_did,
      subjectDid: req.body.subject_did || "",
      entityId: req.body.entity_id || "",
      credentialType: req.body.credential_type,
      credentialSubject: req.body.credential_subject || undefined,
      signingKeyId: req.body.signing_key_id || "",
      expirationDate: req.body.expiration_date ? { seconds: Math.floor(new Date(req.body.expiration_date).getTime() / 1000), nanos: 0 } : undefined,
    });
    reply.code(201).send(result);
  }
);

app.get<{ Params: { id: string } }>(
  "/v1/vc/:id",
  {
    schema: {
      tags: ["vc"],
      params: {
        type: "object",
        properties: { id: { type: "string" } },
      },
    },
  },
  async (req) => {
    return didClient.getCredential({ credentialId: req.params.id });
  }
);

app.post<{ Body: { credential_id: string } }>(
  "/v1/vc/verify",
  {
    schema: {
      tags: ["vc"],
      body: {
        type: "object",
        required: ["credential_id"],
        properties: {
          credential_id: { type: "string" },
        },
      },
    },
  },
  async (req) => {
    return didClient.verifyCredential({
      credentialId: req.body.credential_id,
    });
  }
);

app.post<{ Params: { id: string }; Body: { reason?: string } }>(
  "/v1/vc/:id/revoke",
  {
    schema: {
      tags: ["vc"],
      params: {
        type: "object",
        properties: { id: { type: "string" } },
      },
      body: {
        type: "object",
        properties: { reason: { type: "string" } },
      },
    },
  },
  async (req) => {
    return didClient.revokeCredential({
      credentialId: req.params.id,
      reason: req.body?.reason || "",
    });
  }
);

app.get<{
  Querystring: { entity_id?: string; issuer_did?: string; status_filter?: number; page_size?: number; page?: number };
}>(
  "/v1/vc",
  {
    schema: {
      tags: ["vc"],
      querystring: {
        type: "object",
        properties: {
          entity_id: { type: "string" },
          issuer_did: { type: "string" },
          status_filter: { type: "integer" },
          page_size: { type: "integer", default: 20 },
          page: { type: "integer", default: 1 },
        },
      },
    },
  },
  async (req) => {
    return didClient.listCredentials({
      entityId: req.query.entity_id || "",
      issuerDid: req.query.issuer_did || "",
      statusFilter: req.query.status_filter || 0,
      pageSize: req.query.page_size || 20,
      page: req.query.page || 1,
    });
  }
);

app.post<{
  Body: { holder_did: string; credential_ids: string[]; signing_key_id?: string };
}>(
  "/v1/vp",
  {
    schema: {
      tags: ["vc"],
      body: {
        type: "object",
        required: ["holder_did", "credential_ids"],
        properties: {
          holder_did: { type: "string" },
          credential_ids: { type: "array", items: { type: "string" } },
          signing_key_id: { type: "string" },
        },
      },
    },
  },
  async (req, reply) => {
    const result = await didClient.createPresentation({
      holderDid: req.body.holder_did,
      credentialIds: req.body.credential_ids,
      signingKeyId: req.body.signing_key_id || "",
    });
    reply.code(201).send(result);
  }
);

app.post<{ Body: { presentation_json: string } }>(
  "/v1/vp/verify",
  {
    schema: {
      tags: ["vc"],
      body: {
        type: "object",
        required: ["presentation_json"],
        properties: {
          presentation_json: { type: "string" },
        },
      },
    },
  },
  async (req) => {
    return didClient.verifyPresentation({
      presentationJson: req.body.presentation_json,
    });
  }
);

// --- Start ---

try {
  await app.listen({ port: config.port, host: "0.0.0.0" });
  app.log.info(`CAAS API Gateway running on http://localhost:${config.port}`);
  app.log.info(`Swagger docs at http://localhost:${config.port}/docs`);
} catch (err) {
  app.log.error(err);
  process.exit(1);
}
