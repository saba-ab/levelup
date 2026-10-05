-- +goose Up
-- Policy storage, platform-owned (PRD §7.6): the one table outside the
-- per-module schema rule besides the outbox — authz is platform, not a
-- module. Shape matches casbin gorm-adapter's expectations exactly; the
-- adapter's own auto-migration is turned off (goose owns DDL).
CREATE TABLE casbin_rule (
    id    BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    ptype VARCHAR(100),
    v0    VARCHAR(100),
    v1    VARCHAR(100),
    v2    VARCHAR(100),
    v3    VARCHAR(100),
    v4    VARCHAR(100),
    v5    VARCHAR(100)
);

CREATE UNIQUE INDEX idx_casbin_rule ON casbin_rule (ptype, v0, v1, v2, v3, v4, v5);

-- Permission catalogue, seeded by each module's typed Go migration from the
-- same constants the code enforces (PRD §7.4): a permission can never exist
-- in the database under a name the code does not define.
CREATE TABLE permissions (
    key    TEXT PRIMARY KEY,
    module TEXT NOT NULL
);

-- +goose Down
DROP TABLE permissions;
DROP TABLE casbin_rule;
