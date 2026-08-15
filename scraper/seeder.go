package scraper

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Seeder struct {
	pool          *pgxpool.Pool
	client        *BlizzardClient
	log           *slog.Logger
	recipeWorkers int
}

func NewSeeder(pool *pgxpool.Pool, client *BlizzardClient, logger *slog.Logger, workers ...int) *Seeder {
	recipeWorkers := 5
	if len(workers) > 0 && workers[0] > 0 {
		recipeWorkers = workers[0]
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

const recipeSeedBatchSize = 500

type recipeSeedRecord struct {
	profession ProfessionResponse
	tier       SkillTierResponse
	category   string
	recipe     RecipeResponse
}

type recipeFetchJob struct {
	profession ProfessionResponse
	tier       SkillTierResponse
	category   string
	recipeID   int
}

type recipeFetchResult struct {
	record recipeSeedRecord
	err    error
}

func (s *Seeder) seedRecipesAndReagents(ctx context.Context) error {
	s.log.Info("seeding recipes and reagents", "batch_size", recipeSeedBatchSize, "workers", s.recipeWorkers)
	professions, err := s.client.GetProfessions(ctx)
	if err != nil {
		return err
	}

	seen := make(map[int]recipeFetchJob)
	jobs := make([]recipeFetchJob, 0, 12_000)
	tierRows := make([][]any, 0, len(professions)*12)
	categoryRows := make([][]any, 0, len(professions)*64)
	for _, professionRef := range professions {
		profession, err := s.client.GetProfession(ctx, professionRef.ID)
		if err != nil {
			return fmt.Errorf("get profession %d: %w", professionRef.ID, err)
		}
		for _, tierRef := range profession.SkillTiers {
			tier, err := s.client.GetSkillTier(ctx, profession.ID, tierRef.ID)
			if err != nil {
				return fmt.Errorf("get skill tier %d/%d: %w", profession.ID, tierRef.ID, err)
			}
			tierRows = append(tierRows, []any{
				profession.ID, tier.ID, tier.Name, tier.MinimumSkillLevel, tier.MaximumSkillLevel,
			})
			for _, category := range tier.Categories {
				categoryRows = append(categoryRows, []any{profession.ID, tier.ID, category.Name})
				for _, recipeRef := range category.Recipes {
					job := recipeFetchJob{profession, tier, category.Name, recipeRef.ID}
					if previous, duplicate := seen[recipeRef.ID]; duplicate {
						return fmt.Errorf(
							"recipe %d appears in both %d/%d/%q and %d/%d/%q",
							recipeRef.ID,
							previous.profession.ID, previous.tier.ID, previous.category,
							profession.ID, tier.ID, category.Name,
						)
					}
					seen[recipeRef.ID] = job
					jobs = append(jobs, job)
				}
			}
		}
	}
	if err := s.storeRecipeHierarchy(ctx, tierRows, categoryRows); err != nil {
		return err
	}

	workerCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	jobCh := make(chan recipeFetchJob)
	results := make(chan recipeFetchResult, recipeSeedBatchSize)
	var workers sync.WaitGroup
	for range s.recipeWorkers {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for job := range jobCh {
				recipe, err := s.client.GetRecipe(workerCtx, job.recipeID)
				result := recipeFetchResult{
					record: recipeSeedRecord{job.profession, job.tier, job.category, recipe},
				}
				if err != nil {
					result.err = fmt.Errorf("get recipe %d: %w", job.recipeID, err)
				}
				select {
				case results <- result:
				case <-workerCtx.Done():
					return
				}
				if err != nil {
					cancel()
					return
				}
			}
		}()
	}
	go func() {
		defer close(jobCh)
		for _, job := range jobs {
			select {
			case jobCh <- job:
			case <-workerCtx.Done():
				return
			}
		}
	}()
	go func() {
		workers.Wait()
		close(results)
	}()

	batch := make([]recipeSeedRecord, 0, recipeSeedBatchSize)
	recipeCount, reagentCount := 0, 0
	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		started := time.Now()
		recipes, reagents, err := s.storeRecipeBatch(ctx, batch)
		if err != nil {
			return err
		}
		recipeCount += recipes
		reagentCount += reagents
		s.log.Info("recipe seed batch committed", "batch_size", len(batch), "recipes", recipeCount,
			"reagents", reagentCount, "duration", time.Since(started))
		batch = batch[:0]
		return nil
	}

	var fetchErr error
	for result := range results {
		if result.err != nil {
			if fetchErr == nil {
				fetchErr = result.err
				cancel()
			}
			continue
		}
		if fetchErr != nil {
			continue
		}
		batch = append(batch, result.record)
		if len(batch) == recipeSeedBatchSize {
			if err := flush(); err != nil {
				cancel()
				return err
			}
		}
	}
	if fetchErr != nil {
		return fetchErr
	}
	if err := flush(); err != nil {
		return err
	}
	if err := s.markCompleted(ctx, "recipes", recipeCount); err != nil {
		return err
	}
	return s.markCompleted(ctx, "reagents", reagentCount)
}

