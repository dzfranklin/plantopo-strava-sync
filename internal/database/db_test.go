package database

import (
	"encoding/json"
	"testing"
	"time"
)

func TestDatabaseOperations(t *testing.T) {
	dbPath := t.TempDir() + "/test.db"

	db, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	// Test athlete operations
	t.Run("UpsertAndGetAthlete", func(t *testing.T) {
		athlete := &Athlete{
			AthleteID:      12345,
			AccessToken:    "test_access_token",
			RefreshToken:   "test_refresh_token",
			TokenExpiresAt: time.Now().Add(6 * time.Hour),
			AthleteSummary: json.RawMessage(`{"id": 12345, "username": "testuser"}`),
			CreatedAt:      time.Now(),
			UpdatedAt:      time.Now(),
		}

		err := db.UpsertAthlete(athlete)
		if err != nil {
			t.Fatalf("Failed to upsert athlete: %v", err)
		}

		retrieved, err := db.GetAthlete(12345)
		if err != nil {
			t.Fatalf("Failed to get athlete: %v", err)
		}

		if retrieved == nil {
			t.Fatal("Expected athlete to be found")
		}

		if retrieved.AccessToken != athlete.AccessToken {
			t.Errorf("Expected access token %s, got %s", athlete.AccessToken, retrieved.AccessToken)
		}
	})

	// Test webhook queue operations
	t.Run("WebhookQueue", func(t *testing.T) {
		webhookData := json.RawMessage(`{"object_type": "activity", "object_id": 123}`)

		id, err := db.EnqueueWebhook(webhookData)
		if err != nil {
			t.Fatalf("Failed to enqueue webhook: %v", err)
		}

		if id == 0 {
			t.Fatal("Expected non-zero queue item id")
		}

		length, err := db.GetQueueLength()
		if err != nil {
			t.Fatalf("Failed to get queue length: %v", err)
		}

		if length != 1 {
			t.Errorf("Expected queue length 1, got %d", length)
		}

		readyLength, err := db.GetReadyQueueLength()
		if err != nil {
			t.Fatalf("Failed to get ready queue length: %v", err)
		}

		if readyLength != 1 {
			t.Errorf("Expected ready queue length 1, got %d", readyLength)
		}

		// Claim the webhook
		item, err := db.ClaimWebhook()
		if err != nil {
			t.Fatalf("Failed to claim webhook: %v", err)
		}

		if item == nil {
			t.Fatal("Expected webhook item to be claimed")
		}

		if string(item.Data) != string(webhookData) {
			t.Errorf("Expected data %s, got %s", webhookData, item.Data)
		}

		if item.RetryCount != 0 {
			t.Errorf("Expected retry count 0, got %d", item.RetryCount)
		}

		if item.ProcessingStartedAt == nil {
			t.Error("Expected processing_started_at to be set")
		}

		// Should still be in queue but not ready (being processed)
		length, err = db.GetQueueLength()
		if err != nil {
			t.Fatalf("Failed to get queue length: %v", err)
		}

		if length != 1 {
			t.Errorf("Expected queue length 1 (still in queue), got %d", length)
		}

		readyLength, err = db.GetReadyQueueLength()
		if err != nil {
			t.Fatalf("Failed to get ready queue length: %v", err)
		}

		if readyLength != 0 {
			t.Errorf("Expected ready queue length 0 (being processed), got %d", readyLength)
		}

		// Complete successfully
		err = db.DeleteWebhook(item.ID)
		if err != nil {
			t.Fatalf("Failed to complete webhook: %v", err)
		}

		// Queue should be empty now
		length, err = db.GetQueueLength()
		if err != nil {
			t.Fatalf("Failed to get queue length: %v", err)
		}

		if length != 0 {
			t.Errorf("Expected queue length 0, got %d", length)
		}
	})

	// Test webhook retry logic
	t.Run("WebhookRetry", func(t *testing.T) {
		webhookData := json.RawMessage(`{"object_type": "activity", "object_id": 456}`)

		// Enqueue initial webhook
		_, err := db.EnqueueWebhook(webhookData)
		if err != nil {
			t.Fatalf("Failed to enqueue webhook: %v", err)
		}

		// Claim it
		item, err := db.ClaimWebhook()
		if err != nil {
			t.Fatalf("Failed to claim webhook: %v", err)
		}

		if item == nil {
			t.Fatal("Expected webhook item to be claimed")
		}

		// Simulate failure and release
		released, err := db.ReleaseWebhook(item.ID, item.RetryCount, "rate limit exceeded")
		if err != nil {
			t.Fatalf("Failed to release webhook: %v", err)
		}
		if !released {
			t.Error("Expected webhook to be released (not dropped)")
		}

		// Should be in queue but not ready yet
		length, err := db.GetQueueLength()
		if err != nil {
			t.Fatalf("Failed to get queue length: %v", err)
		}

		if length != 1 {
			t.Errorf("Expected queue length 1, got %d", length)
		}

		readyLength, err := db.GetReadyQueueLength()
		if err != nil {
			t.Fatalf("Failed to get ready queue length: %v", err)
		}

		if readyLength != 0 {
			t.Errorf("Expected ready queue length 0 (waiting for retry), got %d", readyLength)
		}

		// Try to claim - should return nil (not ready yet)
		item, err = db.ClaimWebhook()
		if err != nil {
			t.Fatalf("Failed to claim webhook: %v", err)
		}

		if item != nil {
			t.Error("Expected no item to be claimed (waiting for retry)")
		}

		// Clean up: remove all webhooks from queue
		_, err = db.db.Exec("DELETE FROM webhook_queue")
		if err != nil {
			t.Fatalf("Failed to clean up webhook queue: %v", err)
		}
	})

	// Test concurrent webhook claims (race condition prevention)
	t.Run("WebhookConcurrentClaim", func(t *testing.T) {
		// Enqueue a single webhook
		webhookData := json.RawMessage(`{"object_type": "activity", "object_id": 789}`)
		_, err := db.EnqueueWebhook(webhookData)
		if err != nil {
			t.Fatalf("Failed to enqueue webhook: %v", err)
		}

		// Try to claim it from 10 goroutines simultaneously
		const workers = 10
		claims := make(chan *WebhookQueueItem, workers)
		errors := make(chan error, workers)

		for i := 0; i < workers; i++ {
			go func() {
				item, err := db.ClaimWebhook()
				if err != nil {
					errors <- err
					return
				}
				claims <- item
			}()
		}

		// Collect results
		var claimed []*WebhookQueueItem
		for i := 0; i < workers; i++ {
			select {
			case item := <-claims:
				if item != nil {
					claimed = append(claimed, item)
				}
			case err := <-errors:
				t.Fatalf("Unexpected error claiming webhook: %v", err)
			}
		}

		// Only ONE goroutine should have successfully claimed it
		if len(claimed) != 1 {
			t.Errorf("Expected exactly 1 claim, got %d", len(claimed))
		}

		// Clean up
		if len(claimed) > 0 {
			db.DeleteWebhook(claimed[0].ID)
		}
	})

	// Test max retry limit
	t.Run("WebhookMaxRetries", func(t *testing.T) {
		webhookData := json.RawMessage(`{"object_type": "activity", "object_id": 999}`)

		queueID, err := db.EnqueueWebhook(webhookData)
		if err != nil {
			t.Fatalf("Failed to enqueue webhook: %v", err)
		}

		// Simulate MaxRetries failures
		for i := 0; i < MaxRetries; i++ {
			// Make it immediately claimable for testing
			_, err := db.db.Exec("UPDATE webhook_queue SET next_retry_at = NULL WHERE id = ?", queueID)
			if err != nil {
				t.Fatalf("Failed to reset retry time: %v", err)
			}

			item, err := db.ClaimWebhook()
			if err != nil {
				t.Fatalf("Failed to claim webhook: %v", err)
			}
			if item == nil {
				t.Fatalf("Expected webhook to be claimed on attempt %d", i+1)
			}

			released, err := db.ReleaseWebhook(item.ID, item.RetryCount, "persistent error")
			if err != nil {
				t.Fatalf("Failed to release webhook: %v", err)
			}
			if !released {
				t.Errorf("Expected webhook to be released on attempt %d", i+1)
			}
		}

		// Make it immediately claimable one more time
		_, err = db.db.Exec("UPDATE webhook_queue SET next_retry_at = NULL WHERE id = ?", queueID)
		if err != nil {
			t.Fatalf("Failed to reset retry time: %v", err)
		}

		// The MaxRetries+1 attempt should drop it
		item, err := db.ClaimWebhook()
		if err != nil {
			t.Fatalf("Failed to claim webhook: %v", err)
		}
		if item == nil {
			t.Fatal("Expected webhook to be claimed for final attempt")
		}

		if item.RetryCount != MaxRetries {
			t.Errorf("Expected retry count to be %d, got %d", MaxRetries, item.RetryCount)
		}

		released, err := db.ReleaseWebhook(item.ID, item.RetryCount, "final error")
		if err != nil {
			t.Fatalf("Failed to release webhook on final attempt: %v", err)
		}
		if released {
			t.Error("Expected webhook to be dropped after max retries, but it was released")
		}

		// Queue should be empty now
		length, err := db.GetQueueLength()
		if err != nil {
			t.Fatalf("Failed to get queue length: %v", err)
		}
		if length != 0 {
			t.Errorf("Expected queue to be empty after max retries, got length %d", length)
		}
	})

	// Test event operations
	t.Run("Events", func(t *testing.T) {
		athleteSummary := json.RawMessage(`{"id": 12345, "username": "testuser"}`)

		eventID, err := db.InsertAthleteConnectedEvent(12345, athleteSummary)
		if err != nil {
			t.Fatalf("Failed to insert athlete_connected event: %v", err)
		}

		if eventID == 0 {
			t.Fatal("Expected non-zero event_id")
		}

		events, err := db.GetEvents(0, 10)
		if err != nil {
			t.Fatalf("Failed to get events: %v", err)
		}

		if len(events) != 1 {
			t.Errorf("Expected 1 event, got %d", len(events))
		}

		if events[0].EventType != EventTypeAthleteConnected {
			t.Errorf("Expected event type %s, got %s", EventTypeAthleteConnected, events[0].EventType)
		}

		// Test activity event insertion
		activityID := int64(99999)
		activity := json.RawMessage(`{"id": 99999, "name": "Morning Run"}`)

		webhookData := json.RawMessage(`{"aspect_type":"create","object_type":"activity","object_id":99999,"owner_id":12345}`)
		activityEventID, err := db.InsertActivityEvent(12345, &activityID, activity, webhookData)
		if err != nil {
			t.Fatalf("Failed to insert activity event: %v", err)
		}

		events, err = db.GetEvents(eventID, 10)
		if err != nil {
			t.Fatalf("Failed to get events: %v", err)
		}

		if len(events) != 1 {
			t.Errorf("Expected 1 new event, got %d", len(events))
		}

		if events[0].EventType != EventType("webhook") {
			t.Errorf("Expected event type 'webhook', got %s", events[0].EventType)
		}

		// Test deleting athlete events (except deauth event)
		err = db.DeleteAthleteEvents(12345, activityEventID)
		if err != nil {
			t.Fatalf("Failed to delete athlete events: %v", err)
		}

		// Should only have 1 event left (the webhook event we excluded)
		allEvents, err := db.GetEvents(0, 100)
		if err != nil {
			t.Fatalf("Failed to get all events: %v", err)
		}

		if len(allEvents) != 1 {
			t.Errorf("Expected 1 event remaining, got %d", len(allEvents))
		}
	})
}
