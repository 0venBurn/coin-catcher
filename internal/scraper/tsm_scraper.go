package scraper

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	tsmCopyBatchSize = 5_000
	tsmStaleAfter    = 26 * time.Hour
	tsmStaleBackoff  = 6 * time.Hour
	tsmMaximumJitter = 5 * time.Minute
)

var errTSMSnapshotNotNewer = errors.New("TSM snapshot is not newer than committed state")

type tsmState struct {
	validators TSMValidators
	sourceAt   time.Time
	lastWarnAt time.Time
}

// TSMScraper independently schedules each regional public-data feed.
type TSMScraper struct {
	pool       *pgxpool.Pool
	clients    []*TSMClient
	log        *slog.Logger
	schedule   Schedule
	pollWindow time.Duration
	now        func() time.Time
	wait       func(context.Context, time.Duration) bool
	jitter     func(time.Duration) time.Duration
}

func NewTSMScraper(pool *pgxpool.Pool, clients []*TSMClient, logger *slog.Logger, config Config) *TSMScraper {
	return &TSMScraper{
		pool: pool, clients: clients, log: logger, schedule: config.Schedule, pollWindow: config.TSMPollWindow,
		now: time.Now,
		wait: func(ctx context.Context, duration time.Duration) bool {
			timer := time.NewTimer(duration)
			defer timer.Stop()
			select {
			case <-ctx.Done():
				return false
			case <-timer.C:
				return true
			}
		},
		jitter: func(max time.Duration) time.Duration { return time.Duration(rand.Int64N(int64(max) + 1)) },
	}
}

func (s *TSMScraper) Run(ctx context.Context) error {
	var wg sync.WaitGroup
	for _, client := range s.clients {
		client := client
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.runRegion(ctx, client)
		}()
	}
	wg.Wait()
	return nil
}

func (s *TSMScraper) runRegion(ctx context.Context, client *TSMClient) {
	if delay := s.schedule.StartAt.Sub(s.now()); delay > 0 && !s.wait(ctx, delay) {
		return
	}
	if !s.schedule.StopAt.IsZero() && !s.schedule.StopAt.After(s.now()) {
		return
	}

	state, err := s.loadState(ctx, client.region)
	if err != nil {
		s.log.Error("TSM state load failed", "region", client.region, "error", err)
		return
	}
	for {
		started := s.now()
		result, err := s.storeSnapshot(ctx, client, state)
		duration := s.now().Sub(started)
		switch {
		case errors.Is(err, errTSMSnapshotNotNewer):
			s.log.Info("TSM snapshot not newer", "region", client.region, "source_updated_at", state.sourceAt, "duration", duration)
		case err != nil:
			if ctx.Err() != nil {
				return
			}
			s.log.Warn("TSM poll failed", "region", client.region, "error", err, "duration", duration)
		case !result.Changed:
			s.log.Info("TSM data unchanged", "region", client.region, "etag", state.validators.ETag, "last_modified", state.validators.LastModified, "duration", duration)
		case result.Changed:
			state.validators = TSMValidators{ETag: result.ETag, LastModified: result.LastModified}
			state.sourceAt = result.UpdatedAt
			s.log.Info("TSM snapshot stored", "region", client.region, "rows", result.Rows, "source_updated_at", result.UpdatedAt,
				"etag", result.ETag, "last_modified", result.LastModified, "duration", duration)
		}
		s.warnIfTSMStale(client.region, state, s.now())

		delay := s.pollWindow + s.jitter(tsmMaximumJitter)
		if !s.schedule.StopAt.IsZero() && !s.now().Add(delay).Before(s.schedule.StopAt) {
			return
		}
		if !s.wait(ctx, delay) {
			return
		}
	}
}

const sqlLoadTSMState = `SELECT etag, last_modified, last_source_updated_at FROM tsm_region_item_state WHERE region=$1`

