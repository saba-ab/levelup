# ADR-0016: problem+json with a machine-readable `code`; no Laravel wire shim

**Status:** Accepted · 2026-10-05

## Context
The rewrite plan (`docs/rewrite/00-target-architecture.md` D5) proposed a renderer
reproducing Laravel's ad-hoc error and pagination shapes for client compatibility.
The frontend moves into this monorepo and is migrated in lock-step with the API, so
there is no external client whose shapes must be frozen, and the Laravel shapes were
inconsistent across endpoints.

## Decision
- Errors stay RFC 9457 `application/problem+json` (blueprint `httpx.Error`), extended
  additively with `code`: `errs.WithCode(err, "insufficient_balance")`. Clients branch
  on `code`, never on `detail` text.
- Lists use the blueprint keyset cursor (`shared/pagination`): `?limit=&cursor=` and
  `{"data": [...], "next_cursor": "..."}`. No offset pagination.
- IDs are UUIDv7 strings. Migrated rows keep `legacy_id BIGINT` for the data migration.

## Consequences
The frontend must be adapted when it moves in. Tenant integrations, if any exist,
get one breaking change at cutover, announced with the `/api/v1` launch.
