"use client";

import { useEffect } from "react";
import { buildLoginUrl, getAccessToken } from "../../lib/auth";

export default function LoginPage() {
  useEffect(() => {
    // If already authenticated, go home
    if (getAccessToken()) {
      window.location.href = "/";
      return;
    }
    // Redirect to Logto
    window.location.href = buildLoginUrl();
  }, []);

  return (
    <div style={{ display: "flex", justifyContent: "center", alignItems: "center", minHeight: "60vh" }}>
      <p style={{ color: "#888" }}>Redirecting to sign in...</p>
    </div>
  );
}
