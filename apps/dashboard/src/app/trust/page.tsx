"use client";

import { useState } from "react";
import { DIMENSION_META, type TrustScore, type TrustGraph } from "@/lib/trust-api";

export default function TrustPage() {
  const [entityId, setEntityId] = useState("");
  const [score, setScore] = useState<TrustScore | null>(null);
  const [graph, setGraph] = useState<TrustGraph | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);

  const apiBase = process.env.NEXT_PUBLIC_API_URL || "http://localhost:3001";

  async function handleLookup() {
    if (!entityId) return;
    setLoading(true);
    setError(null);
    setScore(null);
    setGraph(null);

    try {
      const [scoreRes, graphRes] = await Promise.allSettled([
        fetch(`${apiBase}/v1/trust/${entityId}`, { cache: "no-store" }).then((r) => r.json()),
        fetch(`${apiBase}/v1/trust/${entityId}/graph?depth=2`, { cache: "no-store" }).then((r) => r.json()),
      ]);

      if (scoreRes.status === "fulfilled" && scoreRes.value?.score) {
        setScore(scoreRes.value.score);
      }
      if (graphRes.status === "fulfilled" && graphRes.value?.nodes) {
        setGraph(graphRes.value);
      }
      if (scoreRes.status === "rejected" && graphRes.status === "rejected") {
        setError("Could not retrieve trust data. Is the trust-engine running?");
      }
    } catch (e) {
      setError(e instanceof Error ? e.message : "Request failed");
    } finally {
      setLoading(false);
    }
  }

  return (
    <div>
      <h1 style={{ fontSize: 28, fontWeight: 700, marginBottom: 8 }}>Trust Scores</h1>
      <p style={{ color: "#888", marginBottom: 24 }}>
        7-dimension trust scoring powered by Neo4j graph + ML sidecar
      </p>

      {/* Lookup */}
      <div style={{ display: "flex", gap: 12, marginBottom: 24 }}>
        <input
          value={entityId}
          onChange={(e) => setEntityId(e.target.value)}
          placeholder="Enter entity ID..."
          style={{
            flex: 1,
            maxWidth: 400,
            backgroundColor: "#1a1a1a",
            border: "1px solid #333",
            borderRadius: 6,
            padding: "10px 14px",
            color: "#ededed",
            fontSize: 14,
          }}
          onKeyDown={(e) => e.key === "Enter" && handleLookup()}
        />
        <button
          onClick={handleLookup}
          disabled={loading || !entityId}
          style={{
            backgroundColor: "#7c3aed",
            color: "white",
            border: "none",
            borderRadius: 6,
            padding: "10px 20px",
            fontSize: 14,
            fontWeight: 600,
            cursor: loading ? "wait" : "pointer",
            opacity: !entityId ? 0.5 : 1,
          }}
        >
          {loading ? "Loading..." : "Lookup Trust"}
        </button>
      </div>

      {error && (
        <div style={{
          backgroundColor: "#1a1a1a",
          border: "1px solid #333",
          borderRadius: 8,
          padding: 24,
          textAlign: "center",
          color: "#888",
          marginBottom: 24,
        }}>
          {error}
        </div>
      )}

      {/* Score Display */}
      {score && (
        <div style={{ display: "grid", gridTemplateColumns: "300px 1fr", gap: 24, marginBottom: 24 }}>
          {/* Overall Score */}
          <div style={{
            backgroundColor: "#161616",
            border: "1px solid #222",
            borderRadius: 8,
            padding: 24,
            textAlign: "center",
          }}>
            <div style={{ fontSize: 12, color: "#888", textTransform: "uppercase", letterSpacing: 1 }}>
              Overall Trust Score
            </div>
            <div style={{
              fontSize: 64,
              fontWeight: 700,
              margin: "12px 0",
              color: score.overallScore >= 700 ? "#22c55e" :
                     score.overallScore >= 400 ? "#f59e0b" : "#ef4444",
            }}>
              {score.overallScore}
            </div>
            <div style={{ fontSize: 13, color: "#666" }}>out of 1000</div>
            <div style={{
              marginTop: 12,
              width: "100%",
              height: 8,
              backgroundColor: "#222",
              borderRadius: 4,
              overflow: "hidden",
            }}>
              <div style={{
                width: `${(score.overallScore / 1000) * 100}%`,
                height: "100%",
                backgroundColor: score.overallScore >= 700 ? "#22c55e" :
                                 score.overallScore >= 400 ? "#f59e0b" : "#ef4444",
                borderRadius: 4,
                transition: "width 0.5s ease",
              }} />
            </div>
            {score.calculationReason && (
              <div style={{ fontSize: 11, color: "#555", marginTop: 8 }}>
                Reason: {score.calculationReason}
              </div>
            )}
          </div>

          {/* Dimension Breakdown */}
          <div style={{
            backgroundColor: "#161616",
            border: "1px solid #222",
            borderRadius: 8,
            padding: 24,
          }}>
            <div style={{ fontSize: 14, fontWeight: 600, marginBottom: 16 }}>
              Dimension Breakdown
            </div>
            {score.dimensions && Object.entries(DIMENSION_META).map(([key, meta]) => {
              const value = (score.dimensions as any)[key] || 0;
              const pct = (value / meta.max) * 100;
              return (
                <div key={key} style={{ marginBottom: 12 }}>
                  <div style={{ display: "flex", justifyContent: "space-between", fontSize: 13, marginBottom: 4 }}>
                    <span style={{ color: "#aaa" }}>{meta.label}</span>
                    <span style={{ color: meta.color, fontWeight: 600 }}>{value} / {meta.max}</span>
                  </div>
                  <div style={{ width: "100%", height: 6, backgroundColor: "#222", borderRadius: 3 }}>
                    <div style={{
                      width: `${pct}%`,
                      height: "100%",
                      backgroundColor: meta.color,
                      borderRadius: 3,
                      transition: "width 0.5s ease",
                    }} />
                  </div>
                </div>
              );
            })}
          </div>
        </div>
      )}

      {/* Trust Graph */}
      {graph && graph.nodes && graph.nodes.length > 0 && (
        <div style={{
          backgroundColor: "#161616",
          border: "1px solid #222",
          borderRadius: 8,
          padding: 24,
        }}>
          <div style={{ fontSize: 14, fontWeight: 600, marginBottom: 16 }}>
            Trust Graph ({graph.nodes.length} nodes, {graph.edges?.length || 0} edges)
          </div>
          <div style={{ display: "flex", flexWrap: "wrap", gap: 8 }}>
            {graph.nodes.map((node) => (
              <div
                key={node.entityId}
                style={{
                  backgroundColor: node.entityId === entityId ? "#7c3aed22" : "#1a1a1a",
                  border: `1px solid ${node.entityId === entityId ? "#7c3aed" : "#333"}`,
                  borderRadius: 6,
                  padding: "8px 12px",
                  fontSize: 13,
                }}
              >
                <div style={{ fontWeight: 500 }}>{node.displayName || node.entityId.slice(0, 8)}</div>
                <div style={{ fontSize: 11, color: "#888" }}>
                  {node.entityType || "entity"} — score: {node.trustScore || "?"}
                </div>
              </div>
            ))}
          </div>
          {graph.edges && graph.edges.length > 0 && (
            <div style={{ marginTop: 16 }}>
              <div style={{ fontSize: 12, color: "#666", marginBottom: 8 }}>Edges</div>
              <table style={{ width: "100%", borderCollapse: "collapse", fontSize: 12 }}>
                <thead>
                  <tr style={{ borderBottom: "1px solid #222" }}>
                    <th style={{ textAlign: "left", padding: 6, color: "#666" }}>Source</th>
                    <th style={{ textAlign: "left", padding: 6, color: "#666" }}>Target</th>
                    <th style={{ textAlign: "left", padding: 6, color: "#666" }}>Type</th>
                    <th style={{ textAlign: "right", padding: 6, color: "#666" }}>Score</th>
                  </tr>
                </thead>
                <tbody>
                  {graph.edges.map((edge, i) => (
                    <tr key={i} style={{ borderBottom: "1px solid #1a1a1a" }}>
                      <td style={{ padding: 6, color: "#aaa", fontFamily: "monospace" }}>{edge.sourceId.slice(0, 8)}</td>
                      <td style={{ padding: 6, color: "#aaa", fontFamily: "monospace" }}>{edge.targetId.slice(0, 8)}</td>
                      <td style={{ padding: 6, color: "#888" }}>{edge.relationshipType}</td>
                      <td style={{ padding: 6, textAlign: "right", color: "#7c3aed" }}>{edge.trustScore}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
          <div style={{ marginTop: 12, fontSize: 12, color: "#555" }}>
            D3 force-directed visualization coming soon.
          </div>
        </div>
      )}

      {/* Instructions when no data */}
      {!score && !error && !loading && (
        <div style={{
          backgroundColor: "#161616",
          border: "1px solid #222",
          borderRadius: 8,
          padding: 32,
          textAlign: "center",
          color: "#888",
        }}>
          <p style={{ marginBottom: 8 }}>Enter an entity ID to view its trust score and graph.</p>
          <p style={{ fontSize: 13, color: "#666" }}>
            Scores are computed from endorsements, graph position, and ML analysis across 7 dimensions (max 1000).
          </p>
        </div>
      )}
    </div>
  );
}
