package scraper

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestNextHalfHour(t *testing.T) {
	tests := []struct {
		name string
		now  string
		want string
	}{
		{"before half hour", "2026-08-03T10:12:00Z", "2026-08-03T10:30:00Z"},
		{"on half hour", "2026-08-03T10:30:00Z", "2026-08-03T10:30:00Z"},
		{"after half hour", "2026-08-03T10:52:00Z", "2026-08-03T11:30:00Z"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			now, err := time.Parse(time.RFC3339, test.now)
			if err != nil {
				t.Fatal(err)
			}
			want, err := time.Parse(time.RFC3339, test.want)
			if err != nil {
				t.Fatal(err)
			}
			if got := nextHalfHour(now); !got.Equal(want) {
				t.Fatalf("nextHalfHour(%s) = %s, want %s", now, got, want)
			}
		})
	}
}

func TestRecipeResponseModelsExplicitFields(t *testing.T) {
	payload := []byte(`{
		"id": 38729,
		"name": "Monel-Hardened Boots",
		"description": "Create boots.",
		"rank": 2,
		"media": {"id": 38729},
		"crafted_quantity": {"value": 1.0},
		"modified_crafting_slots": [{"slot_type": {"id": 47}, "display_order": 0}]
	}`)
	var recipe RecipeResponse
	if err := json.Unmarshal(payload, &recipe); err != nil {
		t.Fatal(err)
	}
	if recipe.Rank == nil || *recipe.Rank != 2 || recipe.Media.ID != 38729 {
		t.Fatalf("recipe metadata not decoded: %+v", recipe)
	}
	if recipe.CraftedQuantity.Value != 1 || recipe.ModifiedCraftingSlots[0].SlotType.ID != 47 {
		t.Fatalf("recipe crafting fields not decoded: %+v", recipe)
	}
}

func TestLocalizedNameEnglish(t *testing.T) {
	name := LocalizedName{"en_GB": "British", "en_US": "American"}
	if got := name.English(); got != "American" {
		t.Fatalf("English() = %q, want American", got)
	}
}

func TestRetryAfter(t *testing.T) {
	now := time.Date(2026, time.August, 3, 10, 0, 0, 0, time.UTC)
	if got := retryAfter("7", now); !got.Equal(now.Add(7 * time.Second)) {
		t.Fatalf("delta Retry-After = %s", got)
	}
	date := now.Add(12 * time.Second).Format(http.TimeFormat)
	if got := retryAfter(date, now); !got.Equal(now.Add(12 * time.Second)) {
		t.Fatalf("date Retry-After = %s", got)
	}
	if got := retryAfter("invalid", now); !got.IsZero() {
		t.Fatalf("invalid Retry-After = %s, want zero", got)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func TestStreamCommoditiesDecodesAuctionsIncrementally(t *testing.T) {
	httpClient := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Header:     http.Header{"Last-Modified": {"Mon, 03 Aug 2026 10:00:00 GMT"}},
			Body: io.NopCloser(strings.NewReader(`{
				"_links": {},
				"auctions": [
					{"id":1,"item":{"id":10},"quantity":2,"unit_price":300,"time_left":"LONG"},
					{"id":2,"item":{"id":20},"quantity":4,"unit_price":500,"time_left":"SHORT"}
				],
				"tail": true
			}`)),
		}, nil
	})}
	client := NewBlizzardClient(httpClient, "id", "secret", "eu", NewAPIRateLimiter(100_000))
	client.token = "token"
	client.tokenExpires = time.Now().Add(time.Hour)

	var itemIDs []int
	modified, changed, count, err := client.StreamCommodities(context.Background(), "", func(auction CommodityAuction) error {
		itemIDs = append(itemIDs, auction.Item.ID)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !changed || count != 2 || modified != "Mon, 03 Aug 2026 10:00:00 GMT" {
		t.Fatalf("changed=%v count=%d modified=%q", changed, count, modified)
	}
	if len(itemIDs) != 2 || itemIDs[0] != 10 || itemIDs[1] != 20 {
		t.Fatalf("item IDs = %v", itemIDs)
	}
}

func TestConcurrentTokenRefreshUsesOneRequest(t *testing.T) {
	var requests atomic.Int32
	httpClient := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		requests.Add(1)
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`{"access_token":"token","expires_in":3600}`)),
		}, nil
	})}
	client := NewBlizzardClient(httpClient, "id", "secret", "eu", NewAPIRateLimiter(100_000))

	var wg sync.WaitGroup
	errors := make(chan error, 20)
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := client.tokenFor(context.Background())
			errors <- err
		}()
	}
	wg.Wait()
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatal(err)
		}
	}
	if got := requests.Load(); got != 1 {
		t.Fatalf("OAuth requests = %d, want 1", got)
	}
}
