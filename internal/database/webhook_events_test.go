package database

import (
	"testing"
	"time"
)

func TestCreateAndGetWebhookEvent(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	if err := db.Init(); err != nil {
		t.Fatalf("Failed to init db: %v", err)
	}

	event := &WebhookEvent{
		ObjectType:     "activity",
		ObjectID:       98765,
		AspectType:     "create",
		OwnerID:        12345,
		SubscriptionID: 1,
		EventTime:      time.Now().Unix(),
		RawJSON:        `{"object_type":"activity","object_id":98765}`,
		Processed:      false,
	}

	// Create webhook event
	if err := db.CreateWebhookEvent(event); err != nil {
		t.Fatalf("Failed to create webhook event: %v", err)
	}

	if event.ID == 0 {
		t.Error("Expected ID to be set after creation")
	}

	// Get webhook event
	retrieved, err := db.GetWebhookEvent(event.ID)
	if err != nil {
		t.Fatalf("Failed to get webhook event: %v", err)
	}
	if retrieved == nil {
		t.Fatal("Expected webhook event, got nil")
	}

	if retrieved.ObjectType != "activity" {
		t.Errorf("Expected object_type 'activity', got %s", retrieved.ObjectType)
	}
	if retrieved.ObjectID != 98765 {
		t.Errorf("Expected object_id 98765, got %d", retrieved.ObjectID)
	}
	if retrieved.AspectType != "create" {
		t.Errorf("Expected aspect_type 'create', got %s", retrieved.AspectType)
	}
	if retrieved.Processed {
		t.Error("Expected processed false")
	}
}

func TestMarkWebhookEventProcessed(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	if err := db.Init(); err != nil {
		t.Fatalf("Failed to init db: %v", err)
	}

	event := &WebhookEvent{
		ObjectType:     "activity",
		ObjectID:       98765,
		AspectType:     "create",
		OwnerID:        12345,
		SubscriptionID: 1,
		EventTime:      time.Now().Unix(),
		RawJSON:        `{"object_type":"activity"}`,
		Processed:      false,
	}

	if err := db.CreateWebhookEvent(event); err != nil {
		t.Fatalf("Failed to create webhook event: %v", err)
	}

	// Mark as processed
	if err := db.MarkWebhookEventProcessed(event.ID, nil); err != nil {
		t.Fatalf("Failed to mark webhook event processed: %v", err)
	}

	// Verify
	retrieved, err := db.GetWebhookEvent(event.ID)
	if err != nil {
		t.Fatalf("Failed to get webhook event: %v", err)
	}
	if !retrieved.Processed {
		t.Error("Expected processed true")
	}
	if retrieved.ProcessedAt == nil {
		t.Error("Expected processed_at to be set")
	}
}

func TestMarkWebhookEventProcessedWithError(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	if err := db.Init(); err != nil {
		t.Fatalf("Failed to init db: %v", err)
	}

	event := &WebhookEvent{
		ObjectType:     "activity",
		ObjectID:       98765,
		AspectType:     "create",
		OwnerID:        12345,
		SubscriptionID: 1,
		EventTime:      time.Now().Unix(),
		RawJSON:        `{"object_type":"activity"}`,
		Processed:      false,
	}

	if err := db.CreateWebhookEvent(event); err != nil {
		t.Fatalf("Failed to create webhook event: %v", err)
	}

	// Mark as processed with error
	errorMsg := "test error"
	if err := db.MarkWebhookEventProcessed(event.ID, &errorMsg); err != nil {
		t.Fatalf("Failed to mark webhook event processed: %v", err)
	}

	// Verify
	retrieved, err := db.GetWebhookEvent(event.ID)
	if err != nil {
		t.Fatalf("Failed to get webhook event: %v", err)
	}
	if !retrieved.Processed {
		t.Error("Expected processed true")
	}
	if retrieved.Error == nil || *retrieved.Error != "test error" {
		t.Errorf("Expected error 'test error', got %v", retrieved.Error)
	}
}

