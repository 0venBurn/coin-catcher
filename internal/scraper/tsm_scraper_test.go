package scraper

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"
)

func TestTSMRunRegionExitsWhenScheduleAlreadyEnded(t *testing.T) {
	s := NewTSMScraper(nil, nil, slog.New(slog.NewTextHandler(io.Discard, nil)), Config{
		Schedule:      Schedule{StopAt: time.Now().Add(-time.Minute)},
		TSMPollWindow: time.Hour,
	})
	s.runRegion(context.Background(), NewTSMClient(nil, "eu"))
}

func TestTSMRunRegionExitsWhenStartWaitCancelled(t *testing.T) {
	s := NewTSMScraper(nil, nil, slog.New(slog.NewTextHandler(io.Discard, nil)), Config{
		Schedule:      Schedule{StartAt: time.Now().Add(time.Hour)},
		TSMPollWindow: time.Hour,
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	s.runRegion(ctx, NewTSMClient(nil, "eu"))
}

func TestTSMStaleWarningIsBounded(t *testing.T) {
	var output bytes.Buffer
	s := &TSMScraper{log: slog.New(slog.NewTextHandler(&output, nil))}
	base := time.Date(2026, time.August, 22, 0, 0, 0, 0, time.UTC)
	state := &tsmState{sourceAt: base}
	s.warnIfTSMStale("eu", state, base.Add(25*time.Hour))
	s.warnIfTSMStale("eu", state, base.Add(26*time.Hour))
	s.warnIfTSMStale("eu", state, base.Add(27*time.Hour))
	s.warnIfTSMStale("eu", state, base.Add(32*time.Hour))
	if count := strings.Count(output.String(), "TSM data stale"); count != 2 {
		t.Fatalf("warning count = %d: %s", count, output.String())
	}
}

func TestTSMPollWindowConfig(t *testing.T) {
	t.Setenv("ENV_FILE", t.TempDir()+"/missing")
	t.Setenv("CLIENT_ID", "id")
	t.Setenv("CLIENT_SECRET", "secret")
	t.Setenv("TSM_POLL_WINDOW", "90m")
	config, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if config.TSMPollWindow != 90*time.Minute {
		t.Fatalf("TSMPollWindow = %s", config.TSMPollWindow)
	}

	t.Setenv("TSM_POLL_WINDOW", "30s")
	if _, err := LoadConfig(); err == nil || !strings.Contains(err.Error(), "TSM_POLL_WINDOW") {
		t.Fatalf("validation error = %v", err)
	}
}
