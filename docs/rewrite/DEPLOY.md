# Deploying LevelUp

## Topology

| Piece | Where |
|---|---|
| Go backend (api, worker, dispatcher, scheduler) + Postgres 17, PgBouncer, redis-core, redis-cache, RabbitMQ 4 | Docker Compose on the API host (46.224.230.11), project `levelup`, network `172.29.0.0/24` |
| TLS + routing for `api.levelupos.ge` | Caddy on the host, `reverse_proxy 127.0.0.1:18080` |
| Portal (`portal.levelupos.ge`), landing (`levelupos.ge`) | Vercel, root directories `frontend/portal` and `frontend/landing` |

Only the API port is published, and only on `127.0.0.1`:
- `18080` is the API, behind Caddy;
- `18081` is the admin port (`/metrics` and pprof), reachable from the host only.

Nothing else listens on the host.

## Files

- `backend/Dockerfile`: one distroless image with all five binaries.
- `backend/deploy/prod/compose.yaml`: the production stack.
- `backend/deploy/prod/.env.example`: every variable. The real `.env` lives next to `compose.yaml` on the host (`chmod 600`) and is never committed.
- `backend/deploy/prod/deploy.sh <git-ref>`, run on the host:
  1. checks out the ref and builds the image tagged with the commit;
  2. starts the infrastructure, then runs migrations;
  3. disables login for the per-module `*_svc` roles;
  4. starts the app;
  5. waits for `/health`.
- `backend/deploy/prod/Caddyfile.api`: the `api.levelupos.ge` site block.

## First install (once)

```bash
ssh root@46.224.230.11
mkdir -p /opt/levelup && git clone https://github.com/saba-ab/levelup_backend.git /opt/levelup/src
cd /opt/levelup/src/backend/deploy/prod
cp .env.example .env && chmod 600 .env
# fill .env with generated secrets: openssl rand -hex 32 (DB, RabbitMQ, JWT)
```

## Every deploy

```bash
ssh root@46.224.230.11 /opt/levelup/src/backend/deploy/prod/deploy.sh origin/master
```

Migrations are idempotent and forward-only; a deploy never drops data. To roll back the code, run
`deploy.sh <previous-sha>`. A forward migration stays applied, so new migrations must stay backward compatible
for one release.

## Cutover from Laravel (one-time, 2026-10)

The decision is a fresh start: no data migration, and Laravel is retired.

1. Back up the Laravel database: `mysqldump --single-transaction levelupos > /root/backups/levelupos-<date>.sql`.
2. Run `deploy.sh`. The Go stack comes up on 127.0.0.1:18080 while Laravel still serves traffic.
3. Smoke-test on the host:
   - `curl 127.0.0.1:18080/health`;
   - register, then log in;
   - send an activity and confirm it becomes `decided`. This proves the worker, the dispatcher and RabbitMQ.
4. Switch Caddy:
   - back up `/etc/caddy/Caddyfile` to `Caddyfile.pre-go`;
   - replace the `api.levelupos.ge` block with `Caddyfile.api`;
   - remove the `horizon.levelupos.ge` block;
   - run `caddy validate`, then `systemctl reload caddy`.
5. Retire the Laravel workers:
   - `supervisorctl stop horizon nightwatch-agent`;
   - move their conf files out of `/etc/supervisor/conf.d`;
   - run `supervisorctl reread && supervisorctl update`.
   PHP-FPM, MySQL and the host Redis stay installed: other sites on the host may use them. `/var/www/LevelUpOS` is kept.
6. Vercel:
   - point both projects at this repository (root directories `frontend/portal` and `frontend/landing`);
   - deploy `master`.

   The portal's production API URL is already `https://api.levelupos.ge`.

### Rollback to Laravel

```bash
cp /etc/caddy/Caddyfile.pre-go /etc/caddy/Caddyfile && systemctl reload caddy
mv /root/supervisor-laravel/*.conf /etc/supervisor/conf.d/ && supervisorctl reread && supervisorctl update
```

The Go stack can keep running alongside Laravel; it only stops receiving traffic.

## Operations

- Logs: `docker compose logs -f api worker dispatcher scheduler`, run in `backend/deploy/prod`.
- Health: `curl 127.0.0.1:18080/health`.
- Metrics: `curl 127.0.0.1:18081/metrics`. Alert on `outbox_unpublished_age_seconds` above 60s and on any DLQ growth.
- Backups: `docker compose exec -T postgres pg_dump -U levelup levelup | gzip > /root/backups/levelup-$(date +%F).sql.gz`, daily from cron.
