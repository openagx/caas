import type {
  Entity,
  EntityType,
  LifecycleState,
  CheckResult,
  ObjectReference,
  Relationship,
  TrustScore,
  TrustVerification,
  Endorsement,
  TrustAttestation,
  FraudIncident,
  FraudMetrics,
  DecisionWorkflow,
  DecisionVote,
  ReviewerIntegrity,
  VoteType,
  SovereignNode,
  Treaty,
  AuthorityOverride,
  AuthorityTier,
  VerifiableCredential,
} from "./types.js";

export interface CaasClientOptions {
  /** Base URL of the CAAS API Gateway (e.g., "http://localhost:3001") */
  baseUrl: string;
  /** API key for authentication (future use) */
  apiKey?: string;
  /** Custom fetch implementation (for testing or Node.js < 18) */
  fetch?: typeof globalThis.fetch;
  /** Default request timeout in milliseconds */
  timeout?: number;
}

export class CaasClient {
  private baseUrl: string;
  private apiKey?: string;
  private fetchFn: typeof globalThis.fetch;
  private timeout: number;

  readonly entities: EntityClient;
  readonly authz: AuthzClient;
  readonly trust: TrustClient;
  readonly fraud: FraudClient;
  readonly decisions: DecisionClient;
  readonly federation: FederationClient;
  readonly credentials: CredentialClient;

  constructor(options: CaasClientOptions) {
    this.baseUrl = options.baseUrl.replace(/\/$/, "");
    this.apiKey = options.apiKey;
    this.fetchFn = options.fetch || globalThis.fetch;
    this.timeout = options.timeout || 30000;

    this.entities = new EntityClient(this);
    this.authz = new AuthzClient(this);
    this.trust = new TrustClient(this);
    this.fraud = new FraudClient(this);
    this.decisions = new DecisionClient(this);
    this.federation = new FederationClient(this);
    this.credentials = new CredentialClient(this);
  }

  async request<T>(method: string, path: string, body?: unknown): Promise<T> {
    const headers: Record<string, string> = {
      "Content-Type": "application/json",
    };
    if (this.apiKey) {
      headers["Authorization"] = `Bearer ${this.apiKey}`;
    }

    const controller = new AbortController();
    const timer = setTimeout(() => controller.abort(), this.timeout);

    try {
      const res = await this.fetchFn(`${this.baseUrl}${path}`, {
        method,
        headers,
        body: body ? JSON.stringify(body) : undefined,
        signal: controller.signal,
      });

      if (!res.ok) {
        const text = await res.text();
        throw new CaasError(res.status, text, path);
      }

      return res.json() as Promise<T>;
    } finally {
      clearTimeout(timer);
    }
  }

  async health(): Promise<{ status: string }> {
    return this.request("GET", "/health");
  }
}

export class CaasError extends Error {
  constructor(
    public readonly statusCode: number,
    public readonly body: string,
    public readonly path: string,
  ) {
    super(`CAAS API error ${statusCode} on ${path}: ${body}`);
    this.name = "CaasError";
  }
}

// --- Entity Client ---

class EntityClient {
  constructor(private client: CaasClient) {}

  async create(entityType: EntityType, displayName: string, metadata?: Record<string, unknown>): Promise<{ entity: Entity }> {
    return this.client.request("POST", "/v1/entities", {
      entity_type: entityType,
      display_name: displayName,
      metadata,
    });
  }

  async get(id: string): Promise<{ entity: Entity }> {
    return this.client.request("GET", `/v1/entities/${id}`);
  }

  async update(id: string, displayName?: string, metadata?: Record<string, unknown>): Promise<{ entity: Entity }> {
    return this.client.request("PATCH", `/v1/entities/${id}`, {
      display_name: displayName,
      metadata,
    });
  }

  async list(options?: {
    entityType?: EntityType;
    lifecycleState?: LifecycleState;
    pageSize?: number;
    pageToken?: string;
  }): Promise<{ entities: Entity[]; nextPageToken: string }> {
    const params = new URLSearchParams();
    if (options?.entityType) params.set("entity_type", String(options.entityType));
    if (options?.lifecycleState) params.set("lifecycle_state", String(options.lifecycleState));
    if (options?.pageSize) params.set("page_size", String(options.pageSize));
    if (options?.pageToken) params.set("page_token", options.pageToken);
    const qs = params.toString();
    return this.client.request("GET", `/v1/entities${qs ? `?${qs}` : ""}`);
  }

