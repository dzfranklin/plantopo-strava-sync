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

// Start begins processing both webhooks and sync jobs from their respective queues
func (w *Worker) Start(ctx context.Context) error {
	w.logger.Info("Starting worker (webhooks + sync jobs)")

	for {
		select {
		case <-ctx.Done():
			w.logger.Info("Stopping worker")
			return ctx.Err()
		default:
			// Try to claim a webhook first (prioritize real-time webhooks)
			webhook, err := w.db.ClaimWebhook()
			if err != nil {
				w.logger.Error("Failed to claim webhook", "error", err)
				time.Sleep(w.pollInterval)
				continue
			}

			if webhook != nil {
				// Process the webhook
				w.processWebhook(webhook)
				continue
			}

			// No webhook available, try to claim a sync job
			syncJob, err := w.db.ClaimSyncJob()
			if err != nil {
				w.logger.Error("Failed to claim sync job", "error", err)
				time.Sleep(w.pollInterval)
				continue
			}

			if syncJob != nil {
				// Process the sync job
				w.processSyncJob(syncJob)
				continue
			}

			// Nothing to process, wait before trying again
			time.Sleep(w.pollInterval)
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

// processSyncJob handles a single sync job
func (w *Worker) processSyncJob(job *database.SyncJob) {
	w.logger.Info("Processing sync job",
		"id", job.ID,
		"athlete_id", job.AthleteID,
		"job_type", job.JobType,
		"retry_count", job.RetryCount)

	var err error
	switch job.JobType {
	case "sync_all_activities":
		err = w.syncAllActivities(job.AthleteID)
	default:
		w.logger.Warn("Unknown sync job type", "id", job.ID, "job_type", job.JobType)
		// Unknown types are not retryable - complete them
		if err := w.db.DeleteSyncJob(job.ID); err != nil {
			w.logger.Error("Failed to delete unknown sync job", "id", job.ID, "error", err)
		}
		return
	}

	if err != nil {
		w.logger.Error("Failed to process sync job", "id", job.ID, "error", err)
		w.releaseSyncJob(job.ID, job.RetryCount, err.Error())
		return
	}

	// Success - delete sync job from queue
	if err := w.db.DeleteSyncJob(job.ID); err != nil {
		w.logger.Error("Failed to delete completed sync job", "id", job.ID, "error", err)
	} else {
		w.logger.Info("Sync job processed successfully", "id", job.ID)
	}
}

// syncAllActivities processes a sync_all_activities job (fetches all historical activities)
func (w *Worker) syncAllActivities(athleteID int64) error {
	w.logger.Info("Starting sync_all_activities for athlete", "athlete_id", athleteID)

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

		// Sync each activity
		for _, activityID := range activityIDs {
			if err := w.syncActivity(athleteID, activityID); err != nil {
				// Log but continue with other activities
				w.logger.Error("Failed to sync activity",
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
		return w.processWebhookActivity(athleteID, activityID, aspectType, webhookData)

	case "delete":
		// Insert a delete event (no activity data for deletes)
		eventID, err := w.db.InsertActivityEvent(athleteID, &activityID, nil, webhookData)
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
	eventID, err := w.db.InsertActivityEvent(athleteID, nil, nil, webhookData)
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

// processWebhookActivity fetches activity details from Strava and inserts a webhook event
// This is for real Strava webhook events (create/update) with webhook data
func (w *Worker) processWebhookActivity(athleteID, activityID int64, aspectType string, webhookData json.RawMessage) error {
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

	// Insert event with webhook data
	eventID, err := w.db.InsertActivityEvent(athleteID, &activityID, activityData, webhookData)
	if err != nil {
		return fmt.Errorf("failed to insert activity event: %w", err)
	}

	w.logger.Info("Processed webhook activity",
		"athlete_id", athleteID,
		"activity_id", activityID,
		"aspect_type", aspectType,
		"event_id", eventID)

	return nil
}

// syncActivity fetches activity details from Strava during sync operations
// This does NOT create events - sync operations don't generate event stream entries
func (w *Worker) syncActivity(athleteID, activityID int64) error {
	// Fetch activity details
	activityData, err := w.stravaClient.GetActivity(athleteID, activityID)
	if err != nil {
		// Check for specific error types
		if strava.IsNotFound(err) {
			w.logger.Warn("Activity not found during sync, skipping", "activity_id", activityID)
			return nil // Don't retry 404s
		}
		if strava.IsUnauthorized(err) {
			w.logger.Warn("Athlete unauthorized during sync, skipping", "athlete_id", athleteID)
			return nil // Don't retry unauthorized
		}
		if strava.IsTooManyRequests(err) {
			return fmt.Errorf("rate limited: %w", err) // Retry rate limits
		}
		return fmt.Errorf("failed to get activity: %w", err)
	}

	// TODO: Store activity data in activities table once it's implemented
	// For now, just fetch to validate access and prepare for future storage
	w.logger.Debug("Synced activity",
		"athlete_id", athleteID,
		"activity_id", activityID,
		"activity_data_size", len(activityData))

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

// releaseSyncJob releases a sync job back to the queue with exponential backoff
func (w *Worker) releaseSyncJob(jobID int64, currentRetryCount int, errorMsg string) {
	shouldRetry, err := w.db.ReleaseSyncJob(jobID, currentRetryCount, errorMsg)
	if err != nil {
		w.logger.Error("Failed to release sync job", "id", jobID, "error", err)
		return
	}

	if !shouldRetry {
		w.logger.Warn("Sync job exceeded max retries, dropped",
			"id", jobID,
			"retry_count", currentRetryCount)
	} else {
		w.logger.Info("Sync job released for retry",
			"id", jobID,
			"retry_count", currentRetryCount+1)
	}
}
