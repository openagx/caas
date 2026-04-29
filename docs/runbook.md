# AAGFE Operations Runbook

## Common incidents

### API gateway returns 503 on `/ready`

1. Check entity service health/logs:
   ```bash
   docker compose logs --tail=200 entity-service
   ```
2. Verify gateway env endpoint configuration (`ENTITY_ENDPOINT`).
3. Confirm gRPC service reachable from gateway network namespace.

### Spike in 401 responses

1. Confirm Logto availability and OIDC endpoint status.
2. Check `LOGTO_ENDPOINT` and token issuer configuration.
3. Validate API clients are sending bearer tokens.

### High latency in gateway

1. Inspect `/metrics` histogram `caas_api_gateway_http_request_duration_ms`.
2. Check downstream dependencies (especially fraud-pipeline and trust-engine).
3. Increase gateway replicas and enforce per-service autoscaling.

## Backup and recovery baseline

- PostgreSQL: daily snapshot + WAL archiving.
- Neo4j: nightly backup plus transaction log retention.
- Redpanda: topic replication + retention policy alignment.

## Recovery steps

1. Restore database snapshots in isolated environment.
2. Run smoke tests against restored environment.
3. Promote restore target and redirect traffic.
