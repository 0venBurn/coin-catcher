#!/usr/bin/env bash
# Per-boot startup: bring the PostgreSQL/TimescaleDB cluster online. Package
# installation and role/db provisioning live in install.sh; this only starts
# the already-provisioned cluster and waits until it accepts connections.
set -euo pipefail

PG_VERSION=16

sudo pg_ctlcluster "${PG_VERSION}" main start 2>/dev/null || true

for _ in $(seq 1 30); do
  if pg_isready -h localhost -p 5432 >/dev/null 2>&1; then
    echo "PostgreSQL is ready on :5432"
    exit 0
  fi
  sleep 1
done

echo "postgres did not become ready" >&2
exit 1
