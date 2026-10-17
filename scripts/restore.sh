#!/bin/sh
# Only a new Compose project may be restored. Never select the working project.
set -eu
umask 077
cd "$(dirname "$0")/.."
[ "$#" -eq 2 ] || { echo 'Usage: scripts/restore.sh /absolute/backup new-project-name' >&2; exit 1; }
archive=$1
case "$archive" in /*) ;; *) echo 'Use an absolute backup path.' >&2; exit 1;; esac
case "$2" in dogclub|''|[!a-z0-9]*|*[!a-z0-9_-]*) echo 'Choose a new lowercase project name, other than dogclub.' >&2; exit 1;; esac
[ "$(cat "$archive/COMPLETE")" = dogclub-backup-v1 ]
(cd "$archive" && LC_ALL=C shasum -a 256 -c SHA256SUMS)
export COMPOSE_PROJECT_NAME=$2
export NOTIFICATIONS_ENABLED=false
# Refuse existing containers OR volumes, including stopped projects.
[ -z "$(docker ps -aq --filter "label=com.docker.compose.project=$COMPOSE_PROJECT_NAME")" ] || { echo 'Target project already has containers.' >&2; exit 1; }
[ -z "$(docker volume ls -q --filter "label=com.docker.compose.project=$COMPOSE_PROJECT_NAME")" ] || { echo 'Target project already has volumes.' >&2; exit 1; }
# API is not started until restoration and integrity verification both succeed.
docker compose up -d --wait db mailpit
docker compose exec -T db sh -ec 'pg_restore -U "$POSTGRES_USER" -d "$POSTGRES_DB" --exit-on-error --no-owner --no-privileges' < "$archive/database.dump"
# Extract only into the new empty named volume; archive came from the trusted backup tool.
docker compose run --rm -T --no-deps --entrypoint tar api -C /data/documents -xf - < "$archive/documents.tar"
actual=$(mktemp)
trap 'rm -f "$actual"' EXIT
trap 'exit 1' HUP INT TERM
docker compose run --rm -T --no-deps --entrypoint /maintenance api > "$actual"
cmp "$archive/storage.json" "$actual" || { echo 'Restored storage manifest differs; API remains stopped.' >&2; exit 1; }
# The migration service verifies current schema and files once more. Worker remains disabled.
docker compose up -d --wait api
echo 'Restored into the new project; notification worker is disabled. Keep NOTIFICATIONS_ENABLED=false for subsequent commands.'
