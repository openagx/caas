"use client";

import { useEffect, useState } from "react";
import { KRATOS_PUBLIC_URL } from "../../lib/auth";

export default function RegistrationPage() {
  const [flow, setFlow] = useState<any>(null);
  const [error, setError] = useState("");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [firstName, setFirstName] = useState("");
  const [lastName, setLastName] = useState("");
  const [submitting, setSubmitting] = useState(false);

  useEffect(() => {
    const params = new URLSearchParams(window.location.search);
    const flowId = params.get("flow");

    if (flowId) {
      fetch(`${KRATOS_PUBLIC_URL}/self-service/registration/flows?id=${flowId}`, {
        credentials: "include",
      })
        .then((r) => r.json())
        .then(setFlow)
        .catch(() => setError("Failed to load registration flow"));
    } else {
      window.location.href = `${KRATOS_PUBLIC_URL}/self-service/registration/browser?return_to=${encodeURIComponent(window.location.origin + "/")}`;
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
          password,
          traits: {
            email,
            name: { first: firstName, last: lastName },
          },
          csrf_token: csrfNode?.attributes.value || "",
        }),
      });

      if (res.ok && res.redirected) {
        window.location.href = res.url;
        return;
      }

      const data = await res.json();

      if (data.session || data.identity) {
        window.location.href = "/";
        return;
      }

      if (data.ui?.messages?.length) {
        setError(data.ui.messages.map((m: any) => m.text).join(". "));
      } else {
        setFlow(data);
      }
    } catch {
      setError("Registration failed");
    } finally {
      setSubmitting(false);
    }
  }

  if (!flow) {
    return (
      <div style={{ display: "flex", justifyContent: "center", alignItems: "center", minHeight: "60vh" }}>
        <p style={{ color: "#888" }}>Initializing registration...</p>
      </div>
    );
  }

  return (
    <div style={{ display: "flex", justifyContent: "center", alignItems: "center", minHeight: "60vh" }}>
      <div style={{ width: 380, background: "#161616", border: "1px solid #333", borderRadius: 12, padding: 32 }}>
        <h1 style={{ fontSize: 24, fontWeight: 700, marginBottom: 8, textAlign: "center" }}>Create Account</h1>
        <p style={{ color: "#888", fontSize: 13, textAlign: "center", marginBottom: 24 }}>
          Register for CAAS
        </p>

        {error && (
          <div style={{ background: "#7f1d1d", border: "1px solid #ef4444", borderRadius: 6, padding: 12, marginBottom: 16, fontSize: 13, color: "#fca5a5" }}>
            {error}
          </div>
        )}

        <form onSubmit={handleSubmit}>
          <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 12, marginBottom: 16 }}>
            <label>
              <span style={{ fontSize: 13, color: "#aaa", display: "block", marginBottom: 4 }}>First Name</span>
              <input type="text" value={firstName} onChange={(e) => setFirstName(e.target.value)} style={inputStyle} />
            </label>
            <label>
              <span style={{ fontSize: 13, color: "#aaa", display: "block", marginBottom: 4 }}>Last Name</span>
              <input type="text" value={lastName} onChange={(e) => setLastName(e.target.value)} style={inputStyle} />
            </label>
          </div>

          <label style={{ display: "block", marginBottom: 16 }}>
            <span style={{ fontSize: 13, color: "#aaa", display: "block", marginBottom: 4 }}>Email</span>
            <input type="email" value={email} onChange={(e) => setEmail(e.target.value)} required style={inputStyle} placeholder="you@example.com" />
          </label>

          <label style={{ display: "block", marginBottom: 24 }}>
            <span style={{ fontSize: 13, color: "#aaa", display: "block", marginBottom: 4 }}>Password</span>
            <input type="password" value={password} onChange={(e) => setPassword(e.target.value)} required style={inputStyle} minLength={8} />
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
            {submitting ? "Creating account..." : "Create Account"}
          </button>
        </form>

        <p style={{ marginTop: 16, textAlign: "center", fontSize: 13, color: "#666" }}>
          Already have an account?{" "}
          <a href="/login" style={{ color: "#3b82f6", textDecoration: "none" }}>Sign In</a>
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
