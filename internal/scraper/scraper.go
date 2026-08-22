package scraper

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Scraper runs the continuous polling loop over all regional clients,
// committing snapshots transactionally as upstream data changes.
type Scraper struct {
	pool       *pgxpool.Pool
	clients    []*BlizzardClient
	log        *slog.Logger
	schedule   Schedule // resolved [StartAt, StopAt] window; zero StopAt polls forever
	pollWindow time.Duration
}

// NewScraper wires the polling loop; schedule and pollWindow come from
// configuration so runs can be bounded in production without code changes.
func NewScraper(pool *pgxpool.Pool, clients []*BlizzardClient, logger *slog.Logger, config Config) *Scraper {
	return &Scraper{
		pool: pool, clients: clients, log: logger,
		schedule: config.Schedule, pollWindow: config.PollWindow,
	}
}

// Run waits for the configured window to open, then polls each region every
// POLL_WINDOW until the window closes or the context is cancelled, writing
// snapshots whenever the upstream data changes.
func (s *Scraper) Run(ctx context.Context) error {
	if s.schedule.StartAt.After(time.Now()) {
		s.log.Info("waiting for scraping to begin", "starts_at", s.schedule.StartAt, "stops_at", logTime(s.schedule.StopAt))
		timer := time.NewTimer(time.Until(s.schedule.StartAt))
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil
		case <-timer.C:
		}
	}
	if !s.schedule.StopAt.IsZero() && !s.schedule.StopAt.After(time.Now()) {
		s.log.Info("scrape period already ended", "ended_at", s.schedule.StopAt)
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
		if !s.schedule.StopAt.IsZero() && !time.Now().Add(s.pollWindow).Before(s.schedule.StopAt) {
			s.log.Info("scrape period ended", "ended_at", s.schedule.StopAt)
			return nil
		}
		timer := time.NewTimer(s.pollWindow)
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
	lastWarnAt   time.Time // last "commodity data stale" warning, for backoff
}

// scraper_state SQL. The table's shape is shared by these three statements
// (and only these) — change them together.
const (
	sqlLoadRegionState   = `SELECT last_modified, token_last_updated FROM scraper_state WHERE region=$1`
	sqlUpsertRegionState = `INSERT INTO scraper_state (region, last_modified, last_snapshot_time, updated_at)
		VALUES ($1,$2,$3,NOW()) ON CONFLICT (region) DO UPDATE SET
		last_modified=EXCLUDED.last_modified, last_snapshot_time=EXCLUDED.last_snapshot_time, updated_at=NOW()`
	sqlUpdateTokenState = `UPDATE scraper_state SET token_last_updated=$2, updated_at=NOW() WHERE region=$1`
)

// loadRegionState reads one region's dedup state; both columns are nullable,
// hence the pointer scan before copying into the plain struct fields.
func (s *Scraper) loadRegionState(ctx context.Context, region string) (*regionState, error) {
	var lastModified *string
	var tokenUpdated *time.Time
	err := s.pool.QueryRow(ctx, sqlLoadRegionState, region).
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
		stored, err := s.storeStreamedSnapshot(ctx, client, rg.lastModified)
		switch {
		case err != nil:
			s.log.Warn("commodity poll failed", "region", client.region, "error", err)
		case stored == nil:
			s.log.Info("commodity data unchanged", "region", client.region, "last_modified", rg.lastModified)
			s.warnIfStale(client.region, rg, time.Now())
		default:
			rg.lastModified = stored.modified
			rg.lastChange = time.Now()
			rg.lastWarnAt = time.Time{}
			s.log.Info("commodity snapshot stored", "region", client.region, "auctions", stored.auctions, "last_modified", stored.modified)
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
	if err := s.storeToken(ctx, client.region, payload, updated, snapshotTime); err != nil {
		s.log.Warn("token store failed", "region", client.region, "error", err)
		return
	}
	rg.tokenUpdated = updated
	s.log.Info("token price stored", "region", client.region, "price", payload.Price, "blizzard_updated_at", updated, "snapshot_time", snapshotTime)
}

// storeToken atomically records a token price row and advances the region's
// dedup timestamp.
func (s *Scraper) storeToken(ctx context.Context, region string, payload TokenIndexResponse, updated, snapshotTime time.Time) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx,
		`INSERT INTO wow_tokens (region, price, blizzard_updated_at, snapshot_time) VALUES ($1,$2,$3,$4)`,
		region, payload.Price, updated, snapshotTime); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, sqlUpdateTokenState, region, updated); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// A region is warned as stale after 10 minutes without an upstream change,
// then re-warned once per hour so Grafana can alert on persistently stale
// regions.
const (
	staleWarnAfter   = 10 * time.Minute
	staleWarnBackoff = time.Hour
)

func (s *Scraper) warnIfStale(region string, rg *regionState, now time.Time) {
	stale := now.Sub(rg.lastChange)
	if stale < staleWarnAfter || now.Sub(rg.lastWarnAt) < staleWarnBackoff {
		return
	}
	rg.lastWarnAt = now
	s.log.Warn("commodity data stale", "region", region, "stale_for", stale.Round(time.Second))
}

// logTime formats a possibly-zero time for logging: zero renders as
// "never" (string) instead of a timestamp. Deliberate any-return for slog.
func logTime(t time.Time) any {
	if t.IsZero() {
		return "never"
	}
	return t
}

// snapshotCopyBatchSize is the streaming COPY batch size: large enough that
// per-batch round trips are negligible for ~200k auctions, small enough that
// one buffered batch stays a few MB.
const snapshotCopyBatchSize = 10_000

// storedSnapshot reports a committed commodity snapshot; the zero-value
// pointer (nil) means upstream data was unchanged.
type storedSnapshot struct {
	modified string
	auctions int
}

// storeStreamedSnapshot fetches commodities with If-Modified-Since and copies
// any new auctions into auction_snapshots together with the scraper_state
// last_modified update, all in one transaction — so the state pointer never
// advertises data that failed to commit, and a crash mid-snapshot leaves no
// half-written rows. Returns nil when nothing changed. The transaction is
// opened lazily on first flush because an unchanged 304 must not hold one
// open at all.
func (s *Scraper) storeStreamedSnapshot(ctx context.Context, client *BlizzardClient, lastModified string) (*storedSnapshot, error) {
	region := client.region
	snapshotTime := time.Now().UTC()

	var tx pgx.Tx
	defer func() {
		if tx != nil {
			_ = tx.Rollback(ctx)
		}
	}()
	ensureTx := func() error {
		if tx != nil {
			return nil
		}
		begun, err := s.pool.Begin(ctx)
		if err != nil {
			return err
		}
		tx = begun
		return nil
	}

	batch := make([]CommodityAuction, 0, snapshotCopyBatchSize)
	// flush copies the pending batch and resets it; reusing the slice keeps
	// allocation flat across hundreds of thousands of auctions.
	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		if err := ensureTx(); err != nil {
			return err
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
		return nil, err
	}
	// A 304 already comes back as !changed; the second clause also skips a 200
	// whose Last-Modified matches what we sent, which carries no new data.
	if !changed || (lastModified != "" && modified == lastModified) {
		return nil, nil
	}
	if modified == "" {
		return nil, fmt.Errorf("%s commodities response omitted Last-Modified", region)
	}
	if err := flush(); err != nil {
		return nil, err
	}
	if err := ensureTx(); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, sqlUpsertRegionState,
		region, modified, snapshotTime); err != nil {
		return nil, fmt.Errorf("update scraper state: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit auction snapshot: %w", err)
	}
	tx = nil
	return &storedSnapshot{modified: modified, auctions: count}, nil
}
