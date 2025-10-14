package database

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestOpenDatabase(t *testing.T) {
	// Create a temporary directory for test database
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	// Open database
	db, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	// Test health check
	if err := db.Health(); err != nil {
		t.Errorf("Health check failed: %v", err)
	}

	// Verify database file was created
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		t.Error("Database file was not created")
	}
}

func TestInitializeSchema(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// Initialize schema
	if err := db.Init(); err != nil {
		t.Fatalf("Failed to initialize schema: %v", err)
	}

	// Verify all tables exist
	tables := []string{"athletes", "activities", "webhook_events"}
	for _, table := range tables {
		var name string
		query := "SELECT name FROM sqlite_master WHERE type='table' AND name=?"
		err := db.conn.QueryRow(query, table).Scan(&name)
		if err != nil {
			t.Errorf("Table %s does not exist: %v", table, err)
		}
	}
}

func TestAthletesTable(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	if err := db.Init(); err != nil {
		t.Fatalf("Failed to initialize schema: %v", err)
	}

	now := time.Now().Unix()

	// Insert test athlete
	_, err := db.conn.Exec(`
		INSERT INTO athletes (
			athlete_id, access_token, refresh_token, expires_at, scope,
			authorized, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, 12345, "access_token_123", "refresh_token_456", now+3600, "activity:read_all",
		true, now, now)

	if err != nil {
		t.Fatalf("Failed to insert athlete: %v", err)
	}

	// Query athlete
	var athleteID int64
	var accessToken, refreshToken, scope string
	var expiresAt, createdAt, updatedAt int64
	var authorized bool

	err = db.conn.QueryRow("SELECT athlete_id, access_token, refresh_token, expires_at, scope, authorized, created_at, updated_at FROM athletes WHERE athlete_id = ?", 12345).
		Scan(&athleteID, &accessToken, &refreshToken, &expiresAt, &scope, &authorized, &createdAt, &updatedAt)

	if err != nil {
		t.Fatalf("Failed to query athlete: %v", err)
	}

	// Verify data
	if athleteID != 12345 {
		t.Errorf("Expected athlete_id 12345, got %d", athleteID)
	}
	if accessToken != "access_token_123" {
		t.Errorf("Expected access_token 'access_token_123', got %s", accessToken)
	}
	if !authorized {
		t.Error("Expected authorized to be true")
	}
}

func TestActivitiesTable(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	if err := db.Init(); err != nil {
		t.Fatalf("Failed to initialize schema: %v", err)
	}

	now := time.Now().Unix()

	// First insert an athlete (foreign key requirement)
	_, err := db.conn.Exec(`
		INSERT INTO athletes (
			athlete_id, access_token, refresh_token, expires_at, scope,
			authorized, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, 12345, "token", "refresh", now+3600, "activity:read_all", true, now, now)

	if err != nil {
		t.Fatalf("Failed to insert athlete: %v", err)
	}

	// Insert test activity
	_, err = db.conn.Exec(`
		INSERT INTO activities (
			id, athlete_id, has_summary, has_details, deleted,
			summary_json, start_date, activity_type,
			created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, 98765, 12345, true, false, false,
		`{"name":"Morning Run","distance":5000}`, now, "Run", now, now)

	if err != nil {
		t.Fatalf("Failed to insert activity: %v", err)
	}

	// Query activity
	var activityID, athleteID int64
	var hasSummary, hasDetails, deleted bool
	var summaryJSON, activityType string
	var startDate int64

	err = db.conn.QueryRow(`
		SELECT id, athlete_id, has_summary, has_details, deleted,
		       summary_json, start_date, activity_type
		FROM activities WHERE id = ?`, 98765).
		Scan(&activityID, &athleteID, &hasSummary, &hasDetails, &deleted,
			&summaryJSON, &startDate, &activityType)

	if err != nil {
		t.Fatalf("Failed to query activity: %v", err)
	}

	// Verify data
	if activityID != 98765 {
		t.Errorf("Expected activity id 98765, got %d", activityID)
	}
	if athleteID != 12345 {
		t.Errorf("Expected athlete_id 12345, got %d", athleteID)
	}
	if !hasSummary {
		t.Error("Expected has_summary to be true")
	}
	if hasDetails {
		t.Error("Expected has_details to be false")
	}
	if activityType != "Run" {
		t.Errorf("Expected activity_type 'Run', got %s", activityType)
	}
}

func TestWebhookEventsTable(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	if err := db.Init(); err != nil {
		t.Fatalf("Failed to initialize schema: %v", err)
	}

	now := time.Now().Unix()

	// Insert test webhook event
	result, err := db.conn.Exec(`
		INSERT INTO webhook_events (
			object_type, object_id, aspect_type, owner_id, subscription_id,
			event_time, raw_json, processed, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, "activity", 98765, "create", 12345, 1,
		now, `{"object_type":"activity","object_id":98765}`, false, now)

	if err != nil {
		t.Fatalf("Failed to insert webhook event: %v", err)
	}

	// Get auto-incremented ID
	eventID, err := result.LastInsertId()
	if err != nil {
		t.Fatalf("Failed to get last insert ID: %v", err)
	}

	// Query webhook event
	var id, objectID, ownerID, subscriptionID, eventTime, createdAt int64
	var objectType, aspectType, rawJSON string
	var processed bool

	err = db.conn.QueryRow(`
		SELECT id, object_type, object_id, aspect_type, owner_id,
		       subscription_id, event_time, raw_json, processed, created_at
		FROM webhook_events WHERE id = ?`, eventID).
		Scan(&id, &objectType, &objectID, &aspectType, &ownerID,
			&subscriptionID, &eventTime, &rawJSON, &processed, &createdAt)

	if err != nil {
		t.Fatalf("Failed to query webhook event: %v", err)
	}

	// Verify data
	if objectType != "activity" {
		t.Errorf("Expected object_type 'activity', got %s", objectType)
	}
	if objectID != 98765 {
		t.Errorf("Expected object_id 98765, got %d", objectID)
	}
	if aspectType != "create" {
		t.Errorf("Expected aspect_type 'create', got %s", aspectType)
	}
	if processed {
		t.Error("Expected processed to be false")
	}
}

func TestForeignKeyConstraint(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	if err := db.Init(); err != nil {
		t.Fatalf("Failed to initialize schema: %v", err)
	}

	now := time.Now().Unix()

	// Try to insert activity with non-existent athlete (should fail)
	_, err := db.conn.Exec(`
		INSERT INTO activities (
			id, athlete_id, has_summary, has_details, deleted,
			created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?)
	`, 98765, 99999, false, false, false, now, now)

	if err == nil {
		t.Error("Expected foreign key constraint violation, but insert succeeded")
	}
}

func TestUniqueWebhookConstraint(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	if err := db.Init(); err != nil {
		t.Fatalf("Failed to initialize schema: %v", err)
	}

	now := time.Now().Unix()

	// Insert first webhook event
	_, err := db.conn.Exec(`
		INSERT INTO webhook_events (
			object_type, object_id, aspect_type, owner_id, subscription_id,
			event_time, raw_json, processed, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, "activity", 98765, "create", 12345, 1, now, `{"test":1}`, false, now)

	if err != nil {
		t.Fatalf("Failed to insert first webhook event: %v", err)
	}

	// Try to insert duplicate (same event_time, object_id, aspect_type)
	_, err = db.conn.Exec(`
		INSERT INTO webhook_events (
			object_type, object_id, aspect_type, owner_id, subscription_id,
			event_time, raw_json, processed, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, "activity", 98765, "create", 12345, 1, now, `{"test":2}`, false, now)

	if err == nil {
		t.Error("Expected unique constraint violation, but insert succeeded")
	}
}

func TestIndexesExist(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	if err := db.Init(); err != nil {
		t.Fatalf("Failed to initialize schema: %v", err)
	}

	// Check that important indexes exist
	indexes := []string{
		"idx_athletes_authorized",
		"idx_activities_athlete_id",
		"idx_activities_start_date",
		"idx_activities_has_details",
		"idx_webhook_events_processed",
		"idx_webhook_events_unique",
	}

	for _, indexName := range indexes {
		var name string
		query := "SELECT name FROM sqlite_master WHERE type='index' AND name=?"
		err := db.conn.QueryRow(query, indexName).Scan(&name)
		if err != nil {
			t.Errorf("Index %s does not exist: %v", indexName, err)
		}
	}
}

func TestTransactions(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	if err := db.Init(); err != nil {
		t.Fatalf("Failed to initialize schema: %v", err)
	}

	now := time.Now().Unix()

	// Begin transaction
	tx, err := db.Begin()
	if err != nil {
		t.Fatalf("Failed to begin transaction: %v", err)
	}

	// Insert athlete in transaction
	_, err = tx.Exec(`
		INSERT INTO athletes (
			athlete_id, access_token, refresh_token, expires_at, scope,
			authorized, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, 12345, "token", "refresh", now+3600, "activity:read_all", true, now, now)

	if err != nil {
		tx.Rollback()
		t.Fatalf("Failed to insert in transaction: %v", err)
	}

	// Rollback transaction
	if err := tx.Rollback(); err != nil {
		t.Fatalf("Failed to rollback: %v", err)
	}

	// Verify athlete was not inserted
	var count int
	err = db.conn.QueryRow("SELECT COUNT(*) FROM athletes WHERE athlete_id = ?", 12345).Scan(&count)
	if err != nil {
		t.Fatalf("Failed to query: %v", err)
	}
	if count != 0 {
		t.Error("Expected athlete to not exist after rollback")
	}
}

// setupTestDB creates a temporary in-memory database for testing
func setupTestDB(t *testing.T) *DB {
	t.Helper()

	// Use a temporary file instead of :memory: to ensure proper cleanup
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Failed to open test database: %v", err)
	}

	return db
}
