package scraper

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Seeder struct {
	pool          *pgxpool.Pool
	client        *BlizzardClient
	log           *slog.Logger
	recipeWorkers int
}

func NewSeeder(pool *pgxpool.Pool, client *BlizzardClient, logger *slog.Logger, recipeWorkers int) *Seeder {
	if recipeWorkers <= 0 {
		recipeWorkers = 5
	}
	return &Seeder{pool: pool, client: client, log: logger, recipeWorkers: recipeWorkers}
}

func (s *Seeder) Run(ctx context.Context) error {
	itemsDone, err := s.completed(ctx, "items")
	if err != nil {
		return err
	}
	if !itemsDone {
		if err := s.seedItems(ctx); err != nil {
			s.recordError(ctx, "items", err)
			return err
		}
	}

	professionsDone, err := s.completed(ctx, "professions")
	if err != nil {
		return err
	}
	if !professionsDone {
		if err := s.seedProfessions(ctx); err != nil {
			s.recordError(ctx, "professions", err)
			return err
		}
	}

	recipesDone, err := s.completed(ctx, "recipes")
	if err != nil {
		return err
	}
	reagentsDone, err := s.completed(ctx, "reagents")
	if err != nil {
		return err
	}
	if !recipesDone || !reagentsDone {
		if err := s.seedRecipesAndReagents(ctx); err != nil {
			s.recordError(ctx, "recipes", err)
			s.recordError(ctx, "reagents", err)
			return err
		}
	}
	return nil
}

const itemSeedBatchSize = 10_000

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
		if _, err := tx.Exec(ctx, `CREATE TEMP TABLE staged_item_seed (
			id INTEGER, name TEXT, item_level INTEGER, item_class TEXT, item_subclass TEXT,
			inventory_type TEXT, quality TEXT, is_equippable BOOLEAN, is_stackable BOOLEAN,
			required_level INTEGER, sell_price INTEGER, max_stack_size INTEGER
		) ON COMMIT DROP`); err != nil {
			return fmt.Errorf("create item staging table: %w", err)
		}
		columns := []string{"id", "name", "item_level", "item_class", "item_subclass", "inventory_type",
			"quality", "is_equippable", "is_stackable", "required_level", "sell_price", "max_stack_size"}
		if _, err := tx.CopyFrom(ctx, pgx.Identifier{"staged_item_seed"}, columns, pgx.CopyFromRows(rows)); err != nil {
			return fmt.Errorf("copy staged items: %w", err)
		}
		if _, err := tx.Exec(ctx, `INSERT INTO items
			(id, name, item_level, item_class, item_subclass, inventory_type, quality,
			 is_equippable, is_stackable, required_level, sell_price, max_stack_size,
			 metadata_complete, updated_at)
			SELECT id, name, item_level, item_class, item_subclass, inventory_type, quality,
			 is_equippable, is_stackable, required_level, sell_price, max_stack_size, TRUE, NOW()
			FROM staged_item_seed
			ON CONFLICT (id) DO UPDATE SET
			name=EXCLUDED.name, item_level=EXCLUDED.item_level, item_class=EXCLUDED.item_class,
			item_subclass=EXCLUDED.item_subclass, inventory_type=EXCLUDED.inventory_type,
			quality=EXCLUDED.quality, is_equippable=EXCLUDED.is_equippable,
			is_stackable=EXCLUDED.is_stackable, required_level=EXCLUDED.required_level,
			sell_price=EXCLUDED.sell_price, max_stack_size=EXCLUDED.max_stack_size,
			metadata_complete=TRUE, updated_at=NOW()
			WHERE (items.name, items.item_level, items.item_class, items.item_subclass,
				items.inventory_type, items.quality, items.is_equippable, items.is_stackable,
				items.required_level, items.sell_price, items.max_stack_size, items.metadata_complete)
			IS DISTINCT FROM
				(EXCLUDED.name, EXCLUDED.item_level, EXCLUDED.item_class, EXCLUDED.item_subclass,
				 EXCLUDED.inventory_type, EXCLUDED.quality, EXCLUDED.is_equippable,
				 EXCLUDED.is_stackable, EXCLUDED.required_level, EXCLUDED.sell_price,
				 EXCLUDED.max_stack_size, TRUE)`); err != nil {
			return fmt.Errorf("upsert staged items: %w", err)
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
			var inventoryType any
			if item.InventoryType != nil {
				inventoryType = item.InventoryType.Name.English()
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
		var mediaID any
		if detail.Media != nil {
			mediaID = detail.Media.ID
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
	if _, err := tx.Exec(ctx, `
		CREATE TEMP TABLE staged_professions (
			id INTEGER, name TEXT, description TEXT, type_code TEXT, type_name TEXT, media_id INTEGER
		) ON COMMIT DROP;
		CREATE TEMP TABLE staged_profession_tiers (
			profession_id INTEGER, id INTEGER, name TEXT
		) ON COMMIT DROP;`); err != nil {
		return fmt.Errorf("create profession staging tables: %w", err)
	}
	if _, err := tx.CopyFrom(ctx, pgx.Identifier{"staged_professions"},
		[]string{"id", "name", "description", "type_code", "type_name", "media_id"},
		pgx.CopyFromRows(professionRows)); err != nil {
		return fmt.Errorf("copy staged professions: %w", err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO professions
		(id, name, description, type_code, type_name, media_id, updated_at)
		SELECT id, name, description, type_code, type_name, media_id, NOW() FROM staged_professions
		ON CONFLICT (id) DO UPDATE SET name=EXCLUDED.name, description=EXCLUDED.description,
		type_code=EXCLUDED.type_code, type_name=EXCLUDED.type_name,
		media_id=EXCLUDED.media_id, updated_at=NOW()
		WHERE (professions.name, professions.description, professions.type_code,
			professions.type_name, professions.media_id)
		IS DISTINCT FROM (EXCLUDED.name, EXCLUDED.description, EXCLUDED.type_code,
			EXCLUDED.type_name, EXCLUDED.media_id)`); err != nil {
		return fmt.Errorf("upsert staged professions: %w", err)
	}
	if _, err := tx.CopyFrom(ctx, pgx.Identifier{"staged_profession_tiers"},
		[]string{"profession_id", "id", "name"}, pgx.CopyFromRows(tierRows)); err != nil {
		return fmt.Errorf("copy staged profession tiers: %w", err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO profession_skill_tiers (profession_id, id, name)
		SELECT profession_id, id, name FROM staged_profession_tiers
		ON CONFLICT (profession_id, id) DO UPDATE SET name=EXCLUDED.name
		WHERE profession_skill_tiers.name IS DISTINCT FROM EXCLUDED.name`); err != nil {
		return fmt.Errorf("upsert staged profession tiers: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit profession batch: %w", err)
	}
	return s.markCompleted(ctx, "professions", len(professions))
}

func (s *Seeder) completed(ctx context.Context, name string) (bool, error) {
	var completed bool
	err := s.pool.QueryRow(ctx, `SELECT completed FROM seeder_status WHERE seeder_type=$1`, name).Scan(&completed)
	if err == pgx.ErrNoRows {
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

func (s *Seeder) recordError(ctx context.Context, name string, seedErr error) {
	_, err := s.pool.Exec(ctx, `INSERT INTO seeder_status
		(seeder_type, completed, last_error, updated_at) VALUES ($1,FALSE,$2,NOW())
		ON CONFLICT (seeder_type) DO UPDATE SET completed=FALSE, last_error=$2, updated_at=NOW()`, name, seedErr.Error())
	if err != nil {
		s.log.Error("record seeder error", "seeder", name, "error", err)
	}
}
