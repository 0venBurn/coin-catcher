package scraper

import (
	"strings"
	"testing"
)

func TestRecipeVariants(t *testing.T) {
	item := func(id int) *APIReference { return &APIReference{ID: id} }
	tests := []struct {
		name    string
		recipe  RecipeResponse
		want    []recipeVariant
		wantErr string
	}{
		{
			name:   "generic output",
			recipe: RecipeResponse{ID: 1, CraftedItem: item(10)},
			want:   []recipeVariant{{faction: "Neutral", craftedItemID: 10}},
		},
		{
			name: "faction outputs",
			recipe: RecipeResponse{
				ID: 2, AllianceCraftedItem: item(20), HordeCraftedItem: item(21),
			},
			want: []recipeVariant{
				{faction: "Alliance", craftedItemID: 20},
				{faction: "Horde", craftedItemID: 21},
			},
		},
		{
			name:   "no item output",
			recipe: RecipeResponse{ID: 3},
			want:   []recipeVariant{{faction: "Neutral"}},
		},
		{
			name:    "only alliance output",
			recipe:  RecipeResponse{ID: 4, AllianceCraftedItem: item(40)},
			wantErr: "only one faction-specific crafted item",
		},
		{
			name: "generic and faction outputs",
			recipe: RecipeResponse{
				ID: 5, CraftedItem: item(50), AllianceCraftedItem: item(51), HordeCraftedItem: item(52),
			},
			wantErr: "mixes generic and faction-specific crafted items",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := recipeVariants(test.recipe)
			if test.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), test.wantErr) {
					t.Fatalf("recipeVariants() error = %v, want %q", err, test.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if len(got) != len(test.want) {
				t.Fatalf("recipeVariants() = %#v, want %#v", got, test.want)
			}
			for i := range got {
				if got[i].faction != test.want[i].faction || got[i].craftedItemID != test.want[i].craftedItemID {
					t.Fatalf("recipeVariants() = %#v, want %#v", got, test.want)
				}
			}
		})
	}
}
