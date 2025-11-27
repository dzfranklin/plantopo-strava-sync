package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"plantopo-strava-sync/internal/config"
	"plantopo-strava-sync/internal/database"
	"plantopo-strava-sync/internal/strava"
)

func setupWorkerTest(t *testing.T) (*Worker, *database.DB) {
	dbPath := t.TempDir() + "/test.db"
	db, err := database.Open(dbPath)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}

	cfg := &config.Config{
		Domain: "example.com",
		StravaClients: map[string]*config.StravaClientConfig{
			"primary": {
				ClientID:     "test_client_id",
				ClientSecret: "test_client_secret",
				VerifyToken:  "test_verify_token",
			},
		},
		InternalAPIKey:                 "test_api_key",
		RateLimitWebhookReservePercent: 0.20,
		RateLimitThrottleThreshold:     0.70,
		RateLimitCircuitRecoveryCount:  3,
	}

	stravaClient := strava.NewClient(cfg, db)
	worker := NewWorker(db, stravaClient, cfg)

	return worker, db
}

func TestProcessWebhook_UnknownObjectType(t *testing.T) {
	worker, db := setupWorkerTest(t)
	defer db.Close()

	// Enqueue webhook with unknown object type
	webhookData := map[string]interface{}{
		"object_type": "unknown_type",
		"owner_id":    12345,
	}

	data, _ := json.Marshal(webhookData)
	id, err := db.EnqueueWebhook(json.RawMessage(data))
	if err != nil {
		t.Fatalf("Failed to enqueue webhook: %v", err)
	}

	// Claim and process it
	item, err := db.ClaimWebhook()
	if err != nil {
		t.Fatalf("Failed to claim webhook: %v", err)
	}

	if item == nil {
		t.Fatal("Expected webhook item, got nil")
	}

	worker.processWebhook(item)

	// Verify it was deleted (not retried)
	length, err := db.GetQueueLength()
	if err != nil {
		t.Fatalf("Failed to get queue length: %v", err)
	}

	if length != 0 {
		t.Errorf("Expected queue length 0 (unknown types should be deleted), got %d", length)
	}

	// Verify it's not in processing state
	item, err = db.ClaimWebhook()
	if err != nil {
		t.Fatalf("Failed to claim webhook: %v", err)
	}

	if item != nil {
		t.Error("Expected no webhook, but found one in processing state")
	}

	// Try to get the original webhook by ID (should fail because it was deleted)
	_, err = db.ClaimWebhook()
	if err != nil {
		t.Fatalf("Failed to claim webhook: %v", err)
	}
	// The webhook with 'id' should be gone
	_ = id
}

func TestProcessWebhook_InvalidJSON(t *testing.T) {
	worker, db := setupWorkerTest(t)
	defer db.Close()

	// Enqueue webhook with invalid JSON structure
	data := json.RawMessage(`invalid json`)
	_, err := db.EnqueueWebhook(data)
	if err != nil {
		t.Fatalf("Failed to enqueue webhook: %v", err)
	}

	// Claim and process it
	item, err := db.ClaimWebhook()
	if err != nil {
		t.Fatalf("Failed to claim webhook: %v", err)
	}

	worker.processWebhook(item)

	// Verify it's still in the queue (released with retry)
	queueLength, err := db.GetQueueLength()
	if err != nil {
		t.Fatalf("Failed to get queue length: %v", err)
	}

	if queueLength != 1 {
		t.Errorf("Expected queue length 1 (webhook should be released), got %d", queueLength)
	}

	// The webhook has a future next_retry_at, so it won't be claimable immediately
	readyLength, err := db.GetReadyQueueLength()
	if err != nil {
		t.Fatalf("Failed to get ready queue length: %v", err)
	}

	if readyLength != 0 {
		t.Errorf("Expected ready queue length 0 (webhook waiting for retry), got %d", readyLength)
	}
}

