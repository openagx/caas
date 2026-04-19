# CAAS (Continuous Autonomous Authorization System)

CAAS is a zero-trust authorization platform composed of gRPC microservices and a REST API gateway.

## Production quick start

1. Copy environment defaults and fill secrets:
   ```bash
   cp .env.example .env
   ```
2. Start infrastructure and services:
   ```bash
   docker compose --profile app up -d --build
   ```
3. Validate operational endpoints:
   ```bash
   curl -f http://localhost:3001/health
   curl -f http://localhost:3001/ready
   curl -f http://localhost:3001/metrics
   ```

## Operational endpoints

- `GET /health`: liveness probe.
- `GET /ready`: readiness check (validates entity service dependency).
- `GET /metrics`: Prometheus metrics for API gateway latency and outbound fraud service calls.

## CI/CD

- CI workflow runs protobuf lint, Go build/test/vet, TypeScript build/test, and smoke test.
- Release workflow (manual dispatch) builds all services and produces Docker image artifacts metadata.

## Security baseline

- Environment variables are validated at startup (type/range checked).
- API gateway uses structured logs with authorization header redaction.
- API gateway enforces security headers and request rate limiting.
- Authentication can be disabled only via explicit `AUTH_ENABLED=false` for local/internal testing.

## Release process

See:
- `docs/release-checklist.md`
- `docs/runbook.md`
- `CHANGELOG.md`
