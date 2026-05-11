# ADR 001: Scraper ingest mode for hourly WoW auction dump

## Status
Accepted

## Date
2026-05-11

## Context
The scraper ingests an hourly WoW API auction dump of roughly 50,000 rows per scrape target and writes into TimescaleDB.

We considered two ingestion approaches:

1. Decode-all: download raw JSON, decode full payload in memory, normalize rows, insert in DB batches.
2. Streaming: parse response incrementally and normalize rows directly into DB batches.

The system is deployed as separate services/pods in k3s. Early priority is implementation speed and operational simplicity.

## Decision
Use **decode-all (load full dump into memory)** as the initial production ingestion mode for scraper.

Implementation shape:

- Request hourly dump.
- Decode full JSON payload in memory.
- Normalize rows and write to TimescaleDB in bounded insert batches (e.g. 2,500 rows).
- Use idempotent conflict handling in DB for retries.

## Consequences
### Positive
- Simpler implementation and easier debugging.
- Faster delivery and lower code complexity for v1.
- Likely adequate for current scale (single dump of ~50k rows).

### Negative
- Higher peak memory usage than streaming.
- Less headroom if concurrency grows across many scrape targets.
- Potential OOM risk under tight pod memory limits.

## Mitigations
- Set explicit pod memory requests/limits appropriate for decode-all peaks.
- Keep DB insert batching bounded (do not accumulate all normalized rows before writing).
- Add runtime metrics for payload size, parse time, batch flush time, and memory usage.

## Revisit triggers
Re-evaluate this ADR and switch to streaming ingestion if any occur:

- Pod OOM kills or frequent memory pressure.
- Increased concurrent scrape targets causing high peak memory.
- Significant payload growth beyond current hourly dump size.
- Need for stronger reliability/resume behavior that benefits from streamed/file-backed ingest.

## Notes
This ADR selects ingestion mode only. It does not change idempotency strategy or Timescale schema decisions.