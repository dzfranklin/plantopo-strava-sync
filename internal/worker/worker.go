package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"plantopo-strava-sync/internal/database"
	"plantopo-strava-sync/internal/strava"
)

// Worker processes webhooks from the queue
type Worker struct {
	db           *database.DB
	stravaClient *strava.Client
	logger       *slog.Logger
	pollInterval time.Duration
}

// NewWorker creates a new webhook worker
func NewWorker(db *database.DB, stravaClient *strava.Client) *Worker {
	return &Worker{
		db:           db,
		stravaClient: stravaClient,
		logger:       slog.Default(),
		pollInterval: 500 * time.Millisecond,
	}
}

// Start begins processing webhooks from the queue
func (w *Worker) Start(ctx context.Context) error {
	w.logger.Info("Starting webhook worker")

	for {
		select {
		case <-ctx.Done():
			w.logger.Info("Stopping webhook worker")
			return ctx.Err()
		default:
			// Try to claim a webhook
			item, err := w.db.ClaimWebhook()
			if err != nil {
				w.logger.Error("Failed to claim webhook", "error", err)
				time.Sleep(w.pollInterval)
				continue
			}

			// No webhook available, wait before trying again
			if item == nil {
				time.Sleep(w.pollInterval)
				continue
			}

			// Process the webhook
			w.processWebhook(item)
		}
	}
}

// processWebhook handles a single webhook item
func (w *Worker) processWebhook(item *database.WebhookQueueItem) {
	w.logger.Info("Processing webhook", "id", item.ID, "retry_count", item.RetryCount)

	var webhook map[string]interface{}
	if err := json.Unmarshal(item.Data, &webhook); err != nil {
		w.logger.Error("Failed to unmarshal webhook", "id", item.ID, "error", err)
		w.releaseWebhook(item.ID, item.RetryCount, fmt.Sprintf("invalid JSON: %v", err))
		return
	}

	objectType, _ := webhook["object_type"].(string)

	var err error
	switch objectType {
	case "sync_all":
		err = w.handleSyncAll(webhook)
	case "activity":
		err = w.handleActivity(webhook)
	case "athlete":
		err = w.handleAthlete(webhook)
	default:
		w.logger.Warn("Unknown webhook object_type", "id", item.ID, "object_type", objectType)
		// Unknown types are not retryable - complete them
		if err := w.db.DeleteWebhook(item.ID); err != nil {
			w.logger.Error("Failed to delete unknown webhook", "id", item.ID, "error", err)
		}
		return
	}

	if err != nil {
		w.logger.Error("Failed to process webhook", "id", item.ID, "error", err)
		w.releaseWebhook(item.ID, item.RetryCount, err.Error())
		return
	}

	// Success - delete webhook from queue
	if err := w.db.DeleteWebhook(item.ID); err != nil {
		w.logger.Error("Failed to delete completed webhook", "id", item.ID, "error", err)
	} else {
		w.logger.Info("Webhook processed successfully", "id", item.ID)
	}
}

// handleSyncAll processes a sync_all webhook (fetches all historical activities)
func (w *Worker) handleSyncAll(webhook map[string]interface{}) error {
	ownerID, ok := webhook["owner_id"].(float64)
	if !ok {
		return fmt.Errorf("invalid owner_id in sync_all webhook")
	}
	athleteID := int64(ownerID)

	w.logger.Info("Starting sync_all for athlete", "athlete_id", athleteID)

	page := 1
	perPage := 200
	totalActivities := 0

	for {
		activityIDs, hasMore, err := w.stravaClient.ListActivities(athleteID, page, perPage)
		if err != nil {
			// Check if it's a rate limit error
			if strava.IsTooManyRequests(err) {
				return fmt.Errorf("rate limited during sync_all: %w", err)
			}
			// Check if it's an auth error
			if strava.IsUnauthorized(err) {
				w.logger.Warn("Athlete unauthorized during sync, skipping", "athlete_id", athleteID)
				return nil // Don't retry unauthorized athletes
			}
			return fmt.Errorf("failed to list activities (page %d): %w", page, err)
		}

		// Hydrate each activity
		for _, activityID := range activityIDs {
			if err := w.hydrateActivity(athleteID, activityID, "sync", nil); err != nil {
				// Log but continue with other activities
				w.logger.Error("Failed to hydrate activity during sync",
					"athlete_id", athleteID,
					"activity_id", activityID,
					"error", err)
				// Don't fail the entire sync for one activity
			}
		}

		totalActivities += len(activityIDs)
		w.logger.Info("Synced activities page",
			"athlete_id", athleteID,
			"page", page,
			"count", len(activityIDs),
			"total", totalActivities)

		if !hasMore {
			break
		}

		page++

		// Small delay between pages to be respectful of rate limits
		time.Sleep(100 * time.Millisecond)
	}

	w.logger.Info("Completed sync_all for athlete",
		"athlete_id", athleteID,
		"total_activities", totalActivities)

	return nil
}

