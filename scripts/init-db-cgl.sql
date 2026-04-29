-- init-db-cgl.sql
-- AAGFE v2 — CGL PostgreSQL schema additions
-- Append to existing scripts/init-db.sql
-- All tables: soft delete, audit timestamps, Redpanda event on state change

-- ─── Extensions (idempotent) ──────────────────────────────────────────────
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE EXTENSION IF NOT EXISTS "pgcrypto";

-- ─── cgl_tasks ────────────────────────────────────────────────────────────
-- Owner: intent-registry (port 50060)
-- Every agent task must register here before any action is permitted.

CREATE TABLE IF NOT EXISTS cgl_tasks (
    id                  UUID        PRIMARY KEY DEFAULT uuid_generate_v4(),
    agent_entity_id     TEXT        NOT NULL,
    session_id          TEXT        NOT NULL,
    kernel_id           TEXT,                           -- SK kernel instance ID (nullable for AK)
    declared_goal       TEXT        NOT NULL,
    permitted_tools     TEXT[]      NOT NULL DEFAULT '{}',
    permitted_resources TEXT[]      NOT NULL DEFAULT '{}',
    max_turns           INT         NOT NULL DEFAULT 0, -- 0 = unlimited
    max_iterations      INT         NOT NULL DEFAULT 0,
    current_turn        INT         NOT NULL DEFAULT 0,
    current_iteration   INT         NOT NULL DEFAULT 0,
    parent_task_id      UUID        REFERENCES cgl_tasks(id),
    state               TEXT        NOT NULL DEFAULT 'ACTIVE'
                            CHECK (state IN ('ACTIVE','SUSPENDED','COMPLETED','FAILED','REVOKED')),
    intent_hash         TEXT        NOT NULL,           -- SHA-256 of goal+tools+resources
    runtime             TEXT,                           -- sk_dotnet, ak_python, langgraph, etc.
    metadata            JSONB       NOT NULL DEFAULT '{}',
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at          TIMESTAMPTZ
);

CREATE INDEX idx_cgl_tasks_agent     ON cgl_tasks (agent_entity_id);
CREATE INDEX idx_cgl_tasks_session   ON cgl_tasks (session_id);
CREATE INDEX idx_cgl_tasks_state     ON cgl_tasks (state) WHERE deleted_at IS NULL;
CREATE INDEX idx_cgl_tasks_parent    ON cgl_tasks (parent_task_id) WHERE parent_task_id IS NOT NULL;

-- ─── cgl_actions ─────────────────────────────────────────────────────────
-- Owner: behavioral-gate (port 50059)
-- Every behavioral-gate check + outcome per task turn.

