package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfigWithDefaults(t *testing.T) {
	// Set only required env vars
	setTestEnv(t, map[string]string{
		"STRAVA_CLIENT_ID":     "test_client_id",
		"STRAVA_CLIENT_SECRET": "test_client_secret",
		"INTERNAL_API_KEY":     "test_api_key",
	})

	config, err := Load()
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Check defaults
	if config.Host != "localhost" {
		t.Errorf("Expected default host 'localhost', got %s", config.Host)
	}
	if config.Port != 4101 {
		t.Errorf("Expected default port 4101, got %d", config.Port)
	}
	if config.DatabasePath != "./data.db" {
		t.Errorf("Expected default database path './data.db', got %s", config.DatabasePath)
	}
	if config.LogLevel != "info" {
		t.Errorf("Expected default log level 'info', got %s", config.LogLevel)
	}

	// Check required values
	if config.StravaClientID != "test_client_id" {
		t.Errorf("Expected STRAVA_CLIENT_ID 'test_client_id', got %s", config.StravaClientID)
	}
	if config.StravaClientSecret != "test_client_secret" {
		t.Errorf("Expected STRAVA_CLIENT_SECRET 'test_client_secret', got %s", config.StravaClientSecret)
	}
	if config.InternalAPIKey != "test_api_key" {
		t.Errorf("Expected INTERNAL_API_KEY 'test_api_key', got %s", config.InternalAPIKey)
	}
}

func TestLoadConfigFromEnvVars(t *testing.T) {
	setTestEnv(t, map[string]string{
		"HOST":                 "0.0.0.0",
		"PORT":                 "8080",
		"DATABASE_PATH":        "/tmp/test.db",
		"STRAVA_CLIENT_ID":     "custom_client_id",
		"STRAVA_CLIENT_SECRET": "custom_client_secret",
		"INTERNAL_API_KEY":     "custom_api_key",
		"LOG_LEVEL":            "debug",
	})

	config, err := Load()
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Verify all values are loaded from env
	if config.Host != "0.0.0.0" {
		t.Errorf("Expected host '0.0.0.0', got %s", config.Host)
	}
	if config.Port != 8080 {
		t.Errorf("Expected port 8080, got %d", config.Port)
	}
	if config.DatabasePath != "/tmp/test.db" {
		t.Errorf("Expected database path '/tmp/test.db', got %s", config.DatabasePath)
	}
	if config.LogLevel != "debug" {
		t.Errorf("Expected log level 'debug', got %s", config.LogLevel)
	}
	if config.StravaClientID != "custom_client_id" {
		t.Errorf("Expected STRAVA_CLIENT_ID 'custom_client_id', got %s", config.StravaClientID)
	}
}

func TestLoadConfigFromEnvFile(t *testing.T) {
	// Create a temporary .env file
	tmpDir := t.TempDir()
	envFile := filepath.Join(tmpDir, ".env")

	envContent := `# Test .env file
HOST=192.168.1.1
PORT=9000
DATABASE_PATH=/custom/path/data.db
STRAVA_CLIENT_ID=env_file_client_id
STRAVA_CLIENT_SECRET=env_file_client_secret
INTERNAL_API_KEY=env_file_api_key
LOG_LEVEL=warn
`
	if err := os.WriteFile(envFile, []byte(envContent), 0644); err != nil {
		t.Fatalf("Failed to create .env file: %v", err)
	}

	// Change to temp directory
	oldDir, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(oldDir)

	// Clear any existing env vars
	clearTestEnv(t)

	config, err := Load()
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Verify values from .env file
	if config.Host != "192.168.1.1" {
		t.Errorf("Expected host '192.168.1.1' from .env, got %s", config.Host)
	}
	if config.Port != 9000 {
		t.Errorf("Expected port 9000 from .env, got %d", config.Port)
	}
	if config.LogLevel != "warn" {
		t.Errorf("Expected log level 'warn' from .env, got %s", config.LogLevel)
	}
}

