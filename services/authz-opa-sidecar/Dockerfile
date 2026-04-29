# OPA Sidecar for Two-Layer Authorization
# Layer 1: SpiceDB (ReBAC) - determines relationship permission
# Layer 2: OPA (ABAC + Behavioral + Drift) - contextual allow/deny
# Final: ALLOW only if SpiceDB = ALLOW AND OPA = ALLOW

FROM openpolicyagent/opa:0.69.0-static

# OPA listens on port 8181 for the data API
EXPOSE 8181

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
  CMD wget --no-verbose --tries=1 --spider http://localhost:8181/health || exit 1

# Copy policy files
COPY opa/ /policy

# Run OPA with the authorization bundle
# Services: serve API, metrics, and Prometheus format for metrics
CMD ["run", "--server", "--config-file", "/config.yaml", "/policy"]