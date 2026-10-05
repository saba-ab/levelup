# CLAUDE.md (monorepo root)

This is a monorepo. Each app has its own guide, and those guides are binding inside their directory:

- `backend/`: Go modular monolith. Read `backend/CLAUDE.md`. Run Go tooling from `backend/`
  (`cd backend && task check`), or from the root with `task be -- check`.
- `frontend/`: web app (being moved in). It keeps its own tooling and guide.

LevelUp product and architecture docs live in `docs/rewrite/`:
- `00-target-architecture.md`: module list, canonical topic and job names, cross-module flows, requirements
  R50+. It wins over the domain docs (01–06) on naming.
- `IMPLEMENTATION.md`: the rules every backend module follows (tenancy, authz, HTTP, events, tests).
- `01`–`06`: the Laravel predecessor's behaviour, its bugs, and the per-domain Go design.

Cross-app changes (API contract plus frontend) go in one PR. The API contract is problem+json errors with a
`code` field, cursor pagination `{data, next_cursor}`, and UUID ids (backend ADR-0016).
