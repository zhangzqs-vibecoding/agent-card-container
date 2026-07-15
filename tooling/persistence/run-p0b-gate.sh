#!/bin/sh
set -eu

ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
POSTGRES_IMAGE="postgres@sha256:742f40ea20b9ff2ff31db5458d127452988a2164df9e17441e191f3b72252193"
MINIO_IMAGE="minio/minio@sha256:a1ea29fa28355559ef137d71fc570e508a214ec84ff8083e39bc5428980b015e"
MC_IMAGE="minio/mc@sha256:a7fe349ef4bd8521fb8497f55c6042871b2ae640607cf99d9bede5e9bdf11727"

check_dependencies() {
  for command in docker go curl openssl psql sha256sum find sort xargs id; do
    if ! command -v "$command" >/dev/null 2>&1; then
      echo "P0-B gate dependency is missing: $command" >&2
      return 1
    fi
  done
  docker info >/dev/null 2>&1 || {
    echo "P0-B gate cannot reach the Docker daemon" >&2
    return 1
  }
}

check_dependencies
if [ "${1:-}" = "--check" ]; then
  echo "P0-B gate prerequisites: PASS"
  exit 0
fi
if [ "$#" -ne 0 ]; then
  echo "usage: $0 [--check]" >&2
  exit 64
fi

run_id="agentcard-p0b-$(date -u +%Y%m%d%H%M%S)-$$"
network="$run_id-net"
postgres_container="$run_id-postgres"
minio_container="$run_id-minio"
postgres_volume="$run_id-postgres-data"
minio_volume="$run_id-minio-data"
temporary=$(mktemp -d "${TMPDIR:-/tmp}/agentcard-p0b.XXXXXX")
backup_id="backup-$(date -u +%Y%m%dT%H%M%SZ)"
backup_directory="$temporary/$backup_id"
mkdir -p "$backup_directory"

postgres_password=$(openssl rand -hex 24)
minio_access_key="p0b$(openssl rand -hex 10)"
minio_secret_key=$(openssl rand -hex 32)
signing_private_key=$(openssl rand 32 | base64 | tr -d '\n')

cleanup() {
  docker unpause "$postgres_container" "$minio_container" >/dev/null 2>&1 || true
  docker rm -f "$postgres_container" "$minio_container" >/dev/null 2>&1 || true
  docker network rm "$network" >/dev/null 2>&1 || true
  docker volume rm "$postgres_volume" "$minio_volume" >/dev/null 2>&1 || true
  rm -rf "$temporary"
}
trap cleanup EXIT INT TERM

docker network create "$network" >/dev/null
docker volume create "$postgres_volume" >/dev/null
docker volume create "$minio_volume" >/dev/null
docker run -d \
  --name "$postgres_container" \
  --network "$network" \
  --publish 127.0.0.1::5432 \
  --env POSTGRES_USER=agentcard \
  --env POSTGRES_DB=agentcard \
  --env POSTGRES_PASSWORD="$postgres_password" \
  --volume "$postgres_volume:/var/lib/postgresql/data" \
  "$POSTGRES_IMAGE" >/dev/null
docker run -d \
  --name "$minio_container" \
  --network "$network" \
  --publish 127.0.0.1::9000 \
  --env MINIO_ROOT_USER="$minio_access_key" \
  --env MINIO_ROOT_PASSWORD="$minio_secret_key" \
  --volume "$minio_volume:/data" \
  "$MINIO_IMAGE" server /data >/dev/null

postgres_port=$(docker port "$postgres_container" 5432/tcp | sed 's/.*://')
minio_port=$(docker port "$minio_container" 9000/tcp | sed 's/.*://')
database_url="postgres://agentcard:$postgres_password@127.0.0.1:$postgres_port/agentcard?sslmode=disable"

refresh_test_endpoints() {
  postgres_port=$(docker port "$postgres_container" 5432/tcp | sed 's/.*://')
  minio_port=$(docker port "$minio_container" 9000/tcp | sed 's/.*://')
  database_url="postgres://agentcard:$postgres_password@127.0.0.1:$postgres_port/agentcard?sslmode=disable"
  export AGENTCARD_POSTGRES_TEST_DSN="$database_url"
  export AGENTCARD_S3_TEST_ENDPOINT="127.0.0.1:$minio_port"
}

wait_postgres() {
  attempts=0
  while [ "$attempts" -lt 80 ]; do
    if PGPASSWORD="$postgres_password" psql \
      --host 127.0.0.1 --port "$postgres_port" \
      --username agentcard --dbname agentcard \
      --tuples-only --no-align --command 'SELECT 1' >/dev/null 2>&1; then
      return 0
    fi
    attempts=$((attempts + 1))
    sleep 0.25
  done
  echo "PostgreSQL did not become queryable" >&2
  return 1
}

