-- +goose Up
CREATE TABLE wow_tokens (
    region              TEXT NOT NULL,
    price               BIGINT NOT NULL CHECK (price > 0),
    blizzard_updated_at TIMESTAMPTZ,
    snapshot_time       TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (region, snapshot_time)
);

SELECT create_hypertable(
    'wow_tokens',
    by_range('snapshot_time', INTERVAL '1 day')
);
CREATE INDEX idx_wow_tokens_region_time
    ON wow_tokens (region, snapshot_time DESC);
ALTER TABLE wow_tokens SET (
    timescaledb.enable_columnstore = TRUE,
    timescaledb.segmentby = 'region',
    timescaledb.orderby = 'snapshot_time DESC'
);
CALL add_columnstore_policy(
    'wow_tokens',
    after => INTERVAL '1 day',
    schedule_interval => INTERVAL '1 day'
);

ALTER TABLE scraper_state ADD COLUMN token_last_updated TIMESTAMPTZ;

-- +goose Down
ALTER TABLE scraper_state DROP COLUMN IF EXISTS token_last_updated;
DROP TABLE IF EXISTS wow_tokens;
