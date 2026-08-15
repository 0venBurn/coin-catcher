# Scraper

Go service that seeds Blizzard reference data, then stores EU and US commodity-auction snapshots in TimescaleDB.

## Flow

1. Connect to TimescaleDB/PostgreSQL 16 and apply embedded Goose migrations.
2. Read `seeder_status`.
3. Seed unfinished stages in dependency order: `items` → `professions` → one shared `recipes` + `reagents` traversal. Items are bulk-upserted in 10,000-row staging batches. Recipe details use a five-worker pool behind one 20 requests/second limiter; hierarchy and 500-recipe batches use PostgreSQL `COPY`.
4. At `HH:30`, poll EU and then US using each region's stored `Last-Modified` value.
5. Commit each changed region before requesting the next one. Unchanged regions retry every 30 seconds for at most 20 minutes. HTTP 429/5xx failures use exponential retry and honor `Retry-After`.
6. Stream auctions into bounded 10,000-row `COPY` batches. Keep all batches and the regional `scraper_state` update in one transaction.
7. Store snapshots in one-day TimescaleDB chunks. Move chunks older than one day to columnstore on a daily schedule, segmented by region and item.

## Run locally

```bash
cp scraper/.env.example scraper/.env
# Fill CLIENT_ID and CLIENT_SECRET.
docker compose up --build -d
docker compose logs -f scraper db
```

Use `SCRAPE_ON_START=true docker compose up --build -d` to take an immediate snapshot instead of waiting for the next half-hour boundary.

The initial seed is large. It is idempotent; completed stages do not run again. `RECIPE_WORKERS` defaults to 5 and accepts 1–8 for benchmarking. `API_REQUESTS_PER_SECOND` cannot exceed 20.

## Inspect

```bash
# Seeder state
docker compose exec db psql -U coin_catcher -d coin_catcher -c \
  'select * from seeder_status order by seeder_type'

# Table counts and latest scrape state
docker compose exec db psql -U coin_catcher -d coin_catcher -c \
  'select (select count(*) from items) items, (select count(*) from professions) professions, (select count(*) from recipes) recipes, (select count(*) from reagents) reagents, (select count(*) from auction_snapshots) auctions'
docker compose exec db psql -U coin_catcher -d coin_catcher -c \
  'select * from scraper_state'

# Reset local data completely
docker compose down -v
```

Direct process run, with the Compose database already running:

```bash
docker compose up -d db
go run ./cmd/scraper
```

Configuration defaults are in [`scraper/.env.example`](.env.example). Schema changes belong in a new ordered SQL file under [`migrations/`](migrations/); applied versions are tracked in `goose_db_version`.
