-- +goose Up
-- auto_record: activity.received.v1 with event_type = activity_key records a
-- period for the player automatically. Tenants opt out per streak (rule
-- actions and explicit record calls keep working either way).
ALTER TABLE streaks ADD COLUMN auto_record BOOLEAN NOT NULL DEFAULT true;

-- +goose Down
ALTER TABLE streaks DROP COLUMN auto_record;