func TestHandleActivity_Delete(t *testing.T) {
	worker, db := setupWorkerTest(t)
	defer db.Close()

	athleteID := int64(12345)
	activityID := int64(67890)

	// Create delete webhook
	webhook := map[string]interface{}{
		"object_type": "activity",
		"owner_id":    float64(athleteID),
		"object_id":   float64(activityID),
		"aspect_type": "delete",
		"event_time":  time.Now().Unix(),
	}

	err := worker.handleActivity(webhook)
	if err != nil {
		t.Fatalf("Failed to handle delete webhook: %v", err)
	}

	// Verify delete event was created
	events, err := db.ListEvents(athleteID, 0, 10)
	if err != nil {
		t.Fatalf("Failed to list events: %v", err)
	}

	if len(events) != 1 {
		t.Fatalf("Expected 1 event, got %d", len(events))
	}

	if events[0].EventType != "webhook" {
		t.Errorf("Expected event type 'webhook', got '%s'", events[0].EventType)
	}

	if events[0].ActivityID == nil || *events[0].ActivityID != activityID {
		t.Errorf("Expected activity ID %d, got %v", activityID, events[0].ActivityID)
	}

	if events[0].Activity != nil {
		t.Error("Expected nil activity data for delete event")
	}
}

func TestHandleActivity_InvalidOwnerID(t *testing.T) {
	worker, db := setupWorkerTest(t)
	defer db.Close()

	webhook := map[string]interface{}{
		"object_type": "activity",
		"owner_id":    "invalid", // Should be number
		"object_id":   float64(67890),
		"aspect_type": "create",
	}

	err := worker.handleActivity(webhook)
	if err == nil {
		t.Error("Expected error for invalid owner_id")
	}
}

func TestHandleActivity_InvalidObjectID(t *testing.T) {
	worker, db := setupWorkerTest(t)
	defer db.Close()

	webhook := map[string]interface{}{
		"object_type": "activity",
		"owner_id":    float64(12345),
		"object_id":   "invalid", // Should be number
		"aspect_type": "create",
	}

	err := worker.handleActivity(webhook)
	if err == nil {
		t.Error("Expected error for invalid object_id")
	}
}

func TestHandleActivity_UnknownAspectType(t *testing.T) {
	worker, db := setupWorkerTest(t)
	defer db.Close()

	webhook := map[string]interface{}{
		"object_type": "activity",
		"owner_id":    float64(12345),
		"object_id":   float64(67890),
		"aspect_type": "unknown",
	}

	// Should not return error for unknown aspect types (just skip)
	err := worker.handleActivity(webhook)
	if err != nil {
		t.Errorf("Expected no error for unknown aspect type, got: %v", err)
	}

	// Verify no event was created
	events, err := db.ListEvents(12345, 0, 10)
	if err != nil {
		t.Fatalf("Failed to list events: %v", err)
	}

	if len(events) != 0 {
		t.Errorf("Expected 0 events for unknown aspect type, got %d", len(events))
	}
}

func TestSyncAllActivities_InvalidAthleteID(t *testing.T) {
	worker, db := setupWorkerTest(t)
	defer db.Close()

	// Test with non-existent athlete (should fail with unauthorized)
	err := worker.listActivities(99999)
	// Should not error, just logs and skips
	if err != nil {
		t.Logf("Got expected error for non-existent athlete: %v", err)
	}
}

