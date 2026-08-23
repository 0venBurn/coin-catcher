package scraper

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

//go:embed migrations/*.sql
var migrations embed.FS

// Migrate applies all pending embedded Goose migrations before any other
// work runs. Migrations ship inside the binary so the container image is
// self-contained and schema state can never drift from the code version —
// no mounted SQL directory to forget. It reuses the scraper's pool via
// stdlib so no separate connection or config is needed.
func Migrate(ctx context.Context, pool *pgxpool.Pool) error {
	db := stdlib.OpenDBFromPool(pool)
	defer db.Close()

	provider, err := migrationProvider(db)
	if err != nil {
		return err
	}
	if _, err := provider.Up(ctx); err != nil {
		return fmt.Errorf("apply migrations: %w", err)
	}
	return nil
}

// migrateDown rolls back one migration for integration verification. Runtime
// startup only calls Migrate; keeping down support here makes tests exercise
// the same embedded files and Goose provider as production.
func migrateDown(ctx context.Context, pool *pgxpool.Pool) error {
	db := stdlib.OpenDBFromPool(pool)
	defer db.Close()
	provider, err := migrationProvider(db)
	if err != nil {
		return err
	}
	if _, err := provider.Down(ctx); err != nil {
		return fmt.Errorf("roll back migration: %w", err)
	}
	return nil
}

func migrationProvider(db *sql.DB) (*goose.Provider, error) {
	migrationFiles, err := fs.Sub(migrations, "migrations")
	if err != nil {
		return nil, fmt.Errorf("open embedded migrations: %w", err)
	}
	provider, err := goose.NewProvider(goose.DialectPostgres, db, migrationFiles)
	if err != nil {
		return nil, fmt.Errorf("initialize migrations: %w", err)
	}
	return provider, nil
}
