package scraper

import (
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"
)

const (
	tsmBaseURL    = "https://public-data.tradeskillmaster.com/retail"
	tsmMaxRetries = 3
)

var tsmCSVHeader = []string{"itemId", "name", "marketValue", "historical", "avgSalePrice", "saleRate", "soldPerDay", "updatedAt"}

// TSMValidators are persisted conditional-request values for one region.
type TSMValidators struct {
	ETag         string
	LastModified string
}

// TSMItem is one validated row from a regional TSM item snapshot.
type TSMItem struct {
	ItemID           int32
	Name             *string
	MarketValue      int64
	Historical       int64
	AverageSalePrice int64
	SaleRate         float64
	SoldPerDay       float64
	UpdatedAt        time.Time
}

// TSMFetchResult describes the response after its CSV body has been consumed.
type TSMFetchResult struct {
	Changed      bool
	ETag         string
	LastModified string
	Rows         int
	UpdatedAt    time.Time
}

// TSMClient streams one public regional CSV without Blizzard credentials or rate limiting.
type TSMClient struct {
	httpClient *http.Client
	region     string
	baseURL    string
	retryDelay time.Duration
}

func NewTSMClient(httpClient *http.Client, region string) *TSMClient {
	return &TSMClient{httpClient: httpClient, region: region, baseURL: tsmBaseURL, retryDelay: time.Second}
}

// Fetch sends validators directly on GET. A 200 body is parsed row-by-row and
// always closed; a 304 returns without attempting CSV decoding.
func (c *TSMClient) Fetch(ctx context.Context, validators TSMValidators, consume func(TSMItem) error) (TSMFetchResult, error) {
	response, err := c.getWithRetry(ctx, validators)
	if err != nil {
		return TSMFetchResult{}, err
	}
	defer response.Body.Close()

	result := TSMFetchResult{
		Changed: response.StatusCode == http.StatusOK,
		ETag:    response.Header.Get("ETag"), LastModified: response.Header.Get("Last-Modified"),
	}
	if response.StatusCode == http.StatusNotModified {
		return result, nil
	}

	reader := csv.NewReader(response.Body)
	header, err := reader.Read()
	if err != nil {
		return TSMFetchResult{}, fmt.Errorf("read TSM CSV header: %w", err)
	}
	if !slices.Equal(header, tsmCSVHeader) {
		return TSMFetchResult{}, fmt.Errorf("unexpected TSM CSV header: %q", header)
	}
	for line := 2; ; line++ {
		record, err := reader.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return TSMFetchResult{}, fmt.Errorf("read TSM CSV row %d: %w", line, err)
		}
		item, err := parseTSMItem(record)
		if err != nil {
			return TSMFetchResult{}, fmt.Errorf("parse TSM CSV row %d: %w", line, err)
		}
		if result.UpdatedAt.IsZero() {
			result.UpdatedAt = item.UpdatedAt
		} else if !item.UpdatedAt.Equal(result.UpdatedAt) {
			return TSMFetchResult{}, fmt.Errorf("TSM CSV has multiple updatedAt values: %s and %s", result.UpdatedAt, item.UpdatedAt)
		}
		if err := consume(item); err != nil {
			return TSMFetchResult{}, err
		}
		result.Rows++
	}
	if result.Rows == 0 {
		return TSMFetchResult{}, errors.New("TSM CSV contains no item rows")
	}
	return result, nil
}

func (c *TSMClient) getWithRetry(ctx context.Context, validators TSMValidators) (*http.Response, error) {
	var lastErr error
	for attempt := 0; attempt <= tsmMaxRetries; attempt++ {
		if attempt > 0 {
			delay := time.Duration(1<<(attempt-1)) * c.retryDelay
			timer := time.NewTimer(delay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return nil, ctx.Err()
			case <-timer.C:
			}
		}
		request, err := http.NewRequestWithContext(ctx, http.MethodGet,
			fmt.Sprintf("%s/%s/region/items.csv", c.baseURL, c.region), nil)
		if err != nil {
			return nil, err
		}
		request.Header.Set("User-Agent", "coin-catcher/1.0")
		if validators.ETag != "" {
			request.Header.Set("If-None-Match", validators.ETag)
		}
		if validators.LastModified != "" {
			request.Header.Set("If-Modified-Since", validators.LastModified)
		}
		response, err := c.httpClient.Do(request)
		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			lastErr = err
			continue
		}
		if response.StatusCode == http.StatusOK || response.StatusCode == http.StatusNotModified {
			return response, nil
		}
		body, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		response.Body.Close()
		lastErr = fmt.Errorf("TSM HTTP %s: %s", response.Status, strings.TrimSpace(string(body)))
		if response.StatusCode != http.StatusTooManyRequests && response.StatusCode < 500 {
			break
		}
		if retryAt := retryAfter(response.Header.Get("Retry-After"), time.Now()); !retryAt.IsZero() {
			delay := time.Until(retryAt)
			if delay > 0 {
				timer := time.NewTimer(delay)
				select {
				case <-ctx.Done():
					timer.Stop()
					return nil, ctx.Err()
				case <-timer.C:
				}
			}
		}
	}
	return nil, lastErr
}

func parseTSMItem(record []string) (TSMItem, error) {
	if len(record) != len(tsmCSVHeader) {
		return TSMItem{}, fmt.Errorf("got %d columns, want %d", len(record), len(tsmCSVHeader))
	}
	itemID, err := strconv.ParseInt(record[0], 10, 32)
	if err != nil || itemID <= 0 {
		return TSMItem{}, fmt.Errorf("invalid itemId %q", record[0])
	}
	parsePrice := func(index int) (int64, error) {
		value, err := strconv.ParseInt(record[index], 10, 64)
		if err != nil || value < 0 {
			return 0, fmt.Errorf("invalid %s %q", tsmCSVHeader[index], record[index])
		}
		return value, nil
	}
	market, err := parsePrice(2)
	if err != nil {
		return TSMItem{}, err
	}
	historical, err := parsePrice(3)
	if err != nil {
		return TSMItem{}, err
	}
	average, err := parsePrice(4)
	if err != nil {
		return TSMItem{}, err
	}
	saleRate, err := strconv.ParseFloat(record[5], 64)
	if err != nil || math.IsNaN(saleRate) || math.IsInf(saleRate, 0) || saleRate < 0 || saleRate > 1 {
		return TSMItem{}, fmt.Errorf("invalid saleRate %q", record[5])
	}
	sold, err := strconv.ParseFloat(record[6], 64)
	if err != nil || math.IsNaN(sold) || math.IsInf(sold, 0) || sold < 0 {
		return TSMItem{}, fmt.Errorf("invalid soldPerDay %q", record[6])
	}
	updated, err := time.Parse(time.RFC3339, record[7])
	if err != nil {
		return TSMItem{}, fmt.Errorf("invalid updatedAt %q", record[7])
	}
	var name *string
	if record[1] != "" {
		value := record[1]
		name = &value
	}
	return TSMItem{int32(itemID), name, market, historical, average, saleRate, sold, updated.UTC()}, nil
}
