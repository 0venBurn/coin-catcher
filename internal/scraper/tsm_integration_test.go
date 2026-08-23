package scraper

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Run with TEST_DATABASE_URL pointing at a disposable TimescaleDB database.
func TestTSMSnapshotAtomicityAndRestartState(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if err := Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	_, _ = pool.Exec(ctx, "TRUNCATE tsm_region_item_snapshots, tsm_region_item_state")

	body := validTSMCSV
	client := testTSMClient(func(*http.Request) (*http.Response, error) { return tsmResponse(http.StatusOK, body), nil })
	s := NewTSMScraper(pool, []*TSMClient{client}, slog.New(slog.NewTextHandler(io.Discard, nil)), Config{TSMPollWindow: time.Hour})
	state := &tsmState{}
	first, err := s.storeSnapshot(ctx, client, state)
	if err != nil {
		t.Fatal(err)
	}
	state.validators = TSMValidators{ETag: first.ETag, LastModified: first.LastModified}
	state.sourceAt = first.UpdatedAt

	reloaded, err := s.loadState(ctx, "eu")
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.validators.ETag != `"new"` || !reloaded.sourceAt.Equal(first.UpdatedAt) {
		t.Fatalf("reloaded state = %+v", reloaded)
	}

	// COPY at least one full batch, then fail parsing. The transaction must
	// retain both the old two rows and old validators.
	var failed bytes.Buffer
	failed.WriteString(strings.Join(tsmCSVHeader, ",") + "\n")
	for i := 1; i <= tsmCopyBatchSize; i++ {
		fmt.Fprintf(&failed, "%d,item,1,2,3,0.1,1,2026-08-23T02:17:32Z\n", i)
	}
	failed.WriteString("6000,item,1,2,3,0.1,1,2026-08-24T02:17:32Z\n")
	body = failed.String()
	if _, err := s.storeSnapshot(ctx, client, state); err == nil {
		t.Fatal("expected mixed timestamp failure")
	}
	var count int
	var etag string
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM tsm_region_item_snapshots").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, "SELECT etag FROM tsm_region_item_state WHERE region='eu'").Scan(&etag); err != nil {
		t.Fatal(err)
	}
	if count != 2 || etag != `"new"` {
		t.Fatalf("after rollback count=%d etag=%q", count, etag)
	}

	body = validTSMCSV
	if _, err := s.storeSnapshot(ctx, client, state); !errors.Is(err, errTSMSnapshotNotNewer) {
		t.Fatalf("same timestamp error = %v", err)
	}
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM tsm_region_item_snapshots").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("same timestamp created rows: %d", count)
	}

	if err := migrateDown(ctx, pool); err != nil {
		t.Fatalf("migration down: %v", err)
	}
	var snapshotsTable, stateTable *string
	if err := pool.QueryRow(ctx, `SELECT to_regclass('public.tsm_region_item_snapshots')::text, to_regclass('public.tsm_region_item_state')::text`).Scan(&snapshotsTable, &stateTable); err != nil {
		t.Fatal(err)
	}
	if snapshotsTable != nil || stateTable != nil {
		t.Fatalf("down left tables: snapshots=%v state=%v", snapshotsTable, stateTable)
	}
	if err := Migrate(ctx, pool); err != nil {
		t.Fatalf("migration reapply: %v", err)
	}
}

// This opt-in test verifies the real public EU/US contracts and reports a
// representative import duration and relation size without burdening unit tests.
func TestTSMLiveImport(t *testing.T) {
	if os.Getenv("TSM_LIVE_TEST") != "1" {
		t.Skip("TSM_LIVE_TEST not set")
	}
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Fatal("TEST_DATABASE_URL must point at a disposable TimescaleDB database")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if err := Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	var peakHeap atomic.Uint64
	stopSampling := make(chan struct{})
	go func() {
		ticker := time.NewTicker(time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-stopSampling:
				return
			case <-ticker.C:
				var memory runtime.MemStats
				runtime.ReadMemStats(&memory)
				for current := peakHeap.Load(); memory.HeapAlloc > current && !peakHeap.CompareAndSwap(current, memory.HeapAlloc); current = peakHeap.Load() {
				}
			}
		}
	}()
	for _, region := range []string{"eu", "us"} {
		client := NewTSMClient(&http.Client{Timeout: 2 * time.Minute}, region)
		s := NewTSMScraper(pool, []*TSMClient{client}, logger, Config{TSMPollWindow: time.Hour})
		started := time.Now()
		result, err := s.storeSnapshot(ctx, client, &tsmState{})
		if err != nil {
			t.Fatalf("%s import: %v", region, err)
		}
		t.Logf("region=%s rows=%d source=%s duration=%s", region, result.Rows, result.UpdatedAt, time.Since(started))
	}
	close(stopSampling)
	var rows int64
	var size int64
	if err := pool.QueryRow(ctx, "SELECT count(*), hypertable_size('tsm_region_item_snapshots') FROM tsm_region_item_snapshots").Scan(&rows, &size); err != nil {
		t.Fatal(err)
	}
	t.Logf("total_rows=%d relation_bytes=%d peak_heap_bytes=%d", rows, size, peakHeap.Load())
}