  async activate(id: string): Promise<{ entity: Entity }> {
    return this.client.request("POST", `/v1/entities/${id}/activate`);
  }

  async suspend(id: string, reason?: string): Promise<{ entity: Entity }> {
    return this.client.request("POST", `/v1/entities/${id}/suspend`, { reason });
  }

  async revoke(id: string, reason?: string): Promise<{ entity: Entity }> {
    return this.client.request("POST", `/v1/entities/${id}/revoke`, { reason });
  }
}

// --- Authorization Client ---

class AuthzClient {
  constructor(private client: CaasClient) {}

  async check(subject: ObjectReference, permission: string, resource: ObjectReference, context?: Record<string, unknown>): Promise<CheckResult> {
    return this.client.request("POST", "/v1/authz/check", {
      subject,
      permission,
      resource,
      context,
    });
  }

  async writeRelationships(relationships: Relationship[]): Promise<{ writtenAt: string }> {
    return this.client.request("POST", "/v1/relationships", { relationships });
  }

  async readRelationships(filter?: {
    resourceType?: string;
    resourceId?: string;
    relation?: string;
    subjectType?: string;
    subjectId?: string;
  }): Promise<{ relationships: Relationship[] }> {
    const params = new URLSearchParams();
    if (filter?.resourceType) params.set("resource_type", filter.resourceType);
    if (filter?.resourceId) params.set("resource_id", filter.resourceId);
    if (filter?.relation) params.set("relation", filter.relation);
    if (filter?.subjectType) params.set("subject_type", filter.subjectType);
    if (filter?.subjectId) params.set("subject_id", filter.subjectId);
    const qs = params.toString();
    return this.client.request("GET", `/v1/relationships${qs ? `?${qs}` : ""}`);
  }
}

// --- Trust Client ---

class TrustClient {
  constructor(private client: CaasClient) {}

  async getScore(entityId: string): Promise<{ score: TrustScore }> {
    return this.client.request("GET", `/v1/trust/${entityId}`);
  }

  async getHistory(entityId: string, limit?: number): Promise<{ history: TrustScore[] }> {
    const qs = limit ? `?limit=${limit}` : "";
    return this.client.request("GET", `/v1/trust/${entityId}/history${qs}`);
  }

  async getGraph(entityId: string, depth?: number): Promise<{ nodes: unknown[]; edges: unknown[] }> {
    const qs = depth ? `?depth=${depth}` : "";
    return this.client.request("GET", `/v1/trust/${entityId}/graph${qs}`);
  }

  async createEndorsement(
    endorserId: string,
    endorsedId: string,
    endorsementType: string,
    weight?: number,
    evidenceHash?: string,
  ): Promise<{ endorsement: Endorsement }> {
    return this.client.request("POST", "/v1/trust/endorsements", {
      endorser_id: endorserId,
      endorsed_id: endorsedId,
      endorsement_type: endorsementType,
      weight: weight || 1.0,
      evidence_hash: evidenceHash,
    });
  }

  async revokeEndorsement(endorsementId: string): Promise<void> {
    return this.client.request("DELETE", `/v1/trust/endorsements/${endorsementId}`);
  }

  async verify(
    entityId: string,
    minimumScore?: number,
    minimumDimensions?: Record<string, number>,
  ): Promise<TrustVerification> {
    return this.client.request("POST", "/v1/trust/verify", {
      entity_id: entityId,
      minimum_score: minimumScore,
      minimum_dimensions: minimumDimensions,
    });
  }

  /**
   * Privacy-preserving trust attestation.
   * Returns a signed attestation proving an entity meets a trust threshold
   * WITHOUT revealing the actual trust score.
   */
  async attest(entityId: string, threshold: number): Promise<TrustAttestation> {
    return this.client.request("POST", "/v1/trust/attest", {
      entity_id: entityId,
      threshold,
    });
  }
}

// --- Fraud Client ---

