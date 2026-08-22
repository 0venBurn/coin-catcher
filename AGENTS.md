# AGENTS.md

Coin Catcher — monorepo for a WoW commodities market data warehouse. Go service scrapes Blizzard auction house + WoW Token data, stores it in TimescaleDB.

## Layout

- `cmd/scraper/` — entrypoint. Wires logger, pgx pool, and the scraper loop.
- `internal/scraper/` — core package: seeding, polling, API client, schema, config.
  - `blizzard_api_client.go` — Blizzard API HTTP client (rate limiting, retries, `Last-Modified` conditional requests).
  - `seeder.go` — idempotent reference-data seeding: `items` → `professions` → `recipes`/`reagents`.
  - `scraper.go` — continuous EU/US commodity + token polling, transactional batch commits.
  - `migrate.go` — embedded Goose migration runner.
  - `config.go` — env-based configuration (see `.env.example`).
  - `migrations/` — embedded Goose SQL migrations.
  - `Makefile` — build/test helpers.
- `internal/` — internal packages; `internal/scraper/` is currently the only one.
- `docs/` — durable docs. `data-invariants.md` describes storage guarantees.
- `compose.yaml` + `Dockerfile` — local stack: TimescaleDB (pg16) on :5432 + scraper service.

## Run

```bash
cp internal/scraper/.env.example internal/scraper/.env   # fill CLIENT_ID / CLIENT_SECRET
docker compose up --build -d
```

## Notes

- Go 1.23. Deps: `pgx/v5`, `goose/v3`.
- Seeding is idempotent per stage (`seeder_status`). Polling runs forever by default; bound with `POLL_START`/`POLL_END`/`SCRAPE_FROM`/`SCRAPE_UNTIL`.
