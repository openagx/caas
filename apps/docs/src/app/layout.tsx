import type { Metadata } from "next";

export const metadata: Metadata = {
  title: "CAAS Developer Portal",
  description: "Documentation for the Continuous Autonomous Authorization System",
};

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="en">
      <body style={{ margin: 0, fontFamily: "system-ui, -apple-system, sans-serif", backgroundColor: "#0a0a0a", color: "#ededed" }}>
        <div style={{ display: "flex", minHeight: "100vh" }}>
          <nav style={{
            width: 260,
            backgroundColor: "#111",
            borderRight: "1px solid #222",
            padding: "24px 16px",
            display: "flex",
            flexDirection: "column",
            gap: 4,
          }}>
            <div style={{ fontSize: 20, fontWeight: 700, marginBottom: 8, padding: "0 8px" }}>
              CAAS Docs
            </div>
            <div style={{ fontSize: 12, color: "#666", padding: "0 8px", marginBottom: 24 }}>
              Developer Portal
            </div>

            <SectionHeader>Getting Started</SectionHeader>
            <NavLink href="/">Overview</NavLink>
            <NavLink href="/quickstart">Quick Start</NavLink>

            <SectionHeader>Core Concepts</SectionHeader>
            <NavLink href="/entities">Entities</NavLink>
            <NavLink href="/authorization">Authorization</NavLink>
            <NavLink href="/trust">Trust Scoring</NavLink>
            <NavLink href="/fraud">Fraud Detection</NavLink>
            <NavLink href="/decisions">Decision Integrity</NavLink>
            <NavLink href="/federation">Federation</NavLink>

            <SectionHeader>SDKs</SectionHeader>
            <NavLink href="/sdk-typescript">TypeScript</NavLink>
            <NavLink href="/sdk-go">Go</NavLink>
            <NavLink href="/sdk-python">Python</NavLink>

            <SectionHeader>API Reference</SectionHeader>
            <NavLink href="/api">REST API</NavLink>

            <div style={{ marginTop: "auto", padding: "0 8px", fontSize: 12, color: "#555" }}>
              OpenAutonomyx (OPC) Pvt Ltd
            </div>
          </nav>
          <main style={{ flex: 1, padding: "32px 48px", maxWidth: 900 }}>
            {children}
          </main>
        </div>
      </body>
    </html>
  );
}

function SectionHeader({ children }: { children: React.ReactNode }) {
  return (
    <div style={{ padding: "16px 8px 4px", fontSize: 11, fontWeight: 600, color: "#555", textTransform: "uppercase", letterSpacing: 1 }}>
      {children}
    </div>
  );
}

function NavLink({ href, children }: { href: string; children: React.ReactNode }) {
  return (
    <a href={href} style={{ display: "block", padding: "6px 12px", borderRadius: 6, color: "#aaa", textDecoration: "none", fontSize: 13 }}>
      {children}
    </a>
  );
}
