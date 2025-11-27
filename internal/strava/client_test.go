package strava

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"plantopo-strava-sync/internal/config"
	"plantopo-strava-sync/internal/database"
)

func setupTestClient(t *testing.T) (*Client, *database.DB, *httptest.Server) {
	// Create test database
	dbPath := t.TempDir() + "/test.db"
	db, err := database.Open(dbPath)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}

	// Create mock Strava server
	mux := http.NewServeMux()
	server := httptest.NewServer(mux)

	cfg := &config.Config{
		Domain: "example.com",
		StravaClients: map[string]*config.StravaClientConfig{
			"primary": {
				ClientID:     "test_client_id",
				ClientSecret: "test_client_secret",
				VerifyToken:  "test_verify_token",
			},
		},
		InternalAPIKey: "test_api_key",
	}

	client := NewClient(cfg, db)

	return client, db, server
}

func TestExchangeCode(t *testing.T) {
	// Create test database
	dbPath := t.TempDir() + "/test.db"
	db, err := database.Open(dbPath)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	// Create mock token server
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("Expected POST request, got %s", r.Method)
		}

		if err := r.ParseForm(); err != nil {
			http.Error(w, "Failed to parse form", http.StatusBadRequest)
			return
		}

		if r.FormValue("code") != "test_code" {
			http.Error(w, "Invalid code", http.StatusBadRequest)
			return
		}

		if r.FormValue("client_id") != "test_client_id" {
			http.Error(w, "Invalid client_id", http.StatusBadRequest)
			return
		}

		if r.FormValue("grant_type") != "authorization_code" {
			http.Error(w, "Invalid grant_type", http.StatusBadRequest)
			return
		}

		response := TokenResponse{
			AccessToken:  "test_access_token",
			RefreshToken: "test_refresh_token",
			ExpiresAt:    time.Now().Add(6 * time.Hour).Unix(),
			ExpiresIn:    21600,
			Athlete:      json.RawMessage(`{"id": 12345, "username": "testuser"}`),
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer tokenServer.Close()

	// Create client with test configuration
	cfg := &config.Config{
		Domain: "example.com",
		StravaClients: map[string]*config.StravaClientConfig{
			"primary": {
				ClientID:     "test_client_id",
				ClientSecret: "test_client_secret",
				VerifyToken:  "test_verify_token",
			},
		},
		InternalAPIKey: "test_api_key",
	}

	client := NewClient(cfg, db)
	client.SetTokenURL(tokenServer.URL)

	// Test token exchange
	tokenResp, err := client.ExchangeCode("test_code", "primary")
	if err != nil {
		t.Fatalf("Failed to exchange code: %v", err)
	}

	if tokenResp.AccessToken != "test_access_token" {
		t.Errorf("Expected access token 'test_access_token', got '%s'", tokenResp.AccessToken)
	}

	if tokenResp.RefreshToken != "test_refresh_token" {
		t.Errorf("Expected refresh token 'test_refresh_token', got '%s'", tokenResp.RefreshToken)
	}

	if tokenResp.ExpiresIn != 21600 {
		t.Errorf("Expected expires_in 21600, got %d", tokenResp.ExpiresIn)
	}
}

func TestEnsureValidToken_TokenValid(t *testing.T) {
	client, db, server := setupTestClient(t)
	defer db.Close()
	defer server.Close()

	// Insert athlete with valid token
	athlete := &database.Athlete{
		AthleteID:      12345,
		ClientID:       "primary",
		AccessToken:    "valid_token",
		RefreshToken:   "refresh_token",
		TokenExpiresAt: time.Now().Add(1 * time.Hour), // Valid for 1 hour
		AthleteSummary: json.RawMessage(`{"id": 12345}`),
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}

	err := db.UpsertAthlete(athlete)
	if err != nil {
		t.Fatalf("Failed to insert athlete: %v", err)
	}

	// Should not refresh token
	result, err := client.ensureValidToken(12345)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if result.AccessToken != "valid_token" {
		t.Errorf("Token should not have been refreshed")
	}
}

func TestRateLimitTracking(t *testing.T) {
	client, db, server := setupTestClient(t)
	defer db.Close()
	defer server.Close()

	// Insert athlete
	athlete := &database.Athlete{
		AthleteID:      12345,
		ClientID:       "primary",
		AccessToken:    "valid_token",
		RefreshToken:   "refresh_token",
		TokenExpiresAt: time.Now().Add(1 * time.Hour),
		AthleteSummary: json.RawMessage(`{"id": 12345}`),
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}

	err := db.UpsertAthlete(athlete)
	if err != nil {
		t.Fatalf("Failed to insert athlete: %v", err)
	}

	// Create mock response with correct Strava rate limit headers
	// X-RateLimit-*: Overall limits (200/15min, 2000/day)
	// X-ReadRateLimit-*: Read-only limits (100/15min, 1000/day)
	// Format: "15min,daily"
	mockResp := &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"X-Ratelimit-Usage":     []string{"150,1500"},
			"X-Ratelimit-Limit":     []string{"200,2000"},
			"X-Readratelimit-Usage": []string{"75,900"},
			"X-Readratelimit-Limit": []string{"100,1000"},
		},
		Body: http.NoBody,
	}

	client.updateRateLimits(mockResp)

	overallUsage15Min, overallLimit15Min, overallUsageDaily, overallLimitDaily,
		readUsage15Min, readLimit15Min, readUsageDaily, readLimitDaily := client.GetRateLimits()

	// Check overall limits
	if overallUsage15Min != 150 {
		t.Errorf("Expected overall 15min usage 150, got %d", overallUsage15Min)
	}
	if overallLimit15Min != 200 {
		t.Errorf("Expected overall 15min limit 200, got %d", overallLimit15Min)
	}
	if overallUsageDaily != 1500 {
		t.Errorf("Expected overall daily usage 1500, got %d", overallUsageDaily)
	}
	if overallLimitDaily != 2000 {
		t.Errorf("Expected overall daily limit 2000, got %d", overallLimitDaily)
	}

	// Check read limits
	if readUsage15Min != 75 {
		t.Errorf("Expected read 15min usage 75, got %d", readUsage15Min)
	}
	if readLimit15Min != 100 {
		t.Errorf("Expected read 15min limit 100, got %d", readLimit15Min)
	}
	if readUsageDaily != 900 {
		t.Errorf("Expected read daily usage 900, got %d", readUsageDaily)
	}
	if readLimitDaily != 1000 {
		t.Errorf("Expected read daily limit 1000, got %d", readLimitDaily)
	}
}

func TestHTTPError_Helpers(t *testing.T) {
	notFoundErr := &HTTPError{StatusCode: 404, Body: "Not Found"}
	if !IsNotFound(notFoundErr) {
		t.Error("Expected IsNotFound to return true for 404")
	}

	unauthorizedErr := &HTTPError{StatusCode: 401, Body: "Unauthorized"}
	if !IsUnauthorized(unauthorizedErr) {
		t.Error("Expected IsUnauthorized to return true for 401")
	}

	rateLimitErr := &HTTPError{StatusCode: 429, Body: "Too Many Requests"}
	if !IsTooManyRequests(rateLimitErr) {
		t.Error("Expected IsTooManyRequests to return true for 429")
	}
}
