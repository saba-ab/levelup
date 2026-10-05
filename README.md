# LevelUp

Multi-tenant gamification engine: tenants define event types and rules, and ingested player activity drives
points, XP and levels, badges, missions, streaks, rewards and leaderboards.

## Repository layout (monorepo)

| Path | What |
|---|---|
| `backend/` | Go modular monolith (API, worker, dispatcher, scheduler, migrate), built on the Go modular-monolith blueprint. See `backend/README.md` and `backend/CLAUDE.md`. |
| `frontend/` | Web app (moving in from its own repository). |
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
