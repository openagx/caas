# CAAS — Continuous Autonomous Authorization System

## What This Is

CAAS is a **Zero Trust authorization infrastructure** for all digital entities. It combines Google Zanzibar-style relationship-based access control (ReBAC) with social graph trust scoring, real-time fraud detection, and human decision integrity — designed for sovereign deployment.

**Company**: OpenAutonomyx (OPC) Private Limited
**Repo**: github.com/OpenAGX/caas

## Architecture

### Entity Types (6)
1. Human Individual
2. Organization
3. Device/IoT
4. Service/API
5. AI Agent
6. Autonomous System

### Core Services
| Service | Language | Port | Description |
|---------|----------|------|-------------|
| authz-engine | Go | 50051 | SpiceDB wrapper, Check/Expand/Write/Watch |
| entity-service | Go | 50052 | Entity CRUD, lifecycle state machine |
| trust-engine | Go+Python | 50053 | Neo4j social graph, 7-dimension trust scoring |
| fraud-pipeline | Python | 50054 | ML anomaly detection, <500ms kill chain |
| decision-service | Go | 50055 | Blind review, M-of-N approval, integrity scores |
| federation-service | Go | 50056 | Cross-sovereign trust relay |
| surplus-engine | Go | 50057 | Surplus/need matching |
| api-gateway | TypeScript | 3001 | REST/GraphQL gateway |

### Infrastructure (Docker Compose)
| Service | Port | Purpose |
|---------|------|---------|
| PostgreSQL | 5432 | Entity state, audit logs, decisions |
| Redis | 6379 | Caching, rate limiting |
| Neo4j | 7474/7687 | Social graph (trust engine) |
| SpiceDB | 50051/8443 | Zanzibar tuple store |
| Redpanda | 19092 | Event streaming (Kafka-compatible) |

### Key Patterns
- **Zanzibar tuples**: `resource#relation@subject` (e.g., `document:123#viewer@user:alice`)
- **Trust-gated auth**: Permission checks can require minimum trust scores
- **Zookie tokens**: Consistency tokens for read-after-write guarantees
- **Event-driven**: All auth events stream to Redpanda for fraud detection

## Development

```bash
make dev      # Start infrastructure (Docker Compose)
make proto    # Generate code from protobuf schemas
make build    # Build all Go services
make test     # Run all tests
make seed     # Load demo data
make down     # Stop infrastructure
```

## Protobuf

All service contracts defined in `proto/caas/v1/`. Use `buf` for linting and code generation.

- `authorization.proto` — Check, Expand, Write, Watch, Lookup RPCs
- `entity.proto` — Entity CRUD, lifecycle transitions
- `trust.proto` — Trust scores, endorsements, graph queries
- `fraud.proto` — Incident management, attack simulation

Generated code goes to `gen/go/` and `gen/ts/`.

## Conventions

- Go services use standard library + minimal dependencies
- gRPC between services, REST via api-gateway for external consumers
- All state changes produce Redpanda events
- Every authorization decision is audit-logged
- Entity lifecycle: pending → active → suspended → revoked → archived
- Trust scores: 0-1000 across 7 dimensions
- Fraud kill chain target: <500ms detection to containment

## Database

- Schema in `scripts/init-db.sql`
- SpiceDB manages its own schema via `spicedb-migrate`
- Neo4j schema managed by trust-engine on startup

## Environment Variables

SpiceDB:
- `SPICEDB_GRPC_PRESHARED_KEY=caas_dev_key`
- `SPICEDB_ENDPOINT=localhost:50051`

PostgreSQL:
- `DATABASE_URL=postgres://caas:caas_dev@localhost:5432/caas`

Neo4j:
- `NEO4J_URI=bolt://localhost:7687`
- `NEO4J_USER=neo4j`
- `NEO4J_PASSWORD=caas_dev_password`

Redis:
- `REDIS_URL=redis://localhost:6379`

Redpanda:
- `KAFKA_BROKERS=localhost:19092`
