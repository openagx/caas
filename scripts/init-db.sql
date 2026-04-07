-- Initialize databases for CAAS services
-- SpiceDB gets its own database (managed by spicedb-migrate)
CREATE DATABASE spicedb;

-- Entity service tables
CREATE TABLE IF NOT EXISTS entities (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    did TEXT UNIQUE,
    entity_type SMALLINT NOT NULL,
    display_name TEXT NOT NULL,
    lifecycle_state TEXT NOT NULL DEFAULT 'pending',
    metadata JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    suspended_at TIMESTAMPTZ,
    revoked_at TIMESTAMPTZ
);

CREATE INDEX idx_entities_type ON entities(entity_type);
CREATE INDEX idx_entities_state ON entities(lifecycle_state);
CREATE INDEX idx_entities_did ON entities(did) WHERE did IS NOT NULL;

CREATE TABLE IF NOT EXISTS entity_credentials (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    entity_id UUID NOT NULL REFERENCES entities(id) ON DELETE CASCADE,
    credential_type TEXT NOT NULL,
    credential_hash TEXT NOT NULL,
    expires_at TIMESTAMPTZ,
    revoked BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_entity_credentials_entity ON entity_credentials(entity_id);

-- Trust engine tables
CREATE TABLE IF NOT EXISTS trust_score_history (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    entity_id UUID NOT NULL REFERENCES entities(id) ON DELETE CASCADE,
    target_entity_id UUID REFERENCES entities(id) ON DELETE CASCADE,
    overall_score INTEGER NOT NULL,
    dimensions JSONB NOT NULL,
    calculation_reason TEXT NOT NULL,
    calculated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_trust_history_entity ON trust_score_history(entity_id);
CREATE INDEX idx_trust_history_time ON trust_score_history(calculated_at DESC);

CREATE TABLE IF NOT EXISTS trust_endorsements (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    endorser_id UUID NOT NULL REFERENCES entities(id) ON DELETE CASCADE,
    endorsed_id UUID NOT NULL REFERENCES entities(id) ON DELETE CASCADE,
    endorsement_type TEXT NOT NULL,
    weight REAL NOT NULL DEFAULT 1.0,
    evidence_hash TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    revoked_at TIMESTAMPTZ
);

CREATE INDEX idx_endorsements_endorser ON trust_endorsements(endorser_id);
CREATE INDEX idx_endorsements_endorsed ON trust_endorsements(endorsed_id);

-- Fraud detection tables
CREATE TABLE IF NOT EXISTS fraud_incidents (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    incident_type TEXT NOT NULL,
    severity TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'open',
    affected_entity_id UUID NOT NULL REFERENCES entities(id),
    detection_timestamp TIMESTAMPTZ NOT NULL,
    containment_timestamp TIMESTAMPTZ,
    resolution_timestamp TIMESTAMPTZ,
    detection_latency_ms INTEGER,
    evidence JSONB NOT NULL DEFAULT '{}',
    timeline JSONB NOT NULL DEFAULT '[]',
    assigned_to UUID REFERENCES entities(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_fraud_incidents_status ON fraud_incidents(status);
CREATE INDEX idx_fraud_incidents_entity ON fraud_incidents(affected_entity_id);
CREATE INDEX idx_fraud_incidents_severity ON fraud_incidents(severity);

CREATE TABLE IF NOT EXISTS fraud_rules (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    rule_name TEXT NOT NULL,
    rule_type TEXT NOT NULL,
    configuration JSONB NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Decision integrity tables
CREATE TABLE IF NOT EXISTS decision_workflows (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workflow_type TEXT NOT NULL,
    required_approvals INTEGER NOT NULL DEFAULT 2,
    total_reviewers INTEGER NOT NULL DEFAULT 3,
    blind_review BOOLEAN NOT NULL DEFAULT true,
    cool_off_hours INTEGER NOT NULL DEFAULT 168,
    status TEXT NOT NULL DEFAULT 'pending',
    subject_entity_id UUID NOT NULL REFERENCES entities(id),
    case_data JSONB NOT NULL,
    outcome TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    completed_at TIMESTAMPTZ
);

CREATE TABLE IF NOT EXISTS decision_votes (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workflow_id UUID NOT NULL REFERENCES decision_workflows(id) ON DELETE CASCADE,
    reviewer_id UUID NOT NULL REFERENCES entities(id),
    vote TEXT NOT NULL,
    reasoning TEXT,
    confidence REAL,
    time_spent_seconds INTEGER,
    blind_case_id TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS reviewer_integrity (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    reviewer_id UUID NOT NULL REFERENCES entities(id),
    integrity_score INTEGER NOT NULL DEFAULT 500,
    total_reviews INTEGER NOT NULL DEFAULT 0,
    approval_rate REAL,
    avg_time_per_review REAL,
    bias_flags INTEGER NOT NULL DEFAULT 0,
    last_bias_check TIMESTAMPTZ,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(reviewer_id)
);

-- Federation / sovereignty tables
CREATE TABLE IF NOT EXISTS sovereign_nodes (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL,
    did TEXT UNIQUE,
    jurisdiction TEXT NOT NULL,
    endpoint TEXT NOT NULL,
    public_key TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'active',
    metadata JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_sovereign_nodes_status ON sovereign_nodes(status);
CREATE INDEX idx_sovereign_nodes_jurisdiction ON sovereign_nodes(jurisdiction);

CREATE TABLE IF NOT EXISTS treaties (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    source_node_id UUID NOT NULL REFERENCES sovereign_nodes(id),
    target_node_id UUID NOT NULL REFERENCES sovereign_nodes(id),
    status TEXT NOT NULL DEFAULT 'proposed',
    trust_weight REAL NOT NULL DEFAULT 0.5,
    allowed_operations TEXT[] NOT NULL DEFAULT '{}',
    valid_from TIMESTAMPTZ NOT NULL DEFAULT now(),
    valid_until TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_treaties_source ON treaties(source_node_id);
CREATE INDEX idx_treaties_target ON treaties(target_node_id);
CREATE INDEX idx_treaties_status ON treaties(status);

CREATE TABLE IF NOT EXISTS authority_overrides (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workflow_id UUID NOT NULL REFERENCES decision_workflows(id),
    authority_entity_id UUID NOT NULL REFERENCES entities(id),
    tier INTEGER NOT NULL,
    justification TEXT NOT NULL,
    legal_reference TEXT,
    outcome TEXT NOT NULL,
    expires_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_authority_overrides_workflow ON authority_overrides(workflow_id);

CREATE TABLE IF NOT EXISTS verifiable_credentials (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    entity_id UUID NOT NULL REFERENCES entities(id),
    issuer_did TEXT NOT NULL,
    credential_type TEXT NOT NULL,
    claims JSONB NOT NULL DEFAULT '{}',
    proof_signature TEXT NOT NULL,
    issued_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at TIMESTAMPTZ,
    revoked BOOLEAN NOT NULL DEFAULT false
);

CREATE INDEX idx_vc_entity ON verifiable_credentials(entity_id);
CREATE INDEX idx_vc_type ON verifiable_credentials(credential_type);

-- Audit log (immutable)
CREATE TABLE IF NOT EXISTS audit_log (
    id BIGSERIAL PRIMARY KEY,
    event_type TEXT NOT NULL,
    entity_id UUID,
    actor_id UUID,
    action TEXT NOT NULL,
    resource_type TEXT,
    resource_id TEXT,
    detail JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_audit_log_entity ON audit_log(entity_id);
CREATE INDEX idx_audit_log_time ON audit_log(created_at DESC);
CREATE INDEX idx_audit_log_type ON audit_log(event_type);