class FraudClient {
  constructor(private client: CaasClient) {}

  async listIncidents(options?: {
    status?: string;
    minSeverity?: string;
    affectedEntityId?: string;
    pageSize?: number;
  }): Promise<{ incidents: FraudIncident[] }> {
    const params = new URLSearchParams();
    if (options?.status) params.set("status", options.status);
    if (options?.minSeverity) params.set("min_severity", options.minSeverity);
    if (options?.affectedEntityId) params.set("affected_entity_id", options.affectedEntityId);
    if (options?.pageSize) params.set("page_size", String(options.pageSize));
    const qs = params.toString();
    return this.client.request("GET", `/v1/fraud/incidents${qs ? `?${qs}` : ""}`);
  }

  async getIncident(id: string): Promise<{ incident: FraudIncident }> {
    return this.client.request("GET", `/v1/fraud/incidents/${id}`);
  }

  async simulate(attackType: string, targetEntityId: string): Promise<{ incident: FraudIncident }> {
    return this.client.request("POST", "/v1/fraud/simulate", {
      attack_type: attackType,
      target_entity_id: targetEntityId,
    });
  }

  async getMetrics(): Promise<{ metrics: FraudMetrics }> {
    return this.client.request("GET", "/v1/fraud/metrics");
  }
}

// --- Decision Client ---

class DecisionClient {
  constructor(private client: CaasClient) {}

  async createWorkflow(options: {
    workflowType: string;
    subjectEntityId: string;
    caseData?: Record<string, unknown>;
    requiredApprovals?: number;
    totalReviewers?: number;
    blindReview?: boolean;
    coolOffHours?: number;
  }): Promise<{ workflow: DecisionWorkflow }> {
    return this.client.request("POST", "/v1/decisions/workflows", {
      workflow_type: options.workflowType,
      subject_entity_id: options.subjectEntityId,
      case_data: options.caseData,
      required_approvals: options.requiredApprovals || 2,
      total_reviewers: options.totalReviewers || 3,
      blind_review: options.blindReview !== false,
      cool_off_hours: options.coolOffHours || 168,
    });
  }

  async getWorkflow(id: string): Promise<{ workflow: DecisionWorkflow }> {
    return this.client.request("GET", `/v1/decisions/workflows/${id}`);
  }

  async listWorkflows(options?: {
    status?: number;
    subjectEntityId?: string;
    pageSize?: number;
  }): Promise<{ workflows: DecisionWorkflow[]; total: number }> {
    const params = new URLSearchParams();
    if (options?.status) params.set("status", String(options.status));
    if (options?.subjectEntityId) params.set("subject_entity_id", options.subjectEntityId);
    if (options?.pageSize) params.set("page_size", String(options.pageSize));
    const qs = params.toString();
    return this.client.request("GET", `/v1/decisions/workflows${qs ? `?${qs}` : ""}`);
  }

  async submitVote(options: {
    workflowId: string;
    reviewerId: string;
    vote: VoteType;
    reasoning?: string;
    confidence?: number;
    timeSpentSeconds?: number;
  }): Promise<{ vote: DecisionVote }> {
    return this.client.request("POST", "/v1/decisions/votes", {
      workflow_id: options.workflowId,
      reviewer_id: options.reviewerId,
      vote: options.vote,
      reasoning: options.reasoning,
      confidence: options.confidence,
      time_spent_seconds: options.timeSpentSeconds,
    });
  }

  async getBlindCase(workflowId: string, reviewerId: string): Promise<{ blindCaseId: string; caseData: Record<string, unknown> }> {
    return this.client.request("GET", `/v1/decisions/workflows/${workflowId}/blind-case?reviewer_id=${reviewerId}`);
  }

  async getReviewerIntegrity(reviewerId: string): Promise<{ integrity: ReviewerIntegrity }> {
    return this.client.request("GET", `/v1/decisions/reviewers/${reviewerId}/integrity`);
  }
}

// --- Federation Client ---

class FederationClient {
  constructor(private client: CaasClient) {}

