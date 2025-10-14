package config

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Config holds all application configuration
type Config struct {
	// Server configuration
	Host string
	Port int

	// Database configuration
	DatabasePath string

	// Strava API configuration
	StravaClientID     string
	StravaClientSecret string

	// Internal API authentication
	InternalAPIKey string

	// Logging configuration
	LogLevel string
}

// Load loads configuration from environment variables and .env file
// Environment variables take precedence over .env file values
func Load() (*Config, error) {
	// Load .env file if it exists (but don't fail if it doesn't)
	loadEnvFile(".env")

	config := &Config{
		Host:               getEnv("HOST", "localhost"),
		Port:               getEnvInt("PORT", 4101),
		DatabasePath:       getEnv("DATABASE_PATH", "./data.db"),
		StravaClientID:     getEnv("STRAVA_CLIENT_ID", ""),
		StravaClientSecret: getEnv("STRAVA_CLIENT_SECRET", ""),
		InternalAPIKey:     getEnv("INTERNAL_API_KEY", ""),
		LogLevel:           getEnv("LOG_LEVEL", "info"),
	}

	// Validate required fields
	if err := config.Validate(); err != nil {
		return nil, err
	}

	return config, nil
}

// Validate checks that all required configuration values are set
func (c *Config) Validate() error {
	if c.StravaClientID == "" {
		return fmt.Errorf("STRAVA_CLIENT_ID is required")
	}
	if c.StravaClientSecret == "" {
		return fmt.Errorf("STRAVA_CLIENT_SECRET is required")
	}
	if c.InternalAPIKey == "" {
		return fmt.Errorf("INTERNAL_API_KEY is required")
	}
	if c.Port < 1 || c.Port > 65535 {
		return fmt.Errorf("PORT must be between 1 and 65535")
	}
	if c.LogLevel != "debug" && c.LogLevel != "info" && c.LogLevel != "warn" && c.LogLevel != "error" {
		return fmt.Errorf("LOG_LEVEL must be one of: debug, info, warn, error")
	}
	return nil
}

// getEnv retrieves an environment variable or returns a default value
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// getEnvInt retrieves an environment variable as an integer or returns a default value
func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	return defaultValue
}

// loadEnvFile loads environment variables from a .env file
// This is a simple implementation that handles basic .env file format
func loadEnvFile(filename string) error {
	file, err := os.Open(filename)
	if err != nil {
		// It's okay if .env doesn't exist
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Parse KEY=VALUE
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		// Remove quotes if present
		value = strings.Trim(value, `"'`)

		// Only set if not already set in environment
		if os.Getenv(key) == "" {
			os.Setenv(key, value)
		}
	}

	return scanner.Err()
}
