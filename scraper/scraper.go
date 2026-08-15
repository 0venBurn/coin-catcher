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
	pool         *pgxpool.Pool
	clients      []*BlizzardClient
	log          *slog.Logger
	pollInterval time.Duration
	pollWindow   time.Duration
}

func NewScraper(pool *pgxpool.Pool, clients []*BlizzardClient, logger *slog.Logger, config Config) *Scraper {
	return &Scraper{
		pool: pool, clients: clients, log: logger,
		pollInterval: config.PollInterval, pollWindow: config.PollWindow,
	}
}

func (s *Scraper) Run(ctx context.Context, runOnStart bool) error {
	if runOnStart {
		s.log.Info("startup scrape enabled")
		if err := s.pollWindowForChange(ctx); err != nil && ctx.Err() == nil {
			s.log.Error("startup polling window failed", "error", err)
		}
	}

	for {
		next := nextHalfHour(time.Now())
		s.log.Info("waiting for next scrape window", "starts_at", next)
		timer := time.NewTimer(time.Until(next))
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil
		case <-timer.C:
		}

		if err := s.pollWindowForChange(ctx); err != nil {
			if ctx.Err() != nil {
				return nil
			}
			s.log.Error("scrape polling window failed", "error", err)
		}
	}
}

func nextHalfHour(now time.Time) time.Time {
	next := now.Truncate(time.Hour).Add(30 * time.Minute)
	if next.Before(now) {
		next = next.Add(time.Hour)
	}
	return next
}

func (s *Scraper) pollWindowForChange(ctx context.Context) error {
	pending := make(map[string]string, len(s.clients))
	for _, client := range s.clients {
		lastModified, err := s.lastModified(ctx, client.region)
		if err != nil {
			return err
		}
		pending[client.region] = lastModified
	}

	deadline := time.Now().Add(s.pollWindow)
	attempt := 0
	for {
		attempt++
		// Process regions in configured order. Each changed response is committed
		// before requesting the next region.
		for _, client := range s.clients {
			lastModified, waiting := pending[client.region]
			if !waiting {
				continue
			}
			modified, changed, auctionCount, err := s.storeStreamedSnapshot(ctx, client, lastModified)
			if err != nil {
				s.log.Warn("commodity poll failed", "region", client.region, "attempt", attempt, "error", err)
				continue
			}
			if !changed || modified == lastModified && lastModified != "" {
				s.log.Info("commodity data unchanged", "region", client.region, "attempt", attempt, "last_modified", lastModified)
				continue
			}
			s.log.Info("commodity snapshot stored", "region", client.region, "auctions", auctionCount, "last_modified", modified)
			delete(pending, client.region)
		}
		if len(pending) == 0 {
			return nil
		}

		if !time.Now().Add(s.pollInterval).Before(deadline) {
			regions := make([]string, 0, len(pending))
			for _, client := range s.clients {
				if _, waiting := pending[client.region]; waiting {
					regions = append(regions, client.region)
				}
			}
			s.log.Warn("commodity polling window expired", "attempts", attempt, "regions", regions)
			return nil
		}
		timer := time.NewTimer(s.pollInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

func (s *Scraper) lastModified(ctx context.Context, region string) (string, error) {
	var value *string
	err := s.pool.QueryRow(ctx, `SELECT last_modified FROM scraper_state WHERE region=$1`, region).Scan(&value)
	if err == pgx.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("read scraper state: %w", err)
	}
	if value == nil {
		return "", nil
	}
	return *value, nil
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
