"use client";

import { useState, useEffect } from "react";

const API_BASE = process.env.NEXT_PUBLIC_API_URL || "http://localhost:3001";

const STATUS_COLORS: Record<string, string> = {
  pending: "#f59e0b",
  in_review: "#3b82f6",
  approved: "#22c55e",
  rejected: "#ef4444",
  escalated: "#a855f7",
};

const VOTE_COLORS: Record<string, string> = {
  approve: "#22c55e",
  reject: "#ef4444",
  escalate: "#a855f7",
  abstain: "#6b7280",
};

interface Vote {
  id: string;
  workflowId: string;
  reviewerId: string;
  vote: string;
  reasoning: string;
  confidence: number;
  timeSpentSeconds: number;
  blindCaseId: string;
  createdAt: string;
}

interface Workflow {
  id: string;
  workflowType: string;
  requiredApprovals: number;
  totalReviewers: number;
  blindReview: boolean;
  coolOffHours: number;
  status: string;
  subjectEntityId: string;
  outcome: string;
  votes: Vote[];
  createdAt: string;
  completedAt: string;
}

interface ReviewerIntegrity {
  reviewerId: string;
  integrityScore: number;
  totalReviews: number;
  approvalRate: number;
  avgTimePerReview: number;
  biasFlags: number;
}

