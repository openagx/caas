"use client";

import { useState, useEffect } from "react";

const API_BASE = process.env.NEXT_PUBLIC_API_URL || "http://localhost:3001";

const METHOD_LABELS: Record<number, string> = { 1: "did:web", 2: "did:key" };
const STATUS_LABELS: Record<number, string> = { 1: "Active", 2: "Deactivated" };
const STATUS_COLORS: Record<number, string> = { 1: "#22c55e", 2: "#ef4444" };
const KEY_TYPE_LABELS: Record<number, string> = { 1: "Ed25519", 2: "P-256" };
const PURPOSE_LABELS: Record<number, string> = { 1: "Authentication", 2: "Assertion", 3: "Key Agreement", 4: "Capability Invocation" };
const CRED_STATUS_LABELS: Record<number, string> = { 1: "Active", 2: "Revoked", 3: "Expired" };
const CRED_STATUS_COLORS: Record<number, string> = { 1: "#22c55e", 2: "#ef4444", 3: "#f59e0b" };

interface DIDDocument {
  id: string;
  controller: string;
  status: number;
  documentJson: string;
  entityId: string;
  method: number;
  createdAt: string;
}

interface KeyPair {
  keyId: string;
  did: string;
  keyType: number;
  purpose: number;
  publicKeyMultibase: string;
  publicKeyJwk: string;
  rotated: boolean;
  createdAt: string;
}

interface Credential {
  id: string;
  issuerDid: string;
  subjectDid: string;
  entityId: string;
  contextJson: string;
  typeJson: string;
  proofJson: string;
  issuanceDate: string;
  expirationDate: string;
  status: number;
}

type Tab = "dids" | "keys" | "credentials" | "presentations";

