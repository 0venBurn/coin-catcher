package scraper

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Seeder populates reference data (items, professions, recipes, reagents)
// idempotently before polling starts. Progress is tracked per stage in
// seeder_status so a crashed seed resumes instead of restarting.
type Seeder struct {
	pool          *pgxpool.Pool
	client        *BlizzardClient
	log           *slog.Logger
	recipeWorkers int
}

// NewSeeder builds the seeder; recipeWorkers bounds the concurrent recipe
// detail fetches during the final stage (defaulting to 5 when non-positive).
func NewSeeder(pool *pgxpool.Pool, client *BlizzardClient, logger *slog.Logger, recipeWorkers int) *Seeder {
	if recipeWorkers <= 0 {
		recipeWorkers = 5
	}
	return &Seeder{pool: pool, client: client, log: logger, recipeWorkers: recipeWorkers}
}

// Run executes the seed stages in order; each stage lists the seeder_status
// rows it completes (recipes and reagents share one run).
func (s *Seeder) Run(ctx context.Context) error {
	stages := []struct {
		names []string
		run   func(context.Context) error
	}{
		{[]string{"items"}, s.seedItems},
		{[]string{"professions"}, s.seedProfessions},
		{[]string{"recipes", "reagents"}, s.seedRecipesAndReagents},
	}
	for _, stage := range stages {
		done, err := s.allCompleted(ctx, stage.names)
		if err != nil {
			return err
		}
		if done {
			continue
		}
		if err := stage.run(ctx); err != nil {
			for _, name := range stage.names {
				s.recordError(ctx, name, err)
			}
			return err
		}
	}
	return nil
}

// allCompleted reports whether every named seeder_status row is complete.
func (s *Seeder) allCompleted(ctx context.Context, names []string) (bool, error) {
	for _, name := range names {
		completed, err := s.completed(ctx, name)
		if err != nil || !completed {
			return false, err
		}
	}
	return true, nil
}

// itemSeedBatchSize is how many item rows accumulate before a staged
// upsert commits; bounded batches keep transactions and memory small while
// scanning hundreds of thousands of items.
const itemSeedBatchSize = 10_000

// seedItems scans the search endpoint in id order until exhausted, staging
// rows and committing them in batches. startingID always advances to
// lastID+1 rather than trusting page size, so gaps or reordering upstream
// cannot loop forever.
func (s *Seeder) seedItems(ctx context.Context) error {
	s.log.Info("seeding items", "batch_size", itemSeedBatchSize)
	startingID, total, lastID := 1, 0, 0
	rows := make([][]any, 0, itemSeedBatchSize)
	flush := func() error {
		if len(rows) == 0 {
			return nil
		}
		tx, err := s.pool.Begin(ctx)
		if err != nil {
			return err
		}
		defer tx.Rollback(ctx)
		err = execStagedLoads(ctx, tx, []stagedLoad{{
			label:   "staged items",
			table:   "staged_item_seed",
			create:  createStagedItemSeed,
			columns: colsStagedItemSeed,
			rows:    rows,
			upsert:  upsertItemMetadata,
		}})
		if err != nil {
			return err
		}
		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("commit item batch: %w", err)
		}
		total += len(rows)
		rows = rows[:0]
		s.log.Info("item seed progress", "records", total, "last_id", lastID)
		return nil
	}

	for {
		page, err := s.client.SearchItems(ctx, startingID, 1000)
		if err != nil {
			return fmt.Errorf("search items from id %d: %w", startingID, err)
		}
		if len(page.Results) == 0 {
			break
		}
		for _, result := range page.Results {
			item := result.Data
			var inventoryType *string
			if item.InventoryType != nil {
				name := item.InventoryType.Name.English()
				inventoryType = &name
			}
			rows = append(rows, []any{
				item.ID, item.Name.English(), item.Level, item.ItemClass.Name.English(),
				item.ItemSubclass.Name.English(), inventoryType, item.Quality.Name.English(),
				item.IsEquippable, item.IsStackable, item.RequiredLevel, item.SellPrice, item.MaxCount,
			})
			if item.ID > lastID {
				lastID = item.ID
			}
		}
		startingID = lastID + 1
		if len(rows) >= itemSeedBatchSize {
			if err := flush(); err != nil {
				return err
			}
		}
	}
	if err := flush(); err != nil {
		return err
	}
	return s.markCompleted(ctx, "items", total)
}

