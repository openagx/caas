"use client";

import { useState, useEffect } from "react";

const API_BASE = process.env.NEXT_PUBLIC_API_URL || "http://localhost:3001";

const TYPE_LABELS: Record<number, string> = {
  1: "Surplus",
  2: "Need",
};

const TYPE_COLORS: Record<number, string> = {
  1: "#22c55e",
  2: "#f59e0b",
};

const CATEGORY_LABELS: Record<number, string> = {
  1: "Food",
  2: "Medical",
  3: "Shelter",
  4: "Clothing",
  5: "Equipment",
  6: "Transport",
  7: "Compute",
  8: "Energy",
  9: "Labor",
  10: "Other",
};

const URGENCY_LABELS: Record<number, string> = {
  1: "Low",
  2: "Medium",
  3: "High",
  4: "Critical",
};

const URGENCY_COLORS: Record<number, string> = {
  1: "#6b7280",
  2: "#3b82f6",
  3: "#f59e0b",
  4: "#ef4444",
};

const STATUS_LABELS: Record<number, string> = {
  1: "Active",
  2: "Matched",
  3: "Fulfilled",
  4: "Expired",
  5: "Cancelled",
};

const MATCH_STATUS_LABELS: Record<number, string> = {
  1: "Proposed",
  2: "Accepted",
  3: "In Transit",
  4: "Delivered",
  5: "Verified",
  6: "Rejected",
  7: "Cancelled",
};

const MATCH_STATUS_COLORS: Record<number, string> = {
  1: "#6b7280",
  2: "#3b82f6",
  3: "#f59e0b",
  4: "#22c55e",
  5: "#10b981",
  6: "#ef4444",
  7: "#ef4444",
};

interface Listing {
  id: string;
  entityId: string;
  type: number;
  category: number;
  title: string;
  description: string;
  quantity: number;
  unit: string;
  urgency: number;
  location?: { latitude: number; longitude: number; address?: string; region?: string; country?: string };
  maxDistanceKm: number;
  status: number;
  createdAt: string;
}

interface Match {
  id: string;
  surplusListingId: string;
  needListingId: string;
  surplusEntityId: string;
  needEntityId: string;
  distanceKm: number;
  matchScore: number;
  status: number;
  matchedQuantity: number;
  unit: string;
  createdAt: string;
}

interface Metrics {
  totalListings: number;
  activeSurplus: number;
  activeNeeds: number;
  totalMatches: number;
  fulfilledMatches: number;
  avgMatchDistanceKm: number;
  avgQualityScore: number;
  listingsByCategory: Record<string, number>;
}

type Tab = "listings" | "matches" | "metrics";

