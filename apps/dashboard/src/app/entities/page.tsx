import { listEntities, ENTITY_TYPES, LIFECYCLE_STATES, STATE_COLORS } from "@/lib/api";

export const dynamic = "force-dynamic";

export default async function EntitiesPage() {
  let entities: Awaited<ReturnType<typeof listEntities>> | null = null;
  let error: string | null = null;

  try {
    entities = await listEntities({ page_size: 100 });
  } catch (e) {
    error = e instanceof Error ? e.message : "Failed to fetch entities";
  }

  return (
    <div>
      <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center", marginBottom: 24 }}>
        <h1 style={{ fontSize: 28, fontWeight: 700 }}>Entities</h1>
        <span style={{ color: "#888", fontSize: 14 }}>
          {entities ? `${entities.totalCount} total` : ""}
        </span>
      </div>

      {error && (
        <div style={{
          backgroundColor: "#1a1a1a",
          border: "1px solid #333",
          borderRadius: 8,
          padding: 24,
          textAlign: "center",
          color: "#888",
        }}>
          <p style={{ fontSize: 16, marginBottom: 8 }}>Could not connect to API Gateway</p>
          <p style={{ fontSize: 13, color: "#666" }}>
            Start services with <code style={{ color: "#f59e0b" }}>docker compose up</code> then visit this page
          </p>
        </div>
      )}

      {entities && entities.entities && entities.entities.length > 0 && (
        <table style={{ width: "100%", borderCollapse: "collapse" }}>
          <thead>
            <tr style={{ borderBottom: "1px solid #222" }}>
              {["Name", "Type", "State", "ID", "Created"].map((h) => (
                <th key={h} style={{ textAlign: "left", padding: "8px 12px", fontSize: 12, color: "#666", textTransform: "uppercase", letterSpacing: 1 }}>
                  {h}
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {entities.entities.map((e) =>
              e ? (
                <tr key={e.id} style={{ borderBottom: "1px solid #1a1a1a" }}>
                  <td style={{ padding: "10px 12px", fontWeight: 500 }}>
                    <a href={`/entities/${e.id}`} style={{ color: "#60a5fa", textDecoration: "none" }}>
                      {e.displayName}
                    </a>
                  </td>
                  <td style={{ padding: "10px 12px", color: "#aaa", fontSize: 13 }}>
                    {ENTITY_TYPES[e.entityType] || "Unknown"}
                  </td>
                  <td style={{ padding: "10px 12px" }}>
                    <span style={{
                      display: "inline-block",
                      padding: "2px 8px",
                      borderRadius: 4,
                      fontSize: 12,
                      backgroundColor: (STATE_COLORS[e.lifecycleState] || "#333") + "22",
                      color: STATE_COLORS[e.lifecycleState] || "#888",
                      border: `1px solid ${STATE_COLORS[e.lifecycleState] || "#333"}44`,
                    }}>
                      {LIFECYCLE_STATES[e.lifecycleState] || "Unknown"}
                    </span>
                  </td>
                  <td style={{ padding: "10px 12px", color: "#666", fontSize: 12, fontFamily: "monospace" }}>
                    {e.id.slice(0, 8)}…
                  </td>
                  <td style={{ padding: "10px 12px", color: "#666", fontSize: 13 }}>
                    {e.createdAt ? new Date(e.createdAt).toLocaleDateString() : "—"}
                  </td>
                </tr>
              ) : null
            )}
          </tbody>
        </table>
      )}

      {entities && (!entities.entities || entities.entities.length === 0) && (
        <div style={{
          backgroundColor: "#1a1a1a",
          border: "1px solid #222",
          borderRadius: 8,
          padding: 32,
          textAlign: "center",
          color: "#888",
        }}>
          <p>No entities yet.</p>
          <p style={{ fontSize: 13, color: "#666" }}>
            Run <code style={{ color: "#f59e0b" }}>make seed</code> to load demo data
          </p>
        </div>
      )}
    </div>
  );
}
