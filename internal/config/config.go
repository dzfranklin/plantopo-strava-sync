package config

import (
	"fmt"
	"os"
	"strconv"
)

// StravaClientConfig holds configuration for a single Strava client
type StravaClientConfig struct {
	ClientID     string
	ClientSecret string
	VerifyToken  string
}

// Config holds all application configuration
type Config struct {
	// Publicly accessible domain pointing to server
	Domain string

	// Server configuration
	Host string
	Port int

	// Database configuration
	DatabasePath string

	// Strava API configuration (multi-client)
	StravaClients map[string]*StravaClientConfig

	// Internal API configuration
	InternalAPIKey string

	// Logging configuration
	LogLevel string

	// Metrics configuration
	MetricsEnabled bool
	MetricsHost    string
	MetricsPort    int
}

// Load reads configuration from environment variables
// It fails fast if required variables are missing
func Load() (*Config, error) {
	cfg := &Config{
		// Optional values with defaults
		Host:         getEnv("HOST", "127.0.0.1"),
		Port:         getEnvInt("PORT", 4101),
		DatabasePath: getEnv("DATABASE_PATH", "./data.db"),
		LogLevel:     getEnv("LOG_LEVEL", "info"),

		// Metrics defaults
		MetricsEnabled: getEnvBool("METRICS_ENABLED", true),
		MetricsHost:    getEnv("METRICS_HOST", "127.0.0.1"),
		MetricsPort:    getEnvInt("METRICS_PORT", 4102),

		// Initialize Strava clients map
		StravaClients: make(map[string]*StravaClientConfig),
	}

	// Required values
	var missingVars []string

	domain := os.Getenv("DOMAIN")
	if domain == "" {
		missingVars = append(missingVars, "DOMAIN")
	}
	cfg.Domain = domain

	// Load primary client
	primaryClientID := os.Getenv("STRAVA_PRIMARY_CLIENT_ID")
	if primaryClientID == "" {
		missingVars = append(missingVars, "STRAVA_PRIMARY_CLIENT_ID")
	}
	primaryClientSecret := os.Getenv("STRAVA_PRIMARY_CLIENT_SECRET")
	if primaryClientSecret == "" {
		missingVars = append(missingVars, "STRAVA_PRIMARY_CLIENT_SECRET")
	}
	primaryVerifyToken := os.Getenv("STRAVA_PRIMARY_VERIFY_TOKEN")
	if primaryVerifyToken == "" {
		missingVars = append(missingVars, "STRAVA_PRIMARY_VERIFY_TOKEN")
	}

	// Load secondary client (optional)
	secondaryClientID := os.Getenv("STRAVA_SECONDARY_CLIENT_ID")
	secondaryClientSecret := os.Getenv("STRAVA_SECONDARY_CLIENT_SECRET")
	secondaryVerifyToken := os.Getenv("STRAVA_SECONDARY_VERIFY_TOKEN")

	// Check if any secondary variable is set
	hasAnySecondary := secondaryClientID != "" || secondaryClientSecret != "" || secondaryVerifyToken != ""

	cfg.InternalAPIKey = os.Getenv("INTERNAL_API_KEY")
	if cfg.InternalAPIKey == "" {
		missingVars = append(missingVars, "INTERNAL_API_KEY")
	}

	// If any secondary variable is set, all must be set
	if hasAnySecondary {
		if secondaryClientID == "" {
			missingVars = append(missingVars, "STRAVA_SECONDARY_CLIENT_ID")
		}
		if secondaryClientSecret == "" {
			missingVars = append(missingVars, "STRAVA_SECONDARY_CLIENT_SECRET")
		}
		if secondaryVerifyToken == "" {
			missingVars = append(missingVars, "STRAVA_SECONDARY_VERIFY_TOKEN")
		}
	}

	if len(missingVars) > 0 {
		return nil, fmt.Errorf("missing required environment variables: %v", missingVars)
	}

	// Populate Strava clients map
	cfg.StravaClients["primary"] = &StravaClientConfig{
		ClientID:     primaryClientID,
		ClientSecret: primaryClientSecret,
		VerifyToken:  primaryVerifyToken,
	}

	// Only add secondary client if all variables are present
	if hasAnySecondary && secondaryClientID != "" && secondaryClientSecret != "" && secondaryVerifyToken != "" {
		cfg.StravaClients["secondary"] = &StravaClientConfig{
			ClientID:     secondaryClientID,
			ClientSecret: secondaryClientSecret,
			VerifyToken:  secondaryVerifyToken,
		}
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

// getEnvBool gets a boolean environment variable or returns a default value
func getEnvBool(key string, defaultValue bool) bool {
	valueStr := os.Getenv(key)
	if valueStr == "" {
		return defaultValue
	}

	value, err := strconv.ParseBool(valueStr)
	if err != nil {
		return defaultValue
	}

	return value
}

// GetClient returns the Strava client configuration for the given client ID
func (c *Config) GetClient(clientID string) (*StravaClientConfig, error) {
	client, exists := c.StravaClients[clientID]
	if !exists {
		return nil, fmt.Errorf("unknown client ID: %s", clientID)
	}
	return client, nil
}

// HasClient returns true if the given client ID is configured
func (c *Config) HasClient(clientID string) bool {
	_, exists := c.StravaClients[clientID]
	return exists
}

// GetDefaultClientID returns the default client ID ("primary")
func (c *Config) GetDefaultClientID() string {
	return "primary"
}

// GetClientIDs returns a list of all configured client IDs
func (c *Config) GetClientIDs() []string {
	ids := make([]string, 0, len(c.StravaClients))
	for id := range c.StravaClients {
		ids = append(ids, id)
	}
	return ids
}
