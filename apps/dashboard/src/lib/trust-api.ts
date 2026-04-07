const API_BASE = process.env.NEXT_PUBLIC_API_URL || "http://localhost:3001";

async function apiFetch<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(`${API_BASE}${path}`, {
    ...init,
    headers: { "Content-Type": "application/json", ...init?.headers },
    cache: "no-store",
  });
  if (!res.ok) {
    throw new Error(`API error ${res.status}: ${await res.text()}`);
  }
  return res.json();
}

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
  decayRate: number;
  lastUpdated: string;
  calculationReason: string;
}

export interface TrustGraph {
  nodes: Array<{
    entityId: string;
    displayName: string;
    entityType: string;
    trustScore: number;
  }>;
  edges: Array<{
    sourceId: string;
    targetId: string;
    trustScore: number;
    relationshipType: string;
  }>;
}

export const DIMENSION_META: Record<string, { label: string; max: number; color: string }> = {
  identityVerification: { label: "Identity Verification", max: 150, color: "#3b82f6" },
  behavioralConsistency: { label: "Behavioral Consistency", max: 200, color: "#8b5cf6" },
  networkReputation: { label: "Network Reputation", max: 150, color: "#06b6d4" },
  transactionHistory: { label: "Transaction History", max: 150, color: "#10b981" },
  complianceAdherence: { label: "Compliance Adherence", max: 100, color: "#f59e0b" },
  temporalStability: { label: "Temporal Stability", max: 100, color: "#ef4444" },
  peerEndorsement: { label: "Peer Endorsement", max: 150, color: "#ec4899" },
};

export function getTrustScore(entityId: string): Promise<{ score: TrustScore }> {
  return apiFetch(`/v1/trust/${entityId}`);
}

export function getTrustHistory(entityId: string, limit = 50): Promise<{ scores: TrustScore[] }> {
  return apiFetch(`/v1/trust/${entityId}/history?limit=${limit}`);
}

export function getTrustGraph(entityId: string, depth = 2): Promise<TrustGraph> {
  return apiFetch(`/v1/trust/${entityId}/graph?depth=${depth}`);
}

export function verifyTrust(body: {
  entity_id: string;
  minimum_score?: number;
  minimum_dimensions?: Record<string, number>;
}): Promise<{
  meetsRequirements: boolean;
  currentScore: TrustScore;
  failedDimensions: string[];
}> {
  return apiFetch("/v1/trust/verify", { method: "POST", body: JSON.stringify(body) });
}
