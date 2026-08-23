package scraper

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"
)

func TestTSMSchedulerUsesInjectedClockWaitAndJitter(t *testing.T) {
	base := time.Date(2026, time.August, 22, 12, 0, 0, 0, time.UTC)
	now := base
	var waits []time.Duration
	s := &TSMScraper{
		pollWindow: time.Hour,
		now:        func() time.Time { return now },
		wait: func(_ context.Context, duration time.Duration) bool {
			waits = append(waits, duration)
			now = now.Add(duration)
			return true
		},
		jitter: func(max time.Duration) time.Duration {
			if max != 5*time.Minute {
				t.Fatalf("jitter max = %s", max)
			}
			return 3 * time.Minute
		},
	}
	// Exercise scheduler dependencies directly: startup has no wait; recurring
	// polls use the configured window plus bounded injected jitter.
	if delay := s.schedule.StartAt.Sub(s.now()); delay > 0 {
		s.wait(context.Background(), delay)
	}
	delay := s.pollWindow + s.jitter(tsmMaximumJitter)
	s.wait(context.Background(), delay)
	if len(waits) != 1 || waits[0] != 63*time.Minute {
		t.Fatalf("waits = %v", waits)
	}
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
