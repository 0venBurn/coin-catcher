package scraper

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

const oauthTokenURL = "https://oauth.battle.net/token"

// APIRateLimiter spaces requests evenly at a fixed requests-per-second pace.
// Blizzard throttles by request rate, so a steady drip avoids bursts that a
// naive ticker-with-sleep would produce under concurrent callers.
type APIRateLimiter struct {
	mu       sync.Mutex
	interval time.Duration
	next     time.Time // earliest slot not yet handed out; zero means "now"
}

// NewAPIRateLimiter builds a limiter for the given per-second budget, falling
// back to 20 req/s (Blizzard's documented commodity cap) when configured to
// zero or below.
func NewAPIRateLimiter(requestsPerSecond int) *APIRateLimiter {
	if requestsPerSecond <= 0 {
		requestsPerSecond = 20
	}
	return &APIRateLimiter{interval: time.Second / time.Duration(requestsPerSecond)}
}

// Wait blocks until the caller's reserved slot arrives, reserving slots in
// arrival order so concurrency cannot reorder the pacing. The lock is held
// only for reservation; sleeping happens outside it so one slow caller never
// delays later reservations.
func (l *APIRateLimiter) Wait(ctx context.Context) error {
	l.mu.Lock()
	now := time.Now()
	readyAt := now
	if l.next.After(now) {
		readyAt = l.next
	}
	l.next = readyAt.Add(l.interval)
	l.mu.Unlock()

	delay := time.Until(readyAt)
	if delay <= 0 {
		return nil
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// BlizzardClient is an authenticated, rate-limited client for one region of
// the WoW game-data APIs. It caches its OAuth token and shares the limiter
// with sibling regional clients so the combined request rate stays in budget.
type BlizzardClient struct {
	http         *http.Client
	clientID     string
	clientSecret string
	region       string
	apiHost      string
	limiter      *APIRateLimiter
	token        string    // cached OAuth bearer token
	tokenExpires time.Time // refreshed one minute early (see tokenFor)
	mu           sync.Mutex
}

// NewBlizzardClient builds the client for a region; each region has its own
// apiHost because Blizzard namespaces data per region.
func NewBlizzardClient(httpClient *http.Client, clientID, clientSecret, region string, limiter *APIRateLimiter) *BlizzardClient {
	return &BlizzardClient{
		http: httpClient, clientID: clientID, clientSecret: clientSecret,
		region: region, apiHost: region + ".api.blizzard.com", limiter: limiter,
	}
}

// tokenFor returns a valid bearer token, requesting a new one only when the
// cached token is missing or within a minute of expiring. The early refresh
// margin keeps in-flight requests from failing on a token that expires
// between fetch and use. Callers serialize behind the mutex because OAuth is
// per-client, not per-request.
func (c *BlizzardClient) tokenFor(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.token != "" && time.Until(c.tokenExpires) > time.Minute {
		return c.token, nil
	}

	form := url.Values{"grant_type": {"client_credentials"}}.Encode()
	response, err := c.do(ctx, func() (*http.Request, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, oauthTokenURL, strings.NewReader(form))
		if err == nil {
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			req.SetBasicAuth(c.clientID, c.clientSecret)
		}
		return req, err
	})
	if err != nil {
		return "", fmt.Errorf("request OAuth token: %w", err)
	}
	defer response.Body.Close()
	var payload OAuthTokenResponse
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		return "", fmt.Errorf("decode OAuth token: %w", err)
	}
	c.token = payload.AccessToken
	c.tokenExpires = time.Now().Add(time.Duration(payload.ExpiresIn) * time.Second)
	return c.token, nil
}

