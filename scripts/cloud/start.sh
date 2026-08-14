#!/usr/bin/env bash
# Cloud Agent start phase: bring up per-boot runtime state (PostgreSQL) and
# reconcile the app database. Idempotent and safe to re-run.
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$REPO_ROOT"

export PATH="$HOME/.local/bin:$PATH"

DB_USER="${CC_DB_USER:-wowuser}"
DB_PASS="${CC_DB_PASSWORD:-wowpass}"
DB_NAME="${CC_DB_NAME:-coin_catcher}"
DB_URL="postgres://${DB_USER}:${DB_PASS}@localhost:5432/${DB_NAME}"

# 1. Ensure PostgreSQL is running (no systemd in the VM; use pg_ctlcluster).
if command -v pg_lsclusters >/dev/null 2>&1; then
  if ! pg_lsclusters -h 2>/dev/null | awk '{print $4}' | grep -q '^online$'; then
    echo "==> starting PostgreSQL cluster"
    sudo pg_ctlcluster 16 main start || true
  fi
  # Wait for readiness.
  for _ in $(seq 1 30); do
    if pg_isready -h localhost -p 5432 >/dev/null 2>&1; then break; fi
    sleep 1
  done
fi

# 2. Ensure the application role and database exist (idempotent).
if command -v psql >/dev/null 2>&1; then
  sudo -u postgres psql -v ON_ERROR_STOP=1 <<SQL || true
DO \$\$ BEGIN
  IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname='${DB_USER}') THEN
    CREATE ROLE ${DB_USER} LOGIN PASSWORD '${DB_PASS}';
  END IF;
END \$\$;
SQL
  if ! sudo -u postgres psql -tAc "SELECT 1 FROM pg_database WHERE datname='${DB_NAME}'" | grep -q 1; then
    sudo -u postgres createdb -O "${DB_USER}" "${DB_NAME}"
  fi
fi

# 3. Ensure the frontend has a local dev .env (gitignored, so created per boot).
if [ -d cc-app ] && [ ! -f cc-app/.env ]; then
  echo "==> writing cc-app/.env"
  SECRET="${BETTER_AUTH_SECRET:-$(head -c 32 /dev/urandom | base64 | tr -dc 'A-Za-z0-9' | head -c 32)}"
  cat > cc-app/.env <<ENV
DATABASE_URL="${DB_URL}"
ORIGIN="http://localhost:5173"
BETTER_AUTH_SECRET="${SECRET}"
ENV
fi

# 4. Apply the Drizzle schema to the database (idempotent, non-interactive).
if [ -f cc-app/package.json ] && [ -d cc-app/node_modules ]; then
  echo "==> drizzle db:push"
  (cd cc-app && pnpm exec drizzle-kit push --force)
fi

echo "==> start complete"
