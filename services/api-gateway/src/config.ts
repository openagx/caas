export interface AppConfig {
  nodeEnv: "development" | "test" | "production";
  port: number;
  entityEndpoint: string;
  authzEndpoint: string;
  trustEndpoint: string;
  fraudUrl: string;
  decisionEndpoint: string;
  federationEndpoint: string;
  surplusEndpoint: string;
  didEndpoint: string;
  logtoEndpoint: string;
  logtoResource: string;
  authEnabled: boolean;
  requestTimeoutMs: number;
  fraudRetryAttempts: number;
  rateLimitMax: number;
  rateLimitWindowMs: number;
}

const NODE_ENVS = new Set(["development", "test", "production"]);

function getString(name: string, fallback?: string): string {
  const value = process.env[name] ?? fallback;
  if (!value || value.trim() === "") {
    throw new Error(`Missing required environment variable: ${name}`);
  }
  return value;
}

function getInt(name: string, fallback: number, options?: { min?: number; max?: number }): number {
  const raw = process.env[name];
  const value = raw ? Number(raw) : fallback;
  if (!Number.isInteger(value)) {
    throw new Error(`Environment variable ${name} must be an integer`);
  }
  if (options?.min !== undefined && value < options.min) {
    throw new Error(`Environment variable ${name} must be >= ${options.min}`);
  }
  if (options?.max !== undefined && value > options.max) {
    throw new Error(`Environment variable ${name} must be <= ${options.max}`);
  }
  return value;
}

function getBoolean(name: string, fallback: boolean): boolean {
  const raw = process.env[name];
  if (raw === undefined) return fallback;
  if (raw === "true") return true;
  if (raw === "false") return false;
  throw new Error(`Environment variable ${name} must be either true or false`);
}

function parseNodeEnv(): AppConfig["nodeEnv"] {
  const raw = process.env.NODE_ENV ?? "development";
  if (!NODE_ENVS.has(raw)) {
    throw new Error("NODE_ENV must be one of development, test, production");
  }
  return raw as AppConfig["nodeEnv"];
}


function parseDurationMs(value: string, name: string): number {
  const match = value.trim().match(/^(\d+)\s*(second|seconds|minute|minutes|hour|hours|ms)$/i);
  if (!match) {
    throw new Error(`Environment variable ${name} must be like '60 seconds' or '1 minute'`);
  }
  const amount = Number(match[1]);
  const unit = match[2].toLowerCase();
  const multipliers: Record<string, number> = {
    ms: 1,
    second: 1000,
    seconds: 1000,
    minute: 60000,
    minutes: 60000,
    hour: 3600000,
    hours: 3600000,
  };
  return amount * multipliers[unit];
}
export function loadConfig(): AppConfig {
  return {
    nodeEnv: parseNodeEnv(),
    port: getInt("API_PORT", 3001, { min: 1, max: 65535 }),
    entityEndpoint: getString("ENTITY_ENDPOINT", "localhost:50052"),
    authzEndpoint: getString("AUTHZ_ENDPOINT", "localhost:50051"),
    trustEndpoint: getString("TRUST_ENDPOINT", "localhost:50053"),
    fraudUrl: getString("FRAUD_URL", "http://localhost:50054"),
    decisionEndpoint: getString("DECISION_ENDPOINT", "localhost:50055"),
    federationEndpoint: getString("FEDERATION_ENDPOINT", "localhost:50056"),
    surplusEndpoint: getString("SURPLUS_ENDPOINT", "localhost:50057"),
    didEndpoint: getString("DID_ENDPOINT", "localhost:50058"),
    logtoEndpoint: getString("LOGTO_ENDPOINT", "http://localhost:3301"),
    logtoResource: getString("LOGTO_RESOURCE", "https://api.caas.local"),
    authEnabled: getBoolean("AUTH_ENABLED", true),
    requestTimeoutMs: getInt("REQUEST_TIMEOUT_MS", 3000, { min: 100, max: 30000 }),
    fraudRetryAttempts: getInt("FRAUD_RETRY_ATTEMPTS", 2, { min: 0, max: 5 }),
    rateLimitMax: getInt("RATE_LIMIT_MAX", 250, { min: 1 }),
    rateLimitWindowMs: parseDurationMs(getString("RATE_LIMIT_WINDOW", "1 minute"), "RATE_LIMIT_WINDOW"),
  };
}
