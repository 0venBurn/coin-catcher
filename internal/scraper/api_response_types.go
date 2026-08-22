// Package-level wire types for Blizzard API responses. Fields mirror the
// upstream JSON exactly so encoding/json can decode without custom hooks;
// anything the database does not need is simply never scanned.
package scraper

// OAuthTokenResponse is the client-credentials token issued by
// oauth.battle.net. ExpiresIn is in seconds.
type OAuthTokenResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
}

// CommodityItem exists only because the commodities payload nests the item
// id one level down under "item"; there is no other item data on auctions.
type CommodityItem struct {
	ID int `json:"id"`
}

// CommodityAuction is one row of the region-wide commodities auction list.
// UnitPrice is a buyout price in copper.
type CommodityAuction struct {
	ID        int64         `json:"id"`
	Item      CommodityItem `json:"item"`
	Quantity  int           `json:"quantity"`
	UnitPrice int64         `json:"unit_price"`
	TimeLeft  string        `json:"time_left"`
}

// TokenIndexResponse is the WoW Token endpoint payload.
type TokenIndexResponse struct {
	// Epoch milliseconds of Blizzard's own last token price update; used for
	// dedup because the endpoint has no conditional-request support.
	LastUpdatedTimestamp int64 `json:"last_updated_timestamp"`
	Price                int64 `json:"price"` // copper (gold × 10,000)
}

// APIReference is Blizzard's ubiquitous {id, name} object used both as a
// standalone reference and embedded inside index payloads.
type APIReference struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// ProfessionIndexResponse lists every profession; names are localized, but
// only ids are needed for seeding.
type ProfessionIndexResponse struct {
	Professions []APIReference `json:"professions"`
}

// ProfessionResponse is the single-profession detail payload; skill_tiers
// drive which tier detail requests are issued next during seeding.
type ProfessionResponse struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Type        struct {
		Type string `json:"type"`
		Name string `json:"name"`
	} `json:"type"`
	Media      *APIReference  `json:"media"`
	SkillTiers []APIReference `json:"skill_tiers"`
}

// RecipeCategory groups the recipes belonging to one category of a skill tier.
type RecipeCategory struct {
	Name    string         `json:"name"`
	Recipes []APIReference `json:"recipes"`
}

// SkillTierResponse is one profession skill tier (e.g. "Shadowlands
// Alchemy") with its recipe categories.
type SkillTierResponse struct {
	ID                int              `json:"id"`
	Name              string           `json:"name"`
	MinimumSkillLevel int              `json:"minimum_skill_level"`
	MaximumSkillLevel int              `json:"maximum_skill_level"`
	Categories        []RecipeCategory `json:"categories"`
}

// RecipeReagent is one reagent line on a recipe.
type RecipeReagent struct {
	Reagent  APIReference `json:"reagent"`
	Quantity int          `json:"quantity"`
}

// CraftedQuantity is the expected output per craft. It arrives as an object
// with a fractional value even though it is always a whole number upstream.
type CraftedQuantity struct {
	Value float64 `json:"value"`
}

// ModifiedCraftingSlot describes one optional-reagent slot on a recipe.
type ModifiedCraftingSlot struct {
	SlotType     APIReference `json:"slot_type"`
	DisplayOrder int          `json:"display_order"`
}

// RecipeResponse is the full recipe detail payload. The crafted-item fields
// are pointers because some recipes (notably those with faction-specific or
// optional outputs) omit them entirely.
type RecipeResponse struct {
	ID                    int                    `json:"id"`
	Name                  string                 `json:"name"`
	Description           string                 `json:"description"`
	Rank                  *int                   `json:"rank"`
	Media                 *APIReference          `json:"media"`
	CraftedItem           *APIReference          `json:"crafted_item"`
	AllianceCraftedItem   *APIReference          `json:"alliance_crafted_item"`
	HordeCraftedItem      *APIReference          `json:"horde_crafted_item"`
	CraftedQuantity       *CraftedQuantity       `json:"crafted_quantity"`
	Reagents              []RecipeReagent        `json:"reagents"`
	OptionalReagents      []RecipeReagent        `json:"optional_reagents"`
	ModifiedCraftingSlots []ModifiedCraftingSlot `json:"modified_crafting_slots"`
}

// LocalizedName is a locale-keyed name map ({"en_US": "Copper Ore", ...})
// used by the item search endpoint instead of plain strings.
type LocalizedName map[string]string

// English returns the best English name available: en_US first, then en_GB,
// then whatever locale happens to come first in the map. The map iteration
// fallback makes the result non-deterministic for items with no English
// name, which is acceptable for seed data — every useful item has one.
func (n LocalizedName) English() string {
	if value := n["en_US"]; value != "" {
		return value
	}
	if value := n["en_GB"]; value != "" {
		return value
	}
	for _, value := range n {
		return value
	}
	return "Unknown"
}

// ItemSearchResult is one hit from the item search endpoint. Only the nested
// "data" object carries fields; the surrounding envelope adds metadata that
// seeding does not use.
type ItemSearchResult struct {
	Data struct {
		ID            int           `json:"id"`
		Name          LocalizedName `json:"name"`
		Level         int           `json:"level"`
		RequiredLevel int           `json:"required_level"`
		SellPrice     int           `json:"sell_price"`
		MaxCount      int           `json:"max_count"`
		IsEquippable  bool          `json:"is_equippable"`
		IsStackable   bool          `json:"is_stackable"`
		ItemClass     struct {
			Name LocalizedName `json:"name"`
		} `json:"item_class"`
		ItemSubclass struct {
			Name LocalizedName `json:"name"`
		} `json:"item_subclass"`
		InventoryType *struct {
			Name LocalizedName `json:"name"`
		} `json:"inventory_type"`
		Quality struct {
			Name LocalizedName `json:"name"`
		} `json:"quality"`
	} `json:"data"`
}

// ItemSearchResponse is one page of item search results. PageCount bounds
// the pagination loop during item seeding.
type ItemSearchResponse struct {
	Page      int                `json:"page"`
	PageCount int                `json:"pageCount"`
	Results   []ItemSearchResult `json:"results"`
}
