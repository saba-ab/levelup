#!/usr/bin/env bash
# Daily logical backup of the production database, rotated by age.
#   Writes  $BACKUP_DIR/levelup-<UTC timestamp>.dump  (pg_dump custom format)
#   Deletes dumps older than $BACKUP_RETENTION_DAYS.
#
# Install on the host (as root):
#   install -m 700 -d /opt/levelup/backups
#   echo '15 2 * * * root /opt/levelup/src/backend/deploy/prod/backup.sh >>/var/log/levelup-backup.log 2>&1' \
#     > /etc/cron.d/levelup-backup
#
# Restore into an EMPTY database:
#   docker compose exec -T postgres pg_restore -U "$POSTGRES_USER" -d "$POSTGRES_DB" --no-owner < levelup-<ts>.dump
#
# A dump on the same disk protects against operator error, not disk loss:
# also copy $BACKUP_DIR off the host (object storage, rsync to another box).
set -euo pipefail

BACKUP_DIR="${BACKUP_DIR:-/opt/levelup/backups}"
BACKUP_RETENTION_DAYS="${BACKUP_RETENTION_DAYS:-14}"
cd "$(dirname "$0")"
set -a; . ./.env; set +a

umask 077
ts=$(date -u +%Y%m%dT%H%M%SZ)
out="$BACKUP_DIR/levelup-$ts.dump"
tmp="$out.partial"

docker compose exec -T postgres pg_dump -U "$POSTGRES_USER" -d "$POSTGRES_DB" --format=custom --compress=6 >"$tmp"
# A truncated dump fails to list; never rotate good dumps away for a bad one.
docker compose exec -T postgres pg_restore --list >/dev/null <"$tmp"
mv "$tmp" "$out"
echo "$(date -u +%FT%TZ) backup ok: $out ($(du -h "$out" | cut -f1))"

find "$BACKUP_DIR" -name 'levelup-*.dump' -mtime +"$BACKUP_RETENTION_DAYS" -delete