// handleActivity processes an activity webhook (create, update, delete)
func (w *Worker) handleActivity(webhook map[string]interface{}) error {
	ownerID, ok := webhook["owner_id"].(float64)
	if !ok {
		return fmt.Errorf("invalid owner_id in activity webhook")
	}
	athleteID := int64(ownerID)

	objectID, ok := webhook["object_id"].(float64)
	if !ok {
		return fmt.Errorf("invalid object_id in activity webhook")
	}
	activityID := int64(objectID)

	aspectType, _ := webhook["aspect_type"].(string)

	// Marshal webhook back to JSON for storage
	webhookData, err := json.Marshal(webhook)
	if err != nil {
		return fmt.Errorf("failed to marshal webhook data: %w", err)
	}

	w.logger.Info("Processing activity webhook",
		"athlete_id", athleteID,
		"activity_id", activityID,
		"aspect_type", aspectType)

	switch aspectType {
	case "create", "update":
		return w.hydrateActivity(athleteID, activityID, aspectType, webhookData)

	case "delete":
		// Insert a delete event (no activity data for deletes)
		eventID, err := w.db.InsertActivityEvent(athleteID, &activityID, "delete", nil, webhookData)
		if err != nil {
			return fmt.Errorf("failed to insert delete event: %w", err)
		}
		w.logger.Info("Inserted activity delete event",
			"athlete_id", athleteID,
			"activity_id", activityID,
			"event_id", eventID)
		return nil

	default:
		w.logger.Warn("Unknown aspect_type, skipping",
			"aspect_type", aspectType,
			"activity_id", activityID)
		return nil // Don't retry unknown aspect types
	}
}

// handleAthlete processes an athlete webhook (deauthorization)
func (w *Worker) handleAthlete(webhook map[string]interface{}) error {
	ownerID, ok := webhook["owner_id"].(float64)
	if !ok {
		return fmt.Errorf("invalid owner_id in athlete webhook")
	}
	athleteID := int64(ownerID)

	// Check aspect_type - we only care about "update" for deauthorization
	aspectType, _ := webhook["aspect_type"].(string)
	if aspectType != "update" {
		w.logger.Info("Ignoring athlete webhook with non-update aspect",
			"athlete_id", athleteID,
			"aspect_type", aspectType)
		return nil
	}

	// Check if this is a deauthorization (updates.authorized == "false")
	updates, ok := webhook["updates"].(map[string]interface{})
	if !ok {
		return fmt.Errorf("invalid updates in athlete webhook")
	}

	authorized, ok := updates["authorized"].(string)
	if !ok || authorized != "false" {
		w.logger.Info("Ignoring athlete update that is not deauthorization",
			"athlete_id", athleteID,
			"authorized", authorized)
		return nil
	}

	w.logger.Info("Processing athlete deauthorization",
		"athlete_id", athleteID)

	// Marshal webhook back to JSON for storage
	webhookData, err := json.Marshal(webhook)
	if err != nil {
		return fmt.Errorf("failed to marshal webhook data: %w", err)
	}

	// Insert deauthorization event
	eventID, err := w.db.InsertActivityEvent(athleteID, nil, "deauthorize", nil, webhookData)
	if err != nil {
		return fmt.Errorf("failed to insert deauthorization event: %w", err)
	}

	w.logger.Info("Inserted deauthorization event",
		"athlete_id", athleteID,
		"event_id", eventID)

	// Delete all athlete's events except the deauthorization event
	if err := w.db.DeleteAthleteEvents(athleteID, eventID); err != nil {
		return fmt.Errorf("failed to delete athlete events: %w", err)
	}

	w.logger.Info("Deleted athlete events",
		"athlete_id", athleteID,
		"except_event_id", eventID)

	return nil
}

// hydrateActivity fetches activity details from Strava and inserts an event
// webhookData: optional webhook event data (nil for sync events)
func (w *Worker) hydrateActivity(athleteID, activityID int64, aspectType string, webhookData json.RawMessage) error {
	// Fetch activity details
	activityData, err := w.stravaClient.GetActivity(athleteID, activityID)
	if err != nil {
		// Check for specific error types
		if strava.IsNotFound(err) {
			w.logger.Warn("Activity not found, skipping", "activity_id", activityID)
			return nil // Don't retry 404s
		}
		if strava.IsUnauthorized(err) {
			w.logger.Warn("Athlete unauthorized, skipping", "athlete_id", athleteID)
			return nil // Don't retry unauthorized
		}
		if strava.IsTooManyRequests(err) {
			return fmt.Errorf("rate limited: %w", err) // Retry rate limits
		}
		return fmt.Errorf("failed to get activity: %w", err)
	}

	// Insert event
	eventID, err := w.db.InsertActivityEvent(athleteID, &activityID, aspectType, activityData, webhookData)
	if err != nil {
		return fmt.Errorf("failed to insert activity event: %w", err)
	}

	w.logger.Info("Hydrated activity",
		"athlete_id", athleteID,
		"activity_id", activityID,
		"aspect_type", aspectType,
		"event_id", eventID)

	return nil
}

// releaseWebhook releases a webhook back to the queue with exponential backoff
func (w *Worker) releaseWebhook(webhookID int64, currentRetryCount int, errorMsg string) {
	shouldRetry, err := w.db.ReleaseWebhook(webhookID, currentRetryCount, errorMsg)
	if err != nil {
		w.logger.Error("Failed to release webhook", "id", webhookID, "error", err)
		return
	}

	if !shouldRetry {
		w.logger.Warn("Webhook exceeded max retries, dropped",
			"id", webhookID,
			"retry_count", currentRetryCount)
	} else {
		w.logger.Info("Webhook released for retry",
			"id", webhookID,
			"retry_count", currentRetryCount+1)
	}
}
