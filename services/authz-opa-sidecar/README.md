# OPA Sidecar — Two-Layer Authorization

AAGFE uses a **two-layer authorization system**:

1. **Layer 1: SpiceDB (ReBAC)** — determines relationship permission
2. **Layer 2: OPA Sidecar (ABAC + Behavioral + Drift)** — contextual allow/deny

**Final rule**: `ALLOW only if SpiceDB = ALLOW AND OPA = ALLOW`

OPA runs as a sidecar alongside API services and does NOT replace SpiceDB. It is a policy enforcement + behavioral intelligence layer.

---

## Architecture

```
Request Flow:
┌─────────────────┐
│  API Gateway    │
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│  SpiceDB        │ ── Layer 1: ReBAC
│  (Relationship)│
└────────┬────────┘
         │ allowed: true/false
         ▼
┌─────────────────┐
│  OPA Sidecar    │ ── Layer 2: ABAC + Behavioral + Drift
│  (Context)     │
└────────┬────────┘
         │
         ▼
    Final Decision
```

---

## Docker Compose

```bash
docker compose up authz-opa-sidecar
```

Ports:
- `8181` — OPA server (data API)
- `8182` — Prometheus metrics

---

## API

### Decision Endpoint

```bash
POST /v1/data/authz/decision
```

**Input:**

```json
{
  "input": {
    "spicedb": {
      "allowed": true,
      "relationships": ["viewer"]
    },
    "subject": {
      "id": "user:123",
      "entity_type": "user",
      "auth_strength": "mfa",
      "trust_score": 0.8
    },
    "resource": {
      "type": "document",
      "id": "doc:456",
      "sensitivity": "low"
    },
    "context": {
      "ip": "192.168.1.1",
      "user_agent": "Mozilla/5.0",
      "device_id": "device:789",
      "device_trust": 0.9,
      "ip_reputation": 0.2,
      "session_id": "session:abc"
    },
    "behavior": {
      "velocity_ops_5m": 20,
      "velocity_ops_1h": 100,
      "anomaly_score": 0.1,
      "drift_score": 0.1
    }
  }
}
```

**Output:**

```json
{
  "result": {
    "allow": true,
    "reason": [],
    "risk_score": 0.0,
    "recommended_action": "allow"
  }
}
```

---

## Policy Structure

- `policy.rego` — Core authorization decision
- `decision.rego` — Risk score computation + action
- `abac.rego` — MFA, device trust, IP reputation
- `behavioral_gate.rego` — Velocity, anomaly detection
- `drift_scorer.rego` — Drift score + trust gating
- `data.json` — Configurable thresholds

---

## Behavioral Gate

| Scenario | Threshold |
|----------|------------|
| Normal anomaly | < 0.8 |
| High sensitivity anomaly | < 0.5 |
| Velocity 5m | < 150 ops |
| Velocity 5m (high sensitivity) | < 50 ops |

---

## Drift Scoring

| Drift Score | Trust Required | Result |
|------------|----------------|--------|
| < 0.65 | > 0.6 | ALLOW |
| >= 0.65 | any | DENY |

---

## ABAC Context

| Factor | Threshold |
|--------|-----------|
| Auth strength | MFA required |
| Device trust | > 0.5 |
| IP reputation | < 0.7 |

---

## Override Kill Switch

Hard deny if:

- `anomaly_score > 0.9` — Critical anomaly
- `trust_score < 0.3` — Very low trust

---

## Metrics

Prometheus metrics at `http://localhost:8182/metrics`:

- `opa_allow_total` — Total allows
- `opa_deny_total` — Total denials
- `behavioral_gate_failures` — Behavioral gate failures
- `drift_gate_failures` — Drift gate failures

---

## Configuration

All thresholds are configurable in `opa/data.json`:

```json
{
  "thresholds": {
    "anomaly_critical": 0.9,
    "anomaly_normal": 0.8,
    "trust_minimum": 0.3,
    "device_trust_min": 0.5,
    "velocity_5m_max": 150,
    "drift_max": 0.65
  }
}
```

---

## Notes

- OPA does NOT implement relationship logic (SpiceDB owns that)
- OPA does NOT mutate identity state
- Behavioral scores are treated as external inputs only
- All thresholds are configurable via `data.json`

---

## Testing

Run policy tests:

```bash
opa test ./opa --verbose
```

Test cases cover:

- Allowed SpiceDB + clean behavior → ALLOW
- Allowed SpiceDB + high anomaly → DENY
- Low trust → DENY
- Drift spike → DENY
- MFA missing → DENY