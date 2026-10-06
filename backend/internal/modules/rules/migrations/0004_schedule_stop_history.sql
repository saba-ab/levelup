-- +goose Up
-- Grammar v1 completion and v2 history (doc 06 §11.5). Versions 2 and 3 are
-- the Go seeds (seeds.go), so the next SQL migration is 0004.

-- stop_processing: once this version fires, lower-priority rules of the same
-- activity are skipped (execution status skipped_by_stop).
-- schedule: {starts_at, ends_at, days_of_week, hours, timezone} judged on
-- activity.occurred_at (execution status out_of_schedule). JSON null = always.
ALTER TABLE rule_versions
    ADD COLUMN stop_processing BOOLEAN NOT NULL DEFAULT false,
    ADD COLUMN schedule        JSONB   NOT NULL DEFAULT 'null';

-- Per-player history projection read by history.* conditions. One row per
-- (tenant, player, event type, UTC day of occurred_at), bumped once per
-- evaluated activity inside the decision tx (decisions.activity_id UNIQUE
-- makes it exactly-once under redelivery).
CREATE TABLE rule_player_event_days (
    tenant_id  UUID   NOT NULL,
    player_id  UUID   NOT NULL,
    event_type TEXT   NOT NULL,
    day        DATE   NOT NULL,
    count      BIGINT NOT NULL CHECK (count > 0),
    PRIMARY KEY (tenant_id, player_id, event_type, day)
);

-- Rule stats scan a tenant's executions and effects by time range.
CREATE INDEX ix_rule_executions_tenant_time ON rule_executions (tenant_id, created_at);
CREATE INDEX ix_effects_tenant_time ON rule_execution_effects (tenant_id, requested_at);

-- +goose Down
DROP INDEX ix_effects_tenant_time;
DROP INDEX ix_rule_executions_tenant_time;
DROP TABLE rule_player_event_days;
ALTER TABLE rule_versions
    DROP COLUMN schedule,
    DROP COLUMN stop_processing;
