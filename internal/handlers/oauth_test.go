package handlers

import (
	"net/http"
	"net/http/httptest"
	"plantopo-strava-sync/internal/config"
	"plantopo-strava-sync/internal/database"
	"plantopo-strava-sync/internal/oauth"
	"plantopo-strava-sync/internal/strava"
	"strings"
	"testing"
)

func setupOAuthHandlerTest(t *testing.T) (*OAuthHandler, *database.DB, *oauth.Manager) {
	dbPath := t.TempDir() + "/test.db"
	db, err := database.Open(dbPath)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}

	cfg := &config.Config{
		Domain: "localhost:4101",
		StravaClients: map[string]*config.StravaClientConfig{
			"primary": {
				ClientID:     "test_client_id",
				ClientSecret: "test_client_secret",
				VerifyToken:  "test_verify_token",
			},
		},
		InternalAPIKey: "test_api_key",
	}

	stravaClient := strava.NewClient(cfg, db)
	oauthManager := oauth.NewManager(cfg, db, stravaClient)
	handler := NewOAuthHandler(oauthManager, cfg)

	return handler, db, oauthManager
}

func TestHandleAuthStart_Success(t *testing.T) {
	handler, db, _ := setupOAuthHandlerTest(t)
	defer db.Close()

	req := httptest.NewRequest(http.MethodGet, "http://localhost:4101/oauth-start", nil)
	w := httptest.NewRecorder()

	handler.HandleAuthStart(w, req)

	if w.Code != http.StatusTemporaryRedirect {
		t.Errorf("Expected status 307, got %d", w.Code)
	}

	location := w.Header().Get("Location")
	if location == "" {
		t.Fatal("Expected Location header to be set")
	}

	if !strings.Contains(location, "https://www.strava.com/oauth/authorize") {
		t.Errorf("Expected redirect to Strava, got %s", location)
	}

	if !strings.Contains(location, "client_id=test_client_id") {
		t.Error("Expected client_id in redirect URL")
	}

	if !strings.Contains(location, "redirect_uri=https%3A%2F%2Flocalhost%3A4101%2Foauth-callback") {
		t.Error("Expected redirect_uri in redirect URL")
	}

	if !strings.Contains(location, "state=") {
		t.Error("Expected state parameter in redirect URL")
	}
}

func TestHandleAuthStart_WrongMethod(t *testing.T) {
	handler, db, _ := setupOAuthHandlerTest(t)
	defer db.Close()

	req := httptest.NewRequest(http.MethodPost, "/oauth-start", nil)
	w := httptest.NewRecorder()

	handler.HandleAuthStart(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("Expected status 405, got %d", w.Code)
	}
}

func TestHandleCallback_MissingParameters(t *testing.T) {
	handler, db, _ := setupOAuthHandlerTest(t)
	defer db.Close()

	tests := []struct {
		name  string
		query string
	}{
		{"missing code", "?state=test_state"},
		{"missing state", "?code=test_code"},
		{"missing both", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/oauth-callback"+tt.query, nil)
			w := httptest.NewRecorder()

			handler.HandleCallback(w, req)

			if w.Code != http.StatusBadRequest {
				t.Errorf("Expected status 400, got %d", w.Code)
			}
		})
	}
}

func TestHandleCallback_ErrorParameter(t *testing.T) {
	handler, db, _ := setupOAuthHandlerTest(t)
	defer db.Close()

	req := httptest.NewRequest(http.MethodGet, "/oauth-callback?error=access_denied", nil)
	w := httptest.NewRecorder()

	handler.HandleCallback(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", w.Code)
	}

	body := w.Body.String()
	if !strings.Contains(body, "access_denied") {
		t.Error("Expected error message to include 'access_denied'")
	}
}

func TestHandleCallback_InvalidState(t *testing.T) {
	handler, db, _ := setupOAuthHandlerTest(t)
	defer db.Close()

	// Try to use a state that was never generated
	req := httptest.NewRequest(http.MethodGet, "/oauth-callback?code=test_code&state=invalid_state", nil)
	w := httptest.NewRecorder()

	handler.HandleCallback(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", w.Code)
	}

	body := w.Body.String()
	if !strings.Contains(body, "Invalid or expired") {
		t.Error("Expected error message about invalid state")
	}
}

func TestHandleCallback_WrongMethod(t *testing.T) {
	handler, db, _ := setupOAuthHandlerTest(t)
	defer db.Close()

	req := httptest.NewRequest(http.MethodPost, "/oauth-callback", nil)
	w := httptest.NewRecorder()

	handler.HandleCallback(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("Expected status 405, got %d", w.Code)
	}
}

func TestHandleCallback_ConsumedState(t *testing.T) {
	handler, db, oauthManager := setupOAuthHandlerTest(t)
	defer db.Close()

	// Generate a valid state
	_, state, err := oauthManager.GenerateAuthURL("http://localhost:4101/oauth-callback", "primary")
	if err != nil {
		t.Fatalf("Failed to generate auth URL: %v", err)
	}

	// Use the state once (this will fail due to invalid code, but will consume the state)
	req1 := httptest.NewRequest(http.MethodGet, "/oauth-callback?code=invalid_code&state="+state, nil)
	w1 := httptest.NewRecorder()
	handler.HandleCallback(w1, req1)
	// First call will fail at token exchange, but state is now consumed

	// Try to use the same state again - should fail with invalid state error
	req2 := httptest.NewRequest(http.MethodGet, "/oauth-callback?code=test_code&state="+state, nil)
	w2 := httptest.NewRecorder()

	handler.HandleCallback(w2, req2)

	if w2.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", w2.Code)
	}

	body := w2.Body.String()
	if !strings.Contains(body, "Invalid or expired") {
		t.Error("Expected error message about invalid/expired state for reused state")
	}
}
