export default function DashboardPage() {
  return (
    <div>
      <h1 style={{ fontSize: 28, fontWeight: 700, marginBottom: 8 }}>CAAS Dashboard</h1>
      <p style={{ color: "#888", marginBottom: 32 }}>
        Continuous Autonomous Authorization System — Phase 1 MVP
      </p>

      <div style={{ display: "grid", gridTemplateColumns: "repeat(3, 1fr)", gap: 16, marginBottom: 32 }}>
        <StatCard title="Entity Types" value="6" subtitle="Human, Org, Device, Service, AI Agent, Autonomous" />
        <StatCard title="Authorization" value="Zanzibar" subtitle="SpiceDB-powered ReBAC checks" />
        <StatCard title="Target Latency" value="<50ms" subtitle="Permission check p99" />
      </div>

      <div style={{ display: "grid", gridTemplateColumns: "repeat(2, 1fr)", gap: 16 }}>
        <InfoCard title="Services">
          <ul style={{ margin: 0, paddingLeft: 20, color: "#aaa", lineHeight: 2 }}>
            <li>authz-engine (Go) — :50051</li>
            <li>entity-service (Go) — :50052</li>
            <li>api-gateway (Fastify) — :3001</li>
            <li>dashboard (Next.js) — :3000</li>
          </ul>
        </InfoCard>
        <InfoCard title="Infrastructure">
          <ul style={{ margin: 0, paddingLeft: 20, color: "#aaa", lineHeight: 2 }}>
            <li>SpiceDB — Zanzibar tuple store</li>
            <li>PostgreSQL 16 — Entity state</li>
            <li>Redis 7 — Permission cache</li>
            <li>Redpanda — Event streaming</li>
          </ul>
        </InfoCard>
      </div>
    </div>
  );
}

function StatCard({ title, value, subtitle }: { title: string; value: string; subtitle: string }) {
  return (
    <div style={{ backgroundColor: "#161616", border: "1px solid #222", borderRadius: 8, padding: 20 }}>
      <div style={{ fontSize: 12, color: "#888", textTransform: "uppercase", letterSpacing: 1 }}>{title}</div>
      <div style={{ fontSize: 32, fontWeight: 700, margin: "8px 0" }}>{value}</div>
      <div style={{ fontSize: 13, color: "#666" }}>{subtitle}</div>
    </div>
  );
}

function InfoCard({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <div style={{ backgroundColor: "#161616", border: "1px solid #222", borderRadius: 8, padding: 20 }}>
      <div style={{ fontSize: 14, fontWeight: 600, marginBottom: 12 }}>{title}</div>
      {children}
    </div>
  );
}