export default function SurplusPage() {
  const [tab, setTab] = useState<Tab>("listings");
  const [listings, setListings] = useState<Listing[]>([]);
  const [matches, setMatches] = useState<Match[]>([]);
  const [metrics, setMetrics] = useState<Metrics | null>(null);
  const [loading, setLoading] = useState(false);

  // Create listing form
  const [showCreate, setShowCreate] = useState(false);
  const [newEntityId, setNewEntityId] = useState("");
  const [newType, setNewType] = useState(1);
  const [newCategory, setNewCategory] = useState(1);
  const [newTitle, setNewTitle] = useState("");
  const [newDescription, setNewDescription] = useState("");
  const [newQuantity, setNewQuantity] = useState(1);
  const [newUnit, setNewUnit] = useState("units");
  const [newUrgency, setNewUrgency] = useState(2);

  // Find matches form
  const [findListingId, setFindListingId] = useState("");

  useEffect(() => {
    if (tab === "listings") fetchListings();
    if (tab === "matches") fetchMatches();
    if (tab === "metrics") fetchMetrics();
  }, [tab]);

  async function fetchListings() {
    setLoading(true);
    try {
      const res = await fetch(`${API_BASE}/v1/surplus/listings?page_size=50`);
      const data = await res.json();
      setListings(data.listings || []);
    } catch { setListings([]); }
    setLoading(false);
  }

  async function fetchMatches() {
    setLoading(true);
    try {
      const res = await fetch(`${API_BASE}/v1/surplus/matches?page_size=50`);
      const data = await res.json();
      setMatches(data.matches || []);
    } catch { setMatches([]); }
    setLoading(false);
  }

  async function fetchMetrics() {
    setLoading(true);
    try {
      const res = await fetch(`${API_BASE}/v1/surplus/metrics`);
      const data = await res.json();
      setMetrics(data);
    } catch { setMetrics(null); }
    setLoading(false);
  }

  async function createListing() {
    try {
      await fetch(`${API_BASE}/v1/surplus/listings`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          entity_id: newEntityId,
          type: newType,
          category: newCategory,
          title: newTitle,
          description: newDescription,
          quantity: newQuantity,
          unit: newUnit,
          urgency: newUrgency,
        }),
      });
      setShowCreate(false);
      fetchListings();
    } catch (e) {
      alert("Failed to create listing");
    }
  }

  async function findMatches() {
    if (!findListingId) return;
    try {
      const res = await fetch(`${API_BASE}/v1/surplus/matches/find`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ listing_id: findListingId, max_results: 10 }),
      });
      const data = await res.json();
      setMatches(data.matches || []);
      setTab("matches");
    } catch (e) {
      alert("Failed to find matches");
    }
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
      <h1 style={{ fontSize: 24, fontWeight: 700, marginBottom: 8 }}>Surplus Redistribution</h1>
      <p style={{ color: "#888", marginBottom: 24 }}>
        Match surplus resources with needs — geospatial matching with quality verification.
      </p>

      <div style={{ display: "flex", gap: 0, borderBottom: "1px solid #333", marginBottom: 24 }}>
        <button onClick={() => setTab("listings")} style={tabStyle("listings")}>Listings</button>
        <button onClick={() => setTab("matches")} style={tabStyle("matches")}>Matches</button>
        <button onClick={() => setTab("metrics")} style={tabStyle("metrics")}>Metrics</button>
      </div>

      {tab === "listings" && (
        <div>
          <div style={{ display: "flex", gap: 12, marginBottom: 16 }}>
            <button onClick={() => setShowCreate(!showCreate)} style={btnStyle}>
              {showCreate ? "Cancel" : "+ New Listing"}
            </button>
            <button onClick={fetchListings} style={{ ...btnStyle, background: "#333" }}>Refresh</button>
          </div>

          {showCreate && (
            <div style={cardStyle}>
              <h3 style={{ margin: "0 0 12px" }}>Create Listing</h3>
              <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 12 }}>
                <label>
                  Entity ID
                  <input value={newEntityId} onChange={e => setNewEntityId(e.target.value)} style={inputStyle} placeholder="UUID" />
                </label>
                <label>
                  Type
                  <select value={newType} onChange={e => setNewType(+e.target.value)} style={inputStyle}>
                    <option value={1}>Surplus</option>
                    <option value={2}>Need</option>
                  </select>
                </label>
                <label>
                  Category
                  <select value={newCategory} onChange={e => setNewCategory(+e.target.value)} style={inputStyle}>
                    {Object.entries(CATEGORY_LABELS).map(([k, v]) => (
                      <option key={k} value={k}>{v}</option>
                    ))}
                  </select>
                </label>
                <label>
                  Urgency
                  <select value={newUrgency} onChange={e => setNewUrgency(+e.target.value)} style={inputStyle}>
                    {Object.entries(URGENCY_LABELS).map(([k, v]) => (
                      <option key={k} value={k}>{v}</option>
                    ))}
                  </select>
                </label>
                <label style={{ gridColumn: "1 / -1" }}>
                  Title
                  <input value={newTitle} onChange={e => setNewTitle(e.target.value)} style={inputStyle} placeholder="What is being offered/needed?" />
                </label>
                <label style={{ gridColumn: "1 / -1" }}>
                  Description
                  <input value={newDescription} onChange={e => setNewDescription(e.target.value)} style={inputStyle} placeholder="Details..." />
                </label>
                <label>
                  Quantity
                  <input type="number" value={newQuantity} onChange={e => setNewQuantity(+e.target.value)} style={inputStyle} />
                </label>
                <label>
                  Unit
                  <input value={newUnit} onChange={e => setNewUnit(e.target.value)} style={inputStyle} placeholder="kg, units, hours..." />
                </label>
              </div>
              <button onClick={createListing} style={{ ...btnStyle, marginTop: 12 }}>Create</button>
            </div>
          )}

          {/* Find Matches */}
          <div style={{ ...cardStyle, marginBottom: 16 }}>
            <h3 style={{ margin: "0 0 8px" }}>Find Matches</h3>
            <div style={{ display: "flex", gap: 8 }}>
              <input
                value={findListingId}
                onChange={e => setFindListingId(e.target.value)}
                style={{ ...inputStyle, flex: 1 }}
                placeholder="Listing ID to match..."
              />
              <button onClick={findMatches} style={btnStyle}>Find</button>
            </div>
          </div>

          {loading ? (
            <p style={{ color: "#888" }}>Loading...</p>
          ) : (
            <table style={tableStyle}>
              <thead>
                <tr>
                  <th style={thStyle}>Title</th>
                  <th style={thStyle}>Type</th>
                  <th style={thStyle}>Category</th>
                  <th style={thStyle}>Quantity</th>
                  <th style={thStyle}>Urgency</th>
                  <th style={thStyle}>Status</th>
                  <th style={thStyle}>ID</th>
                </tr>
              </thead>
              <tbody>
                {listings.map(l => (
                  <tr key={l.id}>
                    <td style={tdStyle}>{l.title}</td>
                    <td style={tdStyle}>
                      <span style={{ color: TYPE_COLORS[l.type] || "#888" }}>{TYPE_LABELS[l.type] || "?"}</span>
                    </td>
                    <td style={tdStyle}>{CATEGORY_LABELS[l.category] || "?"}</td>
                    <td style={tdStyle}>{l.quantity} {l.unit}</td>
                    <td style={tdStyle}>
                      <span style={{ color: URGENCY_COLORS[l.urgency] || "#888" }}>{URGENCY_LABELS[l.urgency] || "?"}</span>
                    </td>
                    <td style={tdStyle}>{STATUS_LABELS[l.status] || "?"}</td>
                    <td style={{ ...tdStyle, fontSize: 11, fontFamily: "monospace" }}>{l.id?.slice(0, 8)}</td>
                  </tr>
                ))}
                {listings.length === 0 && (
                  <tr><td colSpan={7} style={{ ...tdStyle, textAlign: "center", color: "#666" }}>No listings found</td></tr>
                )}
              </tbody>
            </table>
          )}
        </div>
      )}

      {tab === "matches" && (
        <div>
          <button onClick={fetchMatches} style={{ ...btnStyle, marginBottom: 16, background: "#333" }}>Refresh</button>
          {loading ? (
            <p style={{ color: "#888" }}>Loading...</p>
          ) : (
            <table style={tableStyle}>
              <thead>
                <tr>
                  <th style={thStyle}>Match ID</th>
                  <th style={thStyle}>Score</th>
                  <th style={thStyle}>Distance</th>
                  <th style={thStyle}>Quantity</th>
                  <th style={thStyle}>Status</th>
                  <th style={thStyle}>Created</th>
                </tr>
              </thead>
              <tbody>
                {matches.map(m => (
                  <tr key={m.id}>
                    <td style={{ ...tdStyle, fontSize: 11, fontFamily: "monospace" }}>{m.id?.slice(0, 8)}</td>
                    <td style={tdStyle}>{m.matchScore?.toFixed(1)}</td>
                    <td style={tdStyle}>{m.distanceKm?.toFixed(1)} km</td>
                    <td style={tdStyle}>{m.matchedQuantity} {m.unit}</td>
                    <td style={tdStyle}>
                      <span style={{ color: MATCH_STATUS_COLORS[m.status] || "#888" }}>
                        {MATCH_STATUS_LABELS[m.status] || "?"}
                      </span>
                    </td>
                    <td style={tdStyle}>{m.createdAt ? new Date(m.createdAt).toLocaleDateString() : "—"}</td>
                  </tr>
                ))}
                {matches.length === 0 && (
                  <tr><td colSpan={6} style={{ ...tdStyle, textAlign: "center", color: "#666" }}>No matches found</td></tr>
                )}
              </tbody>
            </table>
          )}
        </div>
      )}

      {tab === "metrics" && (
        <div>
          <button onClick={fetchMetrics} style={{ ...btnStyle, marginBottom: 16, background: "#333" }}>Refresh</button>
          {loading ? (
            <p style={{ color: "#888" }}>Loading...</p>
          ) : metrics ? (
            <div style={{ display: "grid", gridTemplateColumns: "repeat(4, 1fr)", gap: 16 }}>
              <MetricCard label="Total Listings" value={metrics.totalListings} />
              <MetricCard label="Active Surplus" value={metrics.activeSurplus} color="#22c55e" />
              <MetricCard label="Active Needs" value={metrics.activeNeeds} color="#f59e0b" />
              <MetricCard label="Total Matches" value={metrics.totalMatches} />
              <MetricCard label="Fulfilled" value={metrics.fulfilledMatches} color="#10b981" />
              <MetricCard label="Avg Distance" value={`${(metrics.avgMatchDistanceKm || 0).toFixed(1)} km`} />
              <MetricCard label="Avg Quality" value={`${(metrics.avgQualityScore || 0).toFixed(0)}/100`} />
              <div style={cardStyle}>
                <div style={{ fontSize: 12, color: "#888", marginBottom: 8 }}>By Category</div>
                {metrics.listingsByCategory && Object.entries(metrics.listingsByCategory).map(([cat, count]) => (
                  <div key={cat} style={{ display: "flex", justifyContent: "space-between", fontSize: 13, marginBottom: 4 }}>
                    <span>{cat}</span>
                    <span style={{ color: "#fff", fontWeight: 600 }}>{count}</span>
                  </div>
                ))}
              </div>
            </div>
          ) : (
            <p style={{ color: "#666" }}>No metrics available</p>
          )}
        </div>
      )}
    </div>
  );
}

function MetricCard({ label, value, color }: { label: string; value: string | number; color?: string }) {
  return (
    <div style={cardStyle}>
      <div style={{ fontSize: 12, color: "#888", marginBottom: 4 }}>{label}</div>
      <div style={{ fontSize: 28, fontWeight: 700, color: color || "#fff" }}>{value}</div>
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
