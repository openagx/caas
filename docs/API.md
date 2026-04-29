# AAGFE API Documentation

## Core Services

### API Gateway (Port 3001)

**REST Endpoints:**

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | /health | Liveness probe |
| GET | /ready | Readiness check |
| GET | /metrics | Prometheus metrics |
| POST | /v1/entities | Create entity |
| GET | /v1/entities/:id | Get entity |
| PUT | /v1/entities/:id | Update entity |
| DELETE | /v1/entities/:id | Delete entity |
| GET | /v1/relationships | List relationships |
| POST | /v1/relationships | Create relationship |
| GET | /v1/trust/:entity_id | Get trust score |
| POST | /v1/decisions | Submit decision request |

### Auth Engine gRPC (Port 50051)

| Method | Description |
|--------|-------------|
| Check | Check authorization (ReBAC) |
| Expand | Expand relationships |
| Write | Write relationship tuples |
| Watch | Watch for changes |

### Entity Service gRPC (Port 50052)

| Method | Description |
|--------|-------------|
| CreateEntity | Create entity |
| GetEntity | Get entity |
| UpdateEntity | Update entity |
| DeleteEntity | Delete entity |
| ListEntities | List entities |
| TransitionState | Entity lifecycle state transition |

### Trust Engine gRPC (Port 50053)

| Method | Description |
|--------|-------------|
| ComputeTrustScore | Compute trust score |
| GetTrustScore | Get entity trust score |
| UpdateTrust | Update trust relationship |
| QueryGraph | Query social graph |

### Fraud Pipeline gRPC (Port 50054)

| Method | Description |
|--------|-------------|
| DetectAnomaly | Detect behavioral anomaly |
| GetRiskScore | Get entity risk score |
| ReportIncident | Report fraud incident |

### Decision Service gRPC (Port 50055)

| Method | Description |
|--------|-------------|
| RequestDecision | Request M-of-N decision |
| GetDecision | Get decision status |
| Vote | Submit decision vote |

### DID Service gRPC (Port 50058)

| Method | Description |
|--------|-------------|
| CreateDID | Create W3C DID |
| ResolveDID | Resolve DID |
| IssueVC | Issue Verifiable Credential |
| VerifyVC | Verify Verifiable Credential |

### OPA Sidecar (Port 8181)

**REST Endpoints:**

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | /health | Health check |
| POST | /v1/data/authz/allow | Evaluate authorization |

**OPA Input:**

```json
{
  "spicedb": { "allowed": true },
  "behavior": { 
    "anomaly_score": 0.0,
    "velocity_ops_5m": 0,
    "drift_score": 0.0
  },
  "subject": {
    "trust_score": 1.0,
    "auth_strength": "mfa"
  },
  "context": {
    "device_trust": 1.0,
    "ip_reputation": 0.0
  },
  "resource": {
    "sensitivity": "normal"
  }
}
```

**OPA Decision:**

```json
{
  "allow": true,
  "reason": [],
  "risk_score": 0.0,
  "recommended_action": "allow"
}
```

## Cognitive Guard Layer (CGL)

### Intent Registry (Port 50060)

| Method | Description |
|--------|-------------|
| RegisterIntent | Register task intent |
| ValidateIntent | Validate intent |
| GetIntent | Get registered intent |

### CoT Auditor (Port 50061)

| Method | Description |
|--------|-------------|
| AuditCoT | Audit chain-of-thought |
| Compare | Compare CoT changes |

### Constraint Injector (Port 50062)

| Method | Description |
|--------|-------------|
| InjectConstraints | Inject system constraints |
| VerifyConstraints | Verify constraints present |

### Drift Scorer (Port 50063)

| Method | Description |
|--------|-------------|
| ScoreDrift | Calculate drift score |
| GetDriftHistory | Get drift history |

### Memory Isolator (Port 50064)

| Method | Description |
|--------|-------------|
| CreateTaskMemory | Create isolated memory |
| GetTaskMemory | Get task memory |
| ClearTaskMemory | Clear task memory |

### Sycophancy Guard (Port 50066)

| Method | Description |
|--------|-------------|
| CheckCompliance | Check constraint compliance |
| DetectErosion | Detect constraint erosion |

### Planner Iteration (Port 50067)

| Method | Description |
|--------|-------------|
| TrackIteration | Track planner iteration |
| GetIterationCount | Get iteration count |

## Agent Protocols

### Agent Network Protocol (Port 50069)

| Method | Description |
|--------|-------------|
| RegisterAgent | Register on network |
| DiscoverAgents | Discover agents |
| NegotiateTrust | Establish trust |
| Delegate | Grant delegation |
| VerifyDelegation | Verify delegation |

### Agent Communication Protocol (Port 50070)

| Method | Description |
|--------|-------------|
| Send | Send message |
| Request | Request/response |
| Publish | Publish to topic |
| Subscribe | Subscribe to topic |
| Broadcast | Broadcast message |
| GetInbox | Get messages |

### FGA Protocol (Port 8081)

OpenFGA-compatible API:

| Method | Endpoint |
|--------|----------|
| POST | /stores/:id/check |
| POST | /stores/:id/expand |
| POST | /stores/:id/list-objects |
| POST | /stores/:id/read |
| POST | /stores/:id/write |

## gRPC Service Definition

All gRPC services use Protocol Buffers. See `proto/caas/v1/` for full definitions.

### Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| DATABASE_URL | PostgreSQL connection | postgres://caas:caas_dev@localhost:5432/caas |
| REDIS_URL | Redis connection | redis://localhost:6379 |
| SPICEDB_ENDPOINT | SpiceDB gRPC | localhost:50051 |
| NEO4J_URI | Neo4j connection | bolt://localhost:7687 |
| KAFKA_BROKERS | Redpanda brokers | localhost:19092 |