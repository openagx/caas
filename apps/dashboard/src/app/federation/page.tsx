"use client";

import { useState, useEffect } from "react";

const API_BASE = process.env.NEXT_PUBLIC_API_URL || "http://localhost:3001";

const NODE_STATUS_COLORS: Record<string, string> = {
  active: "#22c55e",
  suspended: "#f59e0b",
  revoked: "#ef4444",
};

const TREATY_STATUS_COLORS: Record<string, string> = {
  proposed: "#f59e0b",
  active: "#22c55e",
  suspended: "#f97316",
  terminated: "#ef4444",
};

const TIER_LABELS: Record<number, string> = {
  1: "Individual",
  2: "Institutional",
  3: "Regulatory",
  4: "Judicial",
  5: "Sovereign Emergency",
};

const TIER_COLORS: Record<number, string> = {
  1: "#6b7280",
  2: "#3b82f6",
  3: "#f59e0b",
  4: "#f97316",
  5: "#ef4444",
};

interface SovereignNode {
  id: string;
  name: string;
  did: string;
  jurisdiction: string;
  endpoint: string;
  publicKey: string;
  status: string;
  createdAt: string;
}

interface Treaty {
  id: string;
  sourceNodeId: string;
  targetNodeId: string;
  status: string;
  trustWeight: number;
  allowedOperations: string[];
  validFrom: string;
  validUntil: string;
  createdAt: string;
}

interface AuthorityOverride {
  id: string;
  workflowId: string;
  authorityEntityId: string;
  tier: number;
  justification: string;
  legalReference: string;
  outcome: string;
  expiresAt: string;
  createdAt: string;
}

interface Credential {
  id: string;
  entityId: string;
  issuerDid: string;
  credentialType: string;
  proofSignature: string;
  issuedAt: string;
  expiresAt: string;
  revoked: boolean;
}

type Tab = "nodes" | "treaties" | "authority" | "credentials";

