# Release Checklist

## Pre-release gates

- [ ] `buf lint` passes.
- [ ] `go test ./...` passes in every Go service.
- [ ] `pnpm -r build` and API gateway tests pass.
- [ ] Smoke test (`services/did-service/cmd/smoke`) passes against fresh infra.
- [ ] `docker compose --profile app up -d --build` succeeds from clean workspace.

## Security and compliance checks

- [ ] No credentials committed to git.
- [ ] `.env` values are set from secret manager (not defaults).
- [ ] API gateway auth enabled for production (`AUTH_ENABLED=true`).
- [ ] Dependency audit reviewed (`pnpm audit`, `go list -m -u all`).

## Ops readiness

- [ ] Liveness/readiness/metrics endpoints wired to monitoring.
- [ ] Log aggregation ingesting structured JSON logs.
- [ ] Backups configured for Postgres + Neo4j; restore drill validated.
- [ ] On-call runbook linked in pager rotation docs.

## Launch

- [ ] Release tag created (`vX.Y.Z`).
- [ ] CHANGELOG reviewed and published.
- [ ] Rollback image and migration strategy confirmed.
