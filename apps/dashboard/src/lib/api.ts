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

export interface Entity {
  entity?: {
    id: string;
    entityType: number;
    displayName: string;
    lifecycleState: number;
    did: string;
    metadata: Record<string, unknown>;
    createdAt: string;
    updatedAt: string;
    suspendedAt?: string;
    revokedAt?: string;
  };
}

export interface EntityList {
  entities: Entity["entity"][];
  nextPageToken: string;
  totalCount: number;
}

export const ENTITY_TYPES: Record<number, string> = {
  1: "Human",
  2: "Organization",
  3: "Device",
  4: "Service",
  5: "AI Agent",
  6: "Autonomous System",
};

export const LIFECYCLE_STATES: Record<number, string> = {
  0: "Unspecified",
  1: "Pending",
  2: "Active",
  3: "Suspended",
  4: "Revoked",
  5: "Archived",
};

export const STATE_COLORS: Record<number, string> = {
  1: "#f59e0b", // pending - amber
  2: "#22c55e", // active - green
  3: "#ef4444", // suspended - red
  4: "#6b7280", // revoked - gray
  5: "#374151", // archived - dark gray
};

export function listEntities(params?: {
  entity_type?: number;
  lifecycle_state?: number;
  page_size?: number;
  page_token?: string;
}): Promise<EntityList> {
  const qs = new URLSearchParams();
  if (params?.entity_type) qs.set("entity_type", String(params.entity_type));
  if (params?.lifecycle_state) qs.set("lifecycle_state", String(params.lifecycle_state));
  if (params?.page_size) qs.set("page_size", String(params.page_size));
  if (params?.page_token) qs.set("page_token", params.page_token);
  const query = qs.toString();
  return apiFetch(`/v1/entities${query ? `?${query}` : ""}`);
}

export function getEntity(id: string): Promise<Entity> {
  return apiFetch(`/v1/entities/${id}`);
}

export function createEntity(body: {
  entity_type: number;
  display_name: string;
  metadata?: Record<string, unknown>;
}): Promise<Entity> {
  return apiFetch("/v1/entities", { method: "POST", body: JSON.stringify(body) });
}

export function checkPermission(body: {
  subject: { type: string; id: string };
  permission: string;
  resource: { type: string; id: string };
}): Promise<{ allowed: boolean; checkedAt: string }> {
  return apiFetch("/v1/authz/check", { method: "POST", body: JSON.stringify(body) });
}
