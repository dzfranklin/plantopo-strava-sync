package oauth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"
	"unsafe"

	"plantopo-strava-sync/internal/config"
	"plantopo-strava-sync/internal/database"
	"plantopo-strava-sync/internal/strava"
)

func setupOAuthTest(t *testing.T) (*Manager, *database.DB) {
	dbPath := t.TempDir() + "/test.db"
	db, err := database.Open(dbPath)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}

	cfg := &config.Config{
		StravaClientID:     "test_client_id",
		StravaClientSecret: "test_client_secret",
	}

	stravaClient := strava.NewClient(cfg, db)
	manager := NewManager(cfg, db, stravaClient)

	return manager, db
}

func TestGenerateAuthURL(t *testing.T) {
	manager, db := setupOAuthTest(t)
	defer db.Close()

	redirectURI := "http://localhost:4101/oauth-callback"
	authURL, state, err := manager.GenerateAuthURL(redirectURI)

	if err != nil {
		t.Fatalf("Failed to generate auth URL: %v", err)
	}

	if state == "" {
		t.Error("Expected non-empty state")
	}

	if !strings.Contains(authURL, authorizationURL) {
		t.Errorf("Expected auth URL to contain %s", authorizationURL)
	}

	if !strings.Contains(authURL, "client_id=test_client_id") {
		t.Error("Expected auth URL to contain client_id")
	}

	if !strings.Contains(authURL, "redirect_uri=") {
		t.Error("Expected auth URL to contain redirect_uri")
	}

	if !strings.Contains(authURL, "scope=activity%3Aread_all") {
		t.Error("Expected auth URL to contain scope")
	}

	if !strings.Contains(authURL, "state=") {
		t.Error("Expected auth URL to contain state parameter")
	}

	// Verify the state is properly URL-encoded in the URL
	if !strings.Contains(authURL, url.QueryEscape(state)) && !strings.Contains(authURL, state) {
		t.Error("Expected auth URL to contain the state value")
	}

	// Verify state is stored
	manager.states.mu.RLock()
	_, exists := manager.states.states[state]
	manager.states.mu.RUnlock()

	if !exists {
		t.Error("Expected state to be stored")
	}
}

func TestValidateState_Valid(t *testing.T) {
	manager, db := setupOAuthTest(t)
	defer db.Close()

	// Generate a state
	_, state, err := manager.GenerateAuthURL("http://localhost:4101/oauth-callback")
	if err != nil {
		t.Fatalf("Failed to generate auth URL: %v", err)
	}

	// Validate it
	if !manager.validateState(state) {
		t.Error("Expected state to be valid")
	}

	// State should be removed after first use
	if manager.validateState(state) {
		t.Error("Expected state to be invalid after first use")
	}
}

func TestValidateState_Invalid(t *testing.T) {
	manager, db := setupOAuthTest(t)
	defer db.Close()

	// Try to validate a non-existent state
	if manager.validateState("invalid_state") {
		t.Error("Expected invalid state to fail validation")
	}
}

func TestValidateState_Expired(t *testing.T) {
	manager, db := setupOAuthTest(t)
	defer db.Close()

	// Manually insert an expired state
	state := "expired_state"
	manager.states.mu.Lock()
	manager.states.states[state] = time.Now().Add(-1 * time.Minute)
	manager.states.mu.Unlock()

	// Should be rejected
	if manager.validateState(state) {
		t.Error("Expected expired state to fail validation")
	}

	// Should be removed
	manager.states.mu.RLock()
	_, exists := manager.states.states[state]
	manager.states.mu.RUnlock()

	if exists {
		t.Error("Expected expired state to be removed")
	}
}

