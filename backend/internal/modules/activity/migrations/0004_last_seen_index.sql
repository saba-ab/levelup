-- +goose NO TRANSACTION
-- +goose Up
-- Last seen per player (GET /activities/last-seen, Reader.LastSeen): one
-- DISTINCT ON (player_id) query ordered by occurred_at DESC, id DESC.
-- CONCURRENTLY: the table is live; goose runs this file outside a
-- transaction. Version 4 because Go seeds hold versions 2 and 3.
CREATE INDEX CONCURRENTLY IF NOT EXISTS ix_activities_tenant_player_occurred
    ON activities (tenant_id, player_id, occurred_at DESC, id DESC)
    WHERE player_id IS NOT NULL;

-- +goose Down
DROP INDEX CONCURRENTLY IF EXISTS ix_activities_tenant_player_occurred;
