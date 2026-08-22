package scraper

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config is the fully resolved runtime configuration. Every field has a
// default so the scraper can start with only credentials set; env overrides
// exist for bounding runs and tuning throughput (see .env.example).
type Config struct {
	ClientID     string
	ClientSecret string
	DatabaseURL  string
	Regions      []string
	// Schedule is the resolved polling window: POLL_START (relative delay)
	// and SCRAPE_FROM (absolute) merge into StartAt; POLL_END (relative
	// duration) and SCRAPE_UNTIL (absolute) merge into StopAt. A zero StopAt
	// polls indefinitely.
	Schedule             Schedule
	PollWindow           time.Duration // POLL_WINDOW, sleep between poll passes.
	RequestTimeout       time.Duration
	APIRequestsPerSecond int
	RecipeWorkers        int
}

// Schedule is the concrete [StartAt, StopAt] window the scraper runs in.
type Schedule struct {
	StartAt time.Time
	StopAt  time.Time // zero means unbounded
}

// LoadConfig reads configuration from the environment, after first loading
// a .env file so local runs work without exporting variables by hand. The
// .env lookup order supports both running from the repo root and from
// internal/scraper; real environment variables always win over file values.
// Validation errors are reported per variable with its name so misconfig
// is fixable without reading source.
func LoadConfig() (Config, error) {
	envFile := os.Getenv("ENV_FILE")
	if envFile == "" {
		envFile = ".env"
		if _, err := os.Stat(envFile); errors.Is(err, os.ErrNotExist) {
			envFile = "internal/scraper/.env"
		}
	}
	if err := loadDotEnv(envFile); err != nil && !errors.Is(err, os.ErrNotExist) {
		return Config{}, fmt.Errorf("load %s: %w", envFile, err)
	}

	// Defaults chosen for one container polling two regions well under
	// Blizzard's limits: 20 req/s matches the documented commodity cap,
	// 2 minutes covers slow streaming commodity responses.
	config := Config{
		ClientID:             os.Getenv("CLIENT_ID"),
		ClientSecret:         os.Getenv("CLIENT_SECRET"),
		DatabaseURL:          valueOrDefault("DATABASE_URL", "postgres://coin_catcher:coin_catcher@localhost:5432/coin_catcher?sslmode=disable"),
		Regions:              []string{"eu", "us"},
		PollWindow:           30 * time.Second,
		RequestTimeout:       2 * time.Minute,
		APIRequestsPerSecond: 20,
		RecipeWorkers:        5,
	}
	if config.ClientID == "" || config.ClientSecret == "" {
		return Config{}, fmt.Errorf("CLIENT_ID and CLIENT_SECRET are required")
	}
	var err error
	pollStart, err := envDuration("POLL_START", 0, 0)
	if err != nil {
		return Config{}, err
	}
	pollEnd, err := envDuration("POLL_END", 0, 0)
	if err != nil {
		return Config{}, err
	}
	if config.PollWindow, err = envDuration("POLL_WINDOW", config.PollWindow, time.Second); err != nil {
		return Config{}, err
	}
	scrapeFrom, err := timeValue("SCRAPE_FROM")
	if err != nil {
		return Config{}, err
	}
	scrapeUntil, err := timeValue("SCRAPE_UNTIL")
	if err != nil {
		return Config{}, err
	}
	if !scrapeFrom.IsZero() && !scrapeUntil.IsZero() && scrapeUntil.Before(scrapeFrom) {
		return Config{}, fmt.Errorf("SCRAPE_UNTIL must not be before SCRAPE_FROM")
	}
	config.Schedule = resolveSchedule(time.Now(), pollStart, pollEnd, scrapeFrom, scrapeUntil)
	if config.APIRequestsPerSecond, err = intValue("API_REQUESTS_PER_SECOND", config.APIRequestsPerSecond, 1, 20); err != nil {
		return Config{}, err
	}
	if config.RecipeWorkers, err = intValue("RECIPE_WORKERS", config.RecipeWorkers, 1, 8); err != nil {
		return Config{}, err
	}
	return config, nil
}

// resolveSchedule merges the relative bounds (POLL_START delay, POLL_END
// duration since start) with the absolute ones (SCRAPE_FROM, SCRAPE_UNTIL)
// into one concrete window. StopAt is zero when polling is unbounded.
func resolveSchedule(now time.Time, pollStart, pollEnd time.Duration, scrapeFrom, scrapeUntil time.Time) Schedule {
	startAt := now.Add(pollStart)
	if scrapeFrom.After(startAt) {
		startAt = scrapeFrom
	}
	var stopAt time.Time
	if pollEnd > 0 {
		stopAt = startAt.Add(pollEnd)
	}
	if !scrapeUntil.IsZero() && (stopAt.IsZero() || scrapeUntil.Before(stopAt)) {
		stopAt = scrapeUntil
	}
	return Schedule{StartAt: startAt, StopAt: stopAt}
}

// valueOrDefault returns the env value or fallback when unset/empty.
func valueOrDefault(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}

// envDuration parses a Go duration; empty/unset yields fallback. Values below
// minimum are rejected (minimum 0 accepts zero as "no constraint").
func envDuration(name string, fallback, minimum time.Duration) (time.Duration, error) {
	value := os.Getenv(name)
	if value == "" {
		return fallback, nil
	}
	duration, err := time.ParseDuration(value)
	if err != nil || duration < minimum {
		return 0, fmt.Errorf("%s must be a Go duration of at least %s", name, minimum)
	}
	return duration, nil
}

// timeValue parses an RFC3339 timestamp; empty/unset yields the zero time.
func timeValue(name string) (time.Time, error) {
	value := os.Getenv(name)
	if value == "" {
		return time.Time{}, nil
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}, fmt.Errorf("%s must be an RFC3339 timestamp", name)
	}
	return parsed, nil
}

// intValue parses an integer clamped to [minimum, maximum]; empty/unset
// yields fallback.
func intValue(name string, fallback, minimum, maximum int) (int, error) {
	value := os.Getenv(name)
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < minimum || parsed > maximum {
		return 0, fmt.Errorf("%s must be an integer between %d and %d", name, minimum, maximum)
	}
	return parsed, nil
}

// loadDotEnv applies KEY=VALUE lines from path to the process environment.
// Existing variables are never overwritten, matching dotenv convention so
// explicit exports take precedence over file contents. Supports `export `
// prefixes, comments, blank lines, and matched single/double quotes.
func loadDotEnv(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		name, value, ok := strings.Cut(line, "=")
		if !ok {
			return fmt.Errorf("invalid line %q", line)
		}
		name, value = strings.TrimSpace(name), strings.TrimSpace(value)
		if len(value) >= 2 && ((value[0] == '\'' && value[len(value)-1] == '\'') || (value[0] == '"' && value[len(value)-1] == '"')) {
			value = value[1 : len(value)-1]
		}
		if _, exists := os.LookupEnv(name); !exists {
			if err := os.Setenv(name, value); err != nil {
				return err
			}
		}
	}
	return scanner.Err()
}