func TestHandleCallback_Integration(t *testing.T) {
	manager, db := setupOAuthTest(t)
	defer db.Close()

	// Create mock token server
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "Failed to parse form", http.StatusBadRequest)
			return
		}

		if r.FormValue("code") != "test_auth_code" {
			http.Error(w, "Invalid code", http.StatusBadRequest)
			return
		}

		if r.FormValue("client_id") != "test_client_id" {
			http.Error(w, "Invalid client_id", http.StatusBadRequest)
			return
		}

		response := strava.TokenResponse{
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

	// Override token URL in manager's strava client
	// We need to access the stravaClient field - let's make it public or add a setter
	// For now, use reflection or modify the oauth.Manager to expose the client
	// This demonstrates the full integration pattern

	// Access internal strava client via reflection (not ideal but works for testing)
	stravaClientField := reflect.ValueOf(manager).Elem().FieldByName("stravaClient")
	if !stravaClientField.IsValid() {
		t.Fatal("Cannot access stravaClient field")
	}

	// Make the field accessible
	stravaClientField = reflect.NewAt(stravaClientField.Type(), unsafe.Pointer(stravaClientField.UnsafeAddr())).Elem()
	stravaClient := stravaClientField.Interface().(*strava.Client)
	stravaClient.SetTokenURL(tokenServer.URL)

	// Generate a valid state
	_, state, err := manager.GenerateAuthURL("http://localhost:4101/oauth-callback")
	if err != nil {
		t.Fatalf("Failed to generate auth URL: %v", err)
	}

	// Test OAuth callback
	athleteID, err := manager.HandleCallback("test_auth_code", state)
	if err != nil {
		t.Fatalf("Failed to handle callback: %v", err)
	}

	if athleteID != 12345 {
		t.Errorf("Expected athlete ID 12345, got %d", athleteID)
	}

	// Verify athlete was stored in database
	athlete, err := db.GetAthlete(athleteID)
	if err != nil {
		t.Fatalf("Failed to get athlete: %v", err)
	}

	if athlete.AccessToken != "test_access_token" {
		t.Errorf("Expected access token 'test_access_token', got '%s'", athlete.AccessToken)
	}

	if athlete.RefreshToken != "test_refresh_token" {
		t.Errorf("Expected refresh token 'test_refresh_token', got '%s'", athlete.RefreshToken)
	}

	// Verify athlete_connected event was created
	events, err := db.GetEvents(0, 10)
	if err != nil {
		t.Fatalf("Failed to get events: %v", err)
	}

	if len(events) == 0 {
		t.Fatal("Expected at least one event")
	}

	foundAthleteConnected := false
	for _, event := range events {
		if event.EventType == database.EventTypeAthleteConnected && event.AthleteID == athleteID {
			foundAthleteConnected = true
			break
		}
	}

	if !foundAthleteConnected {
		t.Error("Expected athlete_connected event to be created")
	}

	// Verify sync job was enqueued
	syncJobQueueLength, err := db.GetSyncJobQueueLength()
	if err != nil {
		t.Fatalf("Failed to get sync job queue length: %v", err)
	}

	if syncJobQueueLength != 1 {
		t.Errorf("Expected 1 sync job in queue, got %d", syncJobQueueLength)
	}
}

func TestGenerateRandomState(t *testing.T) {
	state1, err := generateRandomState()
	if err != nil {
		t.Fatalf("Failed to generate state: %v", err)
	}

	state2, err := generateRandomState()
	if err != nil {
		t.Fatalf("Failed to generate second state: %v", err)
	}

	if state1 == state2 {
		t.Error("Expected different random states")
	}

	if len(state1) == 0 {
		t.Error("Expected non-empty state")
	}
}

func TestEnqueueSyncJob(t *testing.T) {
	_, db := setupOAuthTest(t)
	defer db.Close()

	// Manually test enqueueing sync job
	athleteID := int64(12345)

	id, err := db.EnqueueSyncJob(athleteID, "sync_all_activities")
	if err != nil {
		t.Fatalf("Failed to enqueue sync job: %v", err)
	}

	if id == 0 {
		t.Error("Expected non-zero sync job ID")
	}

	// Verify it's in the queue
	length, err := db.GetSyncJobQueueLength()
	if err != nil {
		t.Fatalf("Failed to get sync job queue length: %v", err)
	}

	if length != 1 {
		t.Errorf("Expected sync job queue length 1, got %d", length)
	}

	// Verify the data by claiming it
	job, err := db.ClaimSyncJob()
	if err != nil {
		t.Fatalf("Failed to claim sync job: %v", err)
	}

	if job == nil {
		t.Fatal("Expected to claim a sync job, got nil")
	}

	if job.AthleteID != athleteID {
		t.Errorf("Expected athlete ID %d, got %d", athleteID, job.AthleteID)
	}

	if job.JobType != "sync_all_activities" {
		t.Errorf("Expected job type 'sync_all_activities', got '%s'", job.JobType)
	}
}