// apiRequest issues one authenticated call against the region's dynamic or
// static namespace. A non-empty lastModified is sent as If-Modified-Since so
// unchanged endpoints answer 304 without a body — the core bandwidth saver
// for commodity polling.
func (c *BlizzardClient) apiRequest(ctx context.Context, method, path, namespace string, query url.Values, lastModified string) (*http.Response, error) {
	token, err := c.tokenFor(ctx)
	if err != nil {
		return nil, err
	}
	params := url.Values{}
	for key, values := range query {
		params[key] = values
	}
	params.Set("namespace", namespace+"-"+c.region)
	params.Set("locale", "en_US")
	u := url.URL{Scheme: "https", Host: c.apiHost, Path: path, RawQuery: params.Encode()}
	return c.do(ctx, func() (*http.Request, error) {
		req, err := http.NewRequestWithContext(ctx, method, u.String(), nil)
		if err == nil {
			req.Header.Set("Authorization", "Bearer "+token)
			if lastModified != "" {
				req.Header.Set("If-Modified-Since", lastModified)
			}
		}
		return req, err
	})
}

func (c *BlizzardClient) Region() string {
	return c.region
}

// GetTokenIndex fetches the current WoW Token price for the region.
func (c *BlizzardClient) GetTokenIndex(ctx context.Context) (TokenIndexResponse, error) {
	var payload TokenIndexResponse
	err := c.getJSON(ctx, "/data/wow/token/index", "dynamic", nil, &payload)
	return payload, err
}

// StreamCommodities fetches the region-wide commodity auction list and hands
// each auction to consume as it decodes, so peak memory stays flat no matter
// how large the list grows. Returns (lastModified, changed, count): changed
// is false for a 304 or any response whose Last-Modified equals what was
// sent. The hand-rolled token loop exists because decoding the full payload
// into memory first would double the container's footprint on busy regions.
func (c *BlizzardClient) StreamCommodities(
	ctx context.Context,
	lastModified string,
	consume func(CommodityAuction) error,
) (string, bool, int, error) {
	response, err := c.apiRequest(ctx, http.MethodGet, "/data/wow/auctions/commodities", "dynamic", nil, lastModified)
	if err != nil {
		return "", false, 0, err
	}
	defer response.Body.Close()
	modified := response.Header.Get("Last-Modified")
	if response.StatusCode == http.StatusNotModified {
		return modified, false, 0, nil
	}

	decoder := json.NewDecoder(response.Body)
	opening, err := decoder.Token()
	if err != nil {
		return modified, false, 0, fmt.Errorf("decode commodities object: %w", err)
	}
	if delimiter, ok := opening.(json.Delim); !ok || delimiter != '{' {
		return modified, false, 0, fmt.Errorf("decode commodities: expected object")
	}
	foundAuctions, count := false, 0
	for decoder.More() {
		key, err := decoder.Token()
		if err != nil {
			return modified, false, count, fmt.Errorf("decode commodities field: %w", err)
		}
		if key != "auctions" {
			var discard json.RawMessage
			if err := decoder.Decode(&discard); err != nil {
				return modified, false, count, fmt.Errorf("decode commodities field %q: %w", key, err)
			}
			continue
		}
		foundAuctions = true
		opening, err := decoder.Token()
		if err != nil {
			return modified, false, count, fmt.Errorf("decode auctions array: %w", err)
		}
		if delimiter, ok := opening.(json.Delim); !ok || delimiter != '[' {
			return modified, false, count, fmt.Errorf("decode commodities: auctions is not an array")
		}
		for decoder.More() {
			var auction CommodityAuction
			if err := decoder.Decode(&auction); err != nil {
				return modified, false, count, fmt.Errorf("decode commodity auction %d: %w", count, err)
			}
			if err := consume(auction); err != nil {
				return modified, false, count, err
			}
			count++
		}
		if _, err := decoder.Token(); err != nil {
			return modified, false, count, fmt.Errorf("close auctions array: %w", err)
		}
	}
	if _, err := decoder.Token(); err != nil {
		return modified, false, count, fmt.Errorf("close commodities object: %w", err)
	}
	if !foundAuctions {
		return modified, false, 0, fmt.Errorf("decode commodities: auctions field missing")
	}
	return modified, true, count, nil
}

// SearchItems returns one page of items ordered by id starting at
// startingID; the open-ended "[%d,]" range plus _page=1 turns the search
// endpoint into an id-ordered scan suitable for exhaustive seeding.
func (c *BlizzardClient) SearchItems(ctx context.Context, startingID, pageSize int) (ItemSearchResponse, error) {
	query := url.Values{
		"id":        {fmt.Sprintf("[%d,]", startingID)},
		"orderby":   {"id"},
		"_page":     {"1"},
		"_pageSize": {strconv.Itoa(pageSize)},
	}
	var payload ItemSearchResponse
	err := c.getJSON(ctx, "/data/wow/search/item", "static", query, &payload)
	return payload, err
}

