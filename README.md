# LevelUp

Multi-tenant gamification engine: tenants define event types and rules, and ingested player activity drives
points, XP and levels, badges, missions, streaks, rewards and leaderboards.

## Repository layout (monorepo)

| Path | What |
|---|---|
| `backend/` | Go modular monolith (API, worker, dispatcher, scheduler, migrate), built on the Go modular-monolith blueprint. See `backend/README.md` and `backend/CLAUDE.md`. |
| `frontend/portal/` | Tenant dashboard: Vite + React + shadcn/ui (imported from `levelupos-portal`, history preserved). |
| `frontend/landing/` | Marketing site: Next.js (imported from `levelupos-landing`, history preserved). |
| `docs/rewrite/` | Documentation of the Laravel predecessor and the Go target architecture. `00-target-architecture.md` is canonical, and `IMPLEMENTATION.md` holds the module rules. |
| `.github/workflows/` | One workflow per app, path-filtered. |

## Quick start (backend)

```bash
cd backend
cp .env.example .env
task docker:up && task migrate
task run          # API :8080, admin :8081
task worker       # in another terminal; also: task dispatcher, task scheduler
```

From the repository root, `task be -- <task>` runs any backend task, for example `task be -- check`.

## Quick start (frontend)

```bash
task portal -- ci && task portal -- run dev     # API URLs: frontend/portal/.env.example
task landing -- ci && task landing -- run dev
```

> The portal still speaks the Laravel API contract. Moving it to the Go API (problem+json errors, cursor
> pagination, UUID ids, `/auth/refresh` rotation; see `backend/docs/adr/0016`) is the next step.
