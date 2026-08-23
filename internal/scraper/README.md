# Scraper

Go service that seeds Blizzard reference data, then stores EU and US commodity-auction snapshots, WoW Token prices, and TradeSkillMaster regional item snapshots in TimescaleDB.

## Flow

1. Connect to TimescaleDB/PostgreSQL 16 and apply embedded Goose migrations.
2. Read `seeder_status`.
3. Seed unfinished stages in dependency order: `items` → `professions` → one shared `recipes` + `reagents` traversal. Items are bulk-upserted in 10,000-row staging batches. Recipe details use a five-worker pool behind one 20 requests/second limiter; hierarchy and 500-recipe batches use PostgreSQL `COPY`. Recipes listed under multiple categories are logged and the first listing wins.
4. Once the database is seeded and every regional endpoint answers a ping (token index), poll EU and then US forever: each pass re-requests both regions with their stored `Last-Modified` value, repeating every `POLL_WINDOW` (30s default) for as long as the service runs. Each region's WoW Token price (`/data/wow/token/index`) rides along on the same tick.
5. Commit each changed region before requesting the next one; changed token prices commit with the regional `scraper_state.token_last_updated` update in one transaction. Unchanged regions log at Info and keep polling; a region with no change for 10 minutes logs a stale warning, re-warned hourly until data moves. HTTP 429/5xx failures use exponential retry and honor `Retry-After`.
6. Stream auctions into bounded 10,000-row `COPY` batches. Keep all batches and the regional `scraper_state` update in one transaction.
7. Independently poll `https://public-data.tradeskillmaster.com/retail/{region}/region/items.csv` once at startup and every `TSM_POLL_WINDOW` (1h default), plus 0–5 minutes of jitter. EU and US have separate schedules and failure domains because their daily publications can differ. Direct conditional GETs use PostgreSQL-persisted ETag/Last-Modified values; HTTP 304 bodies are not parsed.
8. Stream each changed TSM CSV into bounded 5,000-row `COPY` batches. The rows and transport/source state commit in one transaction. CSV bodies are discarded after commit—only database rows and `tsm_region_item_state` are durable. A restart reloads validators and the last source timestamp from PostgreSQL, avoiding redundant downloads.
9. Store snapshots in one-day TimescaleDB chunks. Move chunks older than one day to columnstore on a daily schedule, segmented by region and item.

## Run locally

```bash
cp internal/scraper/.env.example internal/scraper/.env
# Fill CLIENT_ID and CLIENT_SECRET.
docker compose up --build -d
docker compose logs -f scraper db
```

The scraper polls continuously by default; no scheduling flags are needed. `POLL_WINDOW` controls Blizzard only; `TSM_POLL_WINDOW` controls TSM and defaults to 1h. To bound both loops, set `POLL_START` (delay before polling begins), `POLL_END` (duration of polling), or `SCRAPE_FROM`/`SCRAPE_UNTIL` (absolute RFC3339 period) in [`internal/scraper/.env.example`](.env.example).

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

# TSM state, latest regional snapshots, and storage size
docker compose exec db psql -U coin_catcher -d coin_catcher -c \
  'select region, etag, last_modified, last_source_updated_at, last_successful_poll from tsm_region_item_state order by region'
docker compose exec db psql -U coin_catcher -d coin_catcher -c \
  'select region, count(*), max(source_updated_at) from tsm_region_item_snapshots group by region order by region'
docker compose exec db psql -U coin_catcher -d coin_catcher -c \
  "select pg_size_pretty(hypertable_size('tsm_region_item_snapshots'))"

# Reset local data completely
docker compose down -v
```

Direct process run, with the Compose database already running:

```bash
docker compose up -d db
go run ./cmd/scraper
```

Configuration defaults are in [`internal/scraper/.env.example`](.env.example). Schema changes belong in a new ordered SQL file under [`migrations/`](migrations/); applied versions are tracked in `goose_db_version`.
