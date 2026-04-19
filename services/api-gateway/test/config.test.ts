import test from "node:test";
import assert from "node:assert/strict";
import { loadConfig } from "../src/config.js";

function withEnv(env: Record<string, string | undefined>, fn: () => void): void {
  const snapshot = { ...process.env };
  for (const [key, value] of Object.entries(env)) {
    if (value === undefined) delete process.env[key];
    else process.env[key] = value;
  }
  try {
    fn();
  } finally {
    process.env = snapshot;
  }
}

test("loadConfig uses defaults", () => {
  withEnv({ API_PORT: undefined, NODE_ENV: "development" }, () => {
    const config = loadConfig();
    assert.equal(config.port, 3001);
    assert.equal(config.rateLimitMax, 250);
    assert.equal(config.authEnabled, true);
  });
});

test("loadConfig rejects invalid boolean", () => {
  withEnv({ AUTH_ENABLED: "yes" }, () => {
    assert.throws(() => loadConfig(), /AUTH_ENABLED/);
  });
});

test("loadConfig rejects out of range API_PORT", () => {
  withEnv({ API_PORT: "70000" }, () => {
    assert.throws(() => loadConfig(), /API_PORT/);
  });
});
