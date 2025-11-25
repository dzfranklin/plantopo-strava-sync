package config

import (
	"fmt"
	"os"
	"strconv"
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
	StravaVerifyToken  string

	// Internal API configuration
	InternalAPIKey string

	// Logging configuration
	LogLevel string
}

// Load reads configuration from environment variables
// It fails fast if required variables are missing
func Load() (*Config, error) {
	cfg := &Config{
		// Optional values with defaults
		Host:         getEnv("HOST", "localhost"),
		Port:         getEnvInt("PORT", 4101),
		DatabasePath: getEnv("DATABASE_PATH", "./data.db"),
		LogLevel:     getEnv("LOG_LEVEL", "info"),
	}

	// Required values
	var missingVars []string

	cfg.StravaClientID = os.Getenv("STRAVA_CLIENT_ID")
	if cfg.StravaClientID == "" {
		missingVars = append(missingVars, "STRAVA_CLIENT_ID")
	}

	cfg.StravaClientSecret = os.Getenv("STRAVA_CLIENT_SECRET")
	if cfg.StravaClientSecret == "" {
		missingVars = append(missingVars, "STRAVA_CLIENT_SECRET")
	}

	cfg.StravaVerifyToken = os.Getenv("STRAVA_VERIFY_TOKEN")
	if cfg.StravaVerifyToken == "" {
		missingVars = append(missingVars, "STRAVA_VERIFY_TOKEN")
	}

	cfg.InternalAPIKey = os.Getenv("INTERNAL_API_KEY")
	if cfg.InternalAPIKey == "" {
		missingVars = append(missingVars, "INTERNAL_API_KEY")
	}

	if len(missingVars) > 0 {
		return nil, fmt.Errorf("missing required environment variables: %v", missingVars)
	}

	return cfg, nil
}

// getEnv gets an environment variable or returns a default value
func getEnv(key, defaultValue string) string {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	return value
}

// getEnvInt gets an integer environment variable or returns a default value
func getEnvInt(key string, defaultValue int) int {
	valueStr := os.Getenv(key)
	if valueStr == "" {
		return defaultValue
	}

	value, err := strconv.Atoi(valueStr)
	if err != nil {
		return defaultValue
	}

	return value
}
