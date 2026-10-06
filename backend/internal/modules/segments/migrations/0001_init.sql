-- +goose Up
-- Schema segments_svc. tenant_id and player_id are bare uuids: no foreign
-- keys into other modules' schemas (P5).

CREATE TABLE segments (
    id                  UUID PRIMARY KEY,
    tenant_id           UUID NOT NULL,
    name                TEXT NOT NULL,
    description         TEXT NOT NULL DEFAULT '',
    conditions          JSONB NOT NULL, -- {"all"|"any": [{"field","op","value"} | group]}
    member_count        INT NOT NULL DEFAULT 0,
    last_refreshed_at   TIMESTAMPTZ,
    -- Refresh lease: one run at a time per segment.
    refresh_run_id      UUID,
    refresh_lease_until TIMESTAMPTZ,
    created_by          TEXT NOT NULL DEFAULT '',
    version             INT NOT NULL DEFAULT 0,
    created_at          TIMESTAMPTZ NOT NULL,
    updated_at          TIMESTAMPTZ NOT NULL,
    deleted_at          TIMESTAMPTZ
);

CREATE UNIQUE INDEX ux_segments_tenant_name ON segments (tenant_id, lower(name)) WHERE deleted_at IS NULL;
CREATE INDEX ix_segments_tenant_created ON segments (tenant_id, created_at DESC, id DESC) WHERE deleted_at IS NULL;

-- Materialized membership. run_id is the refresh run that last confirmed
-- the row; rows a finished run did not confirm are swept.
CREATE TABLE segment_members (
    segment_id UUID NOT NULL REFERENCES segments (id) ON DELETE CASCADE,
    player_id  UUID NOT NULL,
    tenant_id  UUID NOT NULL,
    added_at   TIMESTAMPTZ NOT NULL,
    run_id     UUID NOT NULL,
    PRIMARY KEY (segment_id, player_id)
);

CREATE INDEX ix_segment_members_page ON segment_members (segment_id, added_at DESC, player_id DESC);
CREATE INDEX ix_segment_members_player ON segment_members (tenant_id, player_id);

CREATE TABLE reconcile_markers (
    job      TEXT PRIMARY KEY,
    last_run TIMESTAMPTZ NOT NULL
);

-- +goose Down
DROP TABLE reconcile_markers;
DROP TABLE segment_members;
DROP TABLE segments;
