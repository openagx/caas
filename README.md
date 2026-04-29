# AAGFE (Agentic Automation Governance For Every Entity)

**Zero Trust Authorization Infrastructure for Autonomous AI Agents**

AAGFE combines Google Zanzibar-style ReBAC with behavioral intelligence, drift detection, and deterministic policy enforcement — providing sub-millisecond authorization for agentic AI systems.

---

## Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                        API Gateway (3001)                        │
├─────────────────────────────────────────────────────────────────┤
│  ┌──────────────┐    ┌──────────────┐    ┌──────────────────┐  │
│  │   SpiceDB    │ →  │  OPA Sidecar │ →  │  Final Decision  │  │
│  │   (ReBAC)    │    │  (ABAC+Drift)│    │                  │  │
│  │   Layer 1    │    │   Layer 2    │    │  ALLOW if BOTH   │  │
│  └──────────────┘    └──────────────┘    └──────────────────┘  │
└─────────────────────────────────────────────────────────────────┘
```

## Core Services

| Service | Port | Purpose |
|---------|------|---------|
| api-gateway | 3001 | REST/GraphQL gateway |
| authz-engine | 50051 | SpiceDB wrapper (ReBAC) |
| entity-service | 50052 | Entity CRUD + lifecycle |
| trust-engine | 50053 | Social graph + trust scoring |
| fraud-pipeline | 50054 | ML anomaly detection |
| decision-service | 50055 | M-of-N human approval |
| federation-service | 50056 | Cross-sovereign trust |
| surplus-engine | 50057 | Surplus/need matching |
| did-service | 50058 | W3C DID + VC management |
| authz-opa-sidecar | 8181 | Policy enforcement (OPA) |

## Cognitive Guard Layer (CGL)

| Service | Port | Purpose |
|---------|------|---------|
| intent-registry-service | 50060 | Task intent validation |
| cot-auditor-service | 50061 | Chain-of-thought diff |
| constraint-injector-service | 50062 | System prompt re-injection |
| drift-scorer-service | 50063 | Drift score aggregation |
| memory-isolator-service | 50064 | Task memory isolation |
| sycophancy-guard-service | 50066 | Constraint erosion detection |
| planner-iteration-service | 50067 | Planner loop monitoring |

## Agent Protocols

| Protocol | Port | Purpose |
|----------|------|---------|
| Agent Network Protocol (ANP) | 50069 | Discovery, trust, delegation |
| Agent Communication Protocol (ACP) | 50070 | Messaging, pub/sub |
| FGA Protocol | 8081 | OpenFGA-compatible API |

---

## Two-Layer Authorization

```
FINAL ALLOW = SpiceDB(ALLOW) AND OPA(ALLOW)
```

### Layer 1: SpiceDB (ReBAC)
- Zanzibar tuples: `resource#relation@subject`
- Relationship-based permissions
- Zookies for consistency guarantees

### Layer 2: OPA Sidecar (Rego)
- **Behavioral Gate**: velocity, anomaly score
- **Drift Scoring**: goal deviation, scope creep
- **ABAC**: MFA, device trust, IP reputation
- **Override Kill Switch**: hard deny on trust < 0.3 or anomaly > 0.9

---

## OWASP Agentic Top 10 Coverage

| Risk | AAGFE Countermeasure | Latency |
|------|---------------------|---------|
| ASI01 Goal Hijack | behavioral_gate | <1ms |
| ASI02 Tool Abuse | abac + fraud-pipeline | <1ms |
| ASI03 Privilege Escalation | spicedb + opa trust | <1ms |
| ASI04 Supply Chain | drift_scorer | <1ms |
| ASI05 Memory Poisoning | cot-auditor | <1ms |
| ASI06 Planning Manipulation | planner-iteration | <1ms |
| ASI07 Multi-Agent Trust | boundary-enforcer | <1ms |
| ASI08 Over-reliance | sycophancy-guard | <1ms |
| ASI09 Human Trust Exploit | decision-service | <10ms |
| ASI10 Rogue Agent | intent-registry | <1ms |

---

## Quick Start

```bash
# 1. Configure environment
cp .env.example .env

# 2. Start infrastructure + services
docker compose -f docker-compose.yml -f docker-compose.prod.yml up -d

# 3. Validate
curl -f http://localhost:3001/health
curl -f http://localhost:8181/health
```

## Operational Endpoints

- `GET /health` — Liveness probe
- `GET /ready` — Readiness check
- `GET /metrics` — Prometheus metrics

---

## Documentation

- [Whitepaper](./docs/AAGFE-WHITEPAPER.md) — Full specification
- [OWASP Mapping](./docs/owasp-mapping.md) — Risk mitigation matrix
- [Status](./docs/STATUS.md) — Implementation checklist
- [References](./docs/references.bib) — Academic citations

---

## Security Properties

- **Least Privilege**: Minimum permissions by default
- **Fail Secure**: Deny on any uncertainty
- **Behavioral Bounds**: Constrain agent behavior
- **Human-in-Loop**: Escalate high-risk to M-of-N reviewers
- **Sub-millisecond**: <1ms p99 policy evaluation

---

## License

Apache 2.0 — OpenAutonomyx (OPC) Private Limited
