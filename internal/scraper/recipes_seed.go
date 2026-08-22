package scraper

import (
	"context"
	"fmt"
	"time"

	"golang.org/x/sync/errgroup"
)

// recipeSeedBatchSize bounds both the in-flight result buffer and the
// commit batch: workers never block on results until a full batch is ready,
// and each flush stays a modest transaction.
const recipeSeedBatchSize = 500

// recipeSeedRecord pairs a recipe detail response with the hierarchy job it
// was fetched for, so batches know where each recipe belongs.
type recipeSeedRecord struct {
	job    recipeJob
	recipe RecipeResponse
}

// recipeJob identifies one recipe plus its place in the profession → tier →
// category tree, discovered during the listing pass.
type recipeJob struct {
	professionID int
	tierID       int
	category     string
	recipeID     int
}

// recipeFetchResult carries either a fetched recipe or the error that ended
// its worker; errors travel through the same channel so the consumer can
// drain cleanly before unwinding.
type recipeFetchResult struct {
	record recipeSeedRecord
	err    error
}

// seedRecipesAndReagents runs the final seed stage in two phases: a serial
// listing pass builds the tier/category hierarchy and the deduped job list,
// then a bounded worker pool fetches recipe details while the main goroutine
// commits them in batches. Recipes listed under several categories are kept
// once under their first sighting because recipes table keys on id+faction.
func (s *Seeder) seedRecipesAndReagents(ctx context.Context) error {
	s.log.Info("seeding recipes and reagents", "batch_size", recipeSeedBatchSize, "workers", s.recipeWorkers)
	professions, err := s.client.GetProfessions(ctx)
	if err != nil {
		return err
	}

	seen := make(map[int]recipeJob)
	jobs := make([]recipeJob, 0, 12_000)
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
					job := recipeJob{professionID: profession.ID, tierID: tier.ID, category: category.Name, recipeID: recipeRef.ID}
					if previous, duplicate := seen[recipeRef.ID]; duplicate {
						s.log.Warn("recipe listed under multiple categories; keeping first",
							"recipe_id", recipeRef.ID,
							"kept", fmt.Sprintf("%d/%d/%q", previous.professionID, previous.tierID, previous.category),
							"skipped", fmt.Sprintf("%d/%d/%q", job.professionID, job.tierID, job.category))
						continue
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

	// Fetch recipes with a bounded worker pool while the main goroutine commits
	// results in batches. A fetch error makes its worker return non-nil, which
	// cancels the errgroup context and unwinds the feeder and remaining
	// workers; a commit error calls cancel() for the same effect.
	workCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	g, gctx := errgroup.WithContext(workCtx)
	jobsCh := make(chan recipeJob)
	results := make(chan recipeFetchResult, recipeSeedBatchSize)

	g.Go(func() error {
		defer close(jobsCh)
		for _, job := range jobs {
			select {
			case jobsCh <- job:
			case <-gctx.Done():
				return gctx.Err()
			}
		}
		return nil
	})
	for range s.recipeWorkers {
		g.Go(func() error {
			for job := range jobsCh {
				recipe, err := s.client.GetRecipe(gctx, job.recipeID)
				if err != nil {
					result := recipeFetchResult{err: fmt.Errorf("get recipe %d: %w", job.recipeID, err)}
					select {
					case results <- result:
					case <-gctx.Done():
					}
					return result.err
				}
				select {
				case results <- recipeFetchResult{record: recipeSeedRecord{job: job, recipe: recipe}}:
				case <-gctx.Done():
					return gctx.Err()
				}
			}
			return nil
		})
	}
	go func() {
		_ = g.Wait()
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
			// Keep only the first fetch error; later ones are duplicates of
			// the same unwind. Draining continues so no worker blocks on a
			// full results channel.
			if fetchErr == nil {
				fetchErr = result.err
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

// storeRecipeHierarchy commits tiers and categories ahead of any recipe
// batch, so every recipe row can reference existing parents on insert.
func (s *Seeder) storeRecipeHierarchy(ctx context.Context, tierRows, categoryRows [][]any) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	err = execStagedLoads(ctx, tx, []stagedLoad{
		{
			label:   "staged tier details",
			table:   "staged_tier_details",
			create:  createStagedTierDetails,
			columns: colsStagedTierDetails,
			rows:    tierRows,
			upsert:  upsertProfessionSkillTier,
		},
		{
			label:   "staged categories",
			table:   "staged_categories",
			create:  createStagedCategories,
			columns: colsStagedCategories,
			rows:    categoryRows,
			upsert:  insertProfessionCategory,
		},
	})
	if err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit recipe hierarchy: %w", err)
	}
	s.log.Info("recipe hierarchy stored", "skill_tiers", len(tierRows), "categories", len(categoryRows))
	return nil
}

// recipeVariant is one faction-specific row of a recipe: either the neutral
// form or one side of an Alliance/Horde pair.
type recipeVariant struct {
	faction       string
	craftedItemID any
}

// recipeVariants expands one recipe into its faction rows: neutral-only,
// or an Alliance/Horde pair with distinct crafted items. The mixed and
// half-faction shapes have never been observed upstream but would silently
// corrupt the faction keying if stored naively, hence the hard errors.
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

// storeRecipeBatch flattens a batch of recipes into four staged loads.
// Reagents and slots are keyed maps so a reagent appearing as both optional
// and required (or duplicated across variants) collapses to one row, with
// required winning over optional for the same key. Item stubs commit first
// so recipe/reagent foreign keys resolve within the same transaction.
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
				record.job.professionID, record.job.tierID, record.job.category,
				variant.craftedItemID, craftedQuantity,
			})
			for _, reagent := range recipe.Reagents {
				key := reagentKey{recipe.ID, variant.faction, reagent.Reagent.ID}
				reagentRows[key] = []any{recipe.ID, variant.faction, reagent.Reagent.ID, reagent.Quantity, false}
			}
			for _, reagent := range recipe.OptionalReagents {
				key := reagentKey{recipe.ID, variant.faction, reagent.Reagent.ID}
				// Required reagents already present must not be downgraded to
				// optional when the same item also appears in the optional list.
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

	err = execStagedLoads(ctx, tx, []stagedLoad{
		{
			label:   "staged items",
			table:   "staged_items",
			create:  createStagedItems,
			columns: colsStagedItems,
			rows:    itemRows,
			upsert:  insertItemStub,
		},
		{
			label:   "staged recipes",
			table:   "staged_recipes",
			create:  createStagedRecipes,
			columns: colsStagedRecipes,
			rows:    recipeRows,
			upsert:  upsertRecipe,
		},
		{
			label:   "staged reagents",
			table:   "staged_reagents",
			create:  createStagedReagents,
			columns: colsStagedReagents,
			rows:    reagents,
			upsert:  upsertReagent,
		},
		{
			label:   "staged slots",
			table:   "staged_slots",
			create:  createStagedSlots,
			columns: colsStagedSlots,
			rows:    slots,
			upsert:  upsertModifiedCraftingSlot,
		},
	})
	if err != nil {
		return 0, 0, err
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, 0, fmt.Errorf("commit recipe batch: %w", err)
	}
	return len(recipeRows), len(reagents), nil
}