func TestListUnprocessedWebhookEvents(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	if err := db.Init(); err != nil {
		t.Fatalf("Failed to init db: %v", err)
	}

	now := time.Now().Unix()

	// Create multiple webhook events
	for i := 1; i <= 3; i++ {
		event := &WebhookEvent{
			ObjectType:     "activity",
			ObjectID:       int64(i),
			AspectType:     "create",
			OwnerID:        12345,
			SubscriptionID: 1,
			EventTime:      now + int64(i),
			RawJSON:        `{"test":true}`,
			Processed:      false,
		}
		if err := db.CreateWebhookEvent(event); err != nil {
			t.Fatalf("Failed to create webhook event %d: %v", i, err)
		}
	}

	// Mark one as processed
	if err := db.MarkWebhookEventProcessed(2, nil); err != nil {
		t.Fatalf("Failed to mark webhook event processed: %v", err)
	}

	// List unprocessed events
	unprocessed, err := db.ListUnprocessedWebhookEvents(0, 0)
	if err != nil {
		t.Fatalf("Failed to list unprocessed webhook events: %v", err)
	}
	if len(unprocessed) != 2 {
		t.Errorf("Expected 2 unprocessed events, got %d", len(unprocessed))
	}

	// Test pagination
	limited, err := db.ListUnprocessedWebhookEvents(0, 1)
	if err != nil {
		t.Fatalf("Failed to list limited unprocessed events: %v", err)
	}
	if len(limited) != 1 {
		t.Errorf("Expected 1 event, got %d", len(limited))
	}
}

func TestListWebhookEventsByAthlete(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	if err := db.Init(); err != nil {
		t.Fatalf("Failed to init db: %v", err)
	}

	now := time.Now().Unix()

	// Create events for athlete 12345
	for i := 1; i <= 3; i++ {
		event := &WebhookEvent{
			ObjectType:     "activity",
			ObjectID:       int64(i),
			AspectType:     "create",
			OwnerID:        12345,
			SubscriptionID: 1,
			EventTime:      now + int64(i),
			RawJSON:        `{"test":true}`,
			Processed:      false,
		}
		if err := db.CreateWebhookEvent(event); err != nil {
			t.Fatalf("Failed to create webhook event: %v", err)
		}
	}

	// Create event for different athlete
	event := &WebhookEvent{
		ObjectType:     "activity",
		ObjectID:       99,
		AspectType:     "create",
		OwnerID:        54321,
		SubscriptionID: 1,
		EventTime:      now,
		RawJSON:        `{"test":true}`,
		Processed:      false,
	}
	if err := db.CreateWebhookEvent(event); err != nil {
		t.Fatalf("Failed to create webhook event: %v", err)
	}

	// List events for athlete 12345
	events, err := db.ListWebhookEventsByAthlete(12345, 0, 0)
	if err != nil {
		t.Fatalf("Failed to list webhook events by athlete: %v", err)
	}
	if len(events) != 3 {
		t.Errorf("Expected 3 events for athlete 12345, got %d", len(events))
	}

	// Test pagination
	limited, err := db.ListWebhookEventsByAthlete(12345, 0, 2)
	if err != nil {
		t.Fatalf("Failed to list limited events: %v", err)
	}
	if len(limited) != 2 {
		t.Errorf("Expected 2 events, got %d", len(limited))
	}

	// Verify events are for correct athlete
	for _, e := range events {
		if e.OwnerID != 12345 {
			t.Errorf("Expected owner_id 12345, got %d", e.OwnerID)
		}
	}
}

func TestWebhookEventUniqueConstraint(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	if err := db.Init(); err != nil {
		t.Fatalf("Failed to init db: %v", err)
	}

	now := time.Now().Unix()

	event1 := &WebhookEvent{
		ObjectType:     "activity",
		ObjectID:       98765,
		AspectType:     "create",
		OwnerID:        12345,
		SubscriptionID: 1,
		EventTime:      now,
		RawJSON:        `{"test":1}`,
		Processed:      false,
	}

	if err := db.CreateWebhookEvent(event1); err != nil {
		t.Fatalf("Failed to create first webhook event: %v", err)
	}

	// Try to create duplicate (same event_time, object_id, aspect_type)
	event2 := &WebhookEvent{
		ObjectType:     "activity",
		ObjectID:       98765,
		AspectType:     "create",
		OwnerID:        12345,
		SubscriptionID: 1,
		EventTime:      now,
		RawJSON:        `{"test":2}`,
		Processed:      false,
	}

	err := db.CreateWebhookEvent(event2)
	if err == nil {
		t.Error("Expected unique constraint violation, but create succeeded")
	}
}