  async registerNode(options: {
    name: string;
    jurisdiction: string;
    endpoint: string;
    did?: string;
    publicKey?: string;
  }): Promise<{ node: SovereignNode }> {
    return this.client.request("POST", "/v1/federation/nodes", {
      name: options.name,
      jurisdiction: options.jurisdiction,
      endpoint: options.endpoint,
      did: options.did,
      public_key: options.publicKey,
    });
  }

  async getNode(id: string): Promise<{ node: SovereignNode }> {
    return this.client.request("GET", `/v1/federation/nodes/${id}`);
  }

  async listNodes(options?: { status?: number; pageSize?: number }): Promise<{ nodes: SovereignNode[]; total: number }> {
    const params = new URLSearchParams();
    if (options?.status) params.set("status", String(options.status));
    if (options?.pageSize) params.set("page_size", String(options.pageSize));
    const qs = params.toString();
    return this.client.request("GET", `/v1/federation/nodes${qs ? `?${qs}` : ""}`);
  }

  async proposeTreaty(options: {
    targetNodeId: string;
    trustWeight?: number;
    allowedOperations?: string[];
  }): Promise<{ treaty: Treaty }> {
    return this.client.request("POST", "/v1/federation/treaties", {
      target_node_id: options.targetNodeId,
      trust_weight: options.trustWeight || 0.5,
      allowed_operations: options.allowedOperations,
    });
  }

  async acceptTreaty(treatyId: string): Promise<{ treaty: Treaty }> {
    return this.client.request("POST", `/v1/federation/treaties/${treatyId}/accept`);
  }

  async listTreaties(options?: { nodeId?: string; status?: number }): Promise<{ treaties: Treaty[]; total: number }> {
    const params = new URLSearchParams();
    if (options?.nodeId) params.set("node_id", options.nodeId);
    if (options?.status) params.set("status", String(options.status));
    const qs = params.toString();
    return this.client.request("GET", `/v1/federation/treaties${qs ? `?${qs}` : ""}`);
  }

  async createAuthorityOverride(options: {
    workflowId: string;
    authorityEntityId: string;
    tier: AuthorityTier;
    justification: string;
    outcome: string;
    legalReference?: string;
    expiresAt?: string;
  }): Promise<{ override: AuthorityOverride }> {
    return this.client.request("POST", "/v1/authority/overrides", {
      workflow_id: options.workflowId,
      authority_entity_id: options.authorityEntityId,
      tier: options.tier,
      justification: options.justification,
      outcome: options.outcome,
      legal_reference: options.legalReference,
      expires_at: options.expiresAt,
    });
  }

  async listAuthorityOverrides(workflowId?: string): Promise<{ overrides: AuthorityOverride[]; total: number }> {
    const qs = workflowId ? `?workflow_id=${workflowId}` : "";
    return this.client.request("GET", `/v1/authority/overrides${qs}`);
  }
}

// --- Credential Client ---

class CredentialClient {
  constructor(private client: CaasClient) {}

  async issue(options: {
    entityId: string;
    credentialType: string;
    claims?: Record<string, unknown>;
    expiresAt?: string;
  }): Promise<{ credential: VerifiableCredential }> {
    return this.client.request("POST", "/v1/credentials", {
      entity_id: options.entityId,
      credential_type: options.credentialType,
      claims: options.claims,
      expires_at: options.expiresAt,
    });
  }

  async verify(credentialId: string): Promise<{ valid: boolean; reason: string; credential: VerifiableCredential }> {
    return this.client.request("GET", `/v1/credentials/${credentialId}/verify`);
  }

  async revoke(credentialId: string, reason?: string): Promise<{ credential: VerifiableCredential }> {
    return this.client.request("POST", `/v1/credentials/${credentialId}/revoke`, { reason });
  }

  async list(options?: {
    entityId?: string;
    credentialType?: string;
    pageSize?: number;
  }): Promise<{ credentials: VerifiableCredential[]; total: number }> {
    const params = new URLSearchParams();
    if (options?.entityId) params.set("entity_id", options.entityId);
    if (options?.credentialType) params.set("credential_type", options.credentialType);
    if (options?.pageSize) params.set("page_size", String(options.pageSize));
    const qs = params.toString();
    return this.client.request("GET", `/v1/credentials${qs ? `?${qs}` : ""}`);
  }
}
