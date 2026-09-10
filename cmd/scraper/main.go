package main

// scraper entrypoint: wires logging, the database pool, reference-data
// seeding, and the polling loop, then runs until SIGINT/SIGTERM or a fatal
// error. Startup is deliberately sequential — config, database, schema,
// seed, connectivity probe, poll — so each step proves its precondition
// before the next begins.
import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/0venburn/coin-catcher/internal/scraper"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/sync/errgroup"
)

func main() {
	// Text logs go to stdout; the container runtime owns capture and
	// rotation, so nothing here writes to files.
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	if err := run(logger); err != nil {
		logger.Error("scraper stopped", "error", err)
		os.Exit(1)
	}
}

// run performs startup in dependency order and blocks in the scrape loop
// until the context is cancelled. Database connection retries up to 30 times
// because the db container may still be running its own init when this one
// starts, even with the compose healthcheck.
func run(logger *slog.Logger) error {
	config, err := scraper.LoadConfig()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	var pool *pgxpool.Pool
	for attempt := 1; attempt <= 30; attempt++ {
		pool, err = pgxpool.New(ctx, config.DatabaseURL)
		if err == nil {
			err = pool.Ping(ctx)
		}
		if err == nil {
			break
		}
		if pool != nil {
			pool.Close()
			pool = nil
		}
		// A failed New/Ping can leave a half-built pool behind; drop it so
		// next attempt starts clean instead of leaking connections.
		logger.Warn("database unavailable", "attempt", attempt, "error", err)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
	if pool == nil || err != nil {
		return fmt.Errorf("connect to database after 30 attempts: %w", err)
	}
	defer pool.Close()
	logger.Info("database connected")

	if err := scraper.Migrate(ctx, pool); err != nil {
		return err
	}
	logger.Info("database schema ready")

	httpClient := &http.Client{Timeout: config.RequestTimeout}
	// One shared limiter across all regional clients keeps the combined
	// request rate inside Blizzard's budget regardless of region count.
	limiter := scraper.NewAPIRateLimiter(config.APIRequestsPerSecond)
	clients := make([]*scraper.BlizzardClient, 0, len(config.Regions))
	for _, region := range config.Regions {
		clients = append(clients, scraper.NewBlizzardClient(
			httpClient, config.ClientID, config.ClientSecret, region, limiter,
		))
	}

	// Static profession and item data is shared across regions, so any regional client can seed it.
	seeder := scraper.NewSeeder(pool, clients[0], logger, config.RecipeWorkers)
	if err := seeder.Run(ctx); err != nil {
		return fmt.Errorf("seed database: %w", err)
	}

	// Polling starts only after every regional endpoint answers, so outbound
	// connectivity and credentials are proven before the first snapshot.
	for _, client := range clients {
		if _, err := client.GetTokenIndex(ctx); err != nil {
			return fmt.Errorf("ping %s endpoints: %w", client.Region(), err)
		}
		logger.Info("blizzard api reachable", "region", client.Region())
	}

	// TSM public CSV polling is deliberately separate from Blizzard OAuth,
	// rate limiting, and the fast commodity/token cadence.
	tsmClients := make([]*scraper.TSMClient, 0, len(config.Regions))
	for _, region := range config.Regions {
		tsmClients = append(tsmClients, scraper.NewTSMClient(httpClient, region))
	}

	blizzardLoop := scraper.NewScraper(pool, clients, logger, config)
	tsmLoop := scraper.NewTSMScraper(pool, tsmClients, logger, config)
	group, groupCtx := errgroup.WithContext(ctx)
	group.Go(func() error { return blizzardLoop.Run(groupCtx) })
	group.Go(func() error { return tsmLoop.Run(groupCtx) })
	return group.Wait()
}
