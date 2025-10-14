package database

import (
	"testing"
	"time"
)

func TestCreateAndGetAthlete(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	if err := db.Init(); err != nil {
		t.Fatalf("Failed to init db: %v", err)
	}

	now := time.Now().Unix()
	athlete := &Athlete{
		AthleteID:    12345,
		AccessToken:  "access_token",
		RefreshToken: "refresh_token",
		ExpiresAt:    now + 3600,
		Scope:        "activity:read_all",
		Authorized:   true,
	}

	// Create athlete
	if err := db.CreateAthlete(athlete); err != nil {
		t.Fatalf("Failed to create athlete: %v", err)
	}

	// Get athlete
	retrieved, err := db.GetAthlete(12345)
	if err != nil {
		t.Fatalf("Failed to get athlete: %v", err)
	}
	if retrieved == nil {
		t.Fatal("Expected athlete, got nil")
	}

	if retrieved.AthleteID != 12345 {
		t.Errorf("Expected athlete_id 12345, got %d", retrieved.AthleteID)
	}
	if retrieved.AccessToken != "access_token" {
		t.Errorf("Expected access_token 'access_token', got %s", retrieved.AccessToken)
	}
	if !retrieved.Authorized {
		t.Error("Expected authorized true")
	}
}

func TestGetNonexistentAthlete(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	if err := db.Init(); err != nil {
		t.Fatalf("Failed to init db: %v", err)
	}

	athlete, err := db.GetAthlete(99999)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if athlete != nil {
		t.Error("Expected nil athlete, got non-nil")
	}
}

func TestUpdateAthleteTokens(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	if err := db.Init(); err != nil {
		t.Fatalf("Failed to init db: %v", err)
	}

	now := time.Now().Unix()
	athlete := &Athlete{
		AthleteID:    12345,
		AccessToken:  "old_access",
		RefreshToken: "old_refresh",
		ExpiresAt:    now,
		Scope:        "activity:read_all",
		Authorized:   true,
	}

	if err := db.CreateAthlete(athlete); err != nil {
		t.Fatalf("Failed to create athlete: %v", err)
	}

	// Update tokens
	newExpires := now + 7200
	if err := db.UpdateAthleteTokens(12345, "new_access", "new_refresh", newExpires); err != nil {
		t.Fatalf("Failed to update tokens: %v", err)
	}

	// Verify update
	retrieved, err := db.GetAthlete(12345)
	if err != nil {
		t.Fatalf("Failed to get athlete: %v", err)
	}

	if retrieved.AccessToken != "new_access" {
		t.Errorf("Expected access_token 'new_access', got %s", retrieved.AccessToken)
	}
	if retrieved.RefreshToken != "new_refresh" {
		t.Errorf("Expected refresh_token 'new_refresh', got %s", retrieved.RefreshToken)
	}
	if retrieved.ExpiresAt != newExpires {
		t.Errorf("Expected expires_at %d, got %d", newExpires, retrieved.ExpiresAt)
	}
}

func TestUpdateAthleteSyncState(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	if err := db.Init(); err != nil {
		t.Fatalf("Failed to init db: %v", err)
	}

	athlete := &Athlete{
		AthleteID:    12345,
		AccessToken:  "token",
		RefreshToken: "refresh",
		ExpiresAt:    time.Now().Unix() + 3600,
		Scope:        "activity:read_all",
		Authorized:   true,
	}

	if err := db.CreateAthlete(athlete); err != nil {
		t.Fatalf("Failed to create athlete: %v", err)
	}

	// Update sync state
	now := time.Now().Unix()
	syncError := "test error"
	if err := db.UpdateAthleteSyncState(12345, true, &syncError, &now); err != nil {
		t.Fatalf("Failed to update sync state: %v", err)
	}

	// Verify update
	retrieved, err := db.GetAthlete(12345)
	if err != nil {
		t.Fatalf("Failed to get athlete: %v", err)
	}

	if !retrieved.SyncInProgress {
		t.Error("Expected sync_in_progress true")
	}
	if retrieved.SyncError == nil || *retrieved.SyncError != "test error" {
		t.Errorf("Expected sync_error 'test error', got %v", retrieved.SyncError)
	}
	if retrieved.LastSyncAt == nil || *retrieved.LastSyncAt != now {
		t.Errorf("Expected last_sync_at %d, got %v", now, retrieved.LastSyncAt)
	}
}

