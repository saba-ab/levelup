-- +goose Up
-- Activity leaderboards: ranked by one activity event type
-- (activity.received.v1). config = {"event_type", "value", "property"?};
-- '{}' for every other type. JSONB so later board types can carry their own
-- settings without another column.
ALTER TABLE leaderboards ADD COLUMN config JSONB NOT NULL DEFAULT '{}'::jsonb;

ALTER TABLE leaderboards DROP CONSTRAINT leaderboards_type_check;
ALTER TABLE leaderboards ADD CONSTRAINT leaderboards_type_check
    CHECK (type IN ('points', 'badges', 'missions', 'xp', 'activity'));
ALTER TABLE leaderboards ADD CONSTRAINT leaderboards_activity_config_check
    CHECK (type <> 'activity' OR (config ? 'event_type' AND config ? 'value'));

-- The activity.received.v1 subscriber looks boards up by event type.
CREATE INDEX ix_leaderboards_activity_event ON leaderboards (tenant_id, (config ->> 'event_type'))
    WHERE type = 'activity' AND deleted_at IS NULL AND is_active;

-- +goose Down
DROP INDEX ix_leaderboards_activity_event;
-- Periods and scores of activity boards go with them (ON DELETE CASCADE).
DELETE FROM leaderboards WHERE type = 'activity';
ALTER TABLE leaderboards DROP CONSTRAINT leaderboards_activity_config_check;
ALTER TABLE leaderboards DROP CONSTRAINT leaderboards_type_check;
ALTER TABLE leaderboards ADD CONSTRAINT leaderboards_type_check
    CHECK (type IN ('points', 'badges', 'missions', 'xp'));
ALTER TABLE leaderboards DROP COLUMN config;
