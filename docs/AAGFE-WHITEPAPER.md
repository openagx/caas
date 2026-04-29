# AAGFE: Agentic Automation Governance For Every Entity

## Zero Trust Authorization for Autonomous AI Agents

**Whitepaper v1.0** — OpenAutonomyx (OPC) Private Limited

---

## Abstract

AAGFE (Agentic Automation Governance For Every Entity) is a Zero Trust authorization infrastructure that combines Google Zanzibar-style relationship-based access control (ReBAC) with behavioral-gate intelligence, drift scoring, and deterministic policy enforcement. Designed for sovereign deployment, AAGFE provides sub-millisecond authorization decisions for autonomous AI agents at scale.

---

## 1. Introduction

### The Problem

Autonomous AI agents require fine-grained authorization that traditional RBAC cannot provide. Key challenges:

1. **Relationship-based permissions** — Who can access what, and under what context
2. **Behavioral risk** — Detecting anomalous agent behavior in real-time
3. **Drift detection** — Identifying goal deviation over time
4. **Sub-millisecond latency** — Agents execute at speed; authorization must keep pace

### The Solution

AAGFE implements a two-layer authorization system:

```
Layer 1: SpiceDB (ReBAC) → Relationship permission
Layer 2: OPA Sidecar (ABAC + Behavioral + Drift) → Contextual allow/deny

FINAL: ALLOW only if SpiceDB = ALLOW AND OPA = ALLOW
```

---

## 2. Architecture

### 2.1 Entity Types (6)

| Type | Description |
|------|-------------|
| Human Individual | Natural persons |
| Organization | Corporate entities |
| Device/IoT | Hardware identities |
| Service/API | Microservices |
| AI Agent | Autonomous agents |
| Autonomous System | Agent collectives |

### 2.2 Core Services

| Service | Port | Technology |
|--------|------|------------|
| authz-engine | 50051 | Go + SpiceDB |
| entity-service | 50052 | Go + PostgreSQL |
| trust-engine | 50053 | Go + Python |
| fraud-pipeline | 50054 | Python |
| decision-service | 50055 | Go |
| authz-opa-sidecar | 8181 | OPA/Rego |

### 2.3 Authorization Stack

**Backend:** SpiceDB (Zanzibar database)
**Protocol:** FGA (Fine-Grained Authorization) API for client interoperability

| Component | Technology | Purpose |
|-----------|------------|---------|
| SpiceDB | Database | Zanzibar tuples (ReBAC) |
| OPA Sidecar | Policy | ABAC + behavioral + drift |
| FGA Protocol | API | Client interface (OpenFGA-compatible) |

### 2.4 Data Model

**Zanzibar tuples:** `resource#relation@subject`

Example: `document:123#viewer@user:alice`

### 2.4 Trust Score (0-1000, 7 dimensions)

1. **Identity Verification** — KYC/identity proof
2. **Behavioral Baseline** — Historical pattern match
3. **Device Trust** — Device fingerprint confidence
4. **Network Reputation** — IP/ASN history
5. **Credential Age** — Time since last rotation
6. **Social Graph** — Trust network density
7. **Endorsement** — Verified endorsements

---

## 3. Two-Layer Authorization

### 3.1 Layer 1: SpiceDB (ReBAC)

SpiceDB evaluates relationship permissions:

```go
// Query SpiceDB
allowed, token, err := spicedb.Check(ctx, resType, resID, perm, subType, subID)
```

### 3.2 Layer 2: OPA Sidecar

OPA evaluates contextual risk:

```rego
# policy.rego
allow {
    not override_deny
    spicedb_pass
    abac_pass
    behavioral_pass
    drift_pass
}
```

**Behavioral Gate:**
- Velocity control (5m + 1h)
- Anomaly threshold
- Resource sensitivity escalation

**Drift Scoring:**
- Goal deviation tracking
- Memory contamination detection
- Chain-of-thought anomaly

**ABAC:**
- MFA enforcement
- Device trust threshold
- IP reputation filtering

---

## 4. OWASP Agentic Top 10 Coverage

