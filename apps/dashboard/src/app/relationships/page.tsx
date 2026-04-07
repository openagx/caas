export default function RelationshipsPage() {
  return (
    <div>
      <h1 style={{ fontSize: 28, fontWeight: 700, marginBottom: 8 }}>Relationships</h1>
      <p style={{ color: "#888", marginBottom: 24 }}>
        Zanzibar relationship tuples — resource#relation@subject
      </p>

      <div style={{
        backgroundColor: "#161616",
        border: "1px solid #222",
        borderRadius: 8,
        padding: 32,
        textAlign: "center",
        color: "#888",
      }}>
        <p style={{ fontSize: 16, marginBottom: 8 }}>Relationship Graph</p>
        <p style={{ fontSize: 13, color: "#666" }}>
          D3 force-directed visualization coming in Phase 2.
          <br />
          Use the <a href="/docs" style={{ color: "#60a5fa" }}>API</a> at <code style={{ color: "#f59e0b" }}>GET /v1/relationships</code> to query tuples.
        </p>
      </div>
    </div>
  );
}