func (s *TSMScraper) loadState(ctx context.Context, region string) (*tsmState, error) {
	var etag, modified *string
	var sourceAt *time.Time
	err := s.pool.QueryRow(ctx, sqlLoadTSMState, region).Scan(&etag, &modified, &sourceAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return &tsmState{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read TSM state: %w", err)
	}
	state := &tsmState{}
	if etag != nil {
		state.validators.ETag = *etag
	}
	if modified != nil {
		state.validators.LastModified = *modified
	}
	if sourceAt != nil {
		state.sourceAt = sourceAt.UTC()
	}
	return state, nil
}

func (s *TSMScraper) storeSnapshot(ctx context.Context, client *TSMClient, state *tsmState) (TSMFetchResult, error) {
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

	ingestedAt := s.now().UTC()
	seen := make(map[int32]struct{}, 100_000)
	batch := make([]TSMItem, 0, tsmCopyBatchSize)
	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		if err := ensureTx(); err != nil {
			return err
		}
		_, err := tx.CopyFrom(ctx, pgx.Identifier{"tsm_region_item_snapshots"},
			[]string{"region", "item_id", "source_name", "market_value", "historical", "average_sale_price", "sale_rate", "sold_per_day", "source_updated_at", "ingested_at"},
			pgx.CopyFromSlice(len(batch), func(index int) ([]any, error) {
				item := batch[index]
				return []any{client.region, item.ItemID, item.Name, item.MarketValue, item.Historical, item.AverageSalePrice,
					item.SaleRate, item.SoldPerDay, item.UpdatedAt, ingestedAt}, nil
			}))
		if err != nil {
			return fmt.Errorf("copy TSM snapshot batch: %w", err)
		}
		batch = batch[:0]
		return nil
	}

	result, err := client.Fetch(ctx, state.validators, func(item TSMItem) error {
		if !state.sourceAt.IsZero() && !item.UpdatedAt.After(state.sourceAt) {
			return errTSMSnapshotNotNewer
		}
		if _, duplicate := seen[item.ItemID]; duplicate {
			return fmt.Errorf("duplicate TSM item ID %d", item.ItemID)
		}
		seen[item.ItemID] = struct{}{}
		batch = append(batch, item)
		if len(batch) == tsmCopyBatchSize {
			return flush()
		}
		return nil
	})
	if err != nil {
		return TSMFetchResult{}, err
	}
	polledAt := s.now().UTC()
	if !result.Changed {
		if err := ensureTx(); err != nil {
			return TSMFetchResult{}, err
		}
		_, err = tx.Exec(ctx, `INSERT INTO tsm_region_item_state (region, etag, last_modified, last_source_updated_at, last_successful_poll)
			VALUES ($1,NULL,NULL,NULL,$2) ON CONFLICT (region) DO UPDATE SET last_successful_poll=$2, updated_at=NOW()`, client.region, polledAt)
		if err != nil {
			return TSMFetchResult{}, fmt.Errorf("update unchanged TSM poll state: %w", err)
		}
		if err := tx.Commit(ctx); err != nil {
			return TSMFetchResult{}, fmt.Errorf("commit unchanged TSM poll: %w", err)
		}
		tx = nil
		return result, nil
	}
	if err := flush(); err != nil {
		return TSMFetchResult{}, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO tsm_region_item_state (region, etag, last_modified, last_source_updated_at, last_successful_poll)
		VALUES ($1,$2,$3,$4,$5) ON CONFLICT (region) DO UPDATE SET etag=EXCLUDED.etag, last_modified=EXCLUDED.last_modified,
		last_source_updated_at=EXCLUDED.last_source_updated_at, last_successful_poll=EXCLUDED.last_successful_poll, updated_at=NOW()`,
		client.region, nullableString(result.ETag), nullableString(result.LastModified), result.UpdatedAt, polledAt)
	if err != nil {
		return TSMFetchResult{}, fmt.Errorf("update TSM state: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return TSMFetchResult{}, fmt.Errorf("commit TSM snapshot: %w", err)
	}
	tx = nil
	return result, nil
}

func nullableString(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func (s *TSMScraper) warnIfTSMStale(region string, state *tsmState, now time.Time) {
	if state.sourceAt.IsZero() {
		return
	}
	age := now.Sub(state.sourceAt)
	if age < tsmStaleAfter || (!state.lastWarnAt.IsZero() && now.Sub(state.lastWarnAt) < tsmStaleBackoff) {
		return
	}
	state.lastWarnAt = now
	s.log.Warn("TSM data stale", "region", region, "source_updated_at", state.sourceAt, "age", age.Round(time.Minute))
}
