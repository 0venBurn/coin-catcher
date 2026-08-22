-- +goose Up
-- The TimescaleDB container preloads this extension into the configured database.
CREATE EXTENSION IF NOT EXISTS timescaledb;

CREATE TABLE items (
    id INTEGER PRIMARY KEY,
    name TEXT NOT NULL,
    item_level INTEGER,
    item_class TEXT NOT NULL,
    item_subclass TEXT NOT NULL,
    inventory_type TEXT,
    quality TEXT NOT NULL,
    is_equippable BOOLEAN NOT NULL,
    is_stackable BOOLEAN NOT NULL,
    required_level INTEGER,
    sell_price INTEGER,
    max_stack_size INTEGER,
    metadata_complete BOOLEAN NOT NULL DEFAULT TRUE,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE professions (
    id INTEGER PRIMARY KEY,
    name TEXT NOT NULL,
    description TEXT,
    type_code TEXT,
    type_name TEXT,
    media_id INTEGER,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE profession_skill_tiers (
    profession_id INTEGER NOT NULL REFERENCES professions(id) ON DELETE CASCADE,
    id INTEGER NOT NULL,
    name TEXT NOT NULL,
    minimum_skill_level INTEGER,
    maximum_skill_level INTEGER,
    PRIMARY KEY (profession_id, id)
);

CREATE TABLE profession_categories (
    profession_id INTEGER NOT NULL,
    skill_tier_id INTEGER NOT NULL,
    name TEXT NOT NULL,
    PRIMARY KEY (profession_id, skill_tier_id, name),
    FOREIGN KEY (profession_id, skill_tier_id)
        REFERENCES profession_skill_tiers(profession_id, id) ON DELETE CASCADE
);

CREATE TABLE recipes (
    id INTEGER NOT NULL,
    faction TEXT NOT NULL CHECK (faction IN ('Alliance', 'Horde', 'Neutral')),
    name TEXT NOT NULL,
    description TEXT,
    rank INTEGER,
    media_id INTEGER,
    profession_id INTEGER NOT NULL,
    skill_tier_id INTEGER NOT NULL,
    category_name TEXT NOT NULL,
    crafted_item_id INTEGER REFERENCES items(id),
    crafted_quantity DOUBLE PRECISION,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (id, faction),
    FOREIGN KEY (profession_id, skill_tier_id, category_name)
        REFERENCES profession_categories(profession_id, skill_tier_id, name)
);

CREATE TABLE reagents (
    recipe_id INTEGER NOT NULL,
    recipe_faction TEXT NOT NULL,
    item_id INTEGER NOT NULL REFERENCES items(id),
    quantity INTEGER NOT NULL CHECK (quantity >= 0),
    optional BOOLEAN NOT NULL DEFAULT FALSE,
    PRIMARY KEY (recipe_id, recipe_faction, item_id),
    FOREIGN KEY (recipe_id, recipe_faction) REFERENCES recipes(id, faction) ON DELETE CASCADE
);

CREATE TABLE modified_crafting_slots (
    recipe_id INTEGER NOT NULL,
    recipe_faction TEXT NOT NULL,
    slot_type_id INTEGER NOT NULL,
    display_order INTEGER NOT NULL,
    PRIMARY KEY (recipe_id, recipe_faction, slot_type_id),
    FOREIGN KEY (recipe_id, recipe_faction)
        REFERENCES recipes(id, faction) ON DELETE CASCADE
);

CREATE TABLE seeder_status (
    seeder_type TEXT PRIMARY KEY,
    completed BOOLEAN NOT NULL DEFAULT FALSE,
    completed_at TIMESTAMPTZ,
    records_processed INTEGER NOT NULL DEFAULT 0,
    last_error TEXT,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE scraper_state (
    region TEXT PRIMARY KEY,
    last_modified TEXT,
    last_snapshot_time TIMESTAMPTZ,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE auction_snapshots (
    auction_id BIGINT NOT NULL,
    item_id INTEGER NOT NULL,
    region TEXT NOT NULL,
    unit_price BIGINT NOT NULL,
    quantity INTEGER NOT NULL,
    time_left TEXT NOT NULL,
    snapshot_time TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (region, item_id, auction_id, snapshot_time)
);
SELECT create_hypertable(
    'auction_snapshots',
    by_range('snapshot_time', INTERVAL '1 day')
);
CREATE INDEX idx_auction_snapshots_region_item_time
    ON auction_snapshots (region, item_id, snapshot_time DESC);
ALTER TABLE auction_snapshots SET (
    timescaledb.enable_columnstore = TRUE,
    timescaledb.segmentby = 'region,item_id',
    timescaledb.orderby = 'snapshot_time DESC'
);
CALL add_columnstore_policy(
    'auction_snapshots',
    after => INTERVAL '1 day',
    schedule_interval => INTERVAL '1 day'
);

-- +goose Down
DROP TABLE IF EXISTS auction_snapshots;
DROP TABLE IF EXISTS scraper_state;
DROP TABLE IF EXISTS seeder_status;
DROP TABLE IF EXISTS modified_crafting_slots;
DROP TABLE IF EXISTS reagents;
DROP TABLE IF EXISTS recipes;
DROP TABLE IF EXISTS profession_categories;
DROP TABLE IF EXISTS profession_skill_tiers;
DROP TABLE IF EXISTS professions;
DROP TABLE IF EXISTS items;
