// --- Entity Types ---

export enum EntityType {
  UNSPECIFIED = 0,
  HUMAN = 1,
  ORGANIZATION = 2,
  DEVICE = 3,
  SERVICE = 4,
  AI_AGENT = 5,
  AUTONOMOUS_SYSTEM = 6,
}

export enum LifecycleState {
  UNSPECIFIED = 0,
  PENDING = 1,
  ACTIVE = 2,
  SUSPENDED = 3,
  REVOKED = 4,
  ARCHIVED = 5,
}

export interface Entity {
  id: string;
  did?: string;
  entityType: EntityType;
  displayName: string;
  lifecycleState: LifecycleState;
  metadata?: Record<string, unknown>;
  createdAt: string;
  updatedAt: string;
}

// --- Authorization ---

export interface ObjectReference {
  type: string;
  id: string;
}

export interface CheckResult {
  allowed: boolean;
  checkedAt?: string;
}

export interface Relationship {
  resource: ObjectReference;
  relation: string;
  subject: ObjectReference;
}

// --- Trust ---

export interface TrustDimensions {
  identityVerification: number;
  behavioralConsistency: number;
  networkReputation: number;
  transactionHistory: number;
  complianceAdherence: number;
  temporalStability: number;
  peerEndorsement: number;
}

export interface TrustScore {
  entityId: string;
  overallScore: number;
  dimensions: TrustDimensions;
  calculatedAt: string;
}

export interface TrustVerification {
  trusted: boolean;
  overallScore: number;
  failedDimensions: string[];
}

export interface Endorsement {
  id: string;
  endorserId: string;
  endorsedId: string;
  endorsementType: string;
  weight: number;
  createdAt: string;
}

export interface TrustGraphNode {
  entityId: string;
  displayName: string;
  trustScore: number;
}

export interface TrustGraphEdge {
  sourceEntityId: string;
  targetEntityId: string;
  trustScore: number;
}

// --- Fraud ---

export interface FraudIncident {
  id: string;
  incidentType: string;
  severity: string;
  status: string;
  affectedEntityId: string;
  detectionTimestamp: string;
  containmentTimestamp?: string;
  detectionLatencyMs: number;
  evidence: Record<string, unknown>;
  timeline: FraudTimelineEntry[];
  createdAt: string;
}

export interface FraudTimelineEntry {
  stage: string;
  action: string;
  timestamp: string;
  latencyMs: number;
  detail: string;
}

export interface FraudMetrics {
  totalIncidents: number;
  openIncidents: number;
  avgDetectionLatencyMs: number;
  avgContainmentLatencyMs: number;
  falsePositiveCount: number;
  falsePositiveRate: number;
}

// --- Decisions ---

export enum WorkflowStatus {
  UNSPECIFIED = 0,
  PENDING = 1,
  IN_REVIEW = 2,
  APPROVED = 3,
  REJECTED = 4,
  ESCALATED = 5,
}

export enum VoteType {
  UNSPECIFIED = 0,
  APPROVE = 1,
  REJECT = 2,
  ESCALATE = 3,
  ABSTAIN = 4,
}

export interface DecisionWorkflow {
  id: string;
  workflowType: string;
  requiredApprovals: number;
  totalReviewers: number;
  blindReview: boolean;
  coolOffHours: number;
  status: WorkflowStatus;
  subjectEntityId: string;
  outcome?: string;
  votes: DecisionVote[];
  createdAt: string;
  completedAt?: string;
}

export interface DecisionVote {
  id: string;
  workflowId: string;
  reviewerId: string;
  vote: VoteType;
  reasoning: string;
  confidence: number;
  timeSpentSeconds: number;
  blindCaseId: string;
  createdAt: string;
}

export interface ReviewerIntegrity {
  reviewerId: string;
  integrityScore: number;
  totalReviews: number;
  approvalRate: number;
  avgTimePerReview: number;
  biasFlags: number;
}

// --- Federation ---

export enum SovereignNodeStatus {
  UNSPECIFIED = 0,
  ACTIVE = 1,
  SUSPENDED = 2,
  REVOKED = 3,
}

export enum TreatyStatus {
  UNSPECIFIED = 0,
  PROPOSED = 1,
  ACTIVE = 2,
  SUSPENDED = 3,
  TERMINATED = 4,
}

export enum AuthorityTier {
  UNSPECIFIED = 0,
  INDIVIDUAL = 1,
  INSTITUTIONAL = 2,
  REGULATORY = 3,
  JUDICIAL = 4,
  SOVEREIGN_EMERGENCY = 5,
}

export interface SovereignNode {
  id: string;
  name: string;
  did: string;
  jurisdiction: string;
  endpoint: string;
  status: SovereignNodeStatus;
  createdAt: string;
}

export interface Treaty {
  id: string;
  sourceNodeId: string;
  targetNodeId: string;
  status: TreatyStatus;
  trustWeight: number;
  allowedOperations: string[];
  validFrom: string;
  validUntil?: string;
  createdAt: string;
}

export interface AuthorityOverride {
  id: string;
  workflowId: string;
  authorityEntityId: string;
  tier: AuthorityTier;
  justification: string;
  legalReference?: string;
  outcome: string;
  expiresAt?: string;
  createdAt: string;
}

export interface VerifiableCredential {
  id: string;
  entityId: string;
  issuerDid: string;
  credentialType: string;
  claims: Record<string, unknown>;
  proofSignature: string;
  issuedAt: string;
  expiresAt?: string;
  revoked: boolean;
}

// --- Trust Attestation (Privacy-Preserving) ---

export interface TrustAttestation {
  entityId: string;
  meetsThreshold: boolean;
  threshold: number;
  attestationHash: string;
  expiresAt: string;
  issuedAt: string;
}

// --- Pagination ---

export interface PaginatedResponse<T> {
  items: T[];
  total: number;
  pageToken?: string;
}
