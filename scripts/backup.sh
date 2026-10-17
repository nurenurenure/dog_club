#!/bin/sh
# Stop every API writer before copying the database and private files.
set -eu
umask 077
cd "$(dirname "$0")/.."
mkdir -p var
if ! mkdir var/maintenance.lock 2>/dev/null; then echo 'Another maintenance operation holds the lock.' >&2; exit 1; fi
resume=false
finish() {
 result=$?
 trap - EXIT HUP INT TERM
 if [ "$resume" = true ]; then
  if ! docker compose start api >/dev/null; then echo 'Could not restart API; run docker compose start api.' >&2; result=1; fi
 fi
 rmdir var/maintenance.lock
 exit "$result"
}
trap finish EXIT
trap 'exit 1' HUP INT TERM
[ "$#" -eq 1 ] || { echo 'Usage: scripts/backup.sh /absolute/new-backup-directory' >&2; exit 1; }
case "$1" in /*) destination=$1;; *) echo 'Use an absolute backup path.' >&2; exit 1;; esac
[ ! -e "$destination" ] || { echo 'Backup destination already exists.' >&2; exit 1; }
mkdir -m 700 "$destination"
if [ -n "$(docker compose ps --status running -q api)" ]; then resume=true; fi
docker compose stop api >/dev/null
# Failure leaves an incomplete directory (no COMPLETE marker), never a valid set.
docker compose run --rm -T --no-deps --entrypoint /maintenance api > "$destination/storage.json"
docker compose exec -T db sh -ec 'pg_dump -U "$POSTGRES_USER" -d "$POSTGRES_DB" --format=custom' > "$destination/database.dump"
docker compose run --rm -T --no-deps --entrypoint tar api -C /data/documents -cf - . > "$destination/documents.tar"
docker compose exec -T db postgres --version > "$destination/postgres-version.txt"
docker compose images -q api > "$destination/image-id.txt"
(cd "$destination" && LC_ALL=C shasum -a 256 database.dump documents.tar storage.json postgres-version.txt image-id.txt > SHA256SUMS)
printf '%s\n' 'dogclub-backup-v1' > "$destination/COMPLETE"
chmod 600 "$destination/"*
echo 'Backup complete; checksums and storage manifest saved.'
