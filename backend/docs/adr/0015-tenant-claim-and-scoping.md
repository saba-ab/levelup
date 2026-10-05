# ADR-0015: Tenant id travels in the access token and in every payload

**Status:** Accepted · 2026-10-05

## Context
LevelUpOS is multi-tenant. The Laravel predecessor derived the tenant from the
logged-in user through a global query scope that silently switched off when no
user was present (queue workers, platform admins), so background code read and
wrote across tenants (`docs/rewrite/01-platform-cross-cutting.md` §5).

## Decision
- `platform/authn` access tokens carry a `tid` claim, mapped to
  `authz.Principal.TenantID`. `IssueAccess(userID, tenantID, roleIDs)`.
- Tenant-owned service methods start with `authz.RequireTenant(ctx)`; a principal
  without a tenant gets `PermissionDenied`, never an unscoped read.
- Repositories take `tenantID` as an explicit argument and filter on it. There is
  no global scope. Loading another tenant's row is `NotFound` (404).
- Every event and job payload carries `tenant_id`; consumers never infer it.
- Event types and event categories are the only tables where `tenant_id IS NULL`
  rows (platform-global) are visible to every tenant.

## Consequences
Roles and tenant in a token can be stale for one access TTL (15m). Role changes and
suspensions revoke refresh tokens so the window is bounded.
