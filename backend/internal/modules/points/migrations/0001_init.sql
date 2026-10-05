-- +goose Up
-- Schema points_svc. tenant_id / player_id / created_by are bare uuids:
-- no foreign keys into other modules' schemas (P5).

CREATE TABLE wallets (
    id              UUID PRIMARY KEY,
    tenant_id       UUID NOT NULL,
    player_id       UUID NOT NULL,
    balance         BIGINT NOT NULL DEFAULT 0 CHECK (balance >= 0),
    lifetime_earned BIGINT NOT NULL DEFAULT 0 CHECK (lifetime_earned >= 0),
    lifetime_spent  BIGINT NOT NULL DEFAULT 0 CHECK (lifetime_spent >= 0),
    is_active       BOOLEAN NOT NULL DEFAULT TRUE,
    version         INT NOT NULL DEFAULT 0,
    created_at      TIMESTAMPTZ NOT NULL,
    updated_at      TIMESTAMPTZ NOT NULL
);
-- One wallet per player per tenant; also what makes the player.created.v1
-- subscriber and the lazy open on first credit idempotent.
CREATE UNIQUE INDEX idx_wallets_tenant_player ON wallets (tenant_id, player_id);
CREATE INDEX idx_wallets_tenant_balance ON wallets (tenant_id, balance DESC);
CREATE INDEX idx_wallets_updated ON wallets (updated_at);

-- Append-only: rows are never updated; only a tenant purge deletes them.
CREATE TABLE ledger_entries (
    id              UUID PRIMARY KEY,
    tenant_id       UUID NOT NULL,
    wallet_id       UUID NOT NULL REFERENCES wallets (id),
    player_id       UUID NOT NULL,
    idempotency_key TEXT NOT NULL,
    kind            TEXT NOT NULL,
    direction       SMALLINT NOT NULL CHECK (direction IN (-1, 1)),
    amount          BIGINT NOT NULL CHECK (amount > 0),
    balance_before  BIGINT NOT NULL,
    balance_after   BIGINT NOT NULL,
    wallet_version  INT NOT NULL,
    source_kind     TEXT NOT NULL DEFAULT '',
    source_id       TEXT NOT NULL DEFAULT '',
    activity_id     TEXT NOT NULL DEFAULT '',
    description     TEXT NOT NULL DEFAULT '',
    transfer_id     UUID,
    reversal_of     UUID REFERENCES ledger_entries (id),
    occurred_at     TIMESTAMPTZ NOT NULL,
    created_by      UUID,
    created_at      TIMESTAMPTZ NOT NULL,
    CHECK (balance_after = balance_before + direction * amount)
);
-- The idempotency guarantee: a redelivered command inserts nothing.
CREATE UNIQUE INDEX idx_ledger_tenant_key ON ledger_entries (tenant_id, idempotency_key);
-- A debit is refunded at most once, whatever key the refund carries.
CREATE UNIQUE INDEX idx_ledger_reversal_of ON ledger_entries (reversal_of) WHERE reversal_of IS NOT NULL;
CREATE INDEX idx_ledger_tenant_player_created ON ledger_entries (tenant_id, player_id, created_at DESC, id DESC);
CREATE INDEX idx_ledger_wallet_version ON ledger_entries (wallet_id, wallet_version);
CREATE INDEX idx_ledger_created ON ledger_entries (created_at);
CREATE INDEX idx_ledger_transfer ON ledger_entries (transfer_id) WHERE transfer_id IS NOT NULL;

-- Settled "no" answers to job commands, keyed like the ledger, so
-- OutcomeByKey answers for rejections and a redelivery stays a no-op.
CREATE TABLE rejections (
    id              UUID PRIMARY KEY,
    tenant_id       UUID NOT NULL,
    idempotency_key TEXT NOT NULL,
    command         TEXT NOT NULL CHECK (command IN ('credit', 'debit', 'refund')),
    player_id       UUID,
    amount          BIGINT NOT NULL DEFAULT 0,
    reason          TEXT NOT NULL,
    available       BIGINT NOT NULL DEFAULT 0,
    source_kind     TEXT NOT NULL DEFAULT '',
    source_id       TEXT NOT NULL DEFAULT '',
    created_at      TIMESTAMPTZ NOT NULL
);
CREATE UNIQUE INDEX idx_rejections_tenant_key ON rejections (tenant_id, idempotency_key);

CREATE TABLE reconcile_markers (
    id       INT PRIMARY KEY,
    last_run TIMESTAMPTZ NOT NULL
);

-- +goose Down
DROP TABLE reconcile_markers;
DROP TABLE rejections;
DROP TABLE ledger_entries;
DROP TABLE wallets;
