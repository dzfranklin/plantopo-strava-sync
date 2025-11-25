package config

import (
	"os"
	"testing"
)

func TestLoad(t *testing.T) {
	// Save original environment
	originalEnv := os.Environ()
	defer func() {
		// Restore environment
		os.Clearenv()
		for _, env := range originalEnv {
			if len(env) > 0 {
				for i := 0; i < len(env); i++ {
					if env[i] == '=' {
						os.Setenv(env[:i], env[i+1:])
						break
					}
				}
			}
		}
	}()

	t.Run("WithAllRequiredVariables", func(t *testing.T) {
		os.Clearenv()
		os.Setenv("STRAVA_CLIENT_ID", "test_client_id")
		os.Setenv("STRAVA_CLIENT_SECRET", "test_client_secret")
		os.Setenv("STRAVA_VERIFY_TOKEN", "test_verify_token")
		os.Setenv("INTERNAL_API_KEY", "test_api_key")

		cfg, err := Load()
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		if cfg.StravaClientID != "test_client_id" {
			t.Errorf("Expected StravaClientID='test_client_id', got '%s'", cfg.StravaClientID)
		}
		if cfg.StravaClientSecret != "test_client_secret" {
			t.Errorf("Expected StravaClientSecret='test_client_secret', got '%s'", cfg.StravaClientSecret)
		}
		if cfg.StravaVerifyToken != "test_verify_token" {
			t.Errorf("Expected StravaVerifyToken='test_verify_token', got '%s'", cfg.StravaVerifyToken)
		}
		if cfg.InternalAPIKey != "test_api_key" {
			t.Errorf("Expected InternalAPIKey='test_api_key', got '%s'", cfg.InternalAPIKey)
		}

		// Test defaults
		if cfg.Host != "localhost" {
			t.Errorf("Expected default Host='localhost', got '%s'", cfg.Host)
		}
		if cfg.Port != 4101 {
			t.Errorf("Expected default Port=4101, got %d", cfg.Port)
		}
		if cfg.DatabasePath != "./data.db" {
			t.Errorf("Expected default DatabasePath='./data.db', got '%s'", cfg.DatabasePath)
		}
		if cfg.LogLevel != "info" {
			t.Errorf("Expected default LogLevel='info', got '%s'", cfg.LogLevel)
		}
	})

	t.Run("WithCustomOptionalVariables", func(t *testing.T) {
		os.Clearenv()
		os.Setenv("STRAVA_CLIENT_ID", "test_client_id")
		os.Setenv("STRAVA_CLIENT_SECRET", "test_client_secret")
		os.Setenv("STRAVA_VERIFY_TOKEN", "test_verify_token")
		os.Setenv("INTERNAL_API_KEY", "test_api_key")
		os.Setenv("HOST", "0.0.0.0")
		os.Setenv("PORT", "8080")
		os.Setenv("DATABASE_PATH", "/tmp/test.db")
		os.Setenv("LOG_LEVEL", "debug")

		cfg, err := Load()
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		if cfg.Host != "0.0.0.0" {
			t.Errorf("Expected Host='0.0.0.0', got '%s'", cfg.Host)
		}
		if cfg.Port != 8080 {
			t.Errorf("Expected Port=8080, got %d", cfg.Port)
		}
		if cfg.DatabasePath != "/tmp/test.db" {
			t.Errorf("Expected DatabasePath='/tmp/test.db', got '%s'", cfg.DatabasePath)
		}
		if cfg.LogLevel != "debug" {
			t.Errorf("Expected LogLevel='debug', got '%s'", cfg.LogLevel)
		}
	})

	t.Run("MissingStravaClientID", func(t *testing.T) {
		os.Clearenv()
		os.Setenv("STRAVA_CLIENT_SECRET", "test_client_secret")
		os.Setenv("STRAVA_VERIFY_TOKEN", "test_verify_token")
		os.Setenv("INTERNAL_API_KEY", "test_api_key")

		_, err := Load()
		if err == nil {
			t.Fatal("Expected error for missing STRAVA_CLIENT_ID, got nil")
		}
	})

	t.Run("MissingStravaClientSecret", func(t *testing.T) {
		os.Clearenv()
		os.Setenv("STRAVA_CLIENT_ID", "test_client_id")
		os.Setenv("STRAVA_VERIFY_TOKEN", "test_verify_token")
		os.Setenv("INTERNAL_API_KEY", "test_api_key")

		_, err := Load()
		if err == nil {
			t.Fatal("Expected error for missing STRAVA_CLIENT_SECRET, got nil")
		}
	})

	t.Run("MissingStravaVerifyToken", func(t *testing.T) {
		os.Clearenv()
		os.Setenv("STRAVA_CLIENT_ID", "test_client_id")
		os.Setenv("STRAVA_CLIENT_SECRET", "test_client_secret")
		os.Setenv("INTERNAL_API_KEY", "test_api_key")

		_, err := Load()
		if err == nil {
			t.Fatal("Expected error for missing STRAVA_VERIFY_TOKEN, got nil")
		}
	})

	t.Run("MissingInternalAPIKey", func(t *testing.T) {
		os.Clearenv()
		os.Setenv("STRAVA_CLIENT_ID", "test_client_id")
		os.Setenv("STRAVA_CLIENT_SECRET", "test_client_secret")
		os.Setenv("STRAVA_VERIFY_TOKEN", "test_verify_token")

		_, err := Load()
		if err == nil {
			t.Fatal("Expected error for missing INTERNAL_API_KEY, got nil")
		}
	})

	t.Run("InvalidPortNumber", func(t *testing.T) {
		os.Clearenv()
		os.Setenv("STRAVA_CLIENT_ID", "test_client_id")
		os.Setenv("STRAVA_CLIENT_SECRET", "test_client_secret")
		os.Setenv("STRAVA_VERIFY_TOKEN", "test_verify_token")
		os.Setenv("INTERNAL_API_KEY", "test_api_key")
		os.Setenv("PORT", "not_a_number")

		cfg, err := Load()
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Should fall back to default
		if cfg.Port != 4101 {
			t.Errorf("Expected Port to fallback to 4101, got %d", cfg.Port)
		}
	})
}
