package scraper

// Keeping it here lets the seeding functions show their control flow without
// walls of inline SQL. Each staging table groups its column order, DDL, and
// upsert under a matching name.
//
// Pattern note: every upsert carries a trailing WHERE ... IS DISTINCT FROM
// clause so unchanged rows are skipped entirely — without it, each re-seed
// would rewrite every row and fire updated_at/trigger churn across millions
// of rows for no data change.

// Items stage (seeder.go): full item metadata from SearchItems.
var (
	colsStagedItemSeed = []string{"id", "name", "item_level", "item_class", "item_subclass", "inventory_type",
		"quality", "is_equippable", "is_stackable", "required_level", "sell_price", "max_stack_size"}
)

const (
	createStagedItemSeed = `CREATE TEMP TABLE staged_item_seed (
		id INTEGER, name TEXT, item_level INTEGER, item_class TEXT, item_subclass TEXT,
		inventory_type TEXT, quality TEXT, is_equippable BOOLEAN, is_stackable BOOLEAN,
		required_level INTEGER, sell_price INTEGER, max_stack_size INTEGER
	)`
	upsertItemMetadata = `INSERT INTO items
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
			 EXCLUDED.inventory_type, EXCLUDED.quality, EXCLUDED.is_equippable, EXCLUDED.is_stackable,
			 EXCLUDED.required_level, EXCLUDED.sell_price,
			 EXCLUDED.max_stack_size, TRUE)`
)

// Professions stage (seeder.go).
var (
	colsStagedProfessions     = []string{"id", "name", "description", "type_code", "type_name", "media_id"}
	colsStagedProfessionTiers = []string{"profession_id", "id", "name"}
)

const (
	createStagedProfessions = `CREATE TEMP TABLE staged_professions (
		id INTEGER, name TEXT, description TEXT, type_code TEXT, type_name TEXT, media_id INTEGER
	)`
	upsertProfession = `INSERT INTO professions
		(id, name, description, type_code, type_name, media_id, updated_at)
		SELECT id, name, description, type_code, type_name, media_id, NOW() FROM staged_professions
		ON CONFLICT (id) DO UPDATE SET name=EXCLUDED.name, description=EXCLUDED.description,
		type_code=EXCLUDED.type_code, type_name=EXCLUDED.type_name,
		media_id=EXCLUDED.media_id, updated_at=NOW()
		WHERE (professions.name, professions.description, professions.type_code,
			professions.type_name, professions.media_id)
		IS DISTINCT FROM (EXCLUDED.name, EXCLUDED.description, EXCLUDED.type_code,
			EXCLUDED.type_name, EXCLUDED.media_id)`

	createStagedProfessionTiers = `CREATE TEMP TABLE staged_profession_tiers (
		profession_id INTEGER, id INTEGER, name TEXT
	)`
	upsertProfessionTier = `INSERT INTO profession_skill_tiers (profession_id, id, name)
		SELECT profession_id, id, name FROM staged_profession_tiers
		ON CONFLICT (profession_id, id) DO UPDATE SET name=EXCLUDED.name
		WHERE profession_skill_tiers.name IS DISTINCT FROM EXCLUDED.name`
)

// Recipe hierarchy (recipes_seed.go storeRecipeHierarchy).
// Categories use ON CONFLICT DO NOTHING rather than an upsert: category rows
// are immutable name groupings, and a no-op insert avoids needing a natural
// unique key beyond what the table declares.
var (
	colsStagedTierDetails = []string{"profession_id", "id", "name", "minimum_skill_level", "maximum_skill_level"}
	colsStagedCategories  = []string{"profession_id", "skill_tier_id", "name"}
)

