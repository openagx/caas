# Behavioral Gate Policy
# Handles velocity control, anomaly detection, and resource sensitivity escalation

package authz.behavioral

import future.keywords.if

# ============================================================================
# Default State
# ============================================================================

default pass = false
default anomaly_level = "normal"

# ============================================================================
# Velocity Control
# ============================================================================

# Track operations per time window
velocity_ok {
    input.behavior.velocity_ops_5m < data.thresholds.velocity_5m_max
    input.behavior.velocity_ops_1h < data.thresholds.velocity_1h_max
}

velocity_high_ops {
    input.behavior.velocity_ops_5m >= data.thresholds.velocity_5m_max
}

velocity_high_1h {
    input.behavior.velocity_ops_1h >= data.thresholds.velocity_1h_max
}

# ============================================================================
# Anomaly Detection
# ============================================================================

anomaly_level = "critical" {
    input.behavior.anomaly_score >= data.thresholds.anomaly_critical
}

anomaly_level = "high" {
    input.behavior.anomaly_score >= data.thresholds.anomaly_normal
    input.behavior.anomaly_score < data.thresholds.anomaly_critical
}

anomaly_level = "elevated" {
    input.behavior.anomaly_score >= data.thresholds.anomaly_elevated
    input.behavior.anomaly_score < data.thresholds.anomaly_normal
}

anomaly_level = "normal" {
    input.behavior.anomaly_score < data.thresholds.anomaly_elevated
}

# ============================================================================
# Resource Sensitivity Escalation
# ============================================================================

sensitivity_pass = pass {
    sensitivity := input.resource.sensitivity
    
    # High sensitivity: strict thresholds
    sensitivity == "high"
    input.behavior.anomaly_score < data.thresholds.anomaly_high_sensitivity
    input.behavior.velocity_ops_5m < data.thresholds.velocity_5m_high_sensitivity
    pass := true
}

sensitivity_pass = pass {
    # Medium sensitivity: moderate thresholds
    sensitivity := input.resource.sensitivity
    sensitivity == "medium"
    input.behavior.anomaly_score < data.thresholds.anomaly_normal
    pass := true
}

sensitivity_pass = pass {
    # Low sensitivity: normal thresholds
    sensitivity := input.resource.sensitivity
    sensitivity != "high"
    sensitivity != "medium"
    input.behavior.anomaly_score < data.thresholds.anomaly_normal
    pass := true
}

# ============================================================================
# Combined Behavioral Pass
# ============================================================================

pass {
    not velocity_high_ops
    not velocity_high_1h
    input.behavior.anomaly_score < data.thresholds.anomaly_normal
}

pass {
    input.resource.sensitivity == "high"
    sensitivity_pass == true
}

# ============================================================================
# Telemetry Inputs
# ============================================================================

# Input structure expected from telemetry service:
# {
#   "behavior": {
#     "velocity_ops_5m": 45,
#     "velocity_ops_1h": 320,
#     "anomaly_score": 0.15,
#     "drift_score": 0.12
#   }
# }