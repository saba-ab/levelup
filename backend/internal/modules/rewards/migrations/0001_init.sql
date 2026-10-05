-- +goose Up
-- Lands in schema rewards_svc (doc 05 §11.4). Table names match GORM's
-- derivation: reward → rewards, rewardClaim → reward_claims,
-- reconcileMarker → reconcile_markers. No foreign keys to other modules'
-- schemas (P5): player_id, badge_reward_id, level_reward_id are bare ids.

CREATE TABLE rewards (
    id                UUID PRIMARY KEY,
    tenant_id         UUID NOT NULL,
    legacy_id         BIGINT UNIQUE,
    name              TEXT NOT NULL,
    slug              TEXT NOT NULL,
    description       TEXT,
    type              TEXT NOT NULL CHECK (type IN ('points','discount','item','badge','level','custom')),
    status            TEXT NOT NULL DEFAULT 'draft' CHECK (status IN ('draft','active','paused','expired','depleted')),
    points_cost       BIGINT NOT NULL DEFAULT 0 CHECK (points_cost >= 0),
    value             NUMERIC(10,2),
    value_type        TEXT CHECK (value_type IN ('percentage','fixed')),
    badge_reward_id   UUID,
    level_reward_id   UUID,
    -- Global stock; NULL = unlimited.
    max_redemptions   INT CHECK (max_redemptions >= 1),
    max_per_player    INT CHECK (max_per_player >= 1),
    -- Units held by pending_payment + claimed + redeemed claims, mutated
    -- only under the row lock (fixes the legacy oversell, doc 05 R1/R2).
    stock_used        INT NOT NULL DEFAULT 0 CHECK (stock_used >= 0),
    -- Replaces the hard-coded 30 days for discounts (R7); NULL = no expiry.
    claim_ttl_days    INT CHECK (claim_ttl_days >= 1),
    start_at          TIMESTAMPTZ,
    end_at            TIMESTAMPTZ,
    level_requirement INT CHECK (level_requirement >= 1),
    is_active         BOOLEAN NOT NULL DEFAULT true,
    metadata          JSONB,
    version           INT NOT NULL DEFAULT 0,
    created_at        TIMESTAMPTZ NOT NULL,
    updated_at        TIMESTAMPTZ NOT NULL,
    deleted_at        TIMESTAMPTZ,
    CHECK (max_redemptions IS NULL OR stock_used <= max_redemptions)
);

CREATE UNIQUE INDEX ux_rewards_tenant_slug ON rewards (tenant_id, slug) WHERE deleted_at IS NULL;
CREATE INDEX ix_rewards_tenant_created ON rewards (tenant_id, created_at DESC, id DESC) WHERE deleted_at IS NULL;
CREATE INDEX ix_rewards_ending ON rewards (end_at) WHERE deleted_at IS NULL AND status IN ('active','paused','depleted');

-- Was player_rewards. reward_id is deliberately not a foreign key: a
-- refused rewards.grant for an unknown reward is still recorded under its
-- idempotency key.
CREATE TABLE reward_claims (
    id                UUID PRIMARY KEY,
    tenant_id         UUID NOT NULL,
    legacy_id         BIGINT UNIQUE,
    player_id         UUID NOT NULL,
    reward_id         UUID NOT NULL,
    reward_slug       TEXT NOT NULL DEFAULT '',
    reward_type       TEXT NOT NULL DEFAULT '',
    status            TEXT NOT NULL CHECK (status IN ('pending_payment','claimed','rejected','redeemed','expired','cancelled','refund_pending','refunded')),
    -- Points charged (price snapshot); 0 for free rewards and grants.
    points_cost       BIGINT NOT NULL DEFAULT 0 CHECK (points_cost >= 0),
    -- 'reward_claim:<id>', the points idempotency key of the payment.
    debit_key         TEXT NOT NULL UNIQUE,
    -- Idempotency-Key of the HTTP claim.
    client_request_id TEXT,
    -- GrantCmdV1.idempotency_key of a rewards.grant claim.
    grant_key         TEXT,
    reject_reason     TEXT,
    hold_expires_at   TIMESTAMPTZ,
    claimed_at        TIMESTAMPTZ,
    redeemed_at       TIMESTAMPTZ,
    expires_at        TIMESTAMPTZ,
    cancelled_at      TIMESTAMPTZ,
    -- Voucher code for discount and item rewards.
    code              TEXT,
    metadata          JSONB,
    version           INT NOT NULL DEFAULT 0,
    created_at        TIMESTAMPTZ NOT NULL,
    updated_at        TIMESTAMPTZ NOT NULL
);

CREATE UNIQUE INDEX ux_claims_client_req ON reward_claims (tenant_id, client_request_id) WHERE client_request_id IS NOT NULL;
CREATE UNIQUE INDEX ux_claims_grant_key ON reward_claims (tenant_id, grant_key) WHERE grant_key IS NOT NULL;
CREATE UNIQUE INDEX ux_claims_code ON reward_claims (tenant_id, code) WHERE code IS NOT NULL;
CREATE INDEX ix_claims_player ON reward_claims (tenant_id, player_id, created_at DESC, id DESC);
CREATE INDEX ix_claims_pending ON reward_claims (hold_expires_at) WHERE status = 'pending_payment';
CREATE INDEX ix_claims_expiry ON reward_claims (expires_at) WHERE status = 'claimed';
CREATE INDEX ix_claims_limit ON reward_claims (reward_id, player_id) WHERE status IN ('pending_payment','claimed','redeemed');
CREATE INDEX ix_claims_refunds ON reward_claims (updated_at) WHERE status IN ('cancelled','refund_pending');

CREATE TABLE reconcile_markers (
    job      TEXT PRIMARY KEY,
    last_run TIMESTAMPTZ NOT NULL
);

-- +goose Down
DROP TABLE reconcile_markers;
DROP TABLE reward_claims;
DROP TABLE rewards;
