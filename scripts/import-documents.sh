#!/bin/sh
# Copy the former host directory without moving/removing any original files.
set -eu
umask 077
cd "$(dirname "$0")/.."
[ "$#" -eq 1 ] || { echo 'Usage: scripts/import-documents.sh /absolute/host-documents' >&2; exit 1; }
case "$1" in /*) ;; *) echo 'Use an absolute path.' >&2; exit 1;; esac
[ -d "$1" ]
mkdir -p var
mkdir var/maintenance.lock || { echo 'Maintenance lock is held.' >&2; exit 1; }
trap 'rmdir var/maintenance.lock' EXIT
trap 'exit 1' HUP INT TERM
# Leave API stopped even on success: the operator explicitly starts it after verification.
docker compose stop api >/dev/null
docker compose run --rm -T --no-deps --user 0:0 -v "$1:/import:ro" --entrypoint /bin/sh api -ec '
 [ -z "$(ls -A /data/documents)" ] || { echo "Target storage must be empty." >&2; exit 1; }
 cp -a /import/. /data/documents/
 chown -R 65532:65532 /data/documents
 chmod 0700 /data/documents
 find /data/documents -type f -exec chmod 0600 {} +
'
docker compose run --rm -T --no-deps --entrypoint /maintenance api
echo 'Originals retained; every registered file verified. Start API with docker compose up -d --wait api.'
