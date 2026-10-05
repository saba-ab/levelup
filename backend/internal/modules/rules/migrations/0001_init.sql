-- +goose Up
-- Schema rules_svc (doc 06 §11.11). Table names match GORM's derivation from
-- the unexported models in internal/repo. tenant_id, player_id, program_id
-- are bare uuids: no foreign keys leave this schema (P5).

CREATE TABLE rules (
    id                 UUID PRIMARY KEY,                  -- uuidv7: creation order = tiebreak order
    tenant_id          UUID NOT NULL,
    slug               TEXT NOT NULL,
    name               TEXT NOT NULL,
    description        TEXT NOT NULL DEFAULT '',
    trigger_event      TEXT NOT NULL,
    program_id         UUID NULL,                         -- NULL = tenant-wide
    priority           INT  NOT NULL DEFAULT 0,
    status             TEXT NOT NULL DEFAULT 'draft'
                       CHECK (status IN ('draft', 'active', 'inactive', 'archived')),
    current_version_id UUID NULL,                         -- the ONE live version (R63)
    created_at         TIMESTAMPTZ NOT NULL,
    updated_at         TIMESTAMPTZ NOT NULL,
    deleted_at         TIMESTAMPTZ NULL
);
CREATE UNIQUE INDEX ux_rules_tenant_slug ON rules (tenant_id, slug) WHERE deleted_at IS NULL;
CREATE INDEX ix_rules_tenant_created ON rules (tenant_id, created_at DESC, id DESC);
-- hot path: live rules of a tenant for a trigger
CREATE INDEX ix_rules_live ON rules (tenant_id, trigger_event)
    WHERE status = 'active' AND deleted_at IS NULL AND current_version_id IS NOT NULL;

CREATE TABLE rule_versions (
    id           UUID PRIMARY KEY,
    rule_id      UUID NOT NULL REFERENCES rules (id) ON DELETE CASCADE,  -- same-schema FK
    tenant_id    UUID NOT NULL,
    version      INT  NOT NULL CHECK (version > 0),
    conditions   JSONB NOT NULL DEFAULT 'null',
    actions      JSONB NOT NULL,
    limits       JSONB NOT NULL DEFAULT 'null',
    published_at TIMESTAMPTZ NULL,                         -- set once; immutable afterwards
    created_by   TEXT NOT NULL DEFAULT '',
    created_at   TIMESTAMPTZ NOT NULL,
    UNIQUE (rule_id, version)
);
CREATE INDEX ix_rule_versions_tenant ON rule_versions (tenant_id);

ALTER TABLE rules ADD CONSTRAINT fk_rules_current_version
    FOREIGN KEY (current_version_id) REFERENCES rule_versions (id) DEFERRABLE INITIALLY DEFERRED;

CREATE TABLE ruleset_generations (
    tenant_id  UUID PRIMARY KEY,
    generation BIGINT NOT NULL
);

CREATE TABLE decisions (
    id                 UUID PRIMARY KEY,                  -- uuidv5(activity_id)
    tenant_id          UUID NOT NULL,
    activity_id        UUID NOT NULL UNIQUE,              -- the idempotency guard
    event_id           TEXT NOT NULL DEFAULT '',
    player_id          UUID NULL,
    player_external_id TEXT NOT NULL DEFAULT '',
    event_type         TEXT NOT NULL,
    outcome            TEXT NOT NULL,                     -- matched | no_match | rejected | limit_reached
    reason             TEXT NOT NULL DEFAULT '',
    ruleset_generation BIGINT NOT NULL DEFAULT 0,
    causation_depth    INT NOT NULL DEFAULT 0,
    occurred_at        TIMESTAMPTZ NOT NULL,
    evaluated_at       TIMESTAMPTZ NOT NULL,
    duration_us        BIGINT NOT NULL DEFAULT 0
);
CREATE INDEX ix_decisions_tenant_time ON decisions (tenant_id, evaluated_at DESC, id DESC);
CREATE INDEX ix_decisions_tenant_player ON decisions (tenant_id, player_id, evaluated_at DESC);

CREATE TABLE rule_executions (
    id                UUID PRIMARY KEY,                   -- uuidv5(decision_id, rule_version_id)
    tenant_id         UUID NOT NULL,
    decision_id       UUID NOT NULL REFERENCES decisions (id) ON DELETE CASCADE,
    rule_id           UUID NOT NULL,
    rule_version_id   UUID NOT NULL,
    player_id         UUID NULL,
    status            TEXT NOT NULL,                      -- fired | not_matched | limited | out_of_scope | invalid
    matched           BOOLEAN NOT NULL,
    condition_results JSONB NOT NULL DEFAULT '[]',        -- per-condition trace ("why did the player get X")
    effects_count     INT NOT NULL DEFAULT 0,
    created_at        TIMESTAMPTZ NOT NULL,
    UNIQUE (decision_id, rule_version_id)
);
CREATE INDEX ix_rule_executions_rule ON rule_executions (tenant_id, rule_id, created_at DESC);

CREATE TABLE rule_execution_effects (
    id              UUID PRIMARY KEY,                     -- effect_id (effect.Source.ID)
    tenant_id       UUID NOT NULL,
    decision_id     UUID NOT NULL REFERENCES decisions (id) ON DELETE CASCADE,
    execution_id    UUID NOT NULL REFERENCES rule_executions (id) ON DELETE CASCADE,
    rule_id         UUID NOT NULL,
    rule_version_id UUID NOT NULL,
    player_id       UUID NULL,
    action_index    INT NOT NULL,
    idempotency_key TEXT NOT NULL,                        -- uuidv5(activity|rule_version|action_index)
    type            TEXT NOT NULL,
    params          JSONB NOT NULL DEFAULT '{}',
    target          TEXT NOT NULL,                        -- job topic
    command         JSONB NOT NULL,                       -- the published command, for re-publish sweeps
    status          TEXT NOT NULL DEFAULT 'requested'
                    CHECK (status IN ('requested', 'applied', 'rejected')),
    reason          TEXT NOT NULL DEFAULT '',
    attempts        INT NOT NULL DEFAULT 0,
    requested_at    TIMESTAMPTZ NOT NULL,
    last_attempt_at TIMESTAMPTZ NULL,
    settled_at      TIMESTAMPTZ NULL
);
CREATE UNIQUE INDEX ux_effects_tenant_key ON rule_execution_effects (tenant_id, idempotency_key);
CREATE INDEX ix_effects_decision ON rule_execution_effects (decision_id, action_index);
CREATE INDEX ix_effects_pending ON rule_execution_effects (requested_at) WHERE status = 'requested';

CREATE TABLE rule_player_counters (
    tenant_id     UUID NOT NULL,
    rule_id       UUID NOT NULL,
    player_id     UUID NOT NULL,
    window_key    TEXT NOT NULL,                          -- 'lifetime' | 'cooldown' | 'd:2026-10-05' | 'w:2026-W40'
    count         BIGINT NOT NULL,
    last_fired_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (tenant_id, rule_id, player_id, window_key)
);

-- +goose Down
DROP TABLE rule_player_counters;
DROP TABLE rule_execution_effects;
DROP TABLE rule_executions;
DROP TABLE decisions;
DROP TABLE ruleset_generations;
ALTER TABLE rules DROP CONSTRAINT fk_rules_current_version;
DROP TABLE rule_versions;
DROP TABLE rules;