// seedProfessions loads every profession plus its skill tiers in one pass.
// tierRows is pre-sized generously because most professions have on the
// order of a dozen tiers; both tables commit in a single transaction so
// tiers never reference a missing profession.
func (s *Seeder) seedProfessions(ctx context.Context) error {
	s.log.Info("seeding professions")
	professions, err := s.client.GetProfessions(ctx)
	if err != nil {
		return fmt.Errorf("get professions: %w", err)
	}
	professionRows := make([][]any, 0, len(professions))
	tierRows := make([][]any, 0, len(professions)*12)
	for _, ref := range professions {
		detail, err := s.client.GetProfession(ctx, ref.ID)
		if err != nil {
			return fmt.Errorf("get profession %d: %w", ref.ID, err)
		}
		var mediaID *int
		if detail.Media != nil {
			mediaID = &detail.Media.ID
		}
		professionRows = append(professionRows, []any{
			detail.ID, detail.Name, detail.Description, detail.Type.Type, detail.Type.Name, mediaID,
		})
		for _, tier := range detail.SkillTiers {
			tierRows = append(tierRows, []any{detail.ID, tier.ID, tier.Name})
		}
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	err = execStagedLoads(ctx, tx, []stagedLoad{
		{
			label:   "staged professions",
			table:   "staged_professions",
			create:  createStagedProfessions,
			columns: colsStagedProfessions,
			rows:    professionRows,
			upsert:  upsertProfession,
		},
		{
			label:   "staged profession tiers",
			table:   "staged_profession_tiers",
			create:  createStagedProfessionTiers,
			columns: colsStagedProfessionTiers,
			rows:    tierRows,
			upsert:  upsertProfessionTier,
		},
	})
	if err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit profession batch: %w", err)
	}
	return s.markCompleted(ctx, "professions", len(professions))
}

// completed reports whether the named seeder already finished; ErrNoRows
// means "never run", which counts as not complete.
func (s *Seeder) completed(ctx context.Context, name string) (bool, error) {
	var completed bool
	err := s.pool.QueryRow(ctx, `SELECT completed FROM seeder_status WHERE seeder_type=$1`, name).Scan(&completed)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("check %s seeder status: %w", name, err)
	}
	if completed {
		s.log.Info("seeder already complete", "seeder", name)
	}
	return completed, nil
}

// markCompleted records success with the processed count, clearing any
// previous last_error in the same statement.
func (s *Seeder) markCompleted(ctx context.Context, name string, count int) error {
	_, err := s.pool.Exec(ctx, `INSERT INTO seeder_status
		(seeder_type, completed, completed_at, records_processed, last_error, updated_at)
		VALUES ($1,TRUE,NOW(),$2,NULL,NOW()) ON CONFLICT (seeder_type) DO UPDATE SET
		completed=TRUE, completed_at=NOW(), records_processed=$2, last_error=NULL, updated_at=NOW()`, name, count)
	if err == nil {
		s.log.Info("seeder complete", "seeder", name, "records", count)
	}
	return err
}

// recordError persists the failure reason for observability; a failure to
// even record it is logged but must not mask the original seed error, so
// seedErr keeps propagating via the caller's return.
func (s *Seeder) recordError(ctx context.Context, name string, seedErr error) {
	_, err := s.pool.Exec(ctx, `INSERT INTO seeder_status
		(seeder_type, completed, last_error, updated_at) VALUES ($1,FALSE,$2,NOW())
		ON CONFLICT (seeder_type) DO UPDATE SET completed=FALSE, last_error=$2, updated_at=NOW()`, name, seedErr.Error())
	if err != nil {
		s.log.Error("record seeder error", "seeder", name, "error", err)
	}
}