CREATE TABLE IF NOT EXISTS cgl_actions (
    id                  UUID        PRIMARY KEY DEFAULT uuid_generate_v4(),
    agent_entity_id     TEXT        NOT NULL,
    task_id             UUID        NOT NULL REFERENCES cgl_tasks(id),
    turn_number         INT         NOT NULL,
    iteration_number    INT         NOT NULL DEFAULT 0,
    tool_name           TEXT,
    plugin_name         TEXT,
    resource            TEXT,
    parameters_json     JSONB,
    verdict             TEXT        NOT NULL
                            CHECK (verdict IN ('ALLOW','WARN','STRONG_WARN','BLOCK')),
    drift_score         INT         NOT NULL DEFAULT 0,
    active_signals      TEXT[]      NOT NULL DEFAULT '{}',
    review_token        TEXT,
    remediation         TEXT,
    outcome             TEXT        CHECK (outcome IN ('COMPLETED','FAILED','TIMEOUT','BLOCKED','REVERSED','ESCALATED')),
    outcome_notes       TEXT,
    result_summary      TEXT,
    duration_ms         BIGINT,
    runtime             TEXT,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_cgl_actions_task       ON cgl_actions (task_id);
CREATE INDEX idx_cgl_actions_agent      ON cgl_actions (agent_entity_id);
CREATE INDEX idx_cgl_actions_verdict    ON cgl_actions (verdict);
CREATE INDEX idx_cgl_actions_turn       ON cgl_actions (task_id, turn_number);

-- ─── cgl_reasoning ───────────────────────────────────────────────────────
-- Owner: cot-auditor (port 50061)
-- CoT submissions with anomaly scores and semantic diff tags.
-- NOTE: cot_text may contain sensitive data — apply column-level encryption
--       if agent handles PII. See OQ-03 in PRD.

CREATE TABLE IF NOT EXISTS cgl_reasoning (
    id                  UUID        PRIMARY KEY DEFAULT uuid_generate_v4(),
    agent_entity_id     TEXT        NOT NULL,
    task_id             UUID        NOT NULL REFERENCES cgl_tasks(id),
    turn_number         INT         NOT NULL,
    cot_text            TEXT,                           -- Nullable: absent for closed-box LLMs
    cot_hash            TEXT,                           -- SHA-256 for dedup
    action_taken        TEXT,
    anomaly_score       FLOAT       NOT NULL DEFAULT 0.0 CHECK (anomaly_score BETWEEN 0 AND 1),
    anomaly_tags        TEXT[]      NOT NULL DEFAULT '{}',
    semantic_embedding  VECTOR(1536),                   -- pgvector; requires CREATE EXTENSION vector
    ak_guardrail_verdict TEXT,
    ak_flagged_keywords TEXT[]      NOT NULL DEFAULT '{}',
    runtime             TEXT,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_cgl_reasoning_task      ON cgl_reasoning (task_id);
CREATE INDEX idx_cgl_reasoning_anomaly   ON cgl_reasoning (anomaly_score DESC);
CREATE INDEX idx_cgl_reasoning_turn      ON cgl_reasoning (task_id, turn_number);

-- ─── cgl_planner_iterations ──────────────────────────────────────────────
-- Owner: planner-iteration (port 50068)
-- NEW: First-class CGL concept from IAutoFunctionInvocationFilter.
-- Each row = one LLM "think step" in a planner loop.

CREATE TABLE IF NOT EXISTS cgl_planner_iterations (
    id                         UUID        PRIMARY KEY DEFAULT uuid_generate_v4(),
    iteration_event_id         TEXT        UNIQUE NOT NULL,
    agent_entity_id            TEXT        NOT NULL,
    task_id                    UUID        NOT NULL REFERENCES cgl_tasks(id),
    turn_number                INT         NOT NULL,
    iteration_number           INT         NOT NULL,
    chat_history_json          JSONB,                  -- Snapshot of chat history at this iteration
    requested_function         TEXT,
    requested_plugin           TEXT,
    requested_args_json        JSONB,
    function_count             INT         NOT NULL DEFAULT 0,
    is_final_iteration         BOOLEAN     NOT NULL DEFAULT FALSE,
    reasoning_anomaly_score    FLOAT       NOT NULL DEFAULT 0.0 CHECK (reasoning_anomaly_score BETWEEN 0 AND 1),
    drift_score                INT         NOT NULL DEFAULT 0,
    signals                    JSONB       NOT NULL DEFAULT '[]',
    directive                  TEXT        NOT NULL DEFAULT 'CONTINUE'
                                    CHECK (directive IN ('CONTINUE','INJECT','REVIEW','TERMINATE')),
    constraint_fragment        TEXT,
    terminate_planner          BOOLEAN     NOT NULL DEFAULT FALSE,
    runtime                    TEXT,
    session_id                 TEXT,
    created_at                 TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_cgl_planner_iter_task      ON cgl_planner_iterations (task_id);
CREATE INDEX idx_cgl_planner_iter_agent     ON cgl_planner_iterations (agent_entity_id);
CREATE INDEX idx_cgl_planner_iter_anomaly   ON cgl_planner_iterations (reasoning_anomaly_score DESC);
CREATE INDEX idx_cgl_planner_iter_directive ON cgl_planner_iterations (directive) WHERE directive != 'CONTINUE';

-- ─── cgl_injections ──────────────────────────────────────────────────────
-- Owner: constraint-injector (port 50062)

CREATE TABLE IF NOT EXISTS cgl_injections (
    id              UUID        PRIMARY KEY DEFAULT uuid_generate_v4(),
    agent_entity_id TEXT        NOT NULL,
    task_id         UUID        NOT NULL REFERENCES cgl_tasks(id),
    turn_number     INT         NOT NULL,
    fragment_hash   TEXT        NOT NULL,
    fragment_text   TEXT        NOT NULL,
    reason          TEXT        NOT NULL DEFAULT 'SCHEDULED'
                        CHECK (reason IN ('SCHEDULED','DRIFT_SIGNAL','MANUAL')),
    acknowledged    BOOLEAN     NOT NULL DEFAULT FALSE,
    position        TEXT        NOT NULL DEFAULT 'PREPEND'
                        CHECK (position IN ('PREPEND','APPEND','AFTER_SYSTEM','AFTER_ROLE')),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_cgl_injections_task    ON cgl_injections (task_id);
CREATE INDEX idx_cgl_injections_turn    ON cgl_injections (task_id, turn_number);

-- ─── cgl_drift_scores ────────────────────────────────────────────────────
-- Owner: drift-scorer (port 50063)

CREATE TABLE IF NOT EXISTS cgl_drift_scores (
    id                  UUID        PRIMARY KEY DEFAULT uuid_generate_v4(),
    agent_entity_id     TEXT        NOT NULL,
    task_id             UUID        NOT NULL REFERENCES cgl_tasks(id),
    drift_score         INT         NOT NULL CHECK (drift_score BETWEEN 0 AND 1000),
    dominant_signal     TEXT,
    dimensions          JSONB       NOT NULL DEFAULT '{}',
    turn_number         INT         NOT NULL,
    iteration_number    INT         NOT NULL DEFAULT 0,
    captured_at         TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_cgl_drift_task         ON cgl_drift_scores (task_id);
CREATE INDEX idx_cgl_drift_agent        ON cgl_drift_scores (agent_entity_id);
CREATE INDEX idx_cgl_drift_score_desc   ON cgl_drift_scores (drift_score DESC);
CREATE INDEX idx_cgl_drift_captured     ON cgl_drift_scores (captured_at DESC);

-- ─── cgl_boundaries ──────────────────────────────────────────────────────
-- Owner: memory-isolator (port 50064)

CREATE TABLE IF NOT EXISTS cgl_boundaries (
    id              UUID        PRIMARY KEY DEFAULT uuid_generate_v4(),
    agent_entity_id TEXT        NOT NULL,
    task_id         UUID        NOT NULL REFERENCES cgl_tasks(id),
    boundary_type   TEXT        NOT NULL
                        CHECK (boundary_type IN ('TASK_COMPLETE','SUSPENSION','MANUAL','DRIFT_TRIGGER')),
    enforced        BOOLEAN     NOT NULL DEFAULT FALSE,
    keys_cleared    INT         NOT NULL DEFAULT 0,
    bleed_keys_found INT        NOT NULL DEFAULT 0,
    session_store   TEXT,
    dry_run         BOOLEAN     NOT NULL DEFAULT FALSE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    enforced_at     TIMESTAMPTZ
);

CREATE INDEX idx_cgl_boundaries_task    ON cgl_boundaries (task_id);
CREATE INDEX idx_cgl_boundaries_type    ON cgl_boundaries (boundary_type);

-- ─── cgl_session_keys ────────────────────────────────────────────────────
-- Owner: memory-isolator / ak-reinforcement / sk-reinforcement
-- Task-scoped key registry — provides enumeration capability that
-- AK session API and SK VectorStore API cannot natively provide.

CREATE TABLE IF NOT EXISTS cgl_session_keys (
    id              UUID        PRIMARY KEY DEFAULT uuid_generate_v4(),
    agent_entity_id TEXT        NOT NULL,
    task_id         UUID        NOT NULL REFERENCES cgl_tasks(id),
    session_id      TEXT        NOT NULL,
    kernel_id       TEXT,
    collection      TEXT,                               -- SK VectorStore collection
    key             TEXT        NOT NULL,
    session_store   TEXT        NOT NULL DEFAULT 'IN_MEMORY',
    sensitivity     TEXT        NOT NULL DEFAULT 'NORMAL'
                        CHECK (sensitivity IN ('NORMAL','HIGH')),
    bleed_risk      BOOLEAN     NOT NULL DEFAULT FALSE,
    origin_task_id  UUID        REFERENCES cgl_tasks(id),
    purged          BOOLEAN     NOT NULL DEFAULT FALSE,
    purged_at       TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX idx_cgl_session_keys_unique
    ON cgl_session_keys (task_id, session_id, key) WHERE purged = FALSE;
CREATE INDEX idx_cgl_session_keys_task      ON cgl_session_keys (task_id);
CREATE INDEX idx_cgl_session_keys_bleed     ON cgl_session_keys (bleed_risk) WHERE bleed_risk = TRUE;
CREATE INDEX idx_cgl_session_keys_unpurged  ON cgl_session_keys (task_id, session_id) WHERE purged = FALSE;

-- ─── cgl_sycophancy ──────────────────────────────────────────────────────
-- Owner: sycophancy-guard (port 50065)

CREATE TABLE IF NOT EXISTS cgl_sycophancy (
    id              UUID        PRIMARY KEY DEFAULT uuid_generate_v4(),
    agent_entity_id TEXT        NOT NULL,
    task_id         UUID        NOT NULL REFERENCES cgl_tasks(id),
    turn_number     INT         NOT NULL,
    pattern         TEXT        NOT NULL
                        CHECK (pattern IN (
                            'RETRACTED_REFUSAL','SOFTENED_CONSTRAINT',
                            'SCOPE_EXPANSION','AUTHORITY_OVERRIDE','PERSISTENCE_YIELD'
                        )),
    confidence      FLOAT       NOT NULL CHECK (confidence BETWEEN 0 AND 1),
    detail          TEXT,
    user_message    TEXT,
    agent_response  TEXT,
    prior_refusal   TEXT,
    review_token    TEXT,
    escalated       BOOLEAN     NOT NULL DEFAULT FALSE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_cgl_sycophancy_task        ON cgl_sycophancy (task_id);
CREATE INDEX idx_cgl_sycophancy_pattern     ON cgl_sycophancy (pattern);
CREATE INDEX idx_cgl_sycophancy_confidence  ON cgl_sycophancy (confidence DESC);

-- ─── Trigger: updated_at auto-maintenance ────────────────────────────────

CREATE OR REPLACE FUNCTION set_updated_at()
RETURNS TRIGGER AS $$
BEGIN NEW.updated_at = NOW(); RETURN NEW; END;
$$ LANGUAGE plpgsql;

DO $$
DECLARE t TEXT;
BEGIN
    FOREACH t IN ARRAY ARRAY['cgl_tasks','cgl_actions'] LOOP
        EXECUTE format(
            'CREATE TRIGGER trg_%s_updated_at
             BEFORE UPDATE ON %s
             FOR EACH ROW EXECUTE FUNCTION set_updated_at()',
            t, t);
    END LOOP;
END $$;

-- ─── Comments ────────────────────────────────────────────────────────────

COMMENT ON TABLE cgl_tasks              IS 'intent-registry: task records with declared goals and permitted scopes';
COMMENT ON TABLE cgl_actions            IS 'behavioral-gate: every check and outcome per task turn';
COMMENT ON TABLE cgl_reasoning          IS 'cot-auditor: CoT submissions with anomaly scores';
COMMENT ON TABLE cgl_planner_iterations IS 'planner-iteration: IAutoFunctionInvocationFilter events — reasoning before tool selection';
COMMENT ON TABLE cgl_injections         IS 'constraint-injector: injection history per agent-task pair';
COMMENT ON TABLE cgl_drift_scores       IS 'drift-scorer: rolling score snapshots';
COMMENT ON TABLE cgl_boundaries         IS 'memory-isolator: boundary records with enforcement state';
COMMENT ON TABLE cgl_session_keys       IS 'memory-isolator/adapters: task-scoped key registry for AK/SK session enumeration';
COMMENT ON TABLE cgl_sycophancy         IS 'sycophancy-guard: detected patterns with confidence and review tokens';

-- ─── cgl_autogen_actors ───────────────────────────────────────────────────
-- Owner: autogen-reinforcement (port 50069)
-- Maps AutoGen agent actor IDs to AAGFE entity IDs.

CREATE TABLE IF NOT EXISTS cgl_autogen_actors (
    id                  UUID        PRIMARY KEY DEFAULT uuid_generate_v4(),
    entity_id           TEXT        NOT NULL UNIQUE,
    autogen_agent_id    TEXT        NOT NULL,
    autogen_agent_type  TEXT        NOT NULL,
    task_id             UUID        REFERENCES cgl_tasks(id),
    entity_type         TEXT        NOT NULL DEFAULT 'AI_AGENT'
                            CHECK (entity_type IN ('AI_AGENT','AUTONOMOUS_SYSTEM','SERVICE_API')),
    trust_token         TEXT,
    deregistered_at     TIMESTAMPTZ,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_cgl_actors_task        ON cgl_autogen_actors (task_id);
CREATE INDEX idx_cgl_actors_autogen_id  ON cgl_autogen_actors (autogen_agent_id);

-- ─── cgl_autogen_messages ─────────────────────────────────────────────────
-- Owner: autogen-reinforcement (port 50069)
-- Every intercepted AutoGen inter-agent message.

CREATE TABLE IF NOT EXISTS cgl_autogen_messages (
    id                  UUID        PRIMARY KEY DEFAULT uuid_generate_v4(),
    sender_entity_id    TEXT        NOT NULL,
    receiver_entity_id  TEXT        NOT NULL,
    task_id             UUID        NOT NULL REFERENCES cgl_tasks(id),
    round_number        INT         NOT NULL,
    message_type        TEXT        NOT NULL,
    message_content     TEXT,
    verdict             TEXT        NOT NULL CHECK (verdict IN ('ALLOW','WARN','STRONG_WARN','BLOCK')),
    drift_score         INT         NOT NULL DEFAULT 0,
    lateral_risk_score  FLOAT       NOT NULL DEFAULT 0.0,
    is_handoff          BOOLEAN     NOT NULL DEFAULT FALSE,
    is_cross_task       BOOLEAN     NOT NULL DEFAULT FALSE,
    relay_required      BOOLEAN     NOT NULL DEFAULT FALSE,
    review_token        TEXT,
    signals             JSONB       NOT NULL DEFAULT '[]',
    phase               TEXT        NOT NULL CHECK (phase IN ('PRE','POST')),
    session_id          TEXT,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_cgl_autogen_msgs_task      ON cgl_autogen_messages (task_id);
CREATE INDEX idx_cgl_autogen_msgs_verdict   ON cgl_autogen_messages (verdict);
CREATE INDEX idx_cgl_autogen_msgs_handoff   ON cgl_autogen_messages (is_handoff) WHERE is_handoff = TRUE;
CREATE INDEX idx_cgl_autogen_msgs_round     ON cgl_autogen_messages (task_id, round_number);

-- ─── cgl_state_boundaries ────────────────────────────────────────────────
-- Owner: autogen-reinforcement / memory-isolator (port 50069 / 50064)
-- Full state snapshot boundaries for AutoGen save_state / load_state.
-- AutoGen provides complete state — no key registry needed.

CREATE TABLE IF NOT EXISTS cgl_state_boundaries (
    id                  UUID        PRIMARY KEY DEFAULT uuid_generate_v4(),
    agent_entity_id     TEXT        NOT NULL,
    task_id             UUID        NOT NULL REFERENCES cgl_tasks(id),
    session_id          TEXT,
    state_hash          TEXT        NOT NULL,
    boundary_type       TEXT        NOT NULL CHECK (boundary_type IN ('SAVE','LOAD')),
    task_ending         BOOLEAN     NOT NULL DEFAULT FALSE,
    bleed_detected      BOOLEAN     NOT NULL DEFAULT FALSE,
    bleed_detail        TEXT,
    redacted_keys       TEXT[]      NOT NULL DEFAULT '{}',
    removed_keys        TEXT[]      NOT NULL DEFAULT '{}',
    origin_task_id      UUID        REFERENCES cgl_tasks(id),
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_cgl_state_boundaries_task  ON cgl_state_boundaries (task_id);
CREATE INDEX idx_cgl_state_boundaries_bleed ON cgl_state_boundaries (bleed_detected) WHERE bleed_detected = TRUE;

COMMENT ON TABLE cgl_autogen_actors    IS 'autogen-reinforcement: AutoGen actor → AAGFE entity mapping';
COMMENT ON TABLE cgl_autogen_messages  IS 'autogen-reinforcement: all intercepted inter-agent messages';
COMMENT ON TABLE cgl_state_boundaries  IS 'autogen-reinforcement: save_state/load_state boundaries for memory isolation';
