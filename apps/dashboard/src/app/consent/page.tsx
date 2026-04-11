"use client";

import { useEffect, useState } from "react";

const API_BASE = process.env.NEXT_PUBLIC_API_URL || "http://localhost:3001";

interface ConsentInfo {
  challenge: string;
  requested_scope: string[];
  client: { client_name?: string; client_id: string };
  subject: string;
}

export default function ConsentPage() {
  const [info, setInfo] = useState<ConsentInfo | null>(null);
  const [error, setError] = useState("");

  useEffect(() => {
    const params = new URLSearchParams(window.location.search);
    const challenge = params.get("consent_challenge");
    if (!challenge) {
      setError("No consent challenge provided");
      return;
    }

    fetch(`${API_BASE}/auth/hydra/consent?consent_challenge=${challenge}`, {
      credentials: "include",
      redirect: "manual",
    }).then((res) => {
      if (res.type === "opaqueredirect" || res.redirected) {
        // Auto-accepted (skip=true), follow redirect
        window.location.href = res.url || "/";
        return;
      }
      // Need to show consent UI — extract info from the challenge
      setInfo({
        challenge,
        requested_scope: ["openid", "offline_access"],
        client: { client_id: "caas-dashboard", client_name: "CAAS Dashboard" },
        subject: "",
      });
    }).catch(() => setError("Failed to load consent"));
  }, []);

  async function handleAccept() {
    if (!info) return;
    try {
      const res = await fetch(`${API_BASE}/auth/hydra/consent/accept`, {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        credentials: "include",
        body: JSON.stringify({
          consent_challenge: info.challenge,
          grant_scope: info.requested_scope,
        }),
      });
      const data = await res.json();
      if (data.redirect_to) {
        window.location.href = data.redirect_to;
      } else {
        window.location.href = "/";
      }
    } catch {
      setError("Failed to accept consent");
    }
  }

  if (error) {
    return (
      <div style={{ display: "flex", justifyContent: "center", alignItems: "center", minHeight: "60vh" }}>
        <div style={{ background: "#7f1d1d", border: "1px solid #ef4444", borderRadius: 8, padding: 24, color: "#fca5a5" }}>
          {error}
        </div>
      </div>
    );
  }

  if (!info) {
    return (
      <div style={{ display: "flex", justifyContent: "center", alignItems: "center", minHeight: "60vh" }}>
        <p style={{ color: "#888" }}>Loading consent...</p>
      </div>
    );
  }

  return (
    <div style={{ display: "flex", justifyContent: "center", alignItems: "center", minHeight: "60vh" }}>
      <div style={{ width: 420, background: "#161616", border: "1px solid #333", borderRadius: 12, padding: 32 }}>
        <h1 style={{ fontSize: 22, fontWeight: 700, marginBottom: 8, textAlign: "center" }}>Authorize Access</h1>
        <p style={{ color: "#888", fontSize: 13, textAlign: "center", marginBottom: 24 }}>
          <strong style={{ color: "#ededed" }}>{info.client.client_name || info.client.client_id}</strong> is requesting access to your account.
        </p>

        <div style={{ background: "#1a1a1a", borderRadius: 8, padding: 16, marginBottom: 24 }}>
          <p style={{ fontSize: 13, color: "#aaa", marginBottom: 8 }}>Requested permissions:</p>
          <ul style={{ margin: 0, paddingLeft: 20 }}>
            {info.requested_scope.map((scope) => (
              <li key={scope} style={{ color: "#ccc", fontSize: 13, marginBottom: 4 }}>{scope}</li>
            ))}
          </ul>
        </div>

        <div style={{ display: "flex", gap: 12 }}>
          <button onClick={() => window.location.href = "/"} style={{
            flex: 1, padding: "10px 0", background: "#333", color: "#ccc",
            border: "none", borderRadius: 6, cursor: "pointer", fontSize: 14,
          }}>
            Deny
          </button>
          <button onClick={handleAccept} style={{
            flex: 1, padding: "10px 0", background: "#3b82f6", color: "#fff",
            border: "none", borderRadius: 6, cursor: "pointer", fontSize: 14, fontWeight: 600,
          }}>
            Allow
          </button>
        </div>
      </div>
    </div>
  );
}
