package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/0venburn/coin-catcher/scraper"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	if err := run(logger); err != nil {
		logger.Error("scraper stopped", "error", err)
		os.Exit(1)
	}
}

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

	loop := scraper.NewScraper(pool, clients, logger, config)
	return loop.Run(ctx, config.RunOnStart)
}
