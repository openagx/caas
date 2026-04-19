# Changelog

## v0.2.0-rc.1 - 2026-04-19

### Added
- Typed API gateway runtime configuration with environment validation.
- API gateway operational endpoints: `/ready` and `/metrics`.
- Prometheus instrumentation for HTTP latency and fraud service outbound calls.
- API gateway request rate limiting and security headers.
- API gateway config unit tests.
- Production docs: release checklist and runbook.

### Changed
- API gateway logs now redact authorization headers.
- Fraud service proxy requests now include timeout + retry behavior.

### Notes
- This is a release candidate baseline for production hardening.
