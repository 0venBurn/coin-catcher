-- +goose Up
CREATE TABLE tsm_region_item_snapshots (
    region            TEXT NOT NULL CHECK (region IN ('eu', 'us')),
    item_id           INTEGER NOT NULL CHECK (item_id > 0),
    source_name       TEXT,
    market_value      BIGINT NOT NULL CHECK (market_value >= 0),
    historical        BIGINT NOT NULL CHECK (historical >= 0),
    average_sale_price BIGINT NOT NULL CHECK (average_sale_price >= 0),
    sale_rate         DOUBLE PRECISION NOT NULL CHECK (sale_rate >= 0 AND sale_rate <= 1),
    sold_per_day      DOUBLE PRECISION NOT NULL CHECK (sold_per_day >= 0),
    source_updated_at TIMESTAMPTZ NOT NULL,
    ingested_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (region, item_id, source_updated_at)
);

SELECT create_hypertable(
    'tsm_region_item_snapshots',
    by_range('source_updated_at', INTERVAL '1 day')
);
CREATE INDEX idx_tsm_region_items_region_item_time
    ON tsm_region_item_snapshots (region, item_id, source_updated_at DESC);
ALTER TABLE tsm_region_item_snapshots SET (
    timescaledb.enable_columnstore = TRUE,
    timescaledb.segmentby = 'region,item_id',
    timescaledb.orderby = 'source_updated_at DESC'
);
CALL add_columnstore_policy(
    'tsm_region_item_snapshots',
    after => INTERVAL '1 day',
    schedule_interval => INTERVAL '1 day'
);

CREATE TABLE tsm_region_item_state (
    region                 TEXT PRIMARY KEY CHECK (region IN ('eu', 'us')),
    etag                   TEXT,
    last_modified          TEXT,
    last_source_updated_at TIMESTAMPTZ,
    last_successful_poll   TIMESTAMPTZ NOT NULL,
    updated_at             TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- +goose Down
DROP TABLE IF EXISTS tsm_region_item_state;
DROP TABLE IF EXISTS tsm_region_item_snapshots;
