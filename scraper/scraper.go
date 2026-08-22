package scraper

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Scraper struct {
	pool            *pgxpool.Pool
	clients         []*BlizzardClient
	log             *slog.Logger
	pollStartOffset time.Duration
	pollEnd         time.Duration
	pollInterval    time.Duration
	scrapeFrom      time.Time
	scrapeUntil     time.Time
}

func NewScraper(pool *pgxpool.Pool, clients []*BlizzardClient, logger *slog.Logger, config Config) *Scraper {
	return &Scraper{
		pool: pool, clients: clients, log: logger,
		pollStartOffset: config.PollStartOffset, pollEnd: config.PollEnd,
		pollInterval: config.PollInterval,
		scrapeFrom:   config.ScrapeFrom, scrapeUntil: config.ScrapeUntil,
	}
}

// Run polls each region every interval forever (a continuous 24h scraper by
// default), writing snapshots whenever the upstream data changes. POLL_START
// delays the first poll, POLL_END bounds its duration, and SCRAPE_FROM and
// SCRAPE_UNTIL bound it to an absolute date period.
func (s *Scraper) Run(ctx context.Context) error {
	now := time.Now()
	start := now.Add(s.pollStartOffset)
	if s.scrapeFrom.After(start) {
		start = s.scrapeFrom
	}
	var stopAt time.Time
	if s.pollEnd > 0 {
		stopAt = start.Add(s.pollEnd)
	}
	if !s.scrapeUntil.IsZero() && (stopAt.IsZero() || s.scrapeUntil.Before(stopAt)) {
		stopAt = s.scrapeUntil
	}

	if start.After(now) {
		s.log.Info("waiting for scraping to begin", "starts_at", start, "stops_at", optionalTime(stopAt))
		timer := time.NewTimer(time.Until(start))
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil
		case <-timer.C:
		}
	}
	if !stopAt.IsZero() && !stopAt.After(time.Now()) {
		s.log.Info("scrape period already ended", "ended_at", stopAt)
		return nil
	}

	// Per-region state seeds from the database once and then lives in memory; a
	// region is never considered done, so unchanged data is simply re-requested
	// on every pass.
	state := make(map[string]*regionState, len(s.clients))
	for _, client := range s.clients {
		rg, err := s.loadRegionState(ctx, client.region)
		if err != nil {
			return err
		}
		rg.lastChange = time.Now()
		state[client.region] = rg
	}

	for {
		s.pollPass(ctx, state)
		if ctx.Err() != nil {
			return nil
		}
		if !stopAt.IsZero() && !time.Now().Add(s.pollInterval).Before(stopAt) {
			s.log.Info("scrape period ended", "ended_at", stopAt)
			return nil
		}
		timer := time.NewTimer(s.pollInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil
		case <-timer.C:
		}
	}
}

// regionState caches everything known about one region between ticks so a
// tick never reads dedup state back from the database.
type regionState struct {
	lastModified string    // commodities Last-Modified sent as If-Modified-Since
	tokenUpdated time.Time // token endpoint's last_updated_timestamp, for dedup
	lastChange   time.Time // last detected commodity change
	warnedStale  int       // stale warnings already logged
}

func (s *Scraper) loadRegionState(ctx context.Context, region string) (*regionState, error) {
	var lastModified *string
	var tokenUpdated *time.Time
	err := s.pool.QueryRow(ctx,
		`SELECT last_modified, token_last_updated FROM scraper_state WHERE region=$1`, region).
		Scan(&lastModified, &tokenUpdated)
	if err == pgx.ErrNoRows {
		return &regionState{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read scraper state: %w", err)
	}
	rg := &regionState{}
	if lastModified != nil {
		rg.lastModified = *lastModified
	}
	if tokenUpdated != nil {
		rg.tokenUpdated = *tokenUpdated
	}
	return rg, nil
}

// pollPass makes one tick over all regions: each changed commodity snapshot is
// committed before requesting the next region, and each region's token price
// rides along on the same tick.
func (s *Scraper) pollPass(ctx context.Context, state map[string]*regionState) {
	for _, client := range s.clients {
		rg := state[client.region]
		modified, changed, auctionCount, err := s.storeStreamedSnapshot(ctx, client, rg.lastModified)
		if err != nil {
			s.log.Warn("commodity poll failed", "region", client.region, "error", err)
		} else if !changed {
			s.log.Info("commodity data unchanged", "region", client.region, "last_modified", rg.lastModified)
			s.warnIfStale(client.region, rg, time.Now())
		} else {
			rg.lastModified = modified
			rg.lastChange = time.Now()
			rg.warnedStale = 0
			s.log.Info("commodity snapshot stored", "region", client.region, "auctions", auctionCount, "last_modified", modified)
		}
		s.pollToken(ctx, client, rg)
	}
}

// pollToken fetches the region's WoW Token price and stores it when Blizzard's
// own last_updated_timestamp moved. The endpoint has no conditional-request
// support, so the response body timestamp is the change signal.
func (s *Scraper) pollToken(ctx context.Context, client *BlizzardClient, rg *regionState) {
	payload, err := client.GetTokenIndex(ctx)
	if err != nil {
		s.log.Warn("token poll failed", "region", client.region, "error", err)
		return
	}
	updated := time.UnixMilli(payload.LastUpdatedTimestamp).UTC()
	if !rg.tokenUpdated.IsZero() && !updated.After(rg.tokenUpdated) {
		s.log.Info("token price unchanged", "region", client.region, "price", payload.Price, "blizzard_updated_at", updated)
		return
	}
	snapshotTime := time.Now().UTC()
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		s.log.Warn("token store failed", "region", client.region, "error", err)
		return
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx,
		`INSERT INTO wow_tokens (region, price, blizzard_updated_at, snapshot_time) VALUES ($1,$2,$3,$4)`,
		client.region, payload.Price, updated, snapshotTime); err != nil {
		s.log.Warn("token store failed", "region", client.region, "error", err)
		return
	}
	if _, err := tx.Exec(ctx, `UPDATE scraper_state SET token_last_updated=$2, updated_at=NOW() WHERE region=$1`,
		client.region, updated); err != nil {
		s.log.Warn("token store failed", "region", client.region, "error", err)
		return
	}
	if err := tx.Commit(ctx); err != nil {
		s.log.Warn("token store failed", "region", client.region, "error", err)
		return
	}
	rg.tokenUpdated = updated
	s.log.Info("token price stored", "region", client.region, "price", payload.Price, "blizzard_updated_at", updated, "snapshot_time", snapshotTime)
}