export default function DIDPage() {
  const [tab, setTab] = useState<Tab>("dids");
  const [dids, setDids] = useState<DIDDocument[]>([]);
  const [keys, setKeys] = useState<KeyPair[]>([]);
  const [credentials, setCredentials] = useState<Credential[]>([]);
  const [loading, setLoading] = useState(false);

  // Create DID form
  const [showCreate, setShowCreate] = useState(false);
  const [newEntityId, setNewEntityId] = useState("");
  const [newMethod, setNewMethod] = useState(1);
  const [newKeyType, setNewKeyType] = useState(1);

  // Resolve DID
  const [resolveDid, setResolveDid] = useState("");
  const [resolvedDoc, setResolvedDoc] = useState("");

  // Key management
  const [keyDid, setKeyDid] = useState("");

  // Issue VC form
  const [showIssue, setShowIssue] = useState(false);
  const [vcIssuerDid, setVcIssuerDid] = useState("");
  const [vcSubjectDid, setVcSubjectDid] = useState("");
  const [vcType, setVcType] = useState("IdentityVerification");

  // Verify result
  const [verifyResult, setVerifyResult] = useState<{ valid: boolean; checks: string[] } | null>(null);

  useEffect(() => {
    if (tab === "dids") fetchDIDs();
    if (tab === "credentials") fetchCredentials();
  }, [tab]);

  async function fetchDIDs() {
    setLoading(true);
    try {
      const res = await fetch(`${API_BASE}/v1/did?page_size=50`);
      const data = await res.json();
      setDids(data.documents || []);
    } catch { setDids([]); }
    setLoading(false);
  }

  async function fetchKeys(did: string) {
    try {
      const res = await fetch(`${API_BASE}/v1/did/keys?did=${encodeURIComponent(did)}`);
      const data = await res.json();
      setKeys(data.keys || []);
      setKeyDid(did);
      setTab("keys");
    } catch { setKeys([]); }
  }

  async function fetchCredentials() {
    setLoading(true);
    try {
      const res = await fetch(`${API_BASE}/v1/vc?page_size=50`);
      const data = await res.json();
      setCredentials(data.credentials || []);
    } catch { setCredentials([]); }
    setLoading(false);
  }

  async function createDID() {
    try {
      const res = await fetch(`${API_BASE}/v1/did/create`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          entity_id: newEntityId,
          method: newMethod,
          key_type: newKeyType,
        }),
      });
      if (!res.ok) throw new Error(await res.text());
      setShowCreate(false);
      fetchDIDs();
    } catch (e: any) {
      alert("Failed: " + e.message);
    }
  }

  async function resolve() {
    if (!resolveDid) return;
    try {
      const res = await fetch(`${API_BASE}/v1/did/resolve`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ did: resolveDid }),
      });
      const data = await res.json();
      setResolvedDoc(data.documentJson || JSON.stringify(data, null, 2));
    } catch { setResolvedDoc("Error resolving DID"); }
  }

  async function issueVC() {
    try {
      await fetch(`${API_BASE}/v1/vc`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          issuer_did: vcIssuerDid,
          subject_did: vcSubjectDid,
          credential_type: vcType,
          credential_subject: { name: "Verified Entity" },
        }),
      });
      setShowIssue(false);
      fetchCredentials();
    } catch (e) {
      alert("Failed to issue credential");
    }
  }

  async function verifyVC(id: string) {
    try {
      const res = await fetch(`${API_BASE}/v1/vc/verify`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ credential_id: id }),
      });
      const data = await res.json();
      setVerifyResult({ valid: data.valid, checks: data.checks || [] });
    } catch { setVerifyResult({ valid: false, checks: ["Error verifying"] }); }
  }

  async function revokeVC(id: string) {
    try {
      await fetch(`${API_BASE}/v1/vc/${id}/revoke`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ reason: "Revoked from dashboard" }),
      });
      fetchCredentials();
    } catch { }
  }

  const tabStyle = (t: Tab) => ({
    padding: "8px 16px",
    border: "none",
    borderBottom: tab === t ? "2px solid #3b82f6" : "2px solid transparent",
    background: "none",
    color: tab === t ? "#fff" : "#888",
    cursor: "pointer" as const,
    fontSize: 14,
    fontWeight: tab === t ? 600 : 400,
  });

  return (
    <div>
      <h1 style={{ fontSize: 24, fontWeight: 700, marginBottom: 8 }}>Identity (DID)</h1>
      <p style={{ color: "#888", marginBottom: 24 }}>
        W3C Decentralized Identifiers, key management, and verifiable credentials.
      </p>

      <div style={{ display: "flex", gap: 0, borderBottom: "1px solid #333", marginBottom: 24 }}>
        <button onClick={() => setTab("dids")} style={tabStyle("dids")}>DIDs</button>
        <button onClick={() => setTab("keys")} style={tabStyle("keys")}>Keys</button>
        <button onClick={() => setTab("credentials")} style={tabStyle("credentials")}>Credentials</button>
        <button onClick={() => setTab("presentations")} style={tabStyle("presentations")}>Presentations</button>
      </div>

      {tab === "dids" && (
        <div>
          <div style={{ display: "flex", gap: 12, marginBottom: 16 }}>
            <button onClick={() => setShowCreate(!showCreate)} style={btnStyle}>
              {showCreate ? "Cancel" : "+ Create DID"}
            </button>
            <button onClick={fetchDIDs} style={{ ...btnStyle, background: "#333" }}>Refresh</button>
          </div>

          {showCreate && (
            <div style={cardStyle}>
              <h3 style={{ margin: "0 0 12px" }}>Create DID</h3>
              <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr 1fr", gap: 12 }}>
                <label>
                  Entity ID
                  <input value={newEntityId} onChange={e => setNewEntityId(e.target.value)} style={inputStyle} placeholder="UUID" />
                </label>
                <label>
                  Method
                  <select value={newMethod} onChange={e => setNewMethod(+e.target.value)} style={inputStyle}>
                    <option value={1}>did:web</option>
                    <option value={2}>did:key</option>
                  </select>
                </label>
                <label>
                  Key Type
                  <select value={newKeyType} onChange={e => setNewKeyType(+e.target.value)} style={inputStyle}>
                    <option value={1}>Ed25519</option>
                    <option value={2}>P-256</option>
                  </select>
                </label>
              </div>
              <button onClick={createDID} style={{ ...btnStyle, marginTop: 12 }}>Create</button>
            </div>
          )}

          {/* Resolve */}
          <div style={{ ...cardStyle, marginBottom: 16 }}>
            <h3 style={{ margin: "0 0 8px" }}>Resolve DID</h3>
            <div style={{ display: "flex", gap: 8 }}>
              <input value={resolveDid} onChange={e => setResolveDid(e.target.value)} style={{ ...inputStyle, flex: 1 }} placeholder="did:web:caas.local:entities:..." />
              <button onClick={resolve} style={btnStyle}>Resolve</button>
            </div>
            {resolvedDoc && (
              <pre style={{ marginTop: 12, background: "#1a1a1a", padding: 12, borderRadius: 6, fontSize: 12, overflow: "auto", maxHeight: 300 }}>
                {resolvedDoc}
              </pre>
            )}
          </div>

          {loading ? (
            <p style={{ color: "#888" }}>Loading...</p>
          ) : (
            <table style={tableStyle}>
              <thead>
                <tr>
                  <th style={thStyle}>DID</th>
                  <th style={thStyle}>Method</th>
                  <th style={thStyle}>Entity</th>
                  <th style={thStyle}>Status</th>
                  <th style={thStyle}>Actions</th>
                </tr>
              </thead>
              <tbody>
                {dids.map(d => (
                  <tr key={d.id}>
                    <td style={{ ...tdStyle, fontSize: 11, fontFamily: "monospace", maxWidth: 300, overflow: "hidden", textOverflow: "ellipsis" }}>{d.id}</td>
                    <td style={tdStyle}>{METHOD_LABELS[d.method] || "?"}</td>
                    <td style={{ ...tdStyle, fontSize: 11, fontFamily: "monospace" }}>{d.entityId?.slice(0, 8)}</td>
                    <td style={tdStyle}>
                      <span style={{ color: STATUS_COLORS[d.status] || "#888" }}>{STATUS_LABELS[d.status] || "?"}</span>
                    </td>
                    <td style={tdStyle}>
                      <button onClick={() => fetchKeys(d.id)} style={{ ...btnSmall, marginRight: 4 }}>Keys</button>
                      <button onClick={() => { setResolveDid(d.id); resolve(); }} style={btnSmall}>Resolve</button>
                    </td>
                  </tr>
                ))}
                {dids.length === 0 && (
                  <tr><td colSpan={5} style={{ ...tdStyle, textAlign: "center", color: "#666" }}>No DIDs found</td></tr>
                )}
              </tbody>
            </table>
          )}
        </div>
      )}

      {tab === "keys" && (
        <div>
          <p style={{ color: "#888", marginBottom: 16 }}>
            {keyDid ? <>Keys for <code style={{ color: "#3b82f6" }}>{keyDid}</code></> : "Select a DID to view keys"}
          </p>
          {keys.length > 0 ? (
            <table style={tableStyle}>
              <thead>
                <tr>
                  <th style={thStyle}>Key ID</th>
                  <th style={thStyle}>Type</th>
                  <th style={thStyle}>Purpose</th>
                  <th style={thStyle}>Public Key</th>
                  <th style={thStyle}>Rotated</th>
                </tr>
              </thead>
              <tbody>
                {keys.map(k => (
                  <tr key={k.keyId}>
                    <td style={{ ...tdStyle, fontSize: 11, fontFamily: "monospace" }}>{k.keyId?.split("#")[1] || k.keyId}</td>
                    <td style={tdStyle}>{KEY_TYPE_LABELS[k.keyType] || "?"}</td>
                    <td style={tdStyle}>{PURPOSE_LABELS[k.purpose] || "?"}</td>
                    <td style={{ ...tdStyle, fontSize: 10, fontFamily: "monospace", maxWidth: 200, overflow: "hidden", textOverflow: "ellipsis" }}>
                      {k.publicKeyMultibase?.slice(0, 24)}...
                    </td>
                    <td style={tdStyle}>{k.rotated ? "Yes" : "No"}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          ) : (
            <p style={{ color: "#666" }}>No keys loaded. Select a DID from the DIDs tab.</p>
          )}
        </div>
      )}

      {tab === "credentials" && (
        <div>
          <div style={{ display: "flex", gap: 12, marginBottom: 16 }}>
            <button onClick={() => setShowIssue(!showIssue)} style={btnStyle}>
              {showIssue ? "Cancel" : "+ Issue Credential"}
            </button>
            <button onClick={fetchCredentials} style={{ ...btnStyle, background: "#333" }}>Refresh</button>
          </div>

          {showIssue && (
            <div style={cardStyle}>
              <h3 style={{ margin: "0 0 12px" }}>Issue Verifiable Credential</h3>
              <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr 1fr", gap: 12 }}>
                <label>
                  Issuer DID
                  <input value={vcIssuerDid} onChange={e => setVcIssuerDid(e.target.value)} style={inputStyle} placeholder="did:web:..." />
                </label>
                <label>
                  Subject DID
                  <input value={vcSubjectDid} onChange={e => setVcSubjectDid(e.target.value)} style={inputStyle} placeholder="did:web:..." />
                </label>
                <label>
                  Credential Type
                  <input value={vcType} onChange={e => setVcType(e.target.value)} style={inputStyle} />
                </label>
              </div>
              <button onClick={issueVC} style={{ ...btnStyle, marginTop: 12 }}>Issue</button>
            </div>
          )}

          {verifyResult && (
            <div style={{ ...cardStyle, borderColor: verifyResult.valid ? "#22c55e" : "#ef4444" }}>
              <h3 style={{ margin: "0 0 8px", color: verifyResult.valid ? "#22c55e" : "#ef4444" }}>
                {verifyResult.valid ? "Valid" : "Invalid"}
              </h3>
              {verifyResult.checks.map((c, i) => (
                <div key={i} style={{ fontSize: 12, color: c.startsWith("PASS") ? "#22c55e" : "#ef4444", marginBottom: 2 }}>{c}</div>
              ))}
              <button onClick={() => setVerifyResult(null)} style={{ ...btnSmall, marginTop: 8 }}>Dismiss</button>
            </div>
          )}

          {loading ? (
            <p style={{ color: "#888" }}>Loading...</p>
          ) : (
            <table style={tableStyle}>
              <thead>
                <tr>
                  <th style={thStyle}>ID</th>
                  <th style={thStyle}>Type</th>
                  <th style={thStyle}>Issuer</th>
                  <th style={thStyle}>Subject</th>
                  <th style={thStyle}>Status</th>
                  <th style={thStyle}>Actions</th>
                </tr>
              </thead>
              <tbody>
                {credentials.map(c => {
                  let types: string[] = [];
                  try { types = JSON.parse(c.typeJson); } catch { }
                  return (
                    <tr key={c.id}>
                      <td style={{ ...tdStyle, fontSize: 11, fontFamily: "monospace" }}>{c.id?.slice(0, 8)}</td>
                      <td style={tdStyle}>{types[1] || types[0] || "?"}</td>
                      <td style={{ ...tdStyle, fontSize: 10, fontFamily: "monospace" }}>{c.issuerDid?.slice(-20)}</td>
                      <td style={{ ...tdStyle, fontSize: 10, fontFamily: "monospace" }}>{c.subjectDid?.slice(-20) || "—"}</td>
                      <td style={tdStyle}>
                        <span style={{ color: CRED_STATUS_COLORS[c.status] || "#888" }}>{CRED_STATUS_LABELS[c.status] || "?"}</span>
                      </td>
                      <td style={tdStyle}>
                        <button onClick={() => verifyVC(c.id)} style={{ ...btnSmall, marginRight: 4 }}>Verify</button>
                        {c.status === 1 && <button onClick={() => revokeVC(c.id)} style={{ ...btnSmall, background: "#7f1d1d" }}>Revoke</button>}
                      </td>
                    </tr>
                  );
                })}
                {credentials.length === 0 && (
                  <tr><td colSpan={6} style={{ ...tdStyle, textAlign: "center", color: "#666" }}>No credentials found</td></tr>
                )}
              </tbody>
            </table>
          )}
        </div>
      )}

      {tab === "presentations" && (
        <div>
          <p style={{ color: "#888" }}>
            Verifiable Presentations allow bundling multiple credentials with a holder proof.
            Use the API to create and verify presentations.
          </p>
          <div style={cardStyle}>
            <h3 style={{ margin: "0 0 8px" }}>API Endpoints</h3>
            <div style={{ fontSize: 13, fontFamily: "monospace" }}>
              <div style={{ marginBottom: 4 }}><span style={{ color: "#22c55e" }}>POST</span> /v1/vp — Create presentation</div>
              <div><span style={{ color: "#3b82f6" }}>POST</span> /v1/vp/verify — Verify presentation</div>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}

const btnStyle: React.CSSProperties = {
  padding: "8px 16px",
  background: "#3b82f6",
  color: "#fff",
  border: "none",
  borderRadius: 6,
  cursor: "pointer",
  fontSize: 13,
  fontWeight: 500,
};

const btnSmall: React.CSSProperties = {
  padding: "4px 8px",
  background: "#333",
  color: "#ccc",
  border: "none",
  borderRadius: 4,
  cursor: "pointer",
  fontSize: 11,
};

const cardStyle: React.CSSProperties = {
  background: "#161616",
  border: "1px solid #333",
  borderRadius: 8,
  padding: 16,
  marginBottom: 16,
};

const inputStyle: React.CSSProperties = {
  display: "block",
  width: "100%",
  padding: "6px 10px",
  background: "#1a1a1a",
  border: "1px solid #333",
  borderRadius: 4,
  color: "#ededed",
  fontSize: 13,
  marginTop: 4,
};

const tableStyle: React.CSSProperties = {
  width: "100%",
  borderCollapse: "collapse",
  fontSize: 13,
};

const thStyle: React.CSSProperties = {
  textAlign: "left",
  padding: "8px 12px",
  borderBottom: "1px solid #333",
  color: "#888",
  fontWeight: 500,
  fontSize: 12,
};

const tdStyle: React.CSSProperties = {
  padding: "8px 12px",
  borderBottom: "1px solid #222",
};