export default function FederationPage() {
  const [tab, setTab] = useState<Tab>("nodes");
  const [nodes, setNodes] = useState<SovereignNode[]>([]);
  const [treaties, setTreaties] = useState<Treaty[]>([]);
  const [overrides, setOverrides] = useState<AuthorityOverride[]>([]);
  const [credentials, setCredentials] = useState<Credential[]>([]);
  const [error, setError] = useState<string | null>(null);

  // Node form
  const [nodeName, setNodeName] = useState("");
  const [nodeJurisdiction, setNodeJurisdiction] = useState("");
  const [nodeEndpoint, setNodeEndpoint] = useState("");
  const [creating, setCreating] = useState(false);

  // Treaty form
  const [treatyTarget, setTreatyTarget] = useState("");
  const [treatyWeight, setTreatyWeight] = useState(0.5);
  const [treatyOps, setTreatyOps] = useState("trust_query,entity_lookup,permission_check");

  // Credential form
  const [credEntityId, setCredEntityId] = useState("");
  const [credType, setCredType] = useState("IdentityVerification");

  // Override form
  const [overrideWorkflow, setOverrideWorkflow] = useState("");
  const [overrideAuthority, setOverrideAuthority] = useState("");
  const [overrideTier, setOverrideTier] = useState(3);
  const [overrideJustification, setOverrideJustification] = useState("");
  const [overrideOutcome, setOverrideOutcome] = useState("approved");

  async function loadData() {
    try {
      const [nodesRes, treatiesRes, overridesRes, credsRes] = await Promise.allSettled([
        fetch(`${API_BASE}/v1/federation/nodes`).then((r) => r.json()),
        fetch(`${API_BASE}/v1/federation/treaties`).then((r) => r.json()),
        fetch(`${API_BASE}/v1/authority/overrides`).then((r) => r.json()),
        fetch(`${API_BASE}/v1/credentials`).then((r) => r.json()),
      ]);
      if (nodesRes.status === "fulfilled") setNodes(nodesRes.value.nodes || []);
      if (treatiesRes.status === "fulfilled") setTreaties(treatiesRes.value.treaties || []);
      if (overridesRes.status === "fulfilled") setOverrides(overridesRes.value.overrides || []);
      if (credsRes.status === "fulfilled") setCredentials(credsRes.value.credentials || []);
      setError(null);
    } catch {
      setError("Could not connect to federation service");
    }
  }

  useEffect(() => { loadData(); }, []);

  async function handleCreateNode() {
    if (!nodeName || !nodeJurisdiction || !nodeEndpoint) return;
    setCreating(true);
    try {
      await fetch(`${API_BASE}/v1/federation/nodes`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          name: nodeName,
          jurisdiction: nodeJurisdiction,
          endpoint: nodeEndpoint,
        }),
      });
      setNodeName(""); setNodeJurisdiction(""); setNodeEndpoint("");
      await loadData();
    } catch { setError("Failed to create node"); }
    finally { setCreating(false); }
  }

  async function handleProposeTreaty() {
    if (!treatyTarget) return;
    setCreating(true);
    try {
      await fetch(`${API_BASE}/v1/federation/treaties`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          target_node_id: treatyTarget,
          trust_weight: treatyWeight,
          allowed_operations: treatyOps.split(",").map((s) => s.trim()),
        }),
      });
      setTreatyTarget("");
      await loadData();
    } catch { setError("Failed to propose treaty"); }
    finally { setCreating(false); }
  }

  async function handleAcceptTreaty(id: string) {
    await fetch(`${API_BASE}/v1/federation/treaties/${id}/accept`, { method: "POST" });
    await loadData();
  }

  async function handleIssueCredential() {
    if (!credEntityId) return;
    setCreating(true);
    try {
      await fetch(`${API_BASE}/v1/credentials`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          entity_id: credEntityId,
          credential_type: credType,
          claims: { verified: true, method: "document_check" },
        }),
      });
      setCredEntityId("");
      await loadData();
    } catch { setError("Failed to issue credential"); }
    finally { setCreating(false); }
  }

  async function handleCreateOverride() {
    if (!overrideWorkflow || !overrideAuthority || !overrideJustification) return;
    setCreating(true);
    try {
      await fetch(`${API_BASE}/v1/authority/overrides`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          workflow_id: overrideWorkflow,
          authority_entity_id: overrideAuthority,
          tier: overrideTier,
          justification: overrideJustification,
          outcome: overrideOutcome,
          ...(overrideTier === 5 ? { expires_at: new Date(Date.now() + 24 * 60 * 60 * 1000).toISOString() } : {}),
        }),
      });
      setOverrideWorkflow(""); setOverrideAuthority(""); setOverrideJustification("");
      await loadData();
    } catch { setError("Failed to create override"); }
    finally { setCreating(false); }
  }

  const tabs: { key: Tab; label: string; count: number }[] = [
    { key: "nodes", label: "Sovereign Nodes", count: nodes.length },
    { key: "treaties", label: "Treaties", count: treaties.length },
    { key: "authority", label: "Authority Overrides", count: overrides.length },
    { key: "credentials", label: "Credentials", count: credentials.length },
  ];

  return (
    <div>
      <h1 style={{ fontSize: 28, fontWeight: 700, marginBottom: 8 }}>Federation & Sovereignty</h1>
      <p style={{ color: "#888", marginBottom: 24 }}>
        Sovereign nodes, bilateral treaties, authority overrides, verifiable credentials
      </p>

      {error && (
        <div style={{ backgroundColor: "#1a1a1a", border: "1px solid #333", borderRadius: 8, padding: 16, color: "#ef4444", marginBottom: 24, fontSize: 13 }}>
          {error}
        </div>
      )}

      {/* Tabs */}
      <div style={{ display: "flex", gap: 4, marginBottom: 24 }}>
        {tabs.map((t) => (
          <button
            key={t.key}
            onClick={() => setTab(t.key)}
            style={{
              padding: "8px 16px",
              borderRadius: 6,
              border: "1px solid",
              borderColor: tab === t.key ? "#3b82f6" : "#333",
              backgroundColor: tab === t.key ? "#3b82f622" : "#161616",
              color: tab === t.key ? "#3b82f6" : "#888",
              cursor: "pointer",
              fontSize: 13,
              fontWeight: 600,
            }}
          >
            {t.label} ({t.count})
          </button>
        ))}
      </div>

      {/* Sovereign Nodes Tab */}
      {tab === "nodes" && (
        <>
          <div style={cardStyle}>
            <div style={{ fontSize: 14, fontWeight: 600, marginBottom: 12 }}>Register Sovereign Node</div>
            <div style={{ display: "flex", gap: 12, alignItems: "end" }}>
              <div style={{ flex: 1 }}>
                <label style={labelStyle}>Name</label>
                <input value={nodeName} onChange={(e) => setNodeName(e.target.value)} placeholder="e.g., India Sovereign" style={inputStyle} />
              </div>
              <div style={{ width: 100 }}>
                <label style={labelStyle}>Jurisdiction</label>
                <input value={nodeJurisdiction} onChange={(e) => setNodeJurisdiction(e.target.value)} placeholder="IN" style={inputStyle} />
              </div>
              <div style={{ flex: 1 }}>
                <label style={labelStyle}>Endpoint</label>
                <input value={nodeEndpoint} onChange={(e) => setNodeEndpoint(e.target.value)} placeholder="grpc://node.example.com:443" style={inputStyle} />
              </div>
              <button onClick={handleCreateNode} disabled={creating || !nodeName} style={btnStyle("#3b82f6", !nodeName)}>
                Register
              </button>
            </div>
          </div>

          <div style={cardStyle}>
            <div style={{ fontSize: 14, fontWeight: 600, marginBottom: 12 }}>Sovereign Nodes</div>
            {nodes.length > 0 ? (
              <table style={tableStyle}>
                <thead>
                  <tr style={{ borderBottom: "1px solid #222" }}>
                    {["Name", "Jurisdiction", "DID", "Status", "Endpoint", "Registered"].map((h) => (
                      <th key={h} style={thStyle}>{h}</th>
                    ))}
                  </tr>
                </thead>
                <tbody>
                  {nodes.map((n) => (
                    <tr key={n.id} style={{ borderBottom: "1px solid #1a1a1a" }}>
                      <td style={tdStyle}>{n.name}</td>
                      <td style={{ ...tdStyle, fontWeight: 600 }}>{n.jurisdiction}</td>
                      <td style={{ ...tdStyle, fontFamily: "monospace", fontSize: 11, color: "#888" }}>{n.did?.slice(0, 24) || "—"}</td>
                      <td style={tdStyle}>
                        <span style={{ color: NODE_STATUS_COLORS[n.status] || "#888", fontWeight: 600, fontSize: 11, textTransform: "uppercase" as const }}>
                          {n.status}
                        </span>
                      </td>
                      <td style={{ ...tdStyle, fontSize: 11, color: "#666" }}>{n.endpoint}</td>
                      <td style={{ ...tdStyle, fontSize: 11, color: "#666" }}>{n.createdAt ? new Date(n.createdAt).toLocaleDateString() : "—"}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            ) : (
              <div style={emptyStyle}>No sovereign nodes registered yet.</div>
            )}
          </div>
        </>
      )}

      {/* Treaties Tab */}
      {tab === "treaties" && (
        <>
          <div style={cardStyle}>
            <div style={{ fontSize: 14, fontWeight: 600, marginBottom: 12 }}>Propose Treaty</div>
            <div style={{ display: "flex", gap: 12, alignItems: "end" }}>
              <div style={{ flex: 1 }}>
                <label style={labelStyle}>Target Node ID</label>
                <input value={treatyTarget} onChange={(e) => setTreatyTarget(e.target.value)} placeholder="Node UUID..." style={inputStyle} />
              </div>
              <div style={{ width: 100 }}>
                <label style={labelStyle}>Trust Weight</label>
                <input type="number" value={treatyWeight} onChange={(e) => setTreatyWeight(parseFloat(e.target.value))} min={0} max={1} step={0.1} style={inputStyle} />
              </div>
              <div style={{ flex: 1 }}>
                <label style={labelStyle}>Allowed Operations</label>
                <input value={treatyOps} onChange={(e) => setTreatyOps(e.target.value)} style={inputStyle} />
              </div>
              <button onClick={handleProposeTreaty} disabled={creating || !treatyTarget} style={btnStyle("#22c55e", !treatyTarget)}>
                Propose
              </button>
            </div>
          </div>

          <div style={cardStyle}>
            <div style={{ fontSize: 14, fontWeight: 600, marginBottom: 12 }}>Treaties</div>
            {treaties.length > 0 ? (
              <table style={tableStyle}>
                <thead>
                  <tr style={{ borderBottom: "1px solid #222" }}>
                    {["Source", "Target", "Status", "Trust Weight", "Operations", "Action"].map((h) => (
                      <th key={h} style={thStyle}>{h}</th>
                    ))}
                  </tr>
                </thead>
                <tbody>
                  {treaties.map((t) => (
                    <tr key={t.id} style={{ borderBottom: "1px solid #1a1a1a" }}>
                      <td style={{ ...tdStyle, fontFamily: "monospace", fontSize: 11 }}>{t.sourceNodeId?.slice(0, 8)}</td>
                      <td style={{ ...tdStyle, fontFamily: "monospace", fontSize: 11 }}>{t.targetNodeId?.slice(0, 8)}</td>
                      <td style={tdStyle}>
                        <span style={{
                          padding: "2px 6px", borderRadius: 4, fontSize: 11,
                          backgroundColor: (TREATY_STATUS_COLORS[t.status] || "#333") + "22",
                          color: TREATY_STATUS_COLORS[t.status] || "#888",
                        }}>
                          {t.status}
                        </span>
                      </td>
                      <td style={tdStyle}>{(t.trustWeight * 100).toFixed(0)}%</td>
                      <td style={{ ...tdStyle, fontSize: 11, color: "#888" }}>{(t.allowedOperations || []).join(", ")}</td>
                      <td style={tdStyle}>
                        {t.status === "proposed" && (
                          <button onClick={() => handleAcceptTreaty(t.id)} style={{ ...btnStyle("#22c55e", false), padding: "4px 10px", fontSize: 11 }}>
                            Accept
                          </button>
                        )}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            ) : (
              <div style={emptyStyle}>No treaties yet. Propose one above.</div>
            )}
          </div>
        </>
      )}

      {/* Authority Overrides Tab */}
      {tab === "authority" && (
        <>
          <div style={cardStyle}>
            <div style={{ fontSize: 14, fontWeight: 600, marginBottom: 12 }}>Create Authority Override</div>
            <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr auto auto", gap: 12, alignItems: "end" }}>
              <div>
                <label style={labelStyle}>Workflow ID</label>
                <input value={overrideWorkflow} onChange={(e) => setOverrideWorkflow(e.target.value)} placeholder="Workflow UUID..." style={inputStyle} />
              </div>
              <div>
                <label style={labelStyle}>Authority Entity ID</label>
                <input value={overrideAuthority} onChange={(e) => setOverrideAuthority(e.target.value)} placeholder="Entity UUID..." style={inputStyle} />
              </div>
              <div>
                <label style={labelStyle}>Tier</label>
                <select value={overrideTier} onChange={(e) => setOverrideTier(parseInt(e.target.value))} style={inputStyle}>
                  {[1, 2, 3, 4, 5].map((t) => (
                    <option key={t} value={t}>{TIER_LABELS[t]}</option>
                  ))}
                </select>
              </div>
              <div>
                <label style={labelStyle}>Outcome</label>
                <select value={overrideOutcome} onChange={(e) => setOverrideOutcome(e.target.value)} style={inputStyle}>
                  <option value="approved">Approved</option>
                  <option value="rejected">Rejected</option>
                </select>
              </div>
            </div>
            <div style={{ display: "flex", gap: 12, marginTop: 12, alignItems: "end" }}>
              <div style={{ flex: 1 }}>
                <label style={labelStyle}>Justification</label>
                <input value={overrideJustification} onChange={(e) => setOverrideJustification(e.target.value)} placeholder="Legal basis or reason..." style={inputStyle} />
              </div>
              <button onClick={handleCreateOverride} disabled={creating || !overrideWorkflow || !overrideAuthority} style={btnStyle("#f59e0b", !overrideWorkflow || !overrideAuthority)}>
                Override
              </button>
            </div>

            {/* Tier visual */}
            <div style={{ display: "flex", gap: 8, marginTop: 16 }}>
              {[1, 2, 3, 4, 5].map((t) => (
                <div key={t} style={{
                  flex: 1, padding: "8px 12px", borderRadius: 6, textAlign: "center",
                  backgroundColor: overrideTier === t ? TIER_COLORS[t] + "22" : "#1a1a1a",
                  border: `1px solid ${overrideTier === t ? TIER_COLORS[t] : "#222"}`,
                  cursor: "pointer",
                }} onClick={() => setOverrideTier(t)}>
                  <div style={{ fontSize: 11, fontWeight: 600, color: TIER_COLORS[t] }}>Tier {t}</div>
                  <div style={{ fontSize: 10, color: "#888", marginTop: 2 }}>{TIER_LABELS[t]}</div>
                </div>
              ))}
            </div>
          </div>

          <div style={cardStyle}>
            <div style={{ fontSize: 14, fontWeight: 600, marginBottom: 12 }}>Authority Overrides</div>
            {overrides.length > 0 ? (
              <table style={tableStyle}>
                <thead>
                  <tr style={{ borderBottom: "1px solid #222" }}>
                    {["Workflow", "Authority", "Tier", "Outcome", "Justification", "Expires", "Created"].map((h) => (
                      <th key={h} style={thStyle}>{h}</th>
                    ))}
                  </tr>
                </thead>
                <tbody>
                  {overrides.map((o) => (
                    <tr key={o.id} style={{ borderBottom: "1px solid #1a1a1a" }}>
                      <td style={{ ...tdStyle, fontFamily: "monospace", fontSize: 11 }}>{o.workflowId?.slice(0, 8)}</td>
                      <td style={{ ...tdStyle, fontFamily: "monospace", fontSize: 11 }}>{o.authorityEntityId?.slice(0, 8)}</td>
                      <td style={tdStyle}>
                        <span style={{ color: TIER_COLORS[o.tier] || "#888", fontWeight: 600, fontSize: 11 }}>
                          {TIER_LABELS[o.tier] || `Tier ${o.tier}`}
                        </span>
                      </td>
                      <td style={tdStyle}>
                        <span style={{ color: o.outcome === "approved" ? "#22c55e" : "#ef4444", fontWeight: 600, fontSize: 11, textTransform: "uppercase" as const }}>
                          {o.outcome}
                        </span>
                      </td>
                      <td style={{ ...tdStyle, fontSize: 11, color: "#aaa", maxWidth: 200, overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" as const }}>
                        {o.justification}
                      </td>
                      <td style={{ ...tdStyle, fontSize: 11, color: "#666" }}>{o.expiresAt ? new Date(o.expiresAt).toLocaleString() : "—"}</td>
                      <td style={{ ...tdStyle, fontSize: 11, color: "#666" }}>{o.createdAt ? new Date(o.createdAt).toLocaleString() : "—"}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            ) : (
              <div style={emptyStyle}>No authority overrides yet.</div>
            )}
          </div>
        </>
      )}

      {/* Credentials Tab */}
      {tab === "credentials" && (
        <>
          <div style={cardStyle}>
            <div style={{ fontSize: 14, fontWeight: 600, marginBottom: 12 }}>Issue Verifiable Credential</div>
            <div style={{ display: "flex", gap: 12, alignItems: "end" }}>
              <div style={{ flex: 1 }}>
                <label style={labelStyle}>Entity ID</label>
                <input value={credEntityId} onChange={(e) => setCredEntityId(e.target.value)} placeholder="Entity UUID..." style={inputStyle} />
              </div>
              <div>
                <label style={labelStyle}>Credential Type</label>
                <select value={credType} onChange={(e) => setCredType(e.target.value)} style={inputStyle}>
                  <option value="IdentityVerification">Identity Verification</option>
                  <option value="TrustAttestation">Trust Attestation</option>
                  <option value="ComplianceCertificate">Compliance Certificate</option>
                  <option value="AuthorizationGrant">Authorization Grant</option>
                </select>
              </div>
              <button onClick={handleIssueCredential} disabled={creating || !credEntityId} style={btnStyle("#a855f7", !credEntityId)}>
                Issue
              </button>
            </div>
          </div>

          <div style={cardStyle}>
            <div style={{ fontSize: 14, fontWeight: 600, marginBottom: 12 }}>Verifiable Credentials</div>
            {credentials.length > 0 ? (
              <table style={tableStyle}>
                <thead>
                  <tr style={{ borderBottom: "1px solid #222" }}>
                    {["Entity", "Type", "Issuer DID", "Status", "Issued", "Expires"].map((h) => (
                      <th key={h} style={thStyle}>{h}</th>
                    ))}
                  </tr>
                </thead>
                <tbody>
                  {credentials.map((c) => (
                    <tr key={c.id} style={{ borderBottom: "1px solid #1a1a1a" }}>
                      <td style={{ ...tdStyle, fontFamily: "monospace", fontSize: 11 }}>{c.entityId?.slice(0, 8)}</td>
                      <td style={tdStyle}>{c.credentialType}</td>
                      <td style={{ ...tdStyle, fontFamily: "monospace", fontSize: 11, color: "#888" }}>{c.issuerDid?.slice(0, 24)}</td>
                      <td style={tdStyle}>
                        <span style={{ color: c.revoked ? "#ef4444" : "#22c55e", fontWeight: 600, fontSize: 11 }}>
                          {c.revoked ? "REVOKED" : "VALID"}
                        </span>
                      </td>
                      <td style={{ ...tdStyle, fontSize: 11, color: "#666" }}>{c.issuedAt ? new Date(c.issuedAt).toLocaleDateString() : "—"}</td>
                      <td style={{ ...tdStyle, fontSize: 11, color: "#666" }}>{c.expiresAt ? new Date(c.expiresAt).toLocaleDateString() : "Never"}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            ) : (
              <div style={emptyStyle}>No credentials issued yet.</div>
            )}
          </div>
        </>
      )}
    </div>
  );
}

const inputStyle: React.CSSProperties = {
  backgroundColor: "#1a1a1a",
  border: "1px solid #333",
  borderRadius: 6,
  padding: "8px 12px",
  color: "#ededed",
  fontSize: 13,
  width: "100%",
  boxSizing: "border-box",
};

const labelStyle: React.CSSProperties = {
  display: "block",
  fontSize: 12,
  color: "#888",
  marginBottom: 4,
};

const cardStyle: React.CSSProperties = {
  backgroundColor: "#161616",
  border: "1px solid #222",
  borderRadius: 8,
  padding: 20,
  marginBottom: 24,
};

const tableStyle: React.CSSProperties = {
  width: "100%",
  borderCollapse: "collapse",
};

const thStyle: React.CSSProperties = {
  textAlign: "left",
  padding: "6px 10px",
  fontSize: 11,
  color: "#666",
  textTransform: "uppercase",
};

const tdStyle: React.CSSProperties = {
  padding: "8px 10px",
  fontSize: 13,
};

const emptyStyle: React.CSSProperties = {
  color: "#555",
  fontSize: 13,
  textAlign: "center",
  padding: 24,
};

function btnStyle(color: string, disabled: boolean): React.CSSProperties {
  return {
    backgroundColor: color,
    color: "white",
    border: "none",
    borderRadius: 6,
    padding: "8px 16px",
    fontSize: 13,
    fontWeight: 600,
    cursor: disabled ? "default" : "pointer",
    opacity: disabled ? 0.5 : 1,
    whiteSpace: "nowrap",
  };
}
