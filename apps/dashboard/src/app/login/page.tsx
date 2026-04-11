"use client";

import { useEffect, useState } from "react";
import { KRATOS_PUBLIC_URL } from "../../lib/auth";

export default function LoginPage() {
  const [flow, setFlow] = useState<any>(null);
  const [error, setError] = useState("");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [submitting, setSubmitting] = useState(false);

  useEffect(() => {
    const params = new URLSearchParams(window.location.search);
    const flowId = params.get("flow");

    if (flowId) {
      fetch(`${KRATOS_PUBLIC_URL}/self-service/login/flows?id=${flowId}`, {
        credentials: "include",
      })
        .then((r) => r.json())
        .then(setFlow)
        .catch(() => setError("Failed to load login flow"));
    } else {
      // Redirect to Kratos to initialize a new browser flow
      window.location.href = `${KRATOS_PUBLIC_URL}/self-service/login/browser?return_to=${encodeURIComponent(window.location.origin + "/")}`;
    }
  }, []);

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    if (!flow) return;
    setSubmitting(true);
    setError("");

    try {
      const csrfNode = flow.ui.nodes.find(
        (n: any) => n.attributes.name === "csrf_token"
      );

      const res = await fetch(flow.ui.action, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        credentials: "include",
        body: JSON.stringify({
          method: "password",
          identifier: email,
          password,
          csrf_token: csrfNode?.attributes.value || "",
        }),
      });

      if (res.ok && res.redirected) {
        window.location.href = res.url;
        return;
      }

      const data = await res.json();

      if (data.session) {
        window.location.href = "/";
        return;
      }

      if (data.ui?.messages?.length) {
        setError(data.ui.messages.map((m: any) => m.text).join(". "));
      } else {
        setFlow(data);
      }
    } catch {
      setError("Login failed");
    } finally {
      setSubmitting(false);
    }
  }

  if (!flow) {
    return (
      <div style={{ display: "flex", justifyContent: "center", alignItems: "center", minHeight: "60vh" }}>
        <p style={{ color: "#888" }}>Initializing login...</p>
      </div>
    );
  }

  return (
    <div style={{ display: "flex", justifyContent: "center", alignItems: "center", minHeight: "60vh" }}>
      <div style={{ width: 380, background: "#161616", border: "1px solid #333", borderRadius: 12, padding: 32 }}>
        <h1 style={{ fontSize: 24, fontWeight: 700, marginBottom: 8, textAlign: "center" }}>Sign In</h1>
        <p style={{ color: "#888", fontSize: 13, textAlign: "center", marginBottom: 24 }}>
          CAAS — Continuous Autonomous Authorization System
        </p>

        {error && (
          <div style={{ background: "#7f1d1d", border: "1px solid #ef4444", borderRadius: 6, padding: 12, marginBottom: 16, fontSize: 13, color: "#fca5a5" }}>
            {error}
          </div>
        )}

        <form onSubmit={handleSubmit}>
          <label style={{ display: "block", marginBottom: 16 }}>
            <span style={{ fontSize: 13, color: "#aaa", display: "block", marginBottom: 4 }}>Email</span>
            <input
              type="email"
              value={email}
              onChange={(e) => setEmail(e.target.value)}
              required
              style={inputStyle}
              placeholder="you@example.com"
            />
          </label>

          <label style={{ display: "block", marginBottom: 24 }}>
            <span style={{ fontSize: 13, color: "#aaa", display: "block", marginBottom: 4 }}>Password</span>
            <input
              type="password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              required
              style={inputStyle}
            />
          </label>

          <button type="submit" disabled={submitting} style={{
            width: "100%",
            padding: "10px 0",
            background: submitting ? "#555" : "#3b82f6",
            color: "#fff",
            border: "none",
            borderRadius: 6,
            cursor: submitting ? "not-allowed" : "pointer",
            fontSize: 14,
            fontWeight: 600,
          }}>
            {submitting ? "Signing in..." : "Sign In"}
          </button>
        </form>

        <p style={{ marginTop: 16, textAlign: "center", fontSize: 13, color: "#666" }}>
          No account?{" "}
          <a href="/registration" style={{ color: "#3b82f6", textDecoration: "none" }}>Register</a>
        </p>
      </div>
    </div>
  );
}

const inputStyle: React.CSSProperties = {
  width: "100%",
  padding: "8px 12px",
  background: "#1a1a1a",
  border: "1px solid #333",
  borderRadius: 6,
  color: "#ededed",
  fontSize: 14,
  boxSizing: "border-box",
};
