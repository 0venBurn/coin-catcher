#!/usr/bin/env bash
# Idempotent bootstrap for the Coin Catcher dev environment.
# Installs PostgreSQL 16 + TimescaleDB, provisions the coin_catcher role/db to
# match DATABASE_URL, and prepares the Go build. Safe to run repeatedly.
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PG_VERSION=16

install_timescaledb() {
  if dpkg -s "timescaledb-2-postgresql-${PG_VERSION}" >/dev/null 2>&1; then
    return
  fi
  sudo apt-get update -y
  sudo apt-get install -y gnupg postgresql-common apt-transport-https lsb-release wget
  # PostgreSQL Global Development Group repo (provides postgresql-${PG_VERSION}).
  sudo /usr/share/postgresql-common/pgdg/apt.postgresql.org.sh -y
  # TimescaleDB community repo.
  echo "deb https://packagecloud.io/timescale/timescaledb/ubuntu/ $(lsb_release -c -s) main" \
    | sudo tee /etc/apt/sources.list.d/timescaledb.list
  wget --quiet -O - https://packagecloud.io/timescale/timescaledb/gpgkey \
    | gpg --dearmor | sudo tee /etc/apt/trusted.gpg.d/timescaledb.gpg >/dev/null
  sudo apt-get update -y
  sudo apt-get install -y "timescaledb-2-postgresql-${PG_VERSION}" "postgresql-client-${PG_VERSION}"
  # Wire timescaledb into shared_preload_libraries (change requires restart).
  sudo timescaledb-tune --quiet --yes \
    --pg-config="/usr/lib/postgresql/${PG_VERSION}/bin/pg_config"
}

wait_for_postgres() {
  for _ in $(seq 1 30); do
    if pg_isready -h localhost -p 5432 >/dev/null 2>&1; then
      return 0
    fi
    sleep 1
  done
  echo "postgres did not become ready" >&2
  return 1
}

provision_database() {
  sudo pg_ctlcluster "${PG_VERSION}" main start 2>/dev/null || true
  wait_for_postgres
  # coin_catcher is a superuser so migrations can CREATE EXTENSION timescaledb,
  # mirroring the compose container where it owns the database.
  sudo -u postgres psql -v ON_ERROR_STOP=1 -c \
    "DO \$\$ BEGIN IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname='coin_catcher') THEN CREATE ROLE coin_catcher LOGIN SUPERUSER PASSWORD 'coin_catcher'; END IF; END \$\$;"
  if ! sudo -u postgres psql -tAc "SELECT 1 FROM pg_database WHERE datname='coin_catcher'" | grep -q 1; then
    sudo -u postgres createdb -O coin_catcher coin_catcher
  fi
}

install_timescaledb
provision_database

cd "${REPO_ROOT}"
go mod download
go build ./...

# Local .env for `make run`; fill CLIENT_ID / CLIENT_SECRET (see Secrets).
if [ ! -f internal/scraper/.env ]; then
  cp internal/scraper/.env.example internal/scraper/.env
fi

echo "Coin Catcher dev environment ready."
