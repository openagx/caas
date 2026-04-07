"use client";

import { useState } from "react";

export default function PermissionsPage() {
  const [subjectType, setSubjectType] = useState("user");
  const [subjectId, setSubjectId] = useState("");
  const [permission, setPermission] = useState("view");
  const [resourceType, setResourceType] = useState("resource");
  const [resourceId, setResourceId] = useState("");
  const [result, setResult] = useState<{ allowed?: boolean; error?: string } | null>(null);
  const [loading, setLoading] = useState(false);

  async function handleCheck() {
    setLoading(true);
    setResult(null);
    try {
      const apiBase = process.env.NEXT_PUBLIC_API_URL || "http://localhost:3001";
      const res = await fetch(`${apiBase}/v1/authz/check`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          subject: { type: subjectType, id: subjectId },
          permission,
          resource: { type: resourceType, id: resourceId },
        }),
      });
      const data = await res.json();
      setResult(data);
    } catch (e) {
      setResult({ error: e instanceof Error ? e.message : "Request failed" });
    } finally {
      setLoading(false);
    }
  }

  const inputStyle = {
    backgroundColor: "#1a1a1a",
    border: "1px solid #333",
    borderRadius: 6,
    padding: "8px 12px",
    color: "#ededed",
    fontSize: 14,
    width: "100%",
  };

  const labelStyle = {
    display: "block",
    fontSize: 12,
    color: "#888",
    marginBottom: 4,
    textTransform: "uppercase" as const,
    letterSpacing: 1,
  };

  return (
    <div>
      <h1 style={{ fontSize: 28, fontWeight: 700, marginBottom: 8 }}>Permission Tester</h1>
      <p style={{ color: "#888", marginBottom: 24 }}>
        Test Zanzibar-style permission checks against the authorization engine
      </p>

      <div style={{
        backgroundColor: "#161616",
        border: "1px solid #222",
        borderRadius: 8,
        padding: 24,
        maxWidth: 600,
      }}>
        <div style={{ display: "grid", gridTemplateColumns: "1fr 2fr", gap: 12, marginBottom: 16 }}>
          <div>
            <label style={labelStyle}>Subject Type</label>
            <input style={inputStyle} value={subjectType} onChange={(e) => setSubjectType(e.target.value)} placeholder="user" />
          </div>
          <div>
            <label style={labelStyle}>Subject ID</label>
            <input style={inputStyle} value={subjectId} onChange={(e) => setSubjectId(e.target.value)} placeholder="alice" />
          </div>
        </div>

        <div style={{ marginBottom: 16 }}>
          <label style={labelStyle}>Permission</label>
          <input style={inputStyle} value={permission} onChange={(e) => setPermission(e.target.value)} placeholder="view" />
        </div>

        <div style={{ display: "grid", gridTemplateColumns: "1fr 2fr", gap: 12, marginBottom: 20 }}>
          <div>
            <label style={labelStyle}>Resource Type</label>
            <input style={inputStyle} value={resourceType} onChange={(e) => setResourceType(e.target.value)} placeholder="resource" />
          </div>
          <div>
            <label style={labelStyle}>Resource ID</label>
            <input style={inputStyle} value={resourceId} onChange={(e) => setResourceId(e.target.value)} placeholder="doc-1" />
          </div>
        </div>

        <div style={{ fontSize: 13, color: "#666", marginBottom: 16, fontFamily: "monospace" }}>
          {subjectType}:{subjectId || "?"} → <strong style={{ color: "#f59e0b" }}>{permission}</strong> → {resourceType}:{resourceId || "?"}
        </div>

        <button
          onClick={handleCheck}
          disabled={loading || !subjectId || !resourceId}
          style={{
            backgroundColor: "#2563eb",
            color: "white",
            border: "none",
            borderRadius: 6,
            padding: "10px 24px",
            fontSize: 14,
            fontWeight: 600,
            cursor: loading ? "wait" : "pointer",
            opacity: !subjectId || !resourceId ? 0.5 : 1,
          }}
        >
          {loading ? "Checking..." : "Check Permission"}
        </button>

        {result && (
          <div style={{
            marginTop: 16,
            padding: 16,
            borderRadius: 6,
            backgroundColor: result.error ? "#1a0a0a" : result.allowed ? "#0a1a0a" : "#1a1a0a",
            border: `1px solid ${result.error ? "#4a1a1a" : result.allowed ? "#1a4a1a" : "#4a4a1a"}`,
          }}>
            {result.error ? (
              <span style={{ color: "#ef4444" }}>Error: {result.error}</span>
            ) : (
              <span style={{ color: result.allowed ? "#22c55e" : "#ef4444", fontWeight: 600, fontSize: 16 }}>
                {result.allowed ? "✓ ALLOWED" : "✗ DENIED"}
              </span>
            )}
          </div>
        )}
      </div>
    </div>
  );
}
