#!/usr/bin/env bash
# Deploys a git ref of this repository on the production host.
#   /opt/levelup/src                      checkout (this repo)
#   /opt/levelup/src/backend/deploy/prod/.env   secrets (chmod 600, never committed)
# Usage: deploy.sh <git-ref>     e.g. deploy.sh origin/master
set -euo pipefail

REF="${1:?usage: deploy.sh <git-ref>}"
ROOT=/opt/levelup/src
cd "$ROOT"
git fetch --quiet origin
git checkout --quiet --detach "$REF"
TAG=$(git rev-parse --short HEAD)
echo "==> deploying $REF ($TAG)"

docker build --quiet -t "levelup-backend:$TAG" -t levelup-backend:latest backend >/dev/null
cd backend/deploy/prod
export LEVELUP_IMAGE_TAG="$TAG"
set -a; . ./.env; set +a

echo "==> infrastructure"
docker compose up -d --wait postgres pgbouncer redis-core redis-cache rabbitmq

echo "==> migrations"
docker compose --profile migrate run --rm migrate
# migrate provisions <module>_svc roles with password = role name, for the
# schema-isolation tests. Runtime never logs in as them: disable login.
docker compose exec -T postgres psql -q -U "$POSTGRES_USER" -d "$POSTGRES_DB" -v ON_ERROR_STOP=1 -c \
  "DO \$\$ DECLARE r record; BEGIN
     FOR r IN SELECT rolname FROM pg_roles WHERE rolname LIKE '%\_svc' AND rolcanlogin LOOP
       EXECUTE format('ALTER ROLE %I NOLOGIN', r.rolname);
     END LOOP; END \$\$;"

echo "==> application"
docker compose up -d api worker dispatcher scheduler

for i in $(seq 1 30); do
  if curl -fsS http://127.0.0.1:18080/health >/dev/null 2>&1; then
    echo "==> healthy: $(curl -fsS http://127.0.0.1:18080/health)"
    exit 0
  fi
  sleep 2
done
echo "!! api not healthy after 60s"; docker compose logs --tail=50 api
exit 1
