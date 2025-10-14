package database

import (
	"testing"
	"time"
)

func TestCreateAndGetActivity(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	if err := db.Init(); err != nil {
		t.Fatalf("Failed to init db: %v", err)
	}

	// Create athlete first (foreign key requirement)
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

	summary := `{"name":"Morning Run"}`
	startDate := time.Now().Unix()
	activityType := "Run"

	activity := &Activity{
		ID:           98765,
		AthleteID:    12345,
		HasSummary:   true,
		HasDetails:   false,
		Deleted:      false,
		SummaryJSON:  &summary,
		StartDate:    &startDate,
		ActivityType: &activityType,
	}

	// Create activity
	if err := db.CreateActivity(activity); err != nil {
		t.Fatalf("Failed to create activity: %v", err)
	}

	// Get activity
	retrieved, err := db.GetActivity(98765)
	if err != nil {
		t.Fatalf("Failed to get activity: %v", err)
	}
	if retrieved == nil {
		t.Fatal("Expected activity, got nil")
	}

	if retrieved.ID != 98765 {
		t.Errorf("Expected id 98765, got %d", retrieved.ID)
	}
	if retrieved.AthleteID != 12345 {
		t.Errorf("Expected athlete_id 12345, got %d", retrieved.AthleteID)
	}
	if !retrieved.HasSummary {
		t.Error("Expected has_summary true")
	}
}

func TestUpsertActivitySummary(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	if err := db.Init(); err != nil {
		t.Fatalf("Failed to init db: %v", err)
	}

	// Create athlete
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

	startDate := time.Now().Unix()
	activityType := "Run"

	// Insert new activity via upsert
	summary1 := `{"name":"Morning Run","distance":5000}`
	if err := db.UpsertActivitySummary(98765, 12345, summary1, &startDate, &activityType); err != nil {
		t.Fatalf("Failed to upsert activity: %v", err)
	}

	// Verify insert
	activity, err := db.GetActivity(98765)
	if err != nil {
		t.Fatalf("Failed to get activity: %v", err)
	}
	if activity == nil {
		t.Fatal("Expected activity, got nil")
	}
	if !activity.HasSummary {
		t.Error("Expected has_summary true")
	}

	// Update existing activity via upsert
	summary2 := `{"name":"Morning Run Updated","distance":6000}`
	if err := db.UpsertActivitySummary(98765, 12345, summary2, &startDate, &activityType); err != nil {
		t.Fatalf("Failed to upsert activity: %v", err)
	}

	// Verify update
	updated, err := db.GetActivity(98765)
	if err != nil {
		t.Fatalf("Failed to get activity: %v", err)
	}
	if updated.SummaryJSON == nil || *updated.SummaryJSON != summary2 {
		t.Errorf("Expected updated summary, got %v", updated.SummaryJSON)
	}
}

func TestUpdateActivityDetails(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	if err := db.Init(); err != nil {
		t.Fatalf("Failed to init db: %v", err)
	}

	// Create athlete and activity
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

	summary := `{"name":"Run"}`
	startDate := time.Now().Unix()
	activityType := "Run"
	if err := db.UpsertActivitySummary(98765, 12345, summary, &startDate, &activityType); err != nil {
		t.Fatalf("Failed to create activity: %v", err)
	}

	// Update details
	details := `{"splits":[{"distance":1000,"time":300}]}`
	if err := db.UpdateActivityDetails(98765, details); err != nil {
		t.Fatalf("Failed to update details: %v", err)
	}

	// Verify update
	activity, err := db.GetActivity(98765)
	if err != nil {
		t.Fatalf("Failed to get activity: %v", err)
	}
	if !activity.HasDetails {
		t.Error("Expected has_details true")
	}
	if activity.DetailsJSON == nil || *activity.DetailsJSON != details {
		t.Errorf("Expected details %s, got %v", details, activity.DetailsJSON)
	}
}

