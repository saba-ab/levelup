# ADR-0017: API keys for tenant backends

**Status:** Accepted · 2026-10-05

## Context
Tenant systems (checkout, game servers, CRMs) report activities and move points from their backends. The only
credential the API offered was a person's 15-minute access token with a rotating refresh token. That is wrong
for servers: there is no human to log in, a shared password becomes a service account nobody owns, and
rotation logic leaks into every integration.

## Decision
- **Keys are owned by identity** (`identity_svc.api_keys`). Format: `lvl_live_<prefix>_<secret>`.
  - The prefix is 10 base32 characters, stored in clear for lookup and display.
  - The secret has 160 bits of entropy.
  - Only `sha256(full key)` is stored, and comparison is constant-time.
  - The plaintext is returned once, at creation.
- **Presentation:** `Authorization: Bearer lvl_live_…` or `X-API-Key: lvl_live_…`.
  - `platform/authn` recognises the `lvl_` prefix and calls an `authn.KeyVerifier`.
  - The composition root finds the module implementing that interface (identity), so the platform never imports
    a module.
  - An invalid key leaves the request anonymous, just like an invalid JWT.
- **Principal:** `UserID = key id` (so `created_by` columns record the key), `TenantID`, `RoleIDs` = the key's
  roles, and `APIKeyID` set.
  - A key holds only `admin`, `program_manager` or `developer`, never a role senior to its creator's.
  - Authorization is unchanged: the same casbin role grants apply.
- **Humans only:** creating, listing and revoking keys, user administration, role assignment and tenant changes
  refuse API-key principals (`human_principal_required`), even keys holding the admin role. A leaked key cannot
  mint people or more keys.
- **Verification** is read-through cached on redis-cache with a 60s TTL and negative caching of unknown
  prefixes.
  - Revocation evicts after commit, so it takes effect on the next request.
  - A suspended tenant's keys stop within one TTL.
  - `last_used_at` is written at most every 5 minutes per key.
- **Management:** `GET/POST /api/v1/api-keys`, `DELETE /api/v1/api-keys/{id}`, gated by
  `identity:api_keys_manage` (admin roles).

## Consequences
Integrations get a stable credential and per-key rate limits (the limiter keys on the principal). Revocation is
immediate. There are no per-endpoint key scopes beyond roles; if needed, they become a v2 of this ADR.