func (c *BlizzardClient) GetProfessions(ctx context.Context) ([]APIReference, error) {
	var payload ProfessionIndexResponse
	if err := c.getJSON(ctx, "/data/wow/profession/index", "static", nil, &payload); err != nil {
		return nil, err
	}
	return payload.Professions, nil
}

func (c *BlizzardClient) GetProfession(ctx context.Context, id int) (ProfessionResponse, error) {
	var payload ProfessionResponse
	err := c.getJSON(ctx, fmt.Sprintf("/data/wow/profession/%d", id), "static", nil, &payload)
	return payload, err
}

func (c *BlizzardClient) GetSkillTier(ctx context.Context, professionID, tierID int) (SkillTierResponse, error) {
	var payload SkillTierResponse
	err := c.getJSON(ctx, fmt.Sprintf("/data/wow/profession/%d/skill-tier/%d", professionID, tierID), "static", nil, &payload)
	return payload, err
}

func (c *BlizzardClient) GetRecipe(ctx context.Context, id int) (RecipeResponse, error) {
	var payload RecipeResponse
	err := c.getJSON(ctx, fmt.Sprintf("/data/wow/recipe/%d", id), "static", nil, &payload)
	return payload, err
}

// getJSON is the decode-and-close convenience wrapper for small static
// payloads; large or streaming responses go through apiRequest directly.
func (c *BlizzardClient) getJSON(ctx context.Context, path, namespace string, query url.Values, target any) error {
	response, err := c.apiRequest(ctx, http.MethodGet, path, namespace, query, "")
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if err := json.NewDecoder(response.Body).Decode(target); err != nil {
		return fmt.Errorf("decode %s: %w", path, err)
	}
	return nil
}

// do executes the request factory with retries: up to four attempts with
// exponential backoff (1s, 2s, 4s), extended whenever the server sends a
// Retry-After. Only network errors, 5xx, and 429 are retried — other 4xx
// responses are permanent for this payload shape, so fail fast. The request
// is built fresh per attempt because bodies and contexts are single-use.
// Error bodies are truncated to 4 KB to keep failure logs bounded.
func (c *BlizzardClient) do(ctx context.Context, request func() (*http.Request, error)) (*http.Response, error) {
	var lastErr error
	var retryAt time.Time
	for attempt := 0; attempt < 4; attempt++ {
		if attempt > 0 {
			delay := time.Duration(1<<(attempt-1)) * time.Second
			if retryDelay := time.Until(retryAt); retryDelay > delay {
				delay = retryDelay
			}
			timer := time.NewTimer(delay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return nil, ctx.Err()
			case <-timer.C:
			}
		}
		if err := c.limiter.Wait(ctx); err != nil {
			return nil, err
		}
		req, err := request()
		if err != nil {
			return nil, err
		}
		response, err := c.http.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		if response.StatusCode >= 200 && response.StatusCode < 300 || response.StatusCode == http.StatusNotModified {
			return response, nil
		}
		body, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		response.Body.Close()
		lastErr = fmt.Errorf("HTTP %s: %s", response.Status, strings.TrimSpace(string(body)))
		if response.StatusCode != http.StatusTooManyRequests && response.StatusCode < 500 {
			break
		}
		retryAt = retryAfter(response.Header.Get("Retry-After"), time.Now())
	}
	return nil, lastErr
}

// retryAfter parses a Retry-After header, accepting both the delay-seconds
// and HTTP-date forms; it returns the zero time when absent or malformed,
// which callers treat as "no hint".
func retryAfter(value string, now time.Time) time.Time {
	if seconds, err := strconv.Atoi(strings.TrimSpace(value)); err == nil && seconds >= 0 {
		return now.Add(time.Duration(seconds) * time.Second)
	}
	if at, err := http.ParseTime(value); err == nil {
		return at
	}
	return time.Time{}
}