func TestMarkActivityDeleted(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	if err := db.Init(); err != nil {
		t.Fatalf("Failed to init db: %v", err)
	}

	// Create athlete and activity
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

	summary := `{"name":"Run"}`
	startDate := time.Now().Unix()
	activityType := "Run"
	if err := db.UpsertActivitySummary(98765, 12345, summary, &startDate, &activityType); err != nil {
		t.Fatalf("Failed to create activity: %v", err)
	}

	// Mark deleted
	if err := db.MarkActivityDeleted(98765); err != nil {
		t.Fatalf("Failed to mark activity deleted: %v", err)
	}

	// Verify deletion
	activity, err := db.GetActivity(98765)
	if err != nil {
		t.Fatalf("Failed to get activity: %v", err)
	}
	if !activity.Deleted {
		t.Error("Expected deleted true")
	}
	if activity.SummaryJSON != nil {
		t.Error("Expected summary_json to be cleared")
	}
	if activity.DetailsJSON != nil {
		t.Error("Expected details_json to be cleared")
	}
}

func TestListActivitiesByAthlete(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	if err := db.Init(); err != nil {
		t.Fatalf("Failed to init db: %v", err)
	}

	// Create athlete
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

	// Create multiple activities
	now := time.Now().Unix()
	activityType := "Run"
	for i := 1; i <= 3; i++ {
		summary := `{"name":"Run"}`
		startDate := now - int64(i*1000)
		if err := db.UpsertActivitySummary(int64(i), 12345, summary, &startDate, &activityType); err != nil {
			t.Fatalf("Failed to create activity %d: %v", i, err)
		}
	}

	// Mark one as deleted
	if err := db.MarkActivityDeleted(2); err != nil {
		t.Fatalf("Failed to mark activity deleted: %v", err)
	}

	// List all activities
	all, err := db.ListActivitiesByAthlete(12345, 0, 0, true)
	if err != nil {
		t.Fatalf("Failed to list activities: %v", err)
	}
	if len(all) != 3 {
		t.Errorf("Expected 3 activities, got %d", len(all))
	}

	// List non-deleted activities
	active, err := db.ListActivitiesByAthlete(12345, 0, 0, false)
	if err != nil {
		t.Fatalf("Failed to list active activities: %v", err)
	}
	if len(active) != 2 {
		t.Errorf("Expected 2 active activities, got %d", len(active))
	}

	// Test pagination
	limited, err := db.ListActivitiesByAthlete(12345, 0, 1, false)
	if err != nil {
		t.Fatalf("Failed to list limited activities: %v", err)
	}
	if len(limited) != 1 {
		t.Errorf("Expected 1 activity, got %d", len(limited))
	}
}

func TestListActivitiesNeedingDetails(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	if err := db.Init(); err != nil {
		t.Fatalf("Failed to init db: %v", err)
	}

	// Create athlete
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

	// Create activities
	now := time.Now().Unix()
	activityType := "Run"
	summary := `{"name":"Run"}`

	// Activity 1: needs details
	startDate1 := now - 1000
	if err := db.UpsertActivitySummary(1, 12345, summary, &startDate1, &activityType); err != nil {
		t.Fatalf("Failed to create activity: %v", err)
	}

	// Activity 2: has details
	startDate2 := now - 2000
	if err := db.UpsertActivitySummary(2, 12345, summary, &startDate2, &activityType); err != nil {
		t.Fatalf("Failed to create activity: %v", err)
	}
	details := `{"splits":[]}`
	if err := db.UpdateActivityDetails(2, details); err != nil {
		t.Fatalf("Failed to update details: %v", err)
	}

	// Activity 3: needs details
	startDate3 := now - 3000
	if err := db.UpsertActivitySummary(3, 12345, summary, &startDate3, &activityType); err != nil {
		t.Fatalf("Failed to create activity: %v", err)
	}

	// List activities needing details
	needing, err := db.ListActivitiesNeedingDetails(0, 0)
	if err != nil {
		t.Fatalf("Failed to list activities needing details: %v", err)
	}
	if len(needing) != 2 {
		t.Errorf("Expected 2 activities needing details, got %d", len(needing))
	}

	// Test pagination
	limited, err := db.ListActivitiesNeedingDetails(0, 1)
	if err != nil {
		t.Fatalf("Failed to list limited activities: %v", err)
	}
	if len(limited) != 1 {
		t.Errorf("Expected 1 activity, got %d", len(limited))
	}
}
