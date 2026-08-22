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
	pollStart, err := optionalDurationValue("POLL_START", 0)
	if err != nil {
		return Config{}, err
	}
	pollEnd, err := optionalDurationValue("POLL_END", 0)
	if err != nil {
		return Config{}, err
	}
	if config.PollWindow, err = durationValue("POLL_WINDOW", config.PollWindow); err != nil {
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

func valueOrDefault(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}

func durationValue(name string, fallback time.Duration) (time.Duration, error) {
	value := os.Getenv(name)
	if value == "" {
		return fallback, nil
	}
	duration, err := time.ParseDuration(value)
	if err != nil || duration <= 0 {
		return 0, fmt.Errorf("%s must be a positive Go duration", name)
	}
	return duration, nil
}

// optionalDurationValue parses a Go duration where zero is a valid value
// ("0", empty, or unset all mean "no constraint").
func optionalDurationValue(name string, fallback time.Duration) (time.Duration, error) {
	value := os.Getenv(name)
	if value == "" {
		return fallback, nil
	}
	duration, err := time.ParseDuration(value)
	if err != nil || duration < 0 {
		return 0, fmt.Errorf("%s must be a non-negative Go duration", name)
	}
	return duration, nil
}

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
