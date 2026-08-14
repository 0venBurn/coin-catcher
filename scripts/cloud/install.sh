#!/usr/bin/env bash
# Cloud Agent install phase: idempotent dependency refresh for the monorepo.
# Safe to run repeatedly and against branches where some services are absent.
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$REPO_ROOT"

export PATH="$HOME/.local/bin:$PATH"

# uv is expected to live in the base snapshot; install it if missing so the
# script also works on a plain base image.
if ! command -v uv >/dev/null 2>&1; then
  echo "==> installing uv"
  curl -LsSf https://astral.sh/uv/install.sh | sh
fi

# Python services managed by uv.
for svc in cc-scraper cc-api cc-backtest; do
  if [ -f "$svc/pyproject.toml" ]; then
    echo "==> uv sync ($svc)"
    (cd "$svc" && uv sync)
  fi
done

# Frontend managed by pnpm.
if [ -f "cc-app/package.json" ]; then
  echo "==> pnpm install (cc-app)"
  (cd cc-app && pnpm install)
fi

echo "==> install complete"