// staleWarnThresholds are the no-change durations after which a region logs a
// warning; afterwards one more warning is logged per full additional hour so
// Grafana can alert on persistently stale regions.
var staleWarnThresholds = []time.Duration{10 * time.Minute, 30 * time.Minute, time.Hour}

func staleWarnCount(stale time.Duration) int {
	count := 0
	for _, threshold := range staleWarnThresholds {
		if stale >= threshold {
			count++
		}
	}
	last := staleWarnThresholds[len(staleWarnThresholds)-1]
	if stale >= last {
		count += int((stale - last) / time.Hour)
	}
	return count
}

func (s *Scraper) warnIfStale(region string, rg *regionState, now time.Time) {
	stale := now.Sub(rg.lastChange)
	count := staleWarnCount(stale)
	if count <= rg.warnedStale {
		return
	}
	rg.warnedStale = count
	s.log.Warn("commodity data stale", "region", region, "stale_for", stale.Round(time.Second), "warning", count)
}

func optionalTime(t time.Time) any {
	if t.IsZero() {
		return "never"
	}
	return t
}

const snapshotCopyBatchSize = 10_000

func (s *Scraper) storeStreamedSnapshot(ctx context.Context, client *BlizzardClient, lastModified string) (string, bool, int, error) {
	region := client.region
	var tx pgx.Tx
	defer func() {
		if tx != nil {
			_ = tx.Rollback(ctx)
		}
	}()

	snapshotTime := time.Now().UTC()
	batch := make([]CommodityAuction, 0, snapshotCopyBatchSize)
	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		if tx == nil {
			var err error
			tx, err = s.pool.Begin(ctx)
			if err != nil {
				return err
			}
		}
		_, err := tx.CopyFrom(ctx, pgx.Identifier{"auction_snapshots"},
			[]string{"auction_id", "item_id", "region", "unit_price", "quantity", "time_left", "snapshot_time"},
			pgx.CopyFromSlice(len(batch), func(index int) ([]any, error) {
				auction := batch[index]
				return []any{auction.ID, auction.Item.ID, region, auction.UnitPrice, auction.Quantity, auction.TimeLeft, snapshotTime}, nil
			}))
		if err != nil {
			return fmt.Errorf("copy auction snapshot batch: %w", err)
		}
		batch = batch[:0]
		return nil
	}

	modified, changed, count, err := client.StreamCommodities(ctx, lastModified, func(auction CommodityAuction) error {
		batch = append(batch, auction)
		if len(batch) == snapshotCopyBatchSize {
			return flush()
		}
		return nil
	})
	if err != nil {
		return "", false, 0, err
	}
	if !changed || modified == lastModified && lastModified != "" {
		return modified, false, 0, nil
	}
	if modified == "" {
		return "", false, 0, fmt.Errorf("%s commodities response omitted Last-Modified", region)
	}
	if err := flush(); err != nil {
		return "", false, 0, err
	}
	if tx == nil {
		tx, err = s.pool.Begin(ctx)
		if err != nil {
			return "", false, 0, err
		}
	}
	if _, err := tx.Exec(ctx, `INSERT INTO scraper_state (region, last_modified, last_snapshot_time, updated_at)
		VALUES ($1,$2,$3,NOW()) ON CONFLICT (region) DO UPDATE SET
		last_modified=EXCLUDED.last_modified, last_snapshot_time=EXCLUDED.last_snapshot_time, updated_at=NOW()`,
		region, modified, snapshotTime); err != nil {
		return "", false, 0, fmt.Errorf("update scraper state: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return "", false, 0, fmt.Errorf("commit auction snapshot: %w", err)
	}
	tx = nil
	return modified, true, count, nil
}
