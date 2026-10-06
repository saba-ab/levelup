-- +goose Up
-- The activity subscriber looks up a tenant's active missions by
-- criteria.event_type on every activity.received.v1.
CREATE INDEX ix_missions_auto_event_type
    ON missions (tenant_id, (criteria ->> 'event_type'))
    WHERE deleted_at IS NULL AND status = 'active';

-- Analytics: attempts per mission grouped by status.
CREATE INDEX ix_mission_attempts_stats ON mission_attempts (tenant_id, mission_id, status);

-- +goose Down
DROP INDEX ix_mission_attempts_stats;
DROP INDEX ix_missions_auto_event_type;