const (
	createStagedTierDetails = `CREATE TEMP TABLE staged_tier_details (
		profession_id INTEGER, id INTEGER, name TEXT,
		minimum_skill_level INTEGER, maximum_skill_level INTEGER
	)`
	upsertProfessionSkillTier = `INSERT INTO profession_skill_tiers
		(profession_id, id, name, minimum_skill_level, maximum_skill_level)
		SELECT profession_id, id, name, minimum_skill_level, maximum_skill_level FROM staged_tier_details
		ON CONFLICT (profession_id, id) DO UPDATE SET name=EXCLUDED.name,
		minimum_skill_level=EXCLUDED.minimum_skill_level,
		maximum_skill_level=EXCLUDED.maximum_skill_level
		WHERE (profession_skill_tiers.name, profession_skill_tiers.minimum_skill_level,
			profession_skill_tiers.maximum_skill_level)
		IS DISTINCT FROM (EXCLUDED.name, EXCLUDED.minimum_skill_level,
			EXCLUDED.maximum_skill_level)`

	createStagedCategories = `CREATE TEMP TABLE staged_categories (
		profession_id INTEGER, skill_tier_id INTEGER, name TEXT
	)`
	insertProfessionCategory = `INSERT INTO profession_categories
		(profession_id, skill_tier_id, name)
		SELECT profession_id, skill_tier_id, name FROM staged_categories
		ON CONFLICT DO NOTHING`
)

// Recipe batches (recipes_seed.go storeRecipeBatch).
var (
	colsStagedRecipes = []string{"id", "faction", "name", "description", "rank", "media_id", "profession_id",
		"skill_tier_id", "category_name", "crafted_item_id", "crafted_quantity"}
	colsStagedReagents = []string{"recipe_id", "recipe_faction", "item_id", "quantity", "optional"}
	colsStagedSlots    = []string{"recipe_id", "recipe_faction", "slot_type_id", "display_order"}
	colsStagedItems    = []string{"id", "name"}
)

const (
	createStagedItems = `CREATE TEMP TABLE staged_items (id INTEGER, name TEXT)`
	// insertItemStub seeds placeholder rows so recipe/reagent foreign keys can
	// commit before full item metadata is fetched by the items stage.
	insertItemStub = `INSERT INTO items
		(id, name, item_class, item_subclass, quality, is_equippable, is_stackable, metadata_complete)
		SELECT id, name, 'Unknown', 'Unknown', 'Unknown', FALSE, FALSE, FALSE FROM staged_items
		ON CONFLICT (id) DO NOTHING`

	createStagedRecipes = `CREATE TEMP TABLE staged_recipes (
		id INTEGER, faction TEXT, name TEXT, description TEXT, rank INTEGER, media_id INTEGER,
		profession_id INTEGER, skill_tier_id INTEGER, category_name TEXT,
		crafted_item_id INTEGER, crafted_quantity DOUBLE PRECISION
	)`
	upsertRecipe = `INSERT INTO recipes
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
			EXCLUDED.crafted_item_id, EXCLUDED.crafted_quantity)`

	createStagedReagents = `CREATE TEMP TABLE staged_reagents (
		recipe_id INTEGER, recipe_faction TEXT, item_id INTEGER, quantity INTEGER, optional BOOLEAN
	)`
	upsertReagent = `INSERT INTO reagents
		(recipe_id, recipe_faction, item_id, quantity, optional)
		SELECT recipe_id, recipe_faction, item_id, quantity, optional FROM staged_reagents
		ON CONFLICT (recipe_id, recipe_faction, item_id) DO UPDATE SET
		quantity=EXCLUDED.quantity, optional=EXCLUDED.optional
		WHERE (reagents.quantity, reagents.optional)
		IS DISTINCT FROM (EXCLUDED.quantity, EXCLUDED.optional)`

	createStagedSlots = `CREATE TEMP TABLE staged_slots (
		recipe_id INTEGER, recipe_faction TEXT, slot_type_id INTEGER, display_order INTEGER
	)`
	upsertModifiedCraftingSlot = `INSERT INTO modified_crafting_slots
		(recipe_id, recipe_faction, slot_type_id, display_order)
		SELECT recipe_id, recipe_faction, slot_type_id, display_order FROM staged_slots
		ON CONFLICT (recipe_id, recipe_faction, slot_type_id) DO UPDATE SET
		display_order=EXCLUDED.display_order
		WHERE modified_crafting_slots.display_order IS DISTINCT FROM EXCLUDED.display_order`
)