func TestEnvVarsPrecedenceOverEnvFile(t *testing.T) {
	// Create a temporary .env file
	tmpDir := t.TempDir()
	envFile := filepath.Join(tmpDir, ".env")

	envContent := `HOST=from_file
PORT=9000
STRAVA_CLIENT_ID=file_client_id
STRAVA_CLIENT_SECRET=file_client_secret
INTERNAL_API_KEY=file_api_key
`
	if err := os.WriteFile(envFile, []byte(envContent), 0644); err != nil {
		t.Fatalf("Failed to create .env file: %v", err)
	}

	// Change to temp directory
	oldDir, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(oldDir)

	// Set some env vars that should override .env file
	setTestEnv(t, map[string]string{
		"HOST":             "from_env_var",
		"STRAVA_CLIENT_ID": "env_client_id",
		// Leave other required vars to come from .env file
	})

	config, err := Load()
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Verify env vars take precedence
	if config.Host != "from_env_var" {
		t.Errorf("Expected host 'from_env_var' from env var, got %s", config.Host)
	}
	if config.StravaClientID != "env_client_id" {
		t.Errorf("Expected client ID 'env_client_id' from env var, got %s", config.StravaClientID)
	}

	// Verify .env file values used when env var not set
	if config.Port != 9000 {
		t.Errorf("Expected port 9000 from .env file, got %d", config.Port)
	}
	if config.StravaClientSecret != "file_client_secret" {
		t.Errorf("Expected client secret 'file_client_secret' from .env, got %s", config.StravaClientSecret)
	}
}

func TestValidationMissingClientID(t *testing.T) {
	setTestEnv(t, map[string]string{
		// Missing STRAVA_CLIENT_ID
		"STRAVA_CLIENT_SECRET": "test_client_secret",
		"INTERNAL_API_KEY":     "test_api_key",
	})

	_, err := Load()
	if err == nil {
		t.Error("Expected validation error for missing STRAVA_CLIENT_ID")
	}
	if err.Error() != "STRAVA_CLIENT_ID is required" {
		t.Errorf("Expected 'STRAVA_CLIENT_ID is required' error, got: %v", err)
	}
}

func TestValidationMissingClientSecret(t *testing.T) {
	setTestEnv(t, map[string]string{
		"STRAVA_CLIENT_ID": "test_client_id",
		// Missing STRAVA_CLIENT_SECRET
		"INTERNAL_API_KEY": "test_api_key",
	})

	_, err := Load()
	if err == nil {
		t.Error("Expected validation error for missing STRAVA_CLIENT_SECRET")
	}
	if err.Error() != "STRAVA_CLIENT_SECRET is required" {
		t.Errorf("Expected 'STRAVA_CLIENT_SECRET is required' error, got: %v", err)
	}
}

func TestValidationMissingAPIKey(t *testing.T) {
	setTestEnv(t, map[string]string{
		"STRAVA_CLIENT_ID":     "test_client_id",
		"STRAVA_CLIENT_SECRET": "test_client_secret",
		// Missing INTERNAL_API_KEY
	})

	_, err := Load()
	if err == nil {
		t.Error("Expected validation error for missing INTERNAL_API_KEY")
	}
	if err.Error() != "INTERNAL_API_KEY is required" {
		t.Errorf("Expected 'INTERNAL_API_KEY is required' error, got: %v", err)
	}
}

func TestValidationInvalidPort(t *testing.T) {
	tests := []struct {
		port    string
		wantErr bool
	}{
		{"0", true},
		{"1", false},
		{"80", false},
		{"4101", false},
		{"65535", false},
		{"65536", true},
		{"99999", true},
	}

	for _, tt := range tests {
		t.Run("port_"+tt.port, func(t *testing.T) {
			setTestEnv(t, map[string]string{
				"PORT":                 tt.port,
				"STRAVA_CLIENT_ID":     "test_client_id",
				"STRAVA_CLIENT_SECRET": "test_client_secret",
				"INTERNAL_API_KEY":     "test_api_key",
			})

			_, err := Load()
			if tt.wantErr && err == nil {
				t.Errorf("Expected error for port %s, but got none", tt.port)
			}
			if !tt.wantErr && err != nil {
				t.Errorf("Expected no error for port %s, but got: %v", tt.port, err)
			}
		})
	}
}

