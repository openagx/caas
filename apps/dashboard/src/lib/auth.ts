const LOGTO_ENDPOINT = process.env.NEXT_PUBLIC_LOGTO_ENDPOINT || "http://localhost:3301";
const LOGTO_APP_ID = process.env.NEXT_PUBLIC_LOGTO_APP_ID || "";
const LOGTO_RESOURCE = process.env.NEXT_PUBLIC_LOGTO_RESOURCE || "https://api.caas.local";
const API_BASE = process.env.NEXT_PUBLIC_API_URL || "http://localhost:3001";
const REDIRECT_URI = process.env.NEXT_PUBLIC_LOGTO_REDIRECT_URI || "http://localhost:3000/callback";

export interface SessionInfo {
  authenticated: boolean;
  sub?: string;
  email?: string;
  name?: string;
  entity_id?: string;
  did?: string;
}

export function getAccessToken(): string | null {
  if (typeof window === "undefined") return null;
  return sessionStorage.getItem("caas_access_token");
}

export function setAccessToken(token: string): void {
  sessionStorage.setItem("caas_access_token", token);
}

export function clearAccessToken(): void {
  sessionStorage.removeItem("caas_access_token");
}

export async function getSession(): Promise<SessionInfo> {
  const token = getAccessToken();
  if (!token) return { authenticated: false };

  try {
    const res = await fetch(`${API_BASE}/auth/session`, {
      headers: { Authorization: `Bearer ${token}` },
    });
    if (!res.ok) {
      clearAccessToken();
      return { authenticated: false };
    }
    return res.json();
  } catch {
    return { authenticated: false };
  }
}

export function buildLoginUrl(): string {
  const state = crypto.randomUUID();
  sessionStorage.setItem("logto_state", state);

  const params = new URLSearchParams({
    client_id: LOGTO_APP_ID,
    redirect_uri: REDIRECT_URI,
    response_type: "code",
    scope: "openid profile email",
    state,
    resource: LOGTO_RESOURCE,
  });

  return `${LOGTO_ENDPOINT}/oidc/auth?${params}`;
}

export async function handleCallback(code: string, state: string): Promise<boolean> {
  const savedState = sessionStorage.getItem("logto_state");
  if (state !== savedState) return false;

  try {
    const res = await fetch(`${LOGTO_ENDPOINT}/oidc/token`, {
      method: "POST",
      headers: { "Content-Type": "application/x-www-form-urlencoded" },
      body: new URLSearchParams({
        grant_type: "authorization_code",
        code,
        redirect_uri: REDIRECT_URI,
        client_id: LOGTO_APP_ID,
      }),
    });

    if (!res.ok) return false;
    const data = await res.json();
    setAccessToken(data.access_token);
    sessionStorage.removeItem("logto_state");
    return true;
  } catch {
    return false;
  }
}

export function buildLogoutUrl(): string {
  clearAccessToken();
  const params = new URLSearchParams({
    client_id: LOGTO_APP_ID,
    post_logout_redirect_uri: "http://localhost:3000",
  });
  return `${LOGTO_ENDPOINT}/oidc/session/end?${params}`;
}

export { LOGTO_ENDPOINT, API_BASE };
