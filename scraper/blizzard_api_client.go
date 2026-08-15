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

const OAuthTokenURL = "https://oauth.battle.net/token"

type APIRateLimiter struct {
	mu       sync.Mutex
	interval time.Duration
	next     time.Time
}

func NewAPIRateLimiter(requestsPerSecond int) *APIRateLimiter {
	if requestsPerSecond <= 0 {
		requestsPerSecond = 20
	}
	return &APIRateLimiter{interval: time.Second / time.Duration(requestsPerSecond)}
}

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

type BlizzardClient struct {
	http         *http.Client
	clientID     string
	clientSecret string
	region       string
	apiHost      string
	limiter      *APIRateLimiter
	token        string
	tokenExpires time.Time
	mu           sync.Mutex
}

func NewBlizzardClient(httpClient *http.Client, clientID, clientSecret, region string, limiter *APIRateLimiter) *BlizzardClient {
	return &BlizzardClient{
		http: httpClient, clientID: clientID, clientSecret: clientSecret,
		region: region, apiHost: region + ".api.blizzard.com", limiter: limiter,
	}
}

func (c *BlizzardClient) tokenFor(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.token != "" && time.Until(c.tokenExpires) > time.Minute {
		return c.token, nil
	}

	form := url.Values{"grant_type": {"client_credentials"}}.Encode()
	response, err := c.do(ctx, func() (*http.Request, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, OAuthTokenURL, strings.NewReader(form))
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

func (c *BlizzardClient) apiRequest(ctx context.Context, method, path, namespace string, query url.Values, lastModified string) (*http.Response, error) {
	token, err := c.tokenFor(ctx)
	if err != nil {
		return nil, err
	}
	if query == nil {
		query = url.Values{}
	}
	query.Set("namespace", namespace+"-"+c.region)
	query.Set("locale", "en_US")
	u := url.URL{Scheme: "https", Host: c.apiHost, Path: path, RawQuery: query.Encode()}
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

func (c *BlizzardClient) GetCommodities(ctx context.Context, lastModified string) ([]CommodityAuction, string, bool, error) {
	auctions := make([]CommodityAuction, 0)
	modified, changed, _, err := c.StreamCommodities(ctx, lastModified, func(auction CommodityAuction) error {
		auctions = append(auctions, auction)
		return nil
	})
	return auctions, modified, changed, err
}

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
	response, err := c.apiRequest(ctx, http.MethodGet, fmt.Sprintf("/data/wow/recipe/%d", id), "static", nil, "")
	if err != nil {
		return RecipeResponse{}, err
	}
	defer response.Body.Close()
	var payload RecipeResponse
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		return RecipeResponse{}, fmt.Errorf("decode recipe %d: %w", id, err)
	}
	return payload, nil
}

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

func retryAfter(value string, now time.Time) time.Time {
	if seconds, err := strconv.Atoi(strings.TrimSpace(value)); err == nil && seconds >= 0 {
		return now.Add(time.Duration(seconds) * time.Second)
	}
	if at, err := http.ParseTime(value); err == nil {
		return at
	}
	return time.Time{}
}
