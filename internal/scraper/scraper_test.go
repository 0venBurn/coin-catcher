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

func TestStaleWarnCount(t *testing.T) {
	tests := []struct {
		name  string
		stale time.Duration
		want  int
	}{
		{"fresh", 0, 0},
		{"under first threshold", 9 * time.Minute, 0},
		{"first threshold", 10 * time.Minute, 1},
		{"second threshold", 30 * time.Minute, 2},
		{"third threshold", time.Hour, 3},
		{"one hour past third threshold", 2 * time.Hour, 4},
		{"three hours past third threshold", 4 * time.Hour, 6},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := staleWarnCount(test.stale); got != test.want {
				t.Fatalf("staleWarnCount(%s) = %d, want %d", test.stale, got, test.want)
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

func TestGetTokenIndexDecodesPrice(t *testing.T) {
	httpClient := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`{"_links":{},"last_updated_timestamp":1699285102000,"price":2587950000}`)),
		}, nil
	})}
	client := NewBlizzardClient(httpClient, "id", "secret", "eu", NewAPIRateLimiter(100_000))
	client.token = "token"
	client.tokenExpires = time.Now().Add(time.Hour)

	payload, err := client.GetTokenIndex(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if payload.Price != 2587950000 || payload.LastUpdatedTimestamp != 1699285102000 {
		t.Fatalf("unexpected token index: %+v", payload)
	}
	if updated := time.UnixMilli(payload.LastUpdatedTimestamp).UTC(); updated.Year() != 2023 {
		t.Fatalf("last_updated_timestamp did not decode as epoch millis: %s", updated)
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