wait_minio() {
  attempts=0
  while [ "$attempts" -lt 80 ]; do
    if curl --fail --silent --show-error \
      "http://127.0.0.1:$minio_port/minio/health/ready" >/dev/null 2>&1; then
      return 0
    fi
    attempts=$((attempts + 1))
    sleep 0.25
  done
  echo "MinIO did not become ready" >&2
  return 1
}

export AGENTCARD_POSTGRES_TEST_CONTAINER="$postgres_container"
export AGENTCARD_S3_TEST_CONTAINER="$minio_container"
export AGENTCARD_S3_TEST_ACCESS_KEY="$minio_access_key"
export AGENTCARD_S3_TEST_SECRET_KEY="$minio_secret_key"
export AGENTCARD_S3_TEST_BUCKET="agentcard-p0b"
export AGENTCARD_P0B_TEST_SIGNING_PRIVATE_KEY="$signing_private_key"

refresh_test_endpoints
wait_postgres
wait_minio

cd "$ROOT/services/cloud"
go test ./internal/generation -run Postgres -count=1
go test ./internal/jobs -run Postgres -count=1
go test ./internal/publish -run 'Postgres|S3' -count=1
go test ./internal/bootstrap -run 'ProductionRuntimes|ProductionReadiness' -count=1
echo "P0-B persistence, restart and readiness matrix: PASS"

docker restart "$postgres_container" "$minio_container" >/dev/null
refresh_test_endpoints
wait_postgres
wait_minio
AGENTCARD_P0B_RESTORE_VERIFY=1 \
  go test ./internal/bootstrap -run ProductionPersistenceRestoredSnapshot -count=1
echo "P0-B database/object-store restart recovery: PASS"

docker exec "$postgres_container" \
  pg_dump --username agentcard --dbname agentcard --format custom \
  >"$backup_directory/postgres.dump"
docker run --rm \
  --network "$network" \
  --user "$(id -u):$(id -g)" \
  --volume "$backup_directory:/backup" \
  --env MC_CONFIG_DIR=/tmp/.mc \
  --env "MC_HOST_source=http://$minio_access_key:$minio_secret_key@$minio_container:9000" \
  "$MC_IMAGE" mirror --overwrite \
  "source/$AGENTCARD_S3_TEST_BUCKET" /backup/objects >/dev/null
(
  cd "$backup_directory"
  find . -type f ! -name manifest.sha256 -print0 \
    | sort -z \
    | xargs -0 sha256sum >manifest.sha256
  sha256sum --check manifest.sha256 >/dev/null
)
backup_manifest_sha256=$(sha256sum "$backup_directory/manifest.sha256" | cut -d ' ' -f 1)
echo "P0-B paired backup created: id=$backup_id manifestSha256=$backup_manifest_sha256"

PGPASSWORD="$postgres_password" psql \
  --host 127.0.0.1 --port "$postgres_port" \
  --username agentcard --dbname agentcard \
  --command 'DROP SCHEMA public CASCADE; CREATE SCHEMA public' >/dev/null
docker run --rm \
  --network "$network" \
  --env MC_CONFIG_DIR=/tmp/.mc \
  --env "MC_HOST_source=http://$minio_access_key:$minio_secret_key@$minio_container:9000" \
  "$MC_IMAGE" rm --recursive --force \
  "source/$AGENTCARD_S3_TEST_BUCKET" >/dev/null

docker exec --interactive "$postgres_container" \
  pg_restore --username agentcard --dbname agentcard \
  <"$backup_directory/postgres.dump"
docker run --rm \
  --network "$network" \
  --user "$(id -u):$(id -g)" \
  --volume "$backup_directory:/backup:ro" \
  --env MC_CONFIG_DIR=/tmp/.mc \
  --env "MC_HOST_source=http://$minio_access_key:$minio_secret_key@$minio_container:9000" \
  "$MC_IMAGE" mirror --overwrite \
  /backup/objects "source/$AGENTCARD_S3_TEST_BUCKET" >/dev/null
(
  cd "$backup_directory"
  sha256sum --check manifest.sha256 >/dev/null
)
AGENTCARD_P0B_RESTORE_VERIFY=1 \
  go test ./internal/bootstrap -run ProductionPersistenceRestoredSnapshot -count=1
echo "P0-B paired backup restore and independent artifact verification: PASS"
echo "P0-B persistent test environment gate: PASS"
