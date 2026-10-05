-- +goose Up
-- Lands in schema activity_svc. Table names match GORM's derivation from
-- the repo models (activity → activities, reconcileMarker → reconcile_markers).
CREATE TABLE activities (
    id                  UUID PRIMARY KEY,                 -- uuidv7
    tenant_id           UUID NOT NULL,                    -- bare uuid, no FK (P5)
    event_id            TEXT NOT NULL,                    -- tenant-supplied dedupe key; "sys:<event_id>" for internal
    event_type          TEXT NOT NULL,
    player_external_id  TEXT NULL,
    player_id           UUID NULL,                        -- resolved at ingest when the player exists
    properties          JSONB NOT NULL DEFAULT '{}',
    context             JSONB NOT NULL DEFAULT '{}',
    occurred_at         TIMESTAMPTZ NOT NULL,
    received_at         TIMESTAMPTZ NOT NULL,
    status              TEXT NOT NULL DEFAULT 'pending',  -- pending | decided | rejected
    decision_id         UUID NULL,
    outcome             TEXT NULL,
    reason              TEXT NULL,
    decided_at          TIMESTAMPTZ NULL,
    causation_depth     SMALLINT NOT NULL DEFAULT 0,
    source_event_id     TEXT NULL,                        -- envelope event_id of the source fact (internal activities)
    republish_count     INT NOT NULL DEFAULT 0,           -- stuck sweep re-publishes, capped
    last_republished_at TIMESTAMPTZ NULL,
    created_at          TIMESTAMPTZ NOT NULL,
    updated_at          TIMESTAMPTZ NOT NULL,
    CHECK (status IN ('pending', 'decided', 'rejected')),
    CHECK (player_id IS NOT NULL OR player_external_id IS NOT NULL)
);

-- Idempotent ingest: INSERT ... ON CONFLICT (tenant_id, event_id) DO NOTHING.
CREATE UNIQUE INDEX ux_activities_tenant_event ON activities (tenant_id, event_id);
-- Keyset list, newest first, and its filters.
CREATE INDEX ix_activities_tenant_created ON activities (tenant_id, created_at DESC, id DESC);
CREATE INDEX ix_activities_tenant_player_created ON activities (tenant_id, player_external_id, created_at DESC);
CREATE INDEX ix_activities_tenant_type_created ON activities (tenant_id, event_type, created_at DESC);
-- Stuck sweep: only pending rows are indexed.
CREATE INDEX ix_activities_pending ON activities (received_at) WHERE status = 'pending';

CREATE TABLE reconcile_markers (
    name     TEXT PRIMARY KEY,
    last_run TIMESTAMPTZ NOT NULL
);

-- +goose Down
DROP TABLE reconcile_markers;
DROP TABLE activities;
