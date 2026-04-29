# AAGFE OWASP Agentic Top 10 Mapping

## 10 OWASP Agentic AI Risks with Deterministic Enforcement

| # | OWASP Risk | AAGFE Policy | Enforcement Latency |
|---|-----------|---------------|---------------------|
| ASI01 | Goal Hijack | behavioral_gate: GOAL_DEVIATION | <1ms |
| ASI02 | Tool Abuse | abac: tool_whitelist + fraud-pipeline | <1ms |
| ASI03 | Privilege Escalation | spicedb + opa: trust gate | <1ms |
| ASI04 | Supply Chain | drift_scorer: dependency anomaly | <1ms |
| ASI05 | Memory Poisoning | cot-auditor: context contamination | <1ms |
| ASI06 | Planning Manipulation | planner-iteration: scope_creep | <1ms |
| ASI07 | Multi-Agent Trust | boundary-enforcer: trust_graph | <1ms |
| ASI08 | Over-reliance | sycophancy-guard: retraction detection | <1ms |
| ASI09 | Human Trust Exploit | decision-service: M-of-N hold | <10ms |
| ASI10 | Rogue Agent | intent-registry + lifecycle: SUSPENDED | <1ms |

## AAGFE Countermeasures

### ASI01 — Goal Hijack
```rego
# behavioral_gate.rego
drift_pass {
    input.behavior.goal_deviation < 0.3
}
```

### ASI02 — Tool Abuse  
```rego
# abac.rego
abac_pass {
    input.tool.name in data.allowed_tools
    input.tool.risk_score < data.tool_risk_threshold
}
```

### ASI03 — Privilege Escalation
```rego
# policy.rego
override_deny {
    input.subject.trust_score < data.thresholds.trust_minimum
}
```

### ASI04 — Supply Chain
```rego
# drift_scorer.rego
drift_pass {
    input.behavior.dependency_anomaly < 0.5
}
```

### ASI05 — Memory Poisoning
```rego
# behavioral_gate.rego
behavioral_pass {
    input.behavior.context_contamination < 0.4
}
```

### ASI06 — Planning Manipulation
```rego
# drift_scorer.rego
drift_pass {
    input.planner.scope_creep < 0.3
}
```

### ASI07 — Multi-Agent Trust
```rego
# policy.rego - agent-to-agent verification
spicedb_pass {
    input.agent.trust_score > data.thresholds.agent_trust_min
}
```

### ASI08 — Over-reliance (Sycophany)
```rego
# sycophancy-guard.rego
sycophancy_detected {
    input.agent.retracted_refusal == true
    input.user.pressure_score > 0.7
}
```

### ASI09 — Human Trust Exploit
```rego
# decision.rego - escalate to M-of-N
recommended_action == "step_up_auth" {
    input.risk_score >= 0.5
}
```

### ASI10 — Rogue Agent
```rego
# policy.rego - immediate suspension
override_deny {
    input.entity.lifecycle == "rogue"
}
```

## Latency Guarantees

All policy evaluations run through OPA at <1ms p99 via:
- In-memory policy bundle
- Pre-compiled Rego 
- No external calls in hot path
- Redis cache for threshold data