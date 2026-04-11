const KRATOS_PUBLIC_URL = process.env.NEXT_PUBLIC_KRATOS_URL || "http://localhost:4433";
const API_BASE = process.env.NEXT_PUBLIC_API_URL || "http://localhost:3001";

export interface KratosFlow {
  id: string;
  type: string;
  ui: {
    action: string;
    method: string;
    nodes: KratosNode[];
    messages?: { id: number; text: string; type: string }[];
  };
}

export interface KratosNode {
  type: string;
  group: string;
  attributes: {
    name: string;
    type: string;
    value?: string;
    required?: boolean;
    disabled?: boolean;
    node_type: string;
  };
  messages: { id: number; text: string; type: string }[];
  meta: { label?: { id: number; text: string } };
}

export interface SessionInfo {
  authenticated: boolean;
  identity?: string;
  email?: string;
  name?: { first?: string; last?: string };
  entity_id?: string;
  did?: string;
}

export async function getSession(): Promise<SessionInfo> {
  try {
    const res = await fetch(`${API_BASE}/auth/session`, { credentials: "include" });
    if (!res.ok) return { authenticated: false };
    return res.json();
  } catch {
    return { authenticated: false };
  }
}

export async function getLoginFlow(flowId?: string): Promise<KratosFlow> {
  if (flowId) {
    const res = await fetch(`${KRATOS_PUBLIC_URL}/self-service/login/flows?id=${flowId}`, {
      credentials: "include",
    });
    return res.json();
  }
  // Initialize new flow — browser redirect
  const res = await fetch(`${KRATOS_PUBLIC_URL}/self-service/login/api`, {
    credentials: "include",
  });
  return res.json();
}

export async function getRegistrationFlow(flowId?: string): Promise<KratosFlow> {
  if (flowId) {
    const res = await fetch(`${KRATOS_PUBLIC_URL}/self-service/registration/flows?id=${flowId}`, {
      credentials: "include",
    });
    return res.json();
  }
  const res = await fetch(`${KRATOS_PUBLIC_URL}/self-service/registration/api`, {
    credentials: "include",
  });
  return res.json();
}

export async function submitFlow(action: string, body: Record<string, string>): Promise<any> {
  const res = await fetch(action, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    credentials: "include",
    body: JSON.stringify(body),
  });
  return res.json();
}

export async function logout(): Promise<void> {
  try {
    const res = await fetch(`${KRATOS_PUBLIC_URL}/self-service/logout/browser`, {
      credentials: "include",
    });
    const data = await res.json();
    if (data.logout_url) {
      window.location.href = data.logout_url;
    }
  } catch {
    window.location.href = "/login";
  }
}

export { KRATOS_PUBLIC_URL, API_BASE };
