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
	ClientID             string
	ClientSecret         string
	DatabaseURL          string
	Regions              []string
	PollInterval         time.Duration
	PollWindow           time.Duration
	RequestTimeout       time.Duration
	APIRequestsPerSecond int
	RecipeWorkers        int
	RunOnStart           bool
}

func LoadConfig() (Config, error) {
	envFile := os.Getenv("ENV_FILE")
	if envFile == "" {
		envFile = ".env"
		if _, err := os.Stat(envFile); errors.Is(err, os.ErrNotExist) {
			envFile = "scraper/.env"
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
		PollInterval:         30 * time.Second,
		PollWindow:           20 * time.Minute,
		RequestTimeout:       2 * time.Minute,
		APIRequestsPerSecond: 20,
		RecipeWorkers:        5,
		RunOnStart:           boolValue("SCRAPE_ON_START"),
	}
	if config.ClientID == "" || config.ClientSecret == "" {
		return Config{}, fmt.Errorf("CLIENT_ID and CLIENT_SECRET are required")
	}
	var err error
	if config.PollInterval, err = durationValue("POLL_INTERVAL", config.PollInterval); err != nil {
		return Config{}, err
	}
	if config.PollWindow, err = durationValue("POLL_WINDOW", config.PollWindow); err != nil {
		return Config{}, err
	}
	if config.APIRequestsPerSecond, err = intValue("API_REQUESTS_PER_SECOND", config.APIRequestsPerSecond, 1, 20); err != nil {
		return Config{}, err
	}
	if config.RecipeWorkers, err = intValue("RECIPE_WORKERS", config.RecipeWorkers, 1, 8); err != nil {
		return Config{}, err
	}
	return config, nil
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

func boolValue(name string) bool {
	value, _ := strconv.ParseBool(os.Getenv(name))
	return value
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
