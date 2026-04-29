# ABAC Context Gate Policy
# Handles Attribute-Based Access Control: MFA, device trust, IP reputation

package authz.abac

import future.keywords.if

# ============================================================================
# Default State
# ============================================================================

default pass = false

# ============================================================================
# Auth Strength Requirements
# ============================================================================

# MFA is required for all access
mfa_required {
    input.subject.auth_strength == "mfa"
}

# Strong auth includes MFA + device certificate
strong_auth {
    input.subject.auth_strength == "mfa"
    input.context.device_trust >= data.thresholds.device_trust_min
}

# ============================================================================
# Device Trust
# ============================================================================

device_trusted {
    input.context.device_trust >= data.thresholds.device_trust_min
}

device_highly_trusted {
    input.context.device_trust >= data.thresholds.device_trust_high
}

device_untrusted {
    input.context.device_trust < data.thresholds.device_trust_min
}

# ============================================================================
# IP Reputation
# ============================================================================

ip_reputation_ok {
    input.context.ip_reputation < data.thresholds.ip_reputation_max
}

ip_reputation_suspicious {
    input.context.ip_reputation >= data.thresholds.ip_reputation_max
    input.context.ip_reputation < data.thresholds.ip_reputation_suspicious
}

ip_reputation_blocked {
    input.context.ip_reputation >= data.thresholds.ip_reputation_suspicious
}

# ============================================================================
# Combined ABAC Pass Conditions
# ============================================================================

# Standard: MFA + device trust + IP reputation
pass {
    mfa_required
    device_trusted
    ip_reputation_ok
}

# High security: require strong auth + highly trusted device
pass {
    strong_auth
    device_highly_trusted
}

# Reduced: if device is highly trusted, allow with MFA even if IP is moderate
pass {
    mfa_required
    device_highly_trusted
    input.context.ip_reputation < data.thresholds.ip_reputation_elevated
}

# ============================================================================
# Context Logging
# ============================================================================

auth_context = context {
    context := {
        "auth_strength": input.subject.auth_strength,
        "device_trust": input.context.device_trust,
        "ip_reputation": input.context.ip_reputation,
        "ip_country": input.context.ip_country,
        "session_id": input.context.session_id
    }
}

# ============================================================================
# Input Structure
# ============================================================================

# Expected input:
# {
#   "subject": {
#     "auth_strength": "mfa",
#     "trust_score": 0.75
#   },
#   "context": {
#     "device_trust": 0.85,
#     "ip_reputation": 0.15,
#     "ip_country": "US",
#     "session_id": "sess_abc123"
#   }
# }