"use client";

import { useEffect, useState } from "react";
import { getSession, logout, type SessionInfo } from "../lib/auth";

export default function AuthHeader() {
  const [session, setSession] = useState<SessionInfo | null>(null);

  useEffect(() => {
    getSession().then(setSession);
  }, []);

  if (!session) return null;

  if (!session.authenticated) {
    return (
      <div style={{ display: "flex", gap: 8, alignItems: "center" }}>
        <a href="/login" style={linkStyle}>Sign In</a>
        <a href="/registration" style={{ ...linkStyle, background: "#3b82f6", color: "#fff" }}>Register</a>
      </div>
    );
  }

  return (
    <div style={{ display: "flex", gap: 12, alignItems: "center" }}>
      <span style={{ fontSize: 13, color: "#aaa" }}>
        {session.name?.first ? `${session.name.first} ${session.name.last || ""}`.trim() : session.email}
      </span>
      {session.did && (
        <span style={{ fontSize: 11, color: "#666", fontFamily: "monospace" }}>
          {session.did.length > 30 ? session.did.slice(0, 30) + "..." : session.did}
        </span>
      )}
      <button onClick={() => logout()} style={{
        padding: "4px 12px",
        background: "#333",
        color: "#ccc",
        border: "none",
        borderRadius: 4,
        cursor: "pointer",
        fontSize: 12,
      }}>
        Sign Out
      </button>
    </div>
  );
}

const linkStyle: React.CSSProperties = {
  padding: "6px 14px",
  background: "#222",
  color: "#ccc",
  border: "none",
  borderRadius: 6,
  textDecoration: "none",
  fontSize: 13,
  fontWeight: 500,
};
