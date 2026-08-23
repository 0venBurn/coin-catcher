# TSM Scraper Operations

## Resource profile

A live EU+US import was measured on 2026-08-23 against the public TSM feeds and a disposable Compose TimescaleDB database:

| Region | Rows | Source timestamp | Import duration |
| --- | ---: | --- | ---: |
| EU | 31,098 | 2026-08-22 02:17:32Z | 0.54s |
| US | 29,802 | 2026-08-22 10:26:24Z | 0.38s |

Combined results:

- 60,900 rows
- 13,787,136 bytes (approximately 13 MB) from `hypertable_size('tsm_region_item_snapshots')`
- 33,473,640-byte peak Go heap while importing both regions sequentially in the integration-test process
- 9.98 MiB scraper-container memory observed immediately after the production import
- Production import durations: EU 0.61s, US 0.36s

The test process measurement includes the Go test harness. The scraper does not retain complete CSV rows: it retains one 5,000-row COPY batch and a set of item IDs used to reject duplicates. Response bodies are streamed and discarded.

Run the live resource/contract check against a disposable database:

```bash
TSM_LIVE_TEST=1 \
TEST_DATABASE_URL='postgres://coin_catcher:coin_catcher@localhost:5432/coin_catcher_tsm_test?sslmode=disable' \
go test ./internal/scraper -run TestTSMLiveImport -v -count=1
```

## Recovery and inspection

ETag, Last-Modified, the last committed source timestamp, and the last successful poll are durable in `tsm_region_item_state`. On process or container restart, each regional loop reloads this row before its startup poll. A 304 updates only `last_successful_poll`; it does not create snapshot rows or parse a body.

```sql
SELECT region, etag, last_modified, last_source_updated_at, last_successful_poll
FROM tsm_region_item_state
ORDER BY region;

SELECT region, count(*) AS rows, max(source_updated_at) AS latest_source
FROM tsm_region_item_snapshots
GROUP BY region
ORDER BY region;

SELECT pg_size_pretty(hypertable_size('tsm_region_item_snapshots'));
```

A parse, duplicate-ID, mixed-timestamp, COPY, cancellation, or commit failure rolls back both rows and state. Do not manually advance validators after a failed import; the unchanged committed validators ensure the changed body is requested again.

## Rollback

The migration down path is covered by the disposable-database integration test and followed by a successful reapply. Before a production migration, create and verify a database backup according to the deployment environment's private runbook.

Application rollback procedure:

1. Stop only the scraper: `docker compose stop scraper`.
2. Check out the prior application revision and rebuild the scraper image.
3. If schema rollback is required, run Goose down for migration 3. This drops only `tsm_region_item_state` and `tsm_region_item_snapshots`; migrations 1–2 and Blizzard data remain intact.
4. Start the scraper and verify migration versions plus Blizzard commodity/token logs.

Never use `docker compose down -v` for an application rollback because it deletes the database volume.