func TestValidationInvalidLogLevel(t *testing.T) {
	setTestEnv(t, map[string]string{
		"LOG_LEVEL":            "invalid",
		"STRAVA_CLIENT_ID":     "test_client_id",
		"STRAVA_CLIENT_SECRET": "test_client_secret",
		"INTERNAL_API_KEY":     "test_api_key",
	})

	_, err := Load()
	if err == nil {
		t.Error("Expected validation error for invalid LOG_LEVEL")
	}
	if err.Error() != "LOG_LEVEL must be one of: debug, info, warn, error" {
		t.Errorf("Unexpected error message: %v", err)
	}
}

func TestValidationValidLogLevels(t *testing.T) {
	logLevels := []string{"debug", "info", "warn", "error"}

	for _, level := range logLevels {
		t.Run("log_level_"+level, func(t *testing.T) {
			setTestEnv(t, map[string]string{
				"LOG_LEVEL":            level,
				"STRAVA_CLIENT_ID":     "test_client_id",
				"STRAVA_CLIENT_SECRET": "test_client_secret",
				"INTERNAL_API_KEY":     "test_api_key",
			})

			config, err := Load()
			if err != nil {
				t.Errorf("Expected no error for log level %s, but got: %v", level, err)
			}
			if config.LogLevel != level {
				t.Errorf("Expected log level %s, got %s", level, config.LogLevel)
			}
		})
	}
}

func TestEnvFileWithComments(t *testing.T) {
	tmpDir := t.TempDir()
	envFile := filepath.Join(tmpDir, ".env")

	envContent := `# Comment line
# Another comment

# Required configs
STRAVA_CLIENT_ID=test_id
STRAVA_CLIENT_SECRET=test_secret
INTERNAL_API_KEY=test_key

# Empty line below

# Optional configs
HOST=127.0.0.1
`
	if err := os.WriteFile(envFile, []byte(envContent), 0644); err != nil {
		t.Fatalf("Failed to create .env file: %v", err)
	}

	oldDir, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(oldDir)

	clearTestEnv(t)

	config, err := Load()
	if err != nil {
		t.Fatalf("Failed to load config with comments: %v", err)
	}

	if config.Host != "127.0.0.1" {
		t.Errorf("Expected host '127.0.0.1', got %s", config.Host)
	}
	if config.StravaClientID != "test_id" {
		t.Errorf("Expected client ID 'test_id', got %s", config.StravaClientID)
	}
}

func TestEnvFileWithQuotes(t *testing.T) {
	tmpDir := t.TempDir()
	envFile := filepath.Join(tmpDir, ".env")

	envContent := `STRAVA_CLIENT_ID="quoted_id"
STRAVA_CLIENT_SECRET='single_quoted_secret'
INTERNAL_API_KEY=unquoted_key
HOST="localhost"
`
	if err := os.WriteFile(envFile, []byte(envContent), 0644); err != nil {
		t.Fatalf("Failed to create .env file: %v", err)
	}

	oldDir, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(oldDir)

	clearTestEnv(t)

	config, err := Load()
	if err != nil {
		t.Fatalf("Failed to load config with quotes: %v", err)
	}

	// Values should have quotes removed
	if config.StravaClientID != "quoted_id" {
		t.Errorf("Expected client ID 'quoted_id', got %s", config.StravaClientID)
	}
	if config.StravaClientSecret != "single_quoted_secret" {
		t.Errorf("Expected client secret 'single_quoted_secret', got %s", config.StravaClientSecret)
	}
	if config.InternalAPIKey != "unquoted_key" {
		t.Errorf("Expected API key 'unquoted_key', got %s", config.InternalAPIKey)
	}
	if config.Host != "localhost" {
		t.Errorf("Expected host 'localhost', got %s", config.Host)
	}
}

// Helper function to set test environment variables and clean up after test
func setTestEnv(t *testing.T, vars map[string]string) {
	t.Helper()

	// Clear all relevant env vars first
	clearTestEnv(t)

	// Set provided vars
	for key, value := range vars {
		os.Setenv(key, value)
		t.Cleanup(func() {
			os.Unsetenv(key)
		})
	}
}

// Helper function to clear all config-related environment variables
func clearTestEnv(t *testing.T) {
	t.Helper()

	envVars := []string{
		"HOST", "PORT", "DATABASE_PATH",
		"STRAVA_CLIENT_ID", "STRAVA_CLIENT_SECRET",
		"INTERNAL_API_KEY", "LOG_LEVEL",
	}

	for _, key := range envVars {
		os.Unsetenv(key)
	}
}