| # | OWASP Risk | AAGFE Countermeasure | Latency |
|---|-----------|---------------------|---------|
| ASI01 | Goal Hijack | behavioral_gate | <1ms |
| ASI02 | Tool Abuse | abac + fraud-pipeline | <1ms |
| ASI03 | Privilege Escalation | spicedb + opa trust | <1ms |
| ASI04 | Supply Chain | drift_scorer | <1ms |
| ASI05 | Memory Poisoning | cot-auditor | <1ms |
| ASI06 | Planning Manipulation | planner-iteration | <1ms |
| ASI07 | Multi-Agent Trust | boundary-enforcer | <1ms |
| ASI08 | Over-reliance | sycophancy-guard | <1ms |
| ASI09 | Human Trust Exploit | decision-service | <10ms |
| ASI10 | Rogue Agent | intent-registry | <1ms |

All policies enforce with **<1ms p99 latency**.

---

## 5. Deployment

### 5.1 Docker Compose

```bash
cp .env.example .env
docker compose -f docker-compose.yml -f docker-compose.prod.yml up -d
```

### 5.2 Environment Variables

| Variable | Description |
|----------|-------------|
| SPICEDB_ENDPOINT | SpiceDB host:port |
| SPICEDB_PRESHARED_KEY | Authentication key |
| DATABASE_URL | PostgreSQL connection |
| REDIS_URL | Redis cache |
| KAFKA_BROKERS | Redpanda brokers |

### 5.3 Health Checks

Every service exposes `/health` endpoint with:
- Liveness probe
- Readiness probe
- Prometheus metrics

---

## 6. Observability

### 6.1 Metrics

| Metric | Type | Description |
|--------|------|-------------|
| opa_allow_total | Counter | Total allows |
| opa_deny_total | Counter | Total denials |
| behavioral_gate_failures | Counter | Behavioral blocks |
| drift_gate_failures | Counter | Drift blocks |

### 6.2 Logging

Structured JSON logging with trace IDs:

```json
{
  "trace_id": "abc123",
  "spicedb": {"allowed": true},
  "opa_decision": {"allow": true, "risk_score": 0.1},
  "latency_ms": 0.5
}
```

---

## 7. Security Properties

### 7.1 Tenets

1. **Least Privilege** — Minimum permissions by default
2. **Explicit Verification** — Always verify relationships
3. **Fail Secure** — Deny on any uncertainty
4. **Behavioral Bounds** — Constrain agent behavior
5. **Human-in-Loop** — Escalate high-risk to humans

### 7.2 Defense in Depth

| Layer | Mechanism |
|-------|----------|
| Identity | DID + W3C VC |
| Authorization | SpiceDB ReBAC |
| Context | OPA ABAC |
| Behavior | Fraud ML |
| Decision | M-of-N human |

---

## 8. Comparison

| Feature | AAGFE | OpenZiti | Authzed |
|--------|-------|---------|--------|
| ReBAC (Zanzibar) | ✅ | Partial | ✅ |
| Behavioral Gate | ✅ | ❌ | ❌ |
| Drift Score | ✅ | ❌ | ❌ |
| FGA Protocol | ✅ | ❌ | ✅ |
| OPA Integration | ✅ | ❌ | ❌ |
| Sub-ms Latency | ✅ | ❌ | Partial |
| Agent Support | ✅ | ✅ | Partial |

---

## 9. References

- Google Zanzibar: Google's Consistent, Global Authorization System (ATC '19)
- OpenFGA: Fine-Grained Authorization Protocol (github.com/openfga)
- OWASP Top 10 for Agentic Applications (2025)
- Open Policy Agent: Policy Language and Evaluation
- W3C Decentralized Identifiers (DIDs) v1.0

---

## 10. Conclusion

AAGFE provides production-ready authorization for autonomous AI agents with:

- ✅ Sub-millisecond enforcement
- ✅ OWASP Top 10 coverage
- ✅ Relationship-based access (ReBAC)
- ✅ Behavioral intelligence
- ✅ Drift detection
- ✅ Sovereign deployment

**Contact:** OpenAutonomyx (OPC) Private Limited  
**Repo:** github.com/openagx/caas  
**License:** Apache 2.0