func TestDeauthorizeAthlete(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	if err := db.Init(); err != nil {
		t.Fatalf("Failed to init db: %v", err)
	}

	profile := `{"name":"Test Athlete"}`
	athlete := &Athlete{
		AthleteID:    12345,
		AccessToken:  "token",
		RefreshToken: "refresh",
		ExpiresAt:    time.Now().Unix() + 3600,
		Scope:        "activity:read_all",
		Authorized:   true,
		ProfileJSON:  &profile,
	}

	if err := db.CreateAthlete(athlete); err != nil {
		t.Fatalf("Failed to create athlete: %v", err)
	}

	// Deauthorize
	if err := db.DeauthorizeAthlete(12345); err != nil {
		t.Fatalf("Failed to deauthorize athlete: %v", err)
	}

	// Verify deauthorization
	retrieved, err := db.GetAthlete(12345)
	if err != nil {
		t.Fatalf("Failed to get athlete: %v", err)
	}

	if retrieved.Authorized {
		t.Error("Expected authorized false")
	}
	if retrieved.AccessToken != "" {
		t.Error("Expected access_token to be cleared")
	}
	if retrieved.RefreshToken != "" {
		t.Error("Expected refresh_token to be cleared")
	}
	if retrieved.ProfileJSON != nil {
		t.Error("Expected profile_json to be cleared")
	}
}

func TestListAthletes(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	if err := db.Init(); err != nil {
		t.Fatalf("Failed to init db: %v", err)
	}

	now := time.Now().Unix()

	// Create multiple athletes
	athletes := []*Athlete{
		{
			AthleteID:    1,
			AccessToken:  "token1",
			RefreshToken: "refresh1",
			ExpiresAt:    now + 3600,
			Scope:        "activity:read_all",
			Authorized:   true,
		},
		{
			AthleteID:    2,
			AccessToken:  "token2",
			RefreshToken: "refresh2",
			ExpiresAt:    now + 3600,
			Scope:        "activity:read_all",
			Authorized:   false,
		},
		{
			AthleteID:    3,
			AccessToken:  "token3",
			RefreshToken: "refresh3",
			ExpiresAt:    now + 3600,
			Scope:        "activity:read_all",
			Authorized:   true,
		},
	}

	for _, a := range athletes {
		if err := db.CreateAthlete(a); err != nil {
			t.Fatalf("Failed to create athlete: %v", err)
		}
	}

	// List all athletes
	all, err := db.ListAthletes(false, 0, 0)
	if err != nil {
		t.Fatalf("Failed to list athletes: %v", err)
	}
	if len(all) != 3 {
		t.Errorf("Expected 3 athletes, got %d", len(all))
	}

	// List only authorized athletes
	authorized, err := db.ListAthletes(true, 0, 0)
	if err != nil {
		t.Fatalf("Failed to list authorized athletes: %v", err)
	}
	if len(authorized) != 2 {
		t.Errorf("Expected 2 authorized athletes, got %d", len(authorized))
	}

	// Test pagination
	page1, err := db.ListAthletes(false, 0, 2)
	if err != nil {
		t.Fatalf("Failed to list athletes with limit: %v", err)
	}
	if len(page1) != 2 {
		t.Errorf("Expected 2 athletes on page 1, got %d", len(page1))
	}

	page2, err := db.ListAthletes(false, 2, 2)
	if err != nil {
		t.Fatalf("Failed to list athletes with offset: %v", err)
	}
	if len(page2) != 1 {
		t.Errorf("Expected 1 athlete on page 2, got %d", len(page2))
	}
}
