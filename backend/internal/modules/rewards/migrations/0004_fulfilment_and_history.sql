-- +goose Up
-- Version 4: 2 and 3 are the Go seeds (permissions, default grants).
-- fulfilled_at: when the reward was delivered (fulfilment commands issued
-- for points/badge/level, or the voucher redeemed for discount/item/custom).
ALTER TABLE reward_claims ADD COLUMN fulfilled_at TIMESTAMPTZ;

-- Tenant-wide redemption history (GET /rewards/claims) and per-reward stats.
CREATE INDEX ix_claims_tenant_created ON reward_claims (tenant_id, created_at DESC, id DESC);
CREATE INDEX ix_claims_tenant_reward ON reward_claims (tenant_id, reward_id, created_at DESC, id DESC);

-- +goose Down
DROP INDEX ix_claims_tenant_reward;
DROP INDEX ix_claims_tenant_created;
ALTER TABLE reward_claims DROP COLUMN fulfilled_at;
