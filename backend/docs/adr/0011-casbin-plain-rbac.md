# ADR-0011: Casbin model is plain RBAC; ownership stays in the service layer

**Status:** Accepted · 2026-09-01 · resolves PRD §13 Q1

## Context
Three candidate shapes: plain RBAC, RBAC with domains (tenant first-class), RBAC+ABAC
matcher for ownership. Moving ownership into the matcher later means rewriting
`model.conf` and every stored policy row.

## Decision
Plain RBAC. Subjects are `role:{id}` and `user:{id}` — IDs, never names (renaming a
role must not change who can withdraw money, PRD §7.6). Objects are permission keys
(`wallet:withdraw`). Resource-scoped checks ("may this user withdraw from *this*
wallet") stay in the service layer, which already holds the loaded entity — exactly
what PRD §7.6 constraint 3 mandates. Multi-tenancy is a §3 non-goal;
`authz.Principal` carries `TenantID` so a domain model remains adoptable without
touching call sites.

## Consequences
`model.conf` stays 10 lines and policy rows stay flat. If relationship-based authz is
ever needed, `authz.Enforcer` is the seam (R33) — the model shape is not.

**Amendment 2026-09-01:** casbin v2 was superseded upstream; gorm-adapter v3.41+ and redis-watcher v2.8+ build against casbin/v3. The blueprint uses casbin/v3 — same model language, same API shape, still confined to platform/authz.
