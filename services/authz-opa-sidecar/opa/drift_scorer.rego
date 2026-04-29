# Drift Scoring Gate Policy
# Handles behavioral drift detection and trust-based gating

package authz.drift

import future.keywords.if

# ============================================================================
# Default State
# ============================================================================

default pass = false
default drift_level = "normal"

# ============================================================================
# Drift Score Thresholds
# ============================================================================

drift_pass {
    input.behavior.drift_score < data.thresholds.drift_max
    input.subject.trust_score > data.thresholds.trust_for_drift
}

drift_partial_pass {
    input.behavior.drift_score < data.thresholds.drift_max
    input.subject.trust_score > data.thresholds.trust_minimum
    input.subject.trust_score <= data.thresholds.trust_for_drift
}

drift_fail {
    input.behavior.drift_score >= data.thresholds.drift_max
}

drift_critical {
    input.behavior.drift_score >= data.thresholds.drift_critical
}

# ============================================================================
# Drift Levels
# ============================================================================

drift_level = "critical" {
    input.behavior.drift_score >= data.thresholds.drift_critical
}

drift_level = "elevated" {
    input.behavior.drift_score >= data.thresholds.drift_elevated
    input.behavior.drift_score < data.thresholds.drift_critical
}

drift_level = "moderate" {
    input.behavior.drift_score >= data.thresholds.drift_moderate
    input.behavior.drift_score < data.thresholds.drift_elevated
}

drift_level = "normal" {
    input.behavior.drift_score < data.thresholds.drift_moderate
}

# ============================================================================
# Trust-Combined Drift Analysis
# ============================================================================

# High trust + moderate drift: allow with warning
combined_pass {
    input.subject.trust_score >= data.thresholds.trust_high_combined
    input.behavior.drift_score < data.thresholds.drift_elevated
}

# Medium trust + low drift: allow
combined_pass {
    input.subject.trust_score >= data.thresholds.trust_for_drift
    input.behavior.drift_score < data.thresholds.drift_moderate
}

# ============================================================================
# Drift Telemetry
# ============================================================================

# Input structure expected:
# {
#   "behavior": {
#     "drift_score": 0.35,
#     ...
#   },
#   "subject": {
#     "trust_score": 0.75,
#     ...
#   }
# }