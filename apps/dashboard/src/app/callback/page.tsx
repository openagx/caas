"use client";

import { useEffect, useState } from "react";
import { handleCallback } from "../../lib/auth";

export default function CallbackPage() {
  const [error, setError] = useState("");

  useEffect(() => {
    const params = new URLSearchParams(window.location.search);
    const code = params.get("code");
    const state = params.get("state");
    const err = params.get("error");

    if (err) {
      setError(params.get("error_description") || err);
      return;
    }

    if (!code || !state) {
      setError("Missing authorization code");
      return;
    }

    handleCallback(code, state).then((ok) => {
      if (ok) {
        window.location.href = "/";
      } else {
        setError("Token exchange failed");
      }
    });
  }, []);

  if (error) {
    return (
      <div style={{ display: "flex", justifyContent: "center", alignItems: "center", minHeight: "60vh" }}>
        <div style={{ width: 400, background: "#161616", border: "1px solid #333", borderRadius: 12, padding: 32, textAlign: "center" }}>
          <h2 style={{ color: "#ef4444", marginBottom: 12 }}>Authentication Error</h2>
          <p style={{ color: "#888", fontSize: 14, marginBottom: 24 }}>{error}</p>
          <a href="/login" style={{ color: "#3b82f6", textDecoration: "none", fontSize: 14 }}>Try again</a>
        </div>
      </div>
    );
  }

  return (
    <div style={{ display: "flex", justifyContent: "center", alignItems: "center", minHeight: "60vh" }}>
      <p style={{ color: "#888" }}>Completing sign in...</p>
    </div>
  );
}
