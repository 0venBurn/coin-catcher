package scraper

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const validTSMCSV = "itemId,name,marketValue,historical,avgSalePrice,saleRate,soldPerDay,updatedAt\n" +
	"35,Bent Staff,99999999900,895579900,950095,0.002,0,2026-08-22T02:17:32Z\n" +
	"36,,10,11,12,0.5,3.25,2026-08-22T02:17:32Z\n"

func TestTSMFetchSendsValidatorsAndDoesNotRead304Body(t *testing.T) {
	var closed atomic.Bool
	client := NewTSMClient(&http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if got := request.Header.Get("If-None-Match"); got != `"abc"` {
			t.Fatalf("If-None-Match = %q", got)
		}
		if got := request.Header.Get("If-Modified-Since"); got != "yesterday" {
			t.Fatalf("If-Modified-Since = %q", got)
		}
		return &http.Response{StatusCode: http.StatusNotModified, Header: make(http.Header), Body: &failReadCloser{closed: &closed}}, nil
	})}, "eu")

	result, err := client.Fetch(context.Background(), TSMValidators{ETag: `"abc"`, LastModified: "yesterday"}, func(TSMItem) error {
		t.Fatal("304 invoked row consumer")
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Changed {
		t.Fatal("304 reported changed")
	}
	if !closed.Load() {
		t.Fatal("304 body was not closed")
	}
}

type failReadCloser struct{ closed *atomic.Bool }

func (*failReadCloser) Read([]byte) (int, error) { panic("304 body read") }
func (body *failReadCloser) Close() error        { body.closed.Store(true); return nil }

func TestTSMFetchValidCSVAndBlankName(t *testing.T) {
	client := testTSMClient(func(*http.Request) (*http.Response, error) {
		return tsmResponse(http.StatusOK, validTSMCSV), nil
	})
	var rows []TSMItem
	result, err := client.Fetch(context.Background(), TSMValidators{}, func(item TSMItem) error { rows = append(rows, item); return nil })
	if err != nil {
		t.Fatal(err)
	}
	if !result.Changed || result.Rows != 2 || result.UpdatedAt.Format(time.RFC3339) != "2026-08-22T02:17:32Z" {
		t.Fatalf("result = %+v", result)
	}
	if rows[0].MarketValue != 99_999_999_900 {
		t.Fatalf("large price = %d", rows[0].MarketValue)
	}
	if rows[1].Name != nil {
		t.Fatalf("blank name = %q, want nil", *rows[1].Name)
	}
}

func TestTSMFetchRejectsMalformedInput(t *testing.T) {
	tests := map[string]string{
		"header":             "id,name,marketValue,historical,avgSalePrice,saleRate,soldPerDay,updatedAt\n",
		"overflow":           strings.Replace(validTSMCSV, "99999999900", "999999999999999999999999", 1),
		"timestamp mismatch": strings.Replace(validTSMCSV, "2026-08-22T02:17:32Z\n", "2026-08-23T02:17:32Z\n", 1),
		"NaN":                strings.Replace(validTSMCSV, "0.002", "NaN", 1),
	}
	for name, csv := range tests {
		t.Run(name, func(t *testing.T) {
			client := testTSMClient(func(*http.Request) (*http.Response, error) { return tsmResponse(http.StatusOK, csv), nil })
			if _, err := client.Fetch(context.Background(), TSMValidators{}, func(TSMItem) error { return nil }); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestTSMFetchRetriesRetryableStatus(t *testing.T) {
	var requests atomic.Int32
	client := testTSMClient(func(*http.Request) (*http.Response, error) {
		if requests.Add(1) == 1 {
			return tsmResponse(http.StatusServiceUnavailable, "busy"), nil
		}
		return tsmResponse(http.StatusOK, validTSMCSV), nil
	})
	client.retryDelay = 0
	result, err := client.Fetch(context.Background(), TSMValidators{}, func(TSMItem) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	if requests.Load() != 2 || result.Rows != 2 {
		t.Fatalf("requests=%d result=%+v", requests.Load(), result)
	}
}

func TestTSMFetchCancellation(t *testing.T) {
	client := testTSMClient(func(request *http.Request) (*http.Response, error) {
		<-request.Context().Done()
		return nil, request.Context().Err()
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := client.Fetch(ctx, TSMValidators{}, func(TSMItem) error { return nil })
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v", err)
	}
}

func testTSMClient(transport roundTripFunc) *TSMClient {
	client := NewTSMClient(&http.Client{Transport: transport}, "eu")
	client.baseURL = "https://example.test"
	return client
}

func tsmResponse(status int, body string) *http.Response {
	header := make(http.Header)
	header.Set("ETag", `"new"`)
	header.Set("Last-Modified", "today")
	return &http.Response{StatusCode: status, Status: http.StatusText(status), Header: header, Body: io.NopCloser(strings.NewReader(body))}
}
