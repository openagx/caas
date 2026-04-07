"use client";

import { useState, useEffect } from "react";

const API_BASE = process.env.NEXT_PUBLIC_API_URL || "http://localhost:3001";

const SEVERITY_COLORS: Record<string, string> = {
  low: "#f59e0b",
  medium: "#f97316",
  high: "#ef4444",
  critical: "#dc2626",
};

const STATUS_COLORS: Record<string, string> = {
  open: "#ef4444",
  contained: "#f59e0b",
  investigating: "#3b82f6",
  resolved: "#22c55e",
  false_positive: "#6b7280",
};

interface Incident {
  id: string;
  incident_type: string;
  severity: string;
  status: string;
  affected_entity_id: string;
  detection_timestamp: string;
  containment_timestamp: string | null;
  detection_latency_ms: number;
  evidence: Record<string, unknown>;
  timeline: Array<{
    stage: string;
    action: string;
    timestamp: string;
    latency_ms: number;
    detail: string;
  }>;
  created_at: string;
}

interface Metrics {
  total_incidents: number;
  open_incidents: number;
  avg_detection_latency_ms: number;
  avg_containment_latency_ms: number;
  false_positive_count: number;
  false_positive_rate: number;
}

export default function FraudPage() {
  const [incidents, setIncidents] = useState<Incident[]>([]);
  const [metrics, setMetrics] = useState<Metrics | null>(null);
  const [selectedIncident, setSelectedIncident] = useState<Incident | null>(null);
  const [simAttackType, setSimAttackType] = useState("account_takeover");
  const [simEntityId, setSimEntityId] = useState("");
  const [simLoading, setSimLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function loadData() {
    try {
      const [incRes, metRes] = await Promise.allSettled([
        fetch(`${API_BASE}/v1/fraud/incidents?page_size=20`).then((r) => r.json()),
        fetch(`${API_BASE}/v1/fraud/metrics`).then((r) => r.json()),
      ]);
      if (incRes.status === "fulfilled") setIncidents(incRes.value.incidents || []);
      if (metRes.status === "fulfilled") setMetrics(metRes.value.metrics || null);
      setError(null);
    } catch (e) {
      setError("Could not connect to fraud service");
    }
  }

  useEffect(() => {
    loadData();
  }, []);

  async function handleSimulate() {
    if (!simEntityId) return;
    setSimLoading(true);
    try {
      const res = await fetch(`${API_BASE}/v1/fraud/simulate`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ attack_type: simAttackType, target_entity_id: simEntityId }),
      });
      const data = await res.json();
      if (data.incident) {
        setSelectedIncident(data.incident);
        await loadData();
      }
    } catch (e) {
      setError("Simulation failed");
    } finally {
      setSimLoading(false);
    }
  }

  return (
    <div>
      <h1 style={{ fontSize: 28, fontWeight: 700, marginBottom: 8 }}>Fraud Detection</h1>
      <p style={{ color: "#888", marginBottom: 24 }}>
        6-stage kill chain: Detect → Classify → Contain → Revoke → Notify → Investigate
      </p>

      {/* Metrics */}
      {metrics && (
        <div style={{ display: "grid", gridTemplateColumns: "repeat(4, 1fr)", gap: 16, marginBottom: 24 }}>
          <MetricCard label="Total Incidents" value={metrics.total_incidents} />
          <MetricCard label="Open" value={metrics.open_incidents} color="#ef4444" />
          <MetricCard label="Avg Detection" value={`${metrics.avg_detection_latency_ms.toFixed(0)}ms`} />
          <MetricCard label="Avg Containment" value={`${metrics.avg_containment_latency_ms.toFixed(0)}ms`} />
        </div>
      )}

      {/* Simulate Attack */}
      <div style={{
        backgroundColor: "#161616",
        border: "1px solid #222",
        borderRadius: 8,
        padding: 20,
        marginBottom: 24,
      }}>
        <div style={{ fontSize: 14, fontWeight: 600, marginBottom: 12 }}>Simulate Attack</div>
        <div style={{ display: "flex", gap: 12, alignItems: "end" }}>
          <div>
            <label style={{ display: "block", fontSize: 12, color: "#888", marginBottom: 4 }}>Attack Type</label>
            <select
              value={simAttackType}
              onChange={(e) => setSimAttackType(e.target.value)}
              style={{
                backgroundColor: "#1a1a1a",
                border: "1px solid #333",
                borderRadius: 6,
                padding: "8px 12px",
                color: "#ededed",
                fontSize: 14,
              }}
            >
              <option value="account_takeover">Account Takeover</option>
              <option value="privilege_escalation">Privilege Escalation</option>
              <option value="sybil">Sybil Attack</option>
              <option value="exfiltration">Data Exfiltration</option>
              <option value="impersonation">Impersonation</option>
            </select>
          </div>
          <div style={{ flex: 1 }}>
            <label style={{ display: "block", fontSize: 12, color: "#888", marginBottom: 4 }}>Target Entity ID</label>
            <input
              value={simEntityId}
              onChange={(e) => setSimEntityId(e.target.value)}
              placeholder="Enter entity ID..."
              style={{
                width: "100%",
                backgroundColor: "#1a1a1a",
                border: "1px solid #333",
                borderRadius: 6,
                padding: "8px 12px",
                color: "#ededed",
                fontSize: 14,
              }}
            />
          </div>
          <button
            onClick={handleSimulate}
            disabled={simLoading || !simEntityId}
            style={{
              backgroundColor: "#dc2626",
              color: "white",
              border: "none",
              borderRadius: 6,
              padding: "10px 20px",
              fontSize: 14,
              fontWeight: 600,
              cursor: simLoading ? "wait" : "pointer",
              opacity: !simEntityId ? 0.5 : 1,
              whiteSpace: "nowrap",
            }}
          >
            {simLoading ? "Running..." : "Simulate Attack"}
          </button>
        </div>
      </div>

      {error && (
        <div style={{
          backgroundColor: "#1a1a1a",
          border: "1px solid #333",
          borderRadius: 8,
          padding: 16,
          textAlign: "center",
          color: "#888",
          marginBottom: 24,
        }}>
          {error}
        </div>
      )}

      {/* Incident Detail */}
      {selectedIncident && (
        <div style={{
          backgroundColor: "#161616",
          border: "1px solid #333",
          borderRadius: 8,
          padding: 20,
          marginBottom: 24,
        }}>
          <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center", marginBottom: 16 }}>
            <div style={{ fontSize: 16, fontWeight: 600 }}>
              Incident: {selectedIncident.incident_type.replace(/_/g, " ")}
            </div>
            <button onClick={() => setSelectedIncident(null)} style={{
              background: "none", border: "none", color: "#888", cursor: "pointer", fontSize: 18,
            }}>
              ×
            </button>
          </div>

          <div style={{ display: "grid", gridTemplateColumns: "repeat(3, 1fr)", gap: 12, marginBottom: 16 }}>
            <div style={{ fontSize: 12, color: "#888" }}>
              Severity: <span style={{ color: SEVERITY_COLORS[selectedIncident.severity] || "#888", fontWeight: 600 }}>
                {selectedIncident.severity.toUpperCase()}
              </span>
            </div>
            <div style={{ fontSize: 12, color: "#888" }}>
              Status: <span style={{ color: STATUS_COLORS[selectedIncident.status] || "#888", fontWeight: 600 }}>
                {selectedIncident.status}
              </span>
            </div>
            <div style={{ fontSize: 12, color: "#888" }}>
              Detection: <span style={{ color: "#ededed" }}>{selectedIncident.detection_latency_ms}ms</span>
            </div>
          </div>

          {/* Timeline */}
          {selectedIncident.timeline && selectedIncident.timeline.length > 0 && (
            <div>
              <div style={{ fontSize: 13, fontWeight: 600, marginBottom: 8 }}>Kill Chain Timeline</div>
              {selectedIncident.timeline.map((entry, i) => (
                <div key={i} style={{
                  display: "flex",
                  alignItems: "center",
                  gap: 12,
                  padding: "6px 0",
                  borderLeft: "2px solid #333",
                  paddingLeft: 12,
                  marginLeft: 8,
                }}>
                  <span style={{
                    display: "inline-block",
                    width: 70,
                    fontSize: 11,
                    fontWeight: 600,
                    textTransform: "uppercase",
                    color: entry.stage === "detect" ? "#ef4444" :
                           entry.stage === "classify" ? "#f97316" :
                           entry.stage === "contain" ? "#f59e0b" :
                           entry.stage === "revoke" ? "#dc2626" :
                           entry.stage === "notify" ? "#3b82f6" : "#22c55e",
                  }}>
                    {entry.stage}
                  </span>
                  <span style={{ fontSize: 13, color: "#aaa", flex: 1 }}>{entry.action}</span>
                  <span style={{ fontSize: 11, color: "#666" }}>{entry.latency_ms}ms</span>
                </div>
              ))}
            </div>
          )}
        </div>
      )}

      {/* Incidents List */}
      {incidents.length > 0 && (
        <div style={{
          backgroundColor: "#161616",
          border: "1px solid #222",
          borderRadius: 8,
          padding: 20,
        }}>
          <div style={{ fontSize: 14, fontWeight: 600, marginBottom: 12 }}>Recent Incidents</div>
          <table style={{ width: "100%", borderCollapse: "collapse" }}>
            <thead>
              <tr style={{ borderBottom: "1px solid #222" }}>
                {["Type", "Severity", "Status", "Entity", "Detection", "Time"].map((h) => (
                  <th key={h} style={{ textAlign: "left", padding: "6px 10px", fontSize: 11, color: "#666", textTransform: "uppercase" }}>
                    {h}
                  </th>
                ))}
              </tr>
            </thead>
            <tbody>
              {incidents.map((inc) => (
                <tr
                  key={inc.id}
                  onClick={() => setSelectedIncident(inc)}
                  style={{ borderBottom: "1px solid #1a1a1a", cursor: "pointer" }}
                >
                  <td style={{ padding: "8px 10px", fontSize: 13 }}>
                    {inc.incident_type.replace(/_/g, " ")}
                  </td>
                  <td style={{ padding: "8px 10px" }}>
                    <span style={{
                      fontSize: 11,
                      fontWeight: 600,
                      color: SEVERITY_COLORS[inc.severity] || "#888",
                    }}>
                      {inc.severity}
                    </span>
                  </td>
                  <td style={{ padding: "8px 10px" }}>
                    <span style={{
                      display: "inline-block",
                      padding: "2px 6px",
                      borderRadius: 4,
                      fontSize: 11,
                      backgroundColor: (STATUS_COLORS[inc.status] || "#333") + "22",
                      color: STATUS_COLORS[inc.status] || "#888",
                    }}>
                      {inc.status}
                    </span>
                  </td>
                  <td style={{ padding: "8px 10px", fontSize: 12, fontFamily: "monospace", color: "#888" }}>
                    {inc.affected_entity_id.slice(0, 8)}
                  </td>
                  <td style={{ padding: "8px 10px", fontSize: 12, color: "#aaa" }}>
                    {inc.detection_latency_ms}ms
                  </td>
                  <td style={{ padding: "8px 10px", fontSize: 12, color: "#666" }}>
                    {inc.created_at ? new Date(inc.created_at).toLocaleString() : "—"}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {incidents.length === 0 && !error && (
        <div style={{
          backgroundColor: "#161616",
          border: "1px solid #222",
          borderRadius: 8,
          padding: 32,
          textAlign: "center",
          color: "#888",
        }}>
          <p>No fraud incidents detected yet.</p>
          <p style={{ fontSize: 13, color: "#666" }}>
            Use the simulator above to trigger a demo attack, or wait for the Kafka consumer to detect real anomalies.
          </p>
        </div>
      )}
    </div>
  );
}

function MetricCard({ label, value, color }: { label: string; value: string | number; color?: string }) {
  return (
    <div style={{ backgroundColor: "#161616", border: "1px solid #222", borderRadius: 8, padding: 16 }}>
      <div style={{ fontSize: 11, color: "#888", textTransform: "uppercase", letterSpacing: 1 }}>{label}</div>
      <div style={{ fontSize: 28, fontWeight: 700, color: color || "#ededed", marginTop: 4 }}>{value}</div>
    </div>
  );
}
