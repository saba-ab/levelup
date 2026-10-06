-- +goose NO TRANSACTION
-- +goose Up
-- GET /players?sort=display_name keyset: (lower(COALESCE(display_name,
-- external_id)), id) ASC within a tenant. The created_at sorts (either
-- direction) and created_from/created_to reuse ix_players_tenant_created.
-- CONCURRENTLY: the table is live; goose runs this file outside a
-- transaction. Version 4 because Go seeds hold versions 2 and 3.
CREATE INDEX CONCURRENTLY IF NOT EXISTS ix_players_tenant_sort_name
    ON players (tenant_id, lower(COALESCE(display_name, external_id)), id)
    WHERE deleted_at IS NULL;

-- +goose Down
DROP INDEX CONCURRENTLY IF EXISTS ix_players_tenant_sort_name;