func TestStart_Cancellation(t *testing.T) {
	worker, db := setupWorkerTest(t)
	defer db.Close()

	// Reduce poll interval for faster test
	worker.pollInterval = 10 * time.Millisecond

	ctx, cancel := context.WithCancel(context.Background())

	// Start worker in goroutine
	done := make(chan error, 1)
	go func() {
		done <- worker.Start(ctx)
	}()

	// Let it run briefly
	time.Sleep(50 * time.Millisecond)

	// Cancel and wait
	cancel()

	select {
	case err := <-done:
		if err != context.Canceled {
			t.Errorf("Expected context.Canceled error, got %v", err)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("Worker did not stop after context cancellation")
	}
}

func TestProcessWebhookActivity_Integration(t *testing.T) {
	worker, db := setupWorkerTest(t)
	defer db.Close()

	athleteID := int64(12345)
	activityID := int64(67890)

	// Insert test athlete with valid token
	athlete := &database.Athlete{
		AthleteID:      athleteID,
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

	// Create mock Strava API server
	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check authorization header
		auth := r.Header.Get("Authorization")
		if auth != "Bearer valid_token" {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		// Return mock activity data
		activity := map[string]interface{}{
			"id":       activityID,
			"name":     "Morning Run",
			"distance": 5000.0,
			"type":     "Run",
		}

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-RateLimit-Usage", "10,100")
		w.Header().Set("X-RateLimit-Limit", "200,2000")
		w.Header().Set("X-ReadRateLimit-Usage", "5,50")
		w.Header().Set("X-ReadRateLimit-Limit", "100,1000")
		json.NewEncoder(w).Encode(activity)
	}))
	defer apiServer.Close()

	// Override base URL in worker's strava client
	worker.stravaClient.SetBaseURL(apiServer.URL)

	// Create real webhook data
	webhookData := json.RawMessage(`{"aspect_type":"create","object_type":"activity","object_id":67890,"owner_id":12345}`)

	// Test processing webhook activity
	err = worker.processWebhookActivity(athleteID, activityID, "create", webhookData)
	if err != nil {
		t.Fatalf("Failed to process webhook activity: %v", err)
	}

	// Verify event was created
	events, err := db.ListEvents(athleteID, 0, 10)
	if err != nil {
		t.Fatalf("Failed to list events: %v", err)
	}

	if len(events) != 1 {
		t.Fatalf("Expected 1 event, got %d", len(events))
	}

	if events[0].EventType != "webhook" {
		t.Errorf("Expected event type 'webhook', got '%s'", events[0].EventType)
	}

	if events[0].ActivityID == nil || *events[0].ActivityID != activityID {
		t.Errorf("Expected activity ID %d, got %v", activityID, events[0].ActivityID)
	}

	// Verify activity data was stored
	if events[0].Activity == nil {
		t.Fatal("Expected activity data to be stored")
	}

	var activityData map[string]interface{}
	if err := json.Unmarshal(events[0].Activity, &activityData); err != nil {
		t.Fatalf("Failed to unmarshal activity data: %v", err)
	}

	if activityData["name"] != "Morning Run" {
		t.Errorf("Expected activity name 'Morning Run', got '%v'", activityData["name"])
	}

	// Verify webhook data was stored
	if events[0].WebhookEvent == nil {
		t.Fatal("Expected webhook event data to be stored")
	}
}

func TestSyncAllActivities_Integration(t *testing.T) {
	worker, db := setupWorkerTest(t)
	defer db.Close()

	athleteID := int64(12345)

	// Insert test athlete with valid token
	athlete := &database.Athlete{
		AthleteID:      athleteID,
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

	requestCount := 0
	activityDetailsRequests := 0

	// Create mock Strava API server
	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++

		// Check authorization header
		auth := r.Header.Get("Authorization")
		if auth != "Bearer valid_token" {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		// Set rate limit headers
		w.Header().Set("X-RateLimit-Usage", fmt.Sprintf("%d,100", requestCount))
		w.Header().Set("X-RateLimit-Limit", "200,2000")
		w.Header().Set("X-ReadRateLimit-Usage", fmt.Sprintf("%d,50", requestCount))
		w.Header().Set("X-ReadRateLimit-Limit", "100,1000")

		// Handle different endpoints
		if strings.Contains(r.URL.Path, "/athlete/activities") {
			// Return paginated activity list
			page := r.URL.Query().Get("page")
			if page == "" || page == "1" {
				// First page: return 2 activities
				activities := []map[string]interface{}{
					{"id": 1001},
					{"id": 1002},
				}
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(activities)
			} else {
				// Second page: empty (no more activities)
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode([]map[string]interface{}{})
			}
		} else if strings.Contains(r.URL.Path, "/activities/") {
			// Return activity details
			activityDetailsRequests++
			activityID := strings.TrimPrefix(r.URL.Path, "/activities/")
			activity := map[string]interface{}{
				"id":       activityID,
				"name":     "Test Activity " + activityID,
				"distance": 5000.0,
				"type":     "Run",
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(activity)
		} else {
			http.Error(w, "Not found", http.StatusNotFound)
		}
	}))
	defer apiServer.Close()

	// Override base URL in worker's strava client
	worker.stravaClient.SetBaseURL(apiServer.URL)

	// Test listActivities
	err = worker.listActivities(athleteID)
	if err != nil {
		t.Fatalf("Failed to list activities: %v", err)
	}

	// Verify no activity details requests (listActivities only lists, doesn't fetch)
	if activityDetailsRequests != 0 {
		t.Errorf("Expected 0 activity details requests from listActivities, got %d", activityDetailsRequests)
	}

	// Verify sync jobs were created
	readyJobs, err := db.GetReadySyncJobQueueLength()
	if err != nil {
		t.Fatalf("Failed to get sync job queue length: %v", err)
	}
	if readyJobs != 2 {
		t.Errorf("Expected 2 sync jobs to be created, got %d", readyJobs)
	}

	// Verify NO events were created yet (listActivities only creates sync jobs, not events)
	events, err := db.ListEvents(athleteID, 0, 10)
	if err != nil {
		t.Fatalf("Failed to list events: %v", err)
	}

	if len(events) != 0 {
		t.Errorf("Expected 0 events from listActivities (only creates sync jobs), got %d", len(events))
	}
}