func (s *Seeder) storeRecipeHierarchy(ctx context.Context, tierRows, categoryRows [][]any) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `
		CREATE TEMP TABLE staged_tier_details (
			profession_id INTEGER, id INTEGER, name TEXT,
			minimum_skill_level INTEGER, maximum_skill_level INTEGER
		) ON COMMIT DROP;
		CREATE TEMP TABLE staged_categories (
			profession_id INTEGER, skill_tier_id INTEGER, name TEXT
		) ON COMMIT DROP;`); err != nil {
		return fmt.Errorf("create recipe hierarchy staging tables: %w", err)
	}
	if _, err := tx.CopyFrom(ctx, pgx.Identifier{"staged_tier_details"},
		[]string{"profession_id", "id", "name", "minimum_skill_level", "maximum_skill_level"},
		pgx.CopyFromRows(tierRows)); err != nil {
		return fmt.Errorf("copy staged tier details: %w", err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO profession_skill_tiers
		(profession_id, id, name, minimum_skill_level, maximum_skill_level)
		SELECT profession_id, id, name, minimum_skill_level, maximum_skill_level FROM staged_tier_details
		ON CONFLICT (profession_id, id) DO UPDATE SET name=EXCLUDED.name,
		minimum_skill_level=EXCLUDED.minimum_skill_level,
		maximum_skill_level=EXCLUDED.maximum_skill_level
		WHERE (profession_skill_tiers.name, profession_skill_tiers.minimum_skill_level,
			profession_skill_tiers.maximum_skill_level)
		IS DISTINCT FROM (EXCLUDED.name, EXCLUDED.minimum_skill_level,
			EXCLUDED.maximum_skill_level)`); err != nil {
		return fmt.Errorf("upsert staged tier details: %w", err)
	}
	if len(categoryRows) > 0 {
		if _, err := tx.CopyFrom(ctx, pgx.Identifier{"staged_categories"},
			[]string{"profession_id", "skill_tier_id", "name"}, pgx.CopyFromRows(categoryRows)); err != nil {
			return fmt.Errorf("copy staged categories: %w", err)
		}
		if _, err := tx.Exec(ctx, `INSERT INTO profession_categories
			(profession_id, skill_tier_id, name)
			SELECT profession_id, skill_tier_id, name FROM staged_categories
			ON CONFLICT DO NOTHING`); err != nil {
			return fmt.Errorf("upsert staged categories: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit recipe hierarchy: %w", err)
	}
	s.log.Info("recipe hierarchy stored", "skill_tiers", len(tierRows), "categories", len(categoryRows))
	return nil
}

type recipeVariant struct {
	faction       string
	craftedItemID any
}

func recipeVariants(recipe RecipeResponse) ([]recipeVariant, error) {
	hasGeneric := recipe.CraftedItem != nil
	hasAlliance := recipe.AllianceCraftedItem != nil
	hasHorde := recipe.HordeCraftedItem != nil

	switch {
	case hasGeneric && (hasAlliance || hasHorde):
		return nil, fmt.Errorf("recipe %d mixes generic and faction-specific crafted items", recipe.ID)
	case hasAlliance != hasHorde:
		return nil, fmt.Errorf("recipe %d has only one faction-specific crafted item", recipe.ID)
	case hasGeneric:
		return []recipeVariant{{faction: "Neutral", craftedItemID: recipe.CraftedItem.ID}}, nil
	case hasAlliance:
		return []recipeVariant{
			{faction: "Alliance", craftedItemID: recipe.AllianceCraftedItem.ID},
			{faction: "Horde", craftedItemID: recipe.HordeCraftedItem.ID},
		}, nil
	default:
		// Output-less recipes are common for enchanting and other recipes whose
		// effects are not represented as items by the Blizzard API.
		return []recipeVariant{{faction: "Neutral"}}, nil
	}
}

type reagentKey struct {
	recipeID int
	faction  string
	itemID   int
}

type slotKey struct {
	recipeID   int
	faction    string
	slotTypeID int
}

func (s *Seeder) storeRecipeBatch(ctx context.Context, records []recipeSeedRecord) (int, int, error) {
	itemReferences := make(map[int]string)
	recipeRows := make([][]any, 0, len(records))
	reagentRows := make(map[reagentKey][]any)
	slotRows := make(map[slotKey][]any)

	for _, record := range records {
		recipe := record.recipe
		variants, err := recipeVariants(recipe)
		if err != nil {
			return 0, 0, err
		}

		for _, item := range []*APIReference{recipe.CraftedItem, recipe.AllianceCraftedItem, recipe.HordeCraftedItem} {
			if item != nil {
				itemReferences[item.ID] = item.Name
			}
		}
		for _, reagent := range recipe.Reagents {
			itemReferences[reagent.Reagent.ID] = reagent.Reagent.Name
		}
		for _, reagent := range recipe.OptionalReagents {
			itemReferences[reagent.Reagent.ID] = reagent.Reagent.Name
		}

		var rank, mediaID, craftedQuantity any
		if recipe.Rank != nil {
			rank = *recipe.Rank
		}
		if recipe.Media != nil {
			mediaID = recipe.Media.ID
		}
		if recipe.CraftedQuantity != nil {
			craftedQuantity = recipe.CraftedQuantity.Value
		}

		for _, variant := range variants {
			recipeRows = append(recipeRows, []any{
				recipe.ID, variant.faction, recipe.Name, recipe.Description, rank, mediaID,
				record.profession.ID, record.tier.ID, record.category,
				variant.craftedItemID, craftedQuantity,
			})
			for _, reagent := range recipe.Reagents {
				key := reagentKey{recipe.ID, variant.faction, reagent.Reagent.ID}
				reagentRows[key] = []any{recipe.ID, variant.faction, reagent.Reagent.ID, reagent.Quantity, false}
			}
			for _, reagent := range recipe.OptionalReagents {
				key := reagentKey{recipe.ID, variant.faction, reagent.Reagent.ID}
				if _, required := reagentRows[key]; !required {
					reagentRows[key] = []any{recipe.ID, variant.faction, reagent.Reagent.ID, reagent.Quantity, true}
				}
			}
			for _, slot := range recipe.ModifiedCraftingSlots {
				key := slotKey{recipe.ID, variant.faction, slot.SlotType.ID}
				slotRows[key] = []any{recipe.ID, variant.faction, slot.SlotType.ID, slot.DisplayOrder}
			}
		}
	}

	itemRows := make([][]any, 0, len(itemReferences))
	for id, name := range itemReferences {
		itemRows = append(itemRows, []any{id, name})
	}
	reagents := make([][]any, 0, len(reagentRows))
	for _, row := range reagentRows {
		reagents = append(reagents, row)
	}
	slots := make([][]any, 0, len(slotRows))
	for _, row := range slotRows {
		slots = append(slots, row)
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return 0, 0, err
	}
	defer tx.Rollback(ctx)

	const stagingTables = `
CREATE TEMP TABLE staged_items (id INTEGER, name TEXT) ON COMMIT DROP;
CREATE TEMP TABLE staged_recipes (
	id INTEGER, faction TEXT, name TEXT, description TEXT, rank INTEGER, media_id INTEGER,
	profession_id INTEGER, skill_tier_id INTEGER, category_name TEXT,
	crafted_item_id INTEGER, crafted_quantity DOUBLE PRECISION
) ON COMMIT DROP;
CREATE TEMP TABLE staged_reagents (
	recipe_id INTEGER, recipe_faction TEXT, item_id INTEGER, quantity INTEGER, optional BOOLEAN
) ON COMMIT DROP;
CREATE TEMP TABLE staged_slots (
	recipe_id INTEGER, recipe_faction TEXT, slot_type_id INTEGER, display_order INTEGER
) ON COMMIT DROP;`
	if _, err := tx.Exec(ctx, stagingTables); err != nil {
		return 0, 0, fmt.Errorf("create recipe staging tables: %w", err)
	}

	if _, err := tx.CopyFrom(ctx, pgx.Identifier{"staged_items"}, []string{"id", "name"}, pgx.CopyFromRows(itemRows)); err != nil {
		return 0, 0, fmt.Errorf("copy staged items: %w", err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO items
		(id, name, item_class, item_subclass, quality, is_equippable, is_stackable, metadata_complete)
		SELECT id, name, 'Unknown', 'Unknown', 'Unknown', FALSE, FALSE, FALSE FROM staged_items
		ON CONFLICT (id) DO NOTHING`); err != nil {
		return 0, 0, fmt.Errorf("upsert staged items: %w", err)
	}

	if _, err := tx.CopyFrom(ctx, pgx.Identifier{"staged_recipes"},
		[]string{"id", "faction", "name", "description", "rank", "media_id", "profession_id",
			"skill_tier_id", "category_name", "crafted_item_id", "crafted_quantity"},
		pgx.CopyFromRows(recipeRows)); err != nil {
		return 0, 0, fmt.Errorf("copy staged recipes: %w", err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO recipes
		(id, faction, name, description, rank, media_id, profession_id, skill_tier_id,
		 category_name, crafted_item_id, crafted_quantity, updated_at)
		SELECT id, faction, name, description, rank, media_id, profession_id, skill_tier_id,
		 category_name, crafted_item_id, crafted_quantity, NOW() FROM staged_recipes
		ON CONFLICT (id, faction) DO UPDATE SET name=EXCLUDED.name,
		description=EXCLUDED.description, rank=EXCLUDED.rank, media_id=EXCLUDED.media_id,
		profession_id=EXCLUDED.profession_id, skill_tier_id=EXCLUDED.skill_tier_id,
		category_name=EXCLUDED.category_name, crafted_item_id=EXCLUDED.crafted_item_id,
		crafted_quantity=EXCLUDED.crafted_quantity, updated_at=NOW()
		WHERE (recipes.name, recipes.description, recipes.rank, recipes.media_id,
			recipes.profession_id, recipes.skill_tier_id, recipes.category_name,
			recipes.crafted_item_id, recipes.crafted_quantity)
		IS DISTINCT FROM (EXCLUDED.name, EXCLUDED.description, EXCLUDED.rank, EXCLUDED.media_id,
			EXCLUDED.profession_id, EXCLUDED.skill_tier_id, EXCLUDED.category_name,
			EXCLUDED.crafted_item_id, EXCLUDED.crafted_quantity)`); err != nil {
		return 0, 0, fmt.Errorf("upsert staged recipes: %w", err)
	}

	if len(reagents) > 0 {
		if _, err := tx.CopyFrom(ctx, pgx.Identifier{"staged_reagents"},
			[]string{"recipe_id", "recipe_faction", "item_id", "quantity", "optional"},
			pgx.CopyFromRows(reagents)); err != nil {
			return 0, 0, fmt.Errorf("copy staged reagents: %w", err)
		}
		if _, err := tx.Exec(ctx, `INSERT INTO reagents
			(recipe_id, recipe_faction, item_id, quantity, optional)
			SELECT recipe_id, recipe_faction, item_id, quantity, optional FROM staged_reagents
			ON CONFLICT (recipe_id, recipe_faction, item_id) DO UPDATE SET
			quantity=EXCLUDED.quantity, optional=EXCLUDED.optional
			WHERE (reagents.quantity, reagents.optional)
			IS DISTINCT FROM (EXCLUDED.quantity, EXCLUDED.optional)`); err != nil {
			return 0, 0, fmt.Errorf("upsert staged reagents: %w", err)
		}
	}

	if len(slots) > 0 {
		if _, err := tx.CopyFrom(ctx, pgx.Identifier{"staged_slots"},
			[]string{"recipe_id", "recipe_faction", "slot_type_id", "display_order"},
			pgx.CopyFromRows(slots)); err != nil {
			return 0, 0, fmt.Errorf("copy staged crafting slots: %w", err)
		}
		if _, err := tx.Exec(ctx, `INSERT INTO modified_crafting_slots
			(recipe_id, recipe_faction, slot_type_id, display_order)
			SELECT recipe_id, recipe_faction, slot_type_id, display_order FROM staged_slots
			ON CONFLICT (recipe_id, recipe_faction, slot_type_id) DO UPDATE SET
			display_order=EXCLUDED.display_order
			WHERE modified_crafting_slots.display_order IS DISTINCT FROM EXCLUDED.display_order`); err != nil {
			return 0, 0, fmt.Errorf("upsert staged crafting slots: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, 0, fmt.Errorf("commit recipe batch: %w", err)
	}
	return len(recipeRows), len(reagents), nil
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
