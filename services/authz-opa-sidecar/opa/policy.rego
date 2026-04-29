# OPA Sidecar - Two-Layer Authorization Policy
# Package: authz
# Layer 1: SpiceDB (ReBAC) - determines relationship permission
# Layer 2: OPA (ABAC + Behavioral + Drift) - contextual allow/deny

package authz

import future.keywords.if

# ============================================================================
# Core Authorization Decision
# ============================================================================

default allow = false

# Final decision: ALLOW only if SpiceDB = ALLOW AND OPA = ALLOW
# OPA MUST NOT replace SpiceDB - it is a policy enforcement + behavioral layer
allow {
    not override_deny
    spicedb_pass
    abac_pass
    behavioral_pass
    drift_pass
}

# ============================================================================
# SpiceDB Bridge Contract
# ============================================================================

# OPA input must include spicedb result
# {
#   "spicedb": {
#     "allowed": true,
#     "relationships": ["viewer", "editor"]
#   }
# }

spicedb_pass {
    input.spicedb.allowed == true
}

# ============================================================================
# Override Kill Switch
# ============================================================================

# Hard deny if anomaly score is too high (regardless of other rules)
override_deny {
    input.behavior.anomaly_score > data.thresholds.anomaly_critical
}

# Hard deny if trust score is too low
override_deny {
    input.subject.trust_score < data.thresholds.trust_minimum
}

# ============================================================================
# ABAC Context Gate
# ============================================================================

# Enforce MFA required, device trust threshold, IP reputation filter
abac_pass {
    input.subject.auth_strength == "mfa"
    input.context.device_trust > data.thresholds.device_trust_min
    input.context.ip_reputation < data.thresholds.ip_reputation_max
}

# Allow with higher auth if device trust is high enough
abac_pass {
    input.subject.auth_strength == "mfa"
    input.context.device_trust > data.thresholds.device_trust_high
}

# ============================================================================
# Behavioral Gate
# ============================================================================

# Velocity control: 5m + 1h
# Anomaly threshold
# Resource sensitivity escalation
behavioral_pass {
    input.behavior.anomaly_score < data.thresholds.anomaly_normal
    input.behavior.velocity_ops_5m < data.thresholds.velocity_5m_max
    input.behavior.velocity_ops_1h < data.thresholds.velocity_1h_max
}

# High sensitivity override: stricter thresholds for sensitive resources
behavioral_pass {
    input.resource.sensitivity == "high"
    input.behavior.anomaly_score < data.thresholds.anomaly_high_sensitivity
    input.behavior.velocity_ops_5m < data.thresholds.velocity_5m_high_sensitivity
}

# ============================================================================
# Drift Scoring Gate
# ============================================================================

# Drift score + trust score gating
drift_pass {
    input.behavior.drift_score < data.thresholds.drift_max
    input.subject.trust_score > data.thresholds.trust_for_drift
}