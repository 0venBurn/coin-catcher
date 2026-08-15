package scraper

type OAuthTokenResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
}

type CommodityItem struct {
	ID int `json:"id"`
}

type CommodityAuction struct {
	ID        int64         `json:"id"`
	Item      CommodityItem `json:"item"`
	Quantity  int           `json:"quantity"`
	UnitPrice int64         `json:"unit_price"`
	TimeLeft  string        `json:"time_left"`
}

type CommodityAuctionsAPIResponse struct {
	Auctions []CommodityAuction `json:"auctions"`
}

type APIReference struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type ProfessionIndexResponse struct {
	Professions []APIReference `json:"professions"`
}

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

type RecipeCategory struct {
	Name    string         `json:"name"`
	Recipes []APIReference `json:"recipes"`
}

type SkillTierResponse struct {
	ID                int              `json:"id"`
	Name              string           `json:"name"`
	MinimumSkillLevel int              `json:"minimum_skill_level"`
	MaximumSkillLevel int              `json:"maximum_skill_level"`
	Categories        []RecipeCategory `json:"categories"`
}

type RecipeReagent struct {
	Reagent  APIReference `json:"reagent"`
	Quantity int          `json:"quantity"`
}

type CraftedQuantity struct {
	Value float64 `json:"value"`
}

type ModifiedCraftingSlot struct {
	SlotType     APIReference `json:"slot_type"`
	DisplayOrder int          `json:"display_order"`
}

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

type LocalizedName map[string]string

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

type ItemSearchResponse struct {
	Page      int                `json:"page"`
	PageCount int                `json:"pageCount"`
	Results   []ItemSearchResult `json:"results"`
}
