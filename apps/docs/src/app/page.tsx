export default function DocsHome() {
  return (
    <div>
      <h1 style={{ fontSize: 36, fontWeight: 700, marginBottom: 8 }}>
        CAAS Developer Portal
      </h1>
      <p style={{ fontSize: 18, color: "#888", marginBottom: 32 }}>
        Continuous Autonomous Authorization System
      </p>

      <div style={{ lineHeight: 1.8, color: "#ccc", fontSize: 15 }}>
        <p>
          CAAS is a <strong>Zero Trust authorization infrastructure</strong> for all digital entities.
          It combines Google Zanzibar-style relationship-based access control (ReBAC) with social graph
          trust scoring, real-time fraud detection, human decision integrity, and sovereign federation.
        </p>

        <h2 style={{ fontSize: 22, marginTop: 32, marginBottom: 16 }}>Architecture</h2>

        <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 16, marginBottom: 32 }}>
          <Card title="6 Entity Types" description="Human, Organization, Device/IoT, Service/API, AI Agent, Autonomous System" />
          <Card title="7-Dimension Trust" description="Identity, Behavior, Reputation, Transactions, Compliance, Stability, Peer Endorsement (0-1000)" />
          <Card title="Sub-50ms Auth" description="Zanzibar-style permission checks via SpiceDB with Redis caching" />
          <Card title="6-Stage Kill Chain" description="Detect, Classify, Contain, Revoke, Notify, Investigate — under 500ms" />
          <Card title="Blind Review" description="M-of-N consensus with PII-stripped cases and cool-off enforcement" />
          <Card title="Sovereign Federation" description="Cross-sovereign trust with bilateral treaties and 5-tier authority hierarchy" />
        </div>

        <h2 style={{ fontSize: 22, marginTop: 32, marginBottom: 16 }}>Services</h2>

        <table style={{ width: "100%", borderCollapse: "collapse", marginBottom: 32 }}>
          <thead>
            <tr style={{ borderBottom: "1px solid #333" }}>
              {["Service", "Language", "Port", "Description"].map((h) => (
                <th key={h} style={{ textAlign: "left", padding: "8px 12px", fontSize: 12, color: "#888", textTransform: "uppercase" }}>{h}</th>
              ))}
            </tr>
          </thead>
          <tbody>
            {[
              ["authz-engine", "Go", "50051", "SpiceDB wrapper — Check, Write, Lookup"],
              ["entity-service", "Go", "50052", "Entity CRUD, lifecycle state machine"],
              ["trust-engine", "Go + Python", "50053", "Neo4j social graph, 7-dimension trust scoring"],
              ["fraud-pipeline", "Python", "50054", "ML anomaly detection, kill chain"],
              ["decision-service", "Go", "50055", "Blind review, M-of-N approval, integrity"],
              ["federation-service", "Go", "50056", "Sovereign nodes, treaties, VCs"],
              ["api-gateway", "TypeScript", "3001", "REST gateway with Swagger UI"],
            ].map(([name, lang, port, desc]) => (
              <tr key={name} style={{ borderBottom: "1px solid #1a1a1a" }}>
                <td style={{ padding: "8px 12px", fontFamily: "monospace", fontSize: 13, color: "#3b82f6" }}>{name}</td>
                <td style={{ padding: "8px 12px", fontSize: 13 }}>{lang}</td>
                <td style={{ padding: "8px 12px", fontSize: 13, fontFamily: "monospace", color: "#888" }}>{port}</td>
                <td style={{ padding: "8px 12px", fontSize: 13, color: "#aaa" }}>{desc}</td>
              </tr>
            ))}
          </tbody>
        </table>

        <h2 style={{ fontSize: 22, marginTop: 32, marginBottom: 16 }}>Quick Start</h2>

        <CodeBlock>{`# Install SDKs
npm install @caas/sdk          # TypeScript
go get github.com/OpenAGX/caas/packages/sdk-go  # Go
pip install caas-sdk            # Python`}</CodeBlock>

        <h3 style={{ fontSize: 18, marginTop: 24, marginBottom: 12 }}>TypeScript</h3>
        <CodeBlock>{`import { CaasClient, EntityType } from "@caas/sdk";

const caas = new CaasClient({ baseUrl: "http://localhost:3001" });

// Create an entity
const { entity } = await caas.entities.create(EntityType.AI_AGENT, "Agent-42");

// Check permission
const { allowed } = await caas.authz.check(
  { type: "ai_agent", id: entity.id },
  "invoke",
  { type: "service", id: "payments-api" }
);

// Privacy-preserving trust attestation
const attestation = await caas.trust.attest(entity.id, 600);
console.log(attestation.meetsThreshold); // true/false (score never revealed)`}</CodeBlock>

        <h3 style={{ fontSize: 18, marginTop: 24, marginBottom: 12 }}>Go</h3>
        <CodeBlock>{`import caas "github.com/OpenAGX/caas/packages/sdk-go"

client := caas.NewClient("http://localhost:3001")

// Create entity
entity, _ := client.Entities.Create(ctx, caas.EntityTypeAIAgent, "Agent-42", nil)

// Check permission
result, _ := client.Authz.Check(ctx,
    caas.Ref("ai_agent", entity.ID),
    "invoke",
    caas.Ref("service", "payments-api"),
)

// Privacy-preserving trust attestation
attestation, _ := client.Trust.Attest(ctx, entity.ID, 600)`}</CodeBlock>

        <h3 style={{ fontSize: 18, marginTop: 24, marginBottom: 12 }}>Python</h3>
        <CodeBlock>{`from caas_sdk import CaasClient, EntityType

client = CaasClient("http://localhost:3001")

# Create entity
result = client.entities.create(EntityType.AI_AGENT, "Agent-42")

# Check permission
check = client.authz.check(
    subject={"type": "ai_agent", "id": result["entity"]["id"]},
    permission="invoke",
    resource={"type": "service", "id": "payments-api"},
)

# Privacy-preserving trust attestation
attestation = client.trust.attest(result["entity"]["id"], threshold=600)`}</CodeBlock>

        <h2 style={{ fontSize: 22, marginTop: 32, marginBottom: 16 }}>API Documentation</h2>
        <p>
          The interactive API documentation (Swagger UI) is available at{" "}
          <code style={{ backgroundColor: "#1a1a1a", padding: "2px 6px", borderRadius: 4 }}>
            http://localhost:3001/docs
          </code>{" "}
          when the API gateway is running.
        </p>
      </div>
    </div>
  );
}

function Card({ title, description }: { title: string; description: string }) {
  return (
    <div style={{
      backgroundColor: "#161616",
      border: "1px solid #222",
      borderRadius: 8,
      padding: 16,
    }}>
      <div style={{ fontSize: 14, fontWeight: 600, marginBottom: 4 }}>{title}</div>
      <div style={{ fontSize: 13, color: "#888" }}>{description}</div>
    </div>
  );
}

function CodeBlock({ children }: { children: string }) {
  return (
    <pre style={{
      backgroundColor: "#161616",
      border: "1px solid #222",
      borderRadius: 8,
      padding: 16,
      overflow: "auto",
      fontSize: 13,
      lineHeight: 1.6,
      color: "#ccc",
    }}>
      <code>{children}</code>
    </pre>
  );
}