export default function DecisionsPage() {
  const [workflows, setWorkflows] = useState<Workflow[]>([]);
  const [selectedWorkflow, setSelectedWorkflow] = useState<Workflow | null>(null);
  const [reviewerIntegrity, setReviewerIntegrity] = useState<ReviewerIntegrity | null>(null);
  const [reviewerIdInput, setReviewerIdInput] = useState("");
  const [error, setError] = useState<string | null>(null);

  // Create workflow form
  const [newType, setNewType] = useState("identity_verification");
  const [newSubject, setNewSubject] = useState("");
  const [newApprovals, setNewApprovals] = useState(2);
  const [newReviewers, setNewReviewers] = useState(3);
  const [newBlind, setNewBlind] = useState(true);
  const [creating, setCreating] = useState(false);

  // Vote form
  const [voteReviewer, setVoteReviewer] = useState("");
  const [voteType, setVoteType] = useState("approve");
  const [voteReasoning, setVoteReasoning] = useState("");
  const [voteConfidence, setVoteConfidence] = useState(0.8);
  const [submittingVote, setSubmittingVote] = useState(false);

  async function loadWorkflows() {
    try {
      const res = await fetch(`${API_BASE}/v1/decisions/workflows?page_size=20`);
      const data = await res.json();
      setWorkflows(data.workflows || []);
      setError(null);
    } catch {
      setError("Could not connect to decision service");
    }
  }

  useEffect(() => {
    loadWorkflows();
  }, []);

  async function handleCreateWorkflow() {
    if (!newSubject) return;
    setCreating(true);
    try {
      const res = await fetch(`${API_BASE}/v1/decisions/workflows`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          workflowType: newType,
          subjectEntityId: newSubject,
          requiredApprovals: newApprovals,
          totalReviewers: newReviewers,
          blindReview: newBlind,
          coolOffHours: 168,
          caseData: {
            reason: "Review required",
            entityType: "human",
          },
        }),
      });
      const data = await res.json();
      if (data.workflow) {
        setSelectedWorkflow(data.workflow);
        await loadWorkflows();
        setNewSubject("");
      }
    } catch {
      setError("Failed to create workflow");
    } finally {
      setCreating(false);
    }
  }

  async function handleSubmitVote() {
    if (!selectedWorkflow || !voteReviewer) return;
    setSubmittingVote(true);
    try {
      const res = await fetch(`${API_BASE}/v1/decisions/votes`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          workflowId: selectedWorkflow.id,
          reviewerId: voteReviewer,
          vote: voteType,
          reasoning: voteReasoning,
          confidence: voteConfidence,
          timeSpentSeconds: 60,
        }),
      });
      const data = await res.json();
      if (data.vote) {
        // Reload the workflow to see updated votes
        const wfRes = await fetch(`${API_BASE}/v1/decisions/workflows/${selectedWorkflow.id}`);
        const wfData = await wfRes.json();
        if (wfData.workflow) setSelectedWorkflow(wfData.workflow);
        await loadWorkflows();
        setVoteReviewer("");
        setVoteReasoning("");
      }
    } catch {
      setError("Failed to submit vote");
    } finally {
      setSubmittingVote(false);
    }
  }

  async function handleLookupReviewer() {
    if (!reviewerIdInput) return;
    try {
      const res = await fetch(`${API_BASE}/v1/decisions/reviewers/${reviewerIdInput}/integrity`);
      const data = await res.json();
      setReviewerIntegrity(data.integrity || null);
    } catch {
      setError("Failed to fetch reviewer integrity");
    }
  }

  function statusLabel(status: string): string {
    return status.replace("WORKFLOW_STATUS_", "").replace(/_/g, " ").toLowerCase();
  }

  function voteLabel(vote: string): string {
    return vote.replace("VOTE_TYPE_", "").toLowerCase();
  }

  return (
    <div>
      <h1 style={{ fontSize: 28, fontWeight: 700, marginBottom: 8 }}>Human Decision Integrity</h1>
      <p style={{ color: "#888", marginBottom: 24 }}>
        Blind review, M-of-N consensus, cool-off enforcement, reviewer integrity scoring
      </p>

      {error && (
        <div style={{
          backgroundColor: "#1a1a1a",
          border: "1px solid #333",
          borderRadius: 8,
          padding: 16,
          color: "#ef4444",
          marginBottom: 24,
          fontSize: 13,
        }}>
          {error}
        </div>
      )}

      {/* Create Workflow */}
      <div style={{
        backgroundColor: "#161616",
        border: "1px solid #222",
        borderRadius: 8,
        padding: 20,
        marginBottom: 24,
      }}>
        <div style={{ fontSize: 14, fontWeight: 600, marginBottom: 12 }}>Create Decision Workflow</div>
        <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr 80px 80px auto auto", gap: 12, alignItems: "end" }}>
          <div>
            <label style={{ display: "block", fontSize: 12, color: "#888", marginBottom: 4 }}>Workflow Type</label>
            <select
              value={newType}
              onChange={(e) => setNewType(e.target.value)}
              style={inputStyle}
            >
              <option value="identity_verification">Identity Verification</option>
              <option value="trust_override">Trust Override</option>
              <option value="entity_activation">Entity Activation</option>
              <option value="fraud_appeal">Fraud Appeal</option>
              <option value="privilege_escalation">Privilege Escalation</option>
            </select>
          </div>
          <div>
            <label style={{ display: "block", fontSize: 12, color: "#888", marginBottom: 4 }}>Subject Entity ID</label>
            <input
              value={newSubject}
              onChange={(e) => setNewSubject(e.target.value)}
              placeholder="Entity ID..."
              style={inputStyle}
            />
          </div>
          <div>
            <label style={{ display: "block", fontSize: 12, color: "#888", marginBottom: 4 }}>Approvals</label>
            <input
              type="number"
              value={newApprovals}
              onChange={(e) => setNewApprovals(parseInt(e.target.value))}
              min={1}
              max={10}
              style={inputStyle}
            />
          </div>
          <div>
            <label style={{ display: "block", fontSize: 12, color: "#888", marginBottom: 4 }}>Reviewers</label>
            <input
              type="number"
              value={newReviewers}
              onChange={(e) => setNewReviewers(parseInt(e.target.value))}
              min={1}
              max={10}
              style={inputStyle}
            />
          </div>
          <div>
            <label style={{ display: "block", fontSize: 12, color: "#888", marginBottom: 4 }}>Blind</label>
            <button
              onClick={() => setNewBlind(!newBlind)}
              style={{
                ...inputStyle,
                cursor: "pointer",
                backgroundColor: newBlind ? "#22c55e22" : "#1a1a1a",
                color: newBlind ? "#22c55e" : "#888",
                fontWeight: 600,
                textAlign: "center",
              }}
            >
              {newBlind ? "ON" : "OFF"}
            </button>
          </div>
          <div>
            <label style={{ display: "block", fontSize: 12, color: "#888", marginBottom: 4 }}>&nbsp;</label>
            <button
              onClick={handleCreateWorkflow}
              disabled={creating || !newSubject}
              style={{
                backgroundColor: "#3b82f6",
                color: "white",
                border: "none",
                borderRadius: 6,
                padding: "8px 16px",
                fontSize: 13,
                fontWeight: 600,
                cursor: creating ? "wait" : "pointer",
                opacity: !newSubject ? 0.5 : 1,
                whiteSpace: "nowrap",
              }}
            >
              {creating ? "Creating..." : "Create"}
            </button>
          </div>
        </div>
      </div>

      <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 24, marginBottom: 24 }}>
        {/* Workflow Detail */}
        <div style={{
          backgroundColor: "#161616",
          border: "1px solid #222",
          borderRadius: 8,
          padding: 20,
        }}>
          <div style={{ fontSize: 14, fontWeight: 600, marginBottom: 12 }}>
            {selectedWorkflow ? "Workflow Detail" : "Select a Workflow"}
          </div>

          {selectedWorkflow ? (
            <>
              <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 8, marginBottom: 16 }}>
                <InfoItem label="ID" value={selectedWorkflow.id.slice(0, 8)} mono />
                <InfoItem label="Type" value={selectedWorkflow.workflowType.replace(/_/g, " ")} />
                <InfoItem label="Status" value={statusLabel(selectedWorkflow.status)} color={STATUS_COLORS[statusLabel(selectedWorkflow.status)]} />
                <InfoItem label="Consensus" value={`${selectedWorkflow.requiredApprovals} of ${selectedWorkflow.totalReviewers}`} />
                <InfoItem label="Blind Review" value={selectedWorkflow.blindReview ? "Yes" : "No"} color={selectedWorkflow.blindReview ? "#22c55e" : "#888"} />
                <InfoItem label="Cool-off" value={`${selectedWorkflow.coolOffHours}h`} />
                {selectedWorkflow.outcome && <InfoItem label="Outcome" value={selectedWorkflow.outcome} color={selectedWorkflow.outcome === "approved" ? "#22c55e" : "#ef4444"} />}
              </div>

              {/* Votes */}
              <div style={{ fontSize: 13, fontWeight: 600, marginBottom: 8 }}>
                Votes ({(selectedWorkflow.votes || []).length}/{selectedWorkflow.totalReviewers})
              </div>
              {(selectedWorkflow.votes || []).length > 0 ? (
                <div style={{ display: "flex", flexDirection: "column", gap: 6, marginBottom: 16 }}>
                  {(selectedWorkflow.votes || []).map((v) => (
                    <div key={v.id} style={{
                      display: "flex",
                      alignItems: "center",
                      gap: 8,
                      padding: "6px 8px",
                      backgroundColor: "#1a1a1a",
                      borderRadius: 6,
                      fontSize: 12,
                    }}>
                      <span style={{
                        fontWeight: 600,
                        color: VOTE_COLORS[voteLabel(v.vote)] || "#888",
                        width: 60,
                        textTransform: "uppercase",
                        fontSize: 11,
                      }}>
                        {voteLabel(v.vote)}
                      </span>
                      <span style={{ color: "#888", fontFamily: "monospace", fontSize: 11 }}>
                        {v.blindCaseId || v.reviewerId.slice(0, 8)}
                      </span>
                      <span style={{ color: "#666", flex: 1, fontSize: 11 }}>
                        {v.reasoning ? v.reasoning.slice(0, 40) : "—"}
                      </span>
                      <span style={{ color: "#555", fontSize: 11 }}>
                        {(v.confidence * 100).toFixed(0)}%
                      </span>
                    </div>
                  ))}
                </div>
              ) : (
                <div style={{ color: "#555", fontSize: 12, marginBottom: 16 }}>No votes yet</div>
              )}

              {/* Submit Vote */}
              {statusLabel(selectedWorkflow.status) !== "approved" &&
               statusLabel(selectedWorkflow.status) !== "rejected" && (
                <div style={{ borderTop: "1px solid #222", paddingTop: 12 }}>
                  <div style={{ fontSize: 13, fontWeight: 600, marginBottom: 8 }}>Submit Vote</div>
                  <div style={{ display: "flex", flexDirection: "column", gap: 8 }}>
                    <input
                      value={voteReviewer}
                      onChange={(e) => setVoteReviewer(e.target.value)}
                      placeholder="Reviewer Entity ID..."
                      style={inputStyle}
                    />
                    <div style={{ display: "flex", gap: 8 }}>
                      <select value={voteType} onChange={(e) => setVoteType(e.target.value)} style={{ ...inputStyle, flex: 1 }}>
                        <option value="approve">Approve</option>
                        <option value="reject">Reject</option>
                        <option value="escalate">Escalate</option>
                        <option value="abstain">Abstain</option>
                      </select>
                      <input
                        type="number"
                        value={voteConfidence}
                        onChange={(e) => setVoteConfidence(parseFloat(e.target.value))}
                        min={0}
                        max={1}
                        step={0.1}
                        style={{ ...inputStyle, width: 80 }}
                        title="Confidence (0-1)"
                      />
                    </div>
                    <input
                      value={voteReasoning}
                      onChange={(e) => setVoteReasoning(e.target.value)}
                      placeholder="Reasoning..."
                      style={inputStyle}
                    />
                    <button
                      onClick={handleSubmitVote}
                      disabled={submittingVote || !voteReviewer}
                      style={{
                        backgroundColor: VOTE_COLORS[voteType] || "#3b82f6",
                        color: "white",
                        border: "none",
                        borderRadius: 6,
                        padding: "8px 16px",
                        fontSize: 13,
                        fontWeight: 600,
                        cursor: submittingVote ? "wait" : "pointer",
                        opacity: !voteReviewer ? 0.5 : 1,
                      }}
                    >
                      {submittingVote ? "Submitting..." : `Submit ${voteType}`}
                    </button>
                  </div>
                </div>
              )}
            </>
          ) : (
            <div style={{ color: "#555", fontSize: 13 }}>Click a workflow from the list to view details and submit votes.</div>
          )}
        </div>

        {/* Reviewer Integrity */}
        <div style={{
          backgroundColor: "#161616",
          border: "1px solid #222",
          borderRadius: 8,
          padding: 20,
        }}>
          <div style={{ fontSize: 14, fontWeight: 600, marginBottom: 12 }}>Reviewer Integrity</div>
          <div style={{ display: "flex", gap: 8, marginBottom: 16 }}>
            <input
              value={reviewerIdInput}
              onChange={(e) => setReviewerIdInput(e.target.value)}
              placeholder="Reviewer Entity ID..."
              style={{ ...inputStyle, flex: 1 }}
            />
            <button
              onClick={handleLookupReviewer}
              disabled={!reviewerIdInput}
              style={{
                backgroundColor: "#222",
                color: "#ededed",
                border: "1px solid #333",
                borderRadius: 6,
                padding: "8px 16px",
                fontSize: 13,
                cursor: !reviewerIdInput ? "default" : "pointer",
                opacity: !reviewerIdInput ? 0.5 : 1,
              }}
            >
              Lookup
            </button>
          </div>

          {reviewerIntegrity ? (
            <div>
              <div style={{ display: "flex", alignItems: "center", gap: 16, marginBottom: 16 }}>
                <div>
                  <div style={{ fontSize: 11, color: "#888", textTransform: "uppercase" }}>Integrity Score</div>
                  <div style={{
                    fontSize: 36,
                    fontWeight: 700,
                    color: reviewerIntegrity.integrityScore >= 700 ? "#22c55e" :
                           reviewerIntegrity.integrityScore >= 400 ? "#f59e0b" : "#ef4444",
                  }}>
                    {reviewerIntegrity.integrityScore}
                  </div>
                </div>
                <div style={{ flex: 1 }}>
                  <div style={{ height: 8, backgroundColor: "#222", borderRadius: 4, overflow: "hidden" }}>
                    <div style={{
                      height: "100%",
                      width: `${(reviewerIntegrity.integrityScore / 1000) * 100}%`,
                      backgroundColor: reviewerIntegrity.integrityScore >= 700 ? "#22c55e" :
                                       reviewerIntegrity.integrityScore >= 400 ? "#f59e0b" : "#ef4444",
                      borderRadius: 4,
                    }} />
                  </div>
                </div>
              </div>

              <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 8 }}>
                <InfoItem label="Total Reviews" value={reviewerIntegrity.totalReviews} />
                <InfoItem label="Approval Rate" value={`${(reviewerIntegrity.approvalRate * 100).toFixed(1)}%`} />
                <InfoItem label="Avg Review Time" value={`${reviewerIntegrity.avgTimePerReview.toFixed(0)}s`} />
                <InfoItem label="Bias Flags" value={reviewerIntegrity.biasFlags} color={reviewerIntegrity.biasFlags > 0 ? "#ef4444" : "#22c55e"} />
              </div>
            </div>
          ) : (
            <div style={{ color: "#555", fontSize: 13 }}>
              Enter a reviewer entity ID to check their integrity score, approval patterns, and bias flags.
            </div>
          )}
        </div>
      </div>

      {/* Workflows List */}
      <div style={{
        backgroundColor: "#161616",
        border: "1px solid #222",
        borderRadius: 8,
        padding: 20,
      }}>
        <div style={{ fontSize: 14, fontWeight: 600, marginBottom: 12 }}>Decision Workflows</div>
        {workflows.length > 0 ? (
          <table style={{ width: "100%", borderCollapse: "collapse" }}>
            <thead>
              <tr style={{ borderBottom: "1px solid #222" }}>
                {["Type", "Status", "Subject", "Consensus", "Blind", "Votes", "Created"].map((h) => (
                  <th key={h} style={{ textAlign: "left", padding: "6px 10px", fontSize: 11, color: "#666", textTransform: "uppercase" }}>
                    {h}
                  </th>
                ))}
              </tr>
            </thead>
            <tbody>
              {workflows.map((wf) => {
                const st = statusLabel(wf.status);
                return (
                  <tr
                    key={wf.id}
                    onClick={() => setSelectedWorkflow(wf)}
                    style={{
                      borderBottom: "1px solid #1a1a1a",
                      cursor: "pointer",
                      backgroundColor: selectedWorkflow?.id === wf.id ? "#1a1a2a" : "transparent",
                    }}
                  >
                    <td style={{ padding: "8px 10px", fontSize: 13 }}>
                      {wf.workflowType.replace(/_/g, " ")}
                    </td>
                    <td style={{ padding: "8px 10px" }}>
                      <span style={{
                        display: "inline-block",
                        padding: "2px 6px",
                        borderRadius: 4,
                        fontSize: 11,
                        backgroundColor: (STATUS_COLORS[st] || "#333") + "22",
                        color: STATUS_COLORS[st] || "#888",
                      }}>
                        {st}
                      </span>
                    </td>
                    <td style={{ padding: "8px 10px", fontSize: 12, fontFamily: "monospace", color: "#888" }}>
                      {wf.subjectEntityId.slice(0, 8)}
                    </td>
                    <td style={{ padding: "8px 10px", fontSize: 12, color: "#aaa" }}>
                      {wf.requiredApprovals}/{wf.totalReviewers}
                    </td>
                    <td style={{ padding: "8px 10px", fontSize: 12, color: wf.blindReview ? "#22c55e" : "#666" }}>
                      {wf.blindReview ? "Yes" : "No"}
                    </td>
                    <td style={{ padding: "8px 10px", fontSize: 12, color: "#aaa" }}>
                      {(wf.votes || []).length}
                    </td>
                    <td style={{ padding: "8px 10px", fontSize: 12, color: "#666" }}>
                      {wf.createdAt ? new Date(wf.createdAt).toLocaleString() : "—"}
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        ) : (
          <div style={{ color: "#555", fontSize: 13, textAlign: "center", padding: 24 }}>
            No workflows yet. Create one above to start a decision process.
          </div>
        )}
      </div>
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

function InfoItem({ label, value, color, mono }: { label: string; value: string | number; color?: string; mono?: boolean }) {
  return (
    <div style={{ fontSize: 12 }}>
      <span style={{ color: "#888" }}>{label}: </span>
      <span style={{ color: color || "#ededed", fontFamily: mono ? "monospace" : "inherit" }}>{value}</span>
    </div>
  );
}
