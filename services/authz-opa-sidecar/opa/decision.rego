# Decision Output
# Computes risk score and recommended action based on all gates

package authz.decision

import future.keywords.if

# ============================================================================
# Decision Response
# ============================================================================

# Main decision entry point
response = {
    "allow": allow,
    "reason": reason,
    "risk_score": risk_score,
    "recommended_action": recommended_action
} {
    allow := authz.allow
    reason := combined_reasons
    risk_score := compute_risk_score
    recommended_action := determine_action(allow, risk_score)
}

# ============================================================================
# Risk Score Computation
# ============================================================================

compute_risk_score = score {
    score := (anomaly_risk + velocity_risk + drift_risk + context_risk)
}

anomaly_risk = data.risk_weights.anomaly * input.behavior.anomaly_score {
    input.behavior.anomaly_score > 0
} else = 0

velocity_risk = data.risk_weights.velocity * velocity_normalized {
    velocity_normalized := (input.behavior.velocity_ops_5m / data.thresholds.velocity_5m_max)
}

drift_risk = data.risk_weights.drift * input.behavior.drift_score {
    input.behavior.drift_score > 0
} else = 0

context_risk = data.risk_weights.context * (1 - context_trust_score) {
    context_trust_score := min([input.context.device_trust, 1 - input.context.ip_reputation])
}

# ============================================================================
# Reason Aggregation
# ============================================================================

combined_reasons = reasons {
    reasons := [reason |
        reason := reason_set[_]
    some reason_set in [
        {reason |
            not authz.override_deny == false
            reason := "override_deny"
        },
        {reason |
            not authz.spicedb_pass
            reason := "spicedb_failed"
        },
        {reason |
            not authz.abac_pass
            reason := "abac_failed"
        },
        {reason |
            not authz.behavioral_pass
            reason := "behavioral_failed"
        },
        {reason |
            not authz.drift_pass
            reason := "drift_failed"
        }
    ]
        count(reason_set) > 0
    ]
}

# ============================================================================
# Recommended Action
# ============================================================================

determine_action(allow, risk_score) = action {
    allow == true
    risk_score < 0.3
    action := data.actions.allow
}

determine_action(allow, risk_score) = action {
    allow == true
    risk_score >= 0.3
    risk_score < 0.6
    action := data.actions.step_up_auth
}

determine_action(allow, risk_score) = action {
    allow == true
    risk_score >= 0.6
    action := data.actions.deny
}

determine_action(allow, risk_score) = action {
    allow == false
    action := data.actions.deny
}