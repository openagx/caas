/**
 * OPA Input Builder
 * 
 * Builds normalized input for OPA sidecar from:
 * - User identity + trust score
 * - Session metadata
 * - Request context
 * - SpiceDB response
 * - Behavioral metrics (from telemetry service)
 */

export interface OPAAuthZInput {
  // SpiceDB layer result (from authz-engine service)
  spicedb: {
    allowed: boolean;
    relationships?: string[];
  };

  // Subject (user) information
  subject: {
    id: string;
    entity_type: string;
    auth_strength: string; // "none" | "password" | "mfa" | "certificate"
    trust_score: number;
  };

  // Resource being accessed
  resource: {
    type: string;
    id: string;
    sensitivity: string; // "low" | "medium" | "high"
  };

  // Context from request
  context: {
    ip: string;
    user_agent: string;
    device_id: string;
    device_trust: number;
    ip_reputation: number;
    ip_country?: string;
    session_id: string;
  };

  // Behavioral metrics (from fraud-pipeline or trust-engine)
  behavior: {
    velocity_ops_5m: number;
    velocity_ops_1h: number;
    anomaly_score: number;
    drift_score: number;
  };
}

export interface OPADecision {
  allow: boolean;
  reason: string[];
  risk_score: number;
  recommended_action: "allow" | "step_up_auth" | "deny";
}

export interface BuildOPAInputOptions {
  /** SpiceDB layer result */
  spiceDBAllowed: boolean;
  spiceDBRelationships?: string[];
  
  /** User identity */
  entityId: string;
  
  /** Resource being accessed */
  resourceType: string;
  resourceId: string;
  
  /** Request/session context */
  sessionContext: SessionContext;
  
  /** Optional: trust score from trust-engine */
  trustScore?: number;
  
  /** Optional: behavioral metrics from fraud/telemetry */
  behavioralMetrics?: BehavioralMetrics;
}

export interface SessionContext {
  ip: string;
  userAgent: string;
  deviceId?: string;
  deviceTrust?: number;
  ipReputation?: number;
  sessionId?: string;
  ipCountry?: string;
}

export interface BehavioralMetrics {
  velocityOps5m?: number;
  velocityOps1h?: number;
  anomalyScore?: number;
  driftScore?: number;
}

/**
 * Build the OPA authorization input from request context
 */
export async function buildOPAAuthZInput(options: BuildOPAInputOptions): Promise<OPAAuthZInput> {
  const {
    spiceDBAllowed,
    spiceDBRelationships = [],
    entityId,
    resourceType,
    resourceId,
    sessionContext,
    trustScore = 0.7, // Default to moderate trust
    behavioralMetrics = {},
  } = options;

  // Determine resource sensitivity based on type
  const sensitivity = getResourceSensitivity(resourceType);

  // Determine auth strength (could be enhanced with actual session data)
  const authStrength = getAuthStrength(sessionContext);

  return {
    spicedb: {
      allowed: spiceDBAllowed,
      relationships: spiceDBRelationships,
    },
    subject: {
      id: entityId,
      entity_type: "user",
      auth_strength: authStrength,
      trust_score: trustScore,
    },
    resource: {
      type: resourceType,
      id: resourceId,
      sensitivity,
    },
    context: {
      ip: sessionContext.ip || "unknown",
      user_agent: sessionContext.userAgent || "unknown",
      device_id: sessionContext.deviceId || "",
      device_trust: sessionContext.deviceTrust ?? 0.5,
      ip_reputation: sessionContext.ipReputation ?? 0.5,
      ip_country: sessionContext.ipCountry,
      session_id: sessionContext.sessionId || "",
    },
    behavior: {
      velocity_ops_5m: behavioralMetrics.velocityOps5m ?? 0,
      velocity_ops_1h: behavioralMetrics.velocityOps1h ?? 0,
      anomaly_score: behavioralMetrics.anomalyScore ?? 0,
      drift_score: behavioralMetrics.driftScore ?? 0,
    },
  };
}

/**
 * Determine resource sensitivity based on type
 */
function getResourceSensitivity(resourceType: string): "low" | "medium" | "high" {
  // High sensitivity resources
  const highSensitivity = [
    "credential",
    "key",
    "secret",
    "admin",
    "did",
    "vc",
  ];
  
  // Medium sensitivity
  const mediumSensitivity = [
    "relationship",
    "trust",
    "decision",
    "federation",
  ];

  const lowerType = resourceType.toLowerCase();
  
  if (highSensitivity.includes(lowerType)) {
    return "high";
  }
  
  if (mediumSensitivity.includes(lowerType)) {
    return "medium";
  }
  
  return "low";
}

/**
 * Determine auth strength from session context
 */
function getAuthStrength(context: SessionContext): string {
  // If device trust is high and session exists, likely MFA
  if ((context.deviceTrust ?? 0) > 0.8 && context.sessionId) {
    return "mfa";
  }
  
  // If just device trust, likely password + device
  if ((context.deviceTrust ?? 0) > 0.3) {
    return "password";
  }
  
  return "none";
}