# OPA Policy Tests
# Tests for two-layer authorization: SpiceDB + OPA sidecar

package authz

import future.keywords.if

# ============================================================================
# Test Suite: Allowed SpiceDB + Clean Behavior → ALLOW
# ============================================================================

test_allow_spicedb_pass_clean_behavior {
    # Input: SpiceDB allows, clean behavior, good trust, MFA
    input := {
        "spicedb": {"allowed": true, "relationships": ["viewer"]},
        "subject": {"auth_strength": "mfa", "trust_score": 0.8},
        "resource": {"sensitivity": "low"},
        "context": {"device_trust": 0.9, "ip_reputation": 0.2},
        "behavior": {"anomaly_score": 0.1, "velocity_ops_5m": 20, "velocity_ops_1h": 100, "drift_score": 0.1}
    }
    
    allow == true
    not override_deny
    spicedb_pass
    abac_pass
    behavioral_pass
    drift_pass
}

# ============================================================================
# Test Suite: Allowed SpiceDB + High Anomaly → DENY
# ============================================================================

test_deny_high_anomaly {
    input := {
        "spicedb": {"allowed": true, "relationships": ["viewer"]},
        "subject": {"auth_strength": "mfa", "trust_score": 0.8},
        "resource": {"sensitivity": "low"},
        "context": {"device_trust": 0.9, "ip_reputation": 0.2},
        "behavior": {"anomaly_score": 0.95, "velocity_ops_5m": 20, "velocity_ops_1h": 100, "drift_score": 0.1}
    }
    
    allow == false
    override_deny
}

# ============================================================================
# Test Suite: Low Trust → DENY
# ============================================================================

test_deny_low_trust {
    input := {
        "spicedb": {"allowed": true, "relationships": ["viewer"]},
        "subject": {"auth_strength": "mfa", "trust_score": 0.2},
        "resource": {"sensitivity": "low"},
        "context": {"device_trust": 0.6, "ip_reputation": 0.3},
        "behavior": {"anomaly_score": 0.1, "velocity_ops_5m": 20, "velocity_ops_1h": 100, "drift_score": 0.1}
    }
    
    allow == false
    override_deny
}

# ============================================================================
# Test Suite: Drift Spike → DENY
# ============================================================================

test_deny_drift_spike {
    input := {
        "spicedb": {"allowed": true, "relationships": ["viewer"]},
        "subject": {"auth_strength": "mfa", "trust_score": 0.7},
        "resource": {"sensitivity": "low"},
        "context": {"device_trust": 0.9, "ip_reputation": 0.2},
        "behavior": {"anomaly_score": 0.1, "velocity_ops_5m": 20, "velocity_ops_1h": 100, "drift_score": 0.8}
    }
    
    allow == false
    not override_deny
    spicedb_pass
    not drift_pass
}

# ============================================================================
# Test Suite: MFA Missing → DENY
# ============================================================================

test_deny_no_mfa {
    input := {
        "spicedb": {"allowed": true, "relationships": ["viewer"]},
        "subject": {"auth_strength": "password", "trust_score": 0.8},
        "resource": {"sensitivity": "high"},
        "context": {"device_trust": 0.6, "ip_reputation": 0.3},
        "behavior": {"anomaly_score": 0.1, "velocity_ops_5m": 20, "velocity_ops_1h": 100, "drift_score": 0.1}
    }
    
    allow == false
    not override_deny
    spicedb_pass
    not abac_pass
}

# ============================================================================
# Test Suite: High Sensitivity + Elevated Anomaly → DENY
# ============================================================================

test_deny_high_sensitivity_anomaly {
    input := {
        "spicedb": {"allowed": true, "relationships": ["editor"]},
        "subject": {"auth_strength": "mfa", "trust_score": 0.8},
        "resource": {"sensitivity": "high"},
        "context": {"device_trust": 0.9, "ip_reputation": 0.2},
        "behavior": {"anomaly_score": 0.6, "velocity_ops_5m": 20, "velocity_ops_1h": 100, "drift_score": 0.1}
    }
    
    allow == false
    not override_deny
    spicedb_pass
    not behavioral_pass
}

# ============================================================================
# Test Suite: Low Device Trust → DENY
# ============================================================================

test_deny_low_device_trust {
    input := {
        "spicedb": {"allowed": true, "relationships": ["viewer"]},
        "subject": {"auth_strength": "mfa", "trust_score": 0.8},
        "resource": {"sensitivity": "low"},
        "context": {"device_trust": 0.2, "ip_reputation": 0.3},
        "behavior": {"anomaly_score": 0.1, "velocity_ops_5m": 20, "velocity_ops_1h": 100, "drift_score": 0.1}
    }
    
    allow == false
    not override_deny
    spicedb_pass
    not abac_pass
}

# ============================================================================
# Test Suite: Bad IP Reputation → DENY
# ============================================================================

test_deny_bad_ip_reputation {
    input := {
        "spicedb": {"allowed": true, "relationships": ["viewer"]},
        "subject": {"auth_strength": "mfa", "trust_score": 0.8},
        "resource": {"sensitivity": "low"},
        "context": {"device_trust": 0.9, "ip_reputation": 0.9},
        "behavior": {"anomaly_score": 0.1, "velocity_ops_5m": 20, "velocity_ops_1h": 100, "drift_score": 0.1}
    }
    
    allow == false
    not override_deny
    spicedb_pass
    not abac_pass
}

# ============================================================================
# Test Suite: SpiceDB Failed → DENY (hard requirement)
# ============================================================================

test_deny_spicedb_failed {
    input := {
        "spicedb": {"allowed": false, "relationships": []},
        "subject": {"auth_strength": "mfa", "trust_score": 0.8},
        "resource": {"sensitivity": "low"},
        "context": {"device_trust": 0.9, "ip_reputation": 0.2},
        "behavior": {"anomaly_score": 0.1, "velocity_ops_5m": 20, "velocity_ops_1h": 100, "drift_score": 0.1}
    }
    
    allow == false
    not spicedb_pass
}

# ============================================================================
# Test Suite: Velocity Spike → DENY
# ============================================================================

test_deny_velocity_spike {
    input := {
        "spicedb": {"allowed": true, "relationships": ["viewer"]},
        "subject": {"auth_strength": "mfa", "trust_score": 0.8},
        "resource": {"sensitivity": "low"},
        "context": {"device_trust": 0.9, "ip_reputation": 0.2},
        "behavior": {"anomaly_score": 0.1, "velocity_ops_5m": 200, "velocity_ops_1h": 500, "drift_score": 0.1}
    }
    
    allow == false
    not override_deny
    spicedb_pass
    not behavioral_pass
}

# ============================================================================
# Test Suite: High sensitivity with high device trust still valid
# ============================================================================

test_allow_high_sensitivity_mfa_device {
    input := {
        "spicedb": {"allowed": true, "relationships": ["editor"]},
        "subject": {"auth_strength": "mfa", "trust_score": 0.9},
        "resource": {"sensitivity": "high"},
        "context": {"device_trust": 0.95, "ip_reputation": 0.1},
        "behavior": {"anomaly_score": 0.2, "velocity_ops_5m": 30, "velocity_ops_1h": 150, "drift_score": 0.15}
    }
    
    allow == true
    not override_deny
    spicedb_pass
    abac_pass
    behavioral_pass
    drift_pass
}