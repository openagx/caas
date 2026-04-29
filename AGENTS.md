# OpenAGX Agent Guide

This document defines how agents should work on the OpenAGX cloud agent-as-a-service platform. The goal is to keep agent infrastructure secure, observable, deployable, and easy to operate.

## Mission

OpenAGX provides cloud infrastructure and runtime services for AI agents, computer-use automation, identity-aware workflows, and related platform capabilities. Agents working in this repository should make small, safe, reviewable changes that improve reliability, security, and deployment quality.

## Repository scope

Primary OpenAGX repositories include:

```text
openagx/aagfe       cloud agent-as-a-service platform and deployment configuration
OpenAGX/cai        computer-use AI and agent automation framework
OpenAGX/midpoint   identity governance and access management integration
```

Use `openagx/aagfe` as the source of truth for cloud-agent deployment, runtime, operations, and infrastructure guidance.

## Agent operating principles

1. Prefer small, reviewable pull requests.
2. Never commit production secrets, credentials, tokens, or private keys.
3. Document every required environment variable in `.env.example` or deployment docs.
4. Keep runtime and infrastructure changes reversible.
5. Add health checks for long-running services.
6. Add rollback notes when changing deployment or production behavior.
7. Use least-privilege permissions for cloud credentials, CI jobs, service accounts, and runtime agents.
8. Keep identity, infrastructure, model execution, and UI changes separated where practical.
9. Do not mix unrelated app, infra, and policy changes in one pull request.
10. Prefer explicit deployment commands over undocumented automation.

## Standard service layout

For each deployable service, use this structure where possible:

```text
<service>/
  README.md
  docker-compose.yml
  docker-compose.prod.yml
  .env.example
  deploy/
    docker.md
    coolify.md
    railway.md
    render.md
  hardening/
    nginx.conf
    security-headers.conf
    backup.sh
    healthcheck.sh
  policies/
    permissions.md
    data-access.md
  .github/workflows/
    scan.yml
    validate.yml
```

If the service is not Docker-based, use equivalent platform-native files and document the deployment path clearly.

## Deployment baseline

Every deployable service should support a local or server deployment path:

```bash
cp .env.example .env
docker compose -f docker-compose.yml -f docker-compose.prod.yml up -d
```

For hosted deployment, use the target that fits the service:

```text
Coolify        preferred for self-hosted one-click deployment
Railway        good for hosted service templates and demos
Render         good for Docker web services
Netlify        good for static frontend apps
Docker Compose universal fallback
```

Do not add one-click deployment instructions unless the repo has safe defaults, documented variables, and no required production secrets in source control.

## Docker hardening baseline

Use this baseline where compatible with the service:

```yaml
services:
  app:
    restart: unless-stopped
    read_only: true
    security_opt:
      - no-new-privileges:true
    cap_drop:
      - ALL
    tmpfs:
      - /tmp
    healthcheck:
      test: ["CMD", "curl", "-fsS", "http://localhost:3000/health"]
      interval: 30s
      timeout: 10s
      retries: 3
    env_file:
      - .env
```

Adjust `read_only`, `tmpfs`, volumes, ports, and health checks per service. Some services need writable cache, upload, browser profile, sandbox, or database directories.

## Identity and access rules

OpenAGX agents must treat identity and authorization as core platform features.

When changing identity or access behavior:

```text
Use least privilege by default
Document all roles and scopes
Separate user permissions from agent permissions
Avoid shared production credentials
Rotate credentials after suspected exposure
Audit admin and service-account actions
Protect identity-provider secrets
Document callback URLs and trusted origins
```

For `midpoint` integrations, keep identity-governance changes separate from unrelated runtime work.

## Secrets policy

Allowed in repositories:

```text
.env.example
placeholder values
example domains
safe local-only defaults
comments explaining required variables
```

Forbidden in repositories:

```text
production API keys
OAuth client secrets
database passwords
private keys
cloud access tokens
JWT signing secrets
real webhook secrets
model provider keys
browser automation account passwords
```

Use GitHub Actions secrets, deployment-platform secrets, Docker secrets, or a cloud secret manager for sensitive values.

## CI baseline

Every active service should include basic validation:

```yaml
name: validate

on:
  pull_request:
  push:
    branches: [main]

permissions:
  contents: read

jobs:
  secret-scan:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: gitleaks/gitleaks-action@v2

  compose-check:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - name: Validate Docker Compose
        run: docker compose -f docker-compose.yml -f docker-compose.prod.yml config
```

Grant write permissions only to jobs that publish images, releases, or deployment artifacts.

## Agent runtime safety

Before changing agent runtime code, verify:

```text
Tool permissions are explicit
Dangerous tools require approval or policy gates
Network access is intentional and documented
Filesystem access is scoped
Browser automation accounts are isolated
Prompt, token, and user data logs are redacted
Timeouts and rate limits are configured
Failed tool calls are handled safely
```

Agents should not be given broad access to cloud credentials, production databases, or user data unless the access is required and policy-controlled.

## Model execution checklist

Before changing model execution behavior, verify:

```text
Provider keys are secret-managed
Request limits and timeouts are configured
Fallback behavior is documented
Logs do not leak prompts, tokens, or personal data
CPU, GPU, and memory requirements are documented
Error responses avoid exposing internal configuration
```

## Computer-use automation checklist

Before changing computer-use automation behavior, verify:

```text
Browser profiles are isolated
Downloads and uploads are restricted
External domains are allowlisted where appropriate
Credentials are not stored in screenshots or logs
User approval is required for sensitive actions
Automation sessions have timeouts
```

## Infrastructure change checklist

Before changing infrastructure, verify:

```text
The change is scoped and reversible
Required secrets are documented, not committed
Deployment commands are updated
Rollback steps are documented
Ports, domains, and public endpoints are intentional
Health checks are present
Logs and metrics are available
CI validation passes
```

## README deploy section template

Each deployable repo should include:

```md
## Deploy

### Docker Compose

```bash
cp .env.example .env
docker compose -f docker-compose.yml -f docker-compose.prod.yml up -d
```

### Security defaults

This deployment uses:

- no committed secrets
- documented environment variables
- health checks
- restart policies
- least-privilege runtime settings where supported
- reverse proxy TLS termination where applicable
```

## Definition of done

A repo is deploy-ready when it has:

```text
README deploy instructions
.env.example or equivalent environment documentation
working build or compose validation
health check or readiness probe
secret scanning workflow
rollback notes for infrastructure changes
clear deploy target documentation
no committed production secrets
explicit agent permissions and tool policies
```
