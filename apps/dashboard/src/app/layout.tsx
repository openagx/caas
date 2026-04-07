import type { Metadata } from "next";

export const metadata: Metadata = {
  title: "CAAS Dashboard",
  description: "Continuous Autonomous Authorization System",
};

export default function RootLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    <html lang="en">
      <body style={{ margin: 0, fontFamily: "system-ui, -apple-system, sans-serif", backgroundColor: "#0a0a0a", color: "#ededed" }}>
        <div style={{ display: "flex", minHeight: "100vh" }}>
          <nav style={{
            width: 240,
            backgroundColor: "#111",
            borderRight: "1px solid #222",
            padding: "24px 16px",
            display: "flex",
            flexDirection: "column",
            gap: 4,
          }}>
            <div style={{ fontSize: 20, fontWeight: 700, marginBottom: 24, padding: "0 8px" }}>
              CAAS
            </div>
            <NavLink href="/">Dashboard</NavLink>
            <NavLink href="/entities">Entities</NavLink>
            <NavLink href="/relationships">Relationships</NavLink>
            <NavLink href="/trust">Trust Scores</NavLink>
            <NavLink href="/fraud">Fraud Detection</NavLink>
            <NavLink href="/decisions">Decisions</NavLink>
            <NavLink href="/federation">Federation</NavLink>
            <NavLink href="/surplus">Surplus</NavLink>
            <NavLink href="/permissions">Permission Tester</NavLink>
            <div style={{ marginTop: "auto", padding: "0 8px", fontSize: 12, color: "#666" }}>
              v0.7.0 — Phase 7
            </div>
          </nav>
          <main style={{ flex: 1, padding: 32 }}>
            {children}
          </main>
        </div>
      </body>
    </html>
  );
}

function NavLink({ href, children }: { href: string; children: React.ReactNode }) {
  return (
    <a
      href={href}
      style={{
        display: "block",
        padding: "8px 12px",
        borderRadius: 6,
        color: "#ccc",
        textDecoration: "none",
        fontSize: 14,
      }}
    >
      {children}
    </a>
  );
}