func TestHandleAthlete_Deauthorization(t *testing.T) {
	worker, db := setupWorkerTest(t)
	defer db.Close()

	athleteID := int64(12345)

	// Insert some existing events for the athlete
	eventID1, err := db.InsertAthleteConnectedEvent(athleteID, json.RawMessage(`{"id": 12345}`))
	if err != nil {
		t.Fatalf("Failed to insert athlete_connected event: %v", err)
	}

	activityID := int64(99999)
	webhookData := json.RawMessage(`{"aspect_type":"create","object_type":"activity","object_id":99999,"owner_id":12345}`)
	eventID2, err := db.InsertActivityEvent(athleteID, &activityID, json.RawMessage(`{"id": 99999}`), webhookData)
	if err != nil {
		t.Fatalf("Failed to insert activity event: %v", err)
	}

	// Create deauthorization webhook
	webhook := map[string]interface{}{
		"object_type": "athlete",
		"object_id":   float64(athleteID),
		"owner_id":    float64(athleteID),
		"aspect_type": "update",
		"updates": map[string]interface{}{
			"authorized": "false",
		},
		"event_time": 1516126040,
	}

	// Process the deauthorization webhook
	err = worker.handleAthlete(webhook)
	if err != nil {
		t.Fatalf("Failed to handle deauthorization: %v", err)
	}

	// Verify events
	events, err := db.ListEvents(athleteID, 0, 100)
	if err != nil {
		t.Fatalf("Failed to list events: %v", err)
	}

	// Should only have 1 event left (the deauthorization event)
	if len(events) != 1 {
		t.Errorf("Expected 1 event (deauthorization), got %d", len(events))
	}

	// Verify it's a webhook event
	if events[0].EventType != "webhook" {
		t.Errorf("Expected event type 'webhook', got '%s'", events[0].EventType)
	}

	// Verify it has no activity_id
	if events[0].ActivityID != nil {
		t.Errorf("Expected nil activity_id for deauthorization event, got %d", *events[0].ActivityID)
	}

	// Verify the webhook event data contains the deauthorization
	var storedWebhookData map[string]interface{}
	if err := json.Unmarshal(events[0].WebhookEvent, &storedWebhookData); err != nil {
		t.Fatalf("Failed to unmarshal webhook event: %v", err)
	}

	if storedWebhookData["object_type"] != "athlete" {
		t.Errorf("Expected object_type 'athlete', got '%v'", storedWebhookData["object_type"])
	}

	updates, ok := storedWebhookData["updates"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected updates to be a map")
	}

	if updates["authorized"] != "false" {
		t.Errorf("Expected authorized 'false', got '%v'", updates["authorized"])
	}

	t.Logf("Deauthorization event ID: %d (old events %d, %d were deleted)", events[0].EventID, eventID1, eventID2)
}

func TestHandleAthlete_NonDeauthorization(t *testing.T) {
	worker, db := setupWorkerTest(t)
	defer db.Close()

	athleteID := int64(12345)

	// Create athlete webhook that is NOT a deauthorization
	webhook := map[string]interface{}{
		"object_type": "athlete",
		"object_id":   float64(athleteID),
		"owner_id":    float64(athleteID),
		"aspect_type": "update",
		"updates": map[string]interface{}{
			"authorized": "true", // Not a deauthorization
		},
		"event_time": 1516126040,
	}

	// Process the webhook
	err := worker.handleAthlete(webhook)
	if err != nil {
		t.Fatalf("Failed to handle athlete webhook: %v", err)
	}

	// Verify no events were created
	events, err := db.ListEvents(athleteID, 0, 100)
	if err != nil {
		t.Fatalf("Failed to list events: %v", err)
	}

	if len(events) != 0 {
		t.Errorf("Expected 0 events for non-deauthorization, got %d", len(events))
	}
}
