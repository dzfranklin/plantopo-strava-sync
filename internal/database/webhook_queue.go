package database

import (
	"encoding/json"
	"fmt"
	"time"
)

// WebhookQueueItem represents a webhook awaiting hydration
type WebhookQueueItem struct {
	ID                  int64
	Data                json.RawMessage
	RetryCount          int
	LastError           *string
	NextRetryAt         *time.Time
	ProcessingStartedAt *time.Time
}

const (
	// StaleLockTimeout is how long before a processing lock is considered stale
	StaleLockTimeout = 5 * time.Minute
	// MaxRetries is the maximum number of retry attempts before giving up
	MaxRetries = 10
)

// EnqueueWebhook adds a webhook to the processing queue
func (d *DB) EnqueueWebhook(data json.RawMessage) (int64, error) {
	query := `INSERT INTO webhook_queue (data) VALUES (?)`

	result, err := d.db.Exec(query, data)
	if err != nil {
		return 0, fmt.Errorf("failed to enqueue webhook: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("failed to get queue item id: %w", err)
	}

	return id, nil
}

// ClaimWebhook claims the next ready webhook for processing
// Marks it as processing and returns it. Returns nil if no items are ready.
// Items are considered ready if:
// - next_retry_at is NULL or in the past
// - processing_started_at is NULL or stale (older than StaleLockTimeout)
// Uses UPDATE to atomically claim the webhook, preventing race conditions
func (d *DB) ClaimWebhook() (*WebhookQueueItem, error) {
	now := time.Now()
	staleThreshold := now.Add(-StaleLockTimeout).Unix()

	// Atomically claim the oldest ready webhook by updating it first
	// This prevents race conditions between concurrent workers
	updateQuery := `
		UPDATE webhook_queue
		SET processing_started_at = ?
		WHERE id = (
			SELECT id
			FROM webhook_queue
			WHERE (next_retry_at IS NULL OR next_retry_at <= ?)
			  AND (processing_started_at IS NULL OR processing_started_at < ?)
			ORDER BY id ASC
			LIMIT 1
		)
		RETURNING id, data, retry_count, last_error, next_retry_at
	`

	var item WebhookQueueItem
	var lastError *string
	var nextRetryAt *int64

	err := d.db.QueryRow(updateQuery, now.Unix(), now.Unix(), staleThreshold).Scan(
		&item.ID,
		&item.Data,
		&item.RetryCount,
		&lastError,
		&nextRetryAt,
	)
	if err != nil {
		if err.Error() == "sql: no rows in result set" {
			return nil, nil // No items ready
		}
		return nil, fmt.Errorf("failed to claim webhook: %w", err)
	}

	item.LastError = lastError
	if nextRetryAt != nil {
		t := time.Unix(*nextRetryAt, 0)
		item.NextRetryAt = &t
	}
	item.ProcessingStartedAt = &now

	return &item, nil
}

// DeleteWebhook deletes a processed webhook from the queue
func (d *DB) DeleteWebhook(id int64) error {
	query := `DELETE FROM webhook_queue WHERE id = ?`

	_, err := d.db.Exec(query, id)
	if err != nil {
		return fmt.Errorf("failed to complete webhook: %w", err)
	}

	return nil
}

// ReleaseWebhook releases a failed webhook back to the queue with retry tracking
// Uses exponential backoff: 1min, 5min, 15min, 30min, 1hr, etc.
// Returns true if the webhook was released, false if it was dropped due to max retries
func (d *DB) ReleaseWebhook(id int64, retryCount int, errMsg string) (bool, error) {
	newRetryCount := retryCount + 1

	// Drop webhook if it has exceeded max retries
	if newRetryCount > MaxRetries {
		err := d.DeleteWebhook(id)
		if err != nil {
			return false, fmt.Errorf("failed to drop webhook after max retries: %w", err)
		}
		return false, nil // Dropped
	}

	// Calculate exponential backoff
	backoffMinutes := []int{1, 5, 15, 30, 60, 120, 240}
	backoffIdx := newRetryCount - 1
	if backoffIdx >= len(backoffMinutes) {
		backoffIdx = len(backoffMinutes) - 1
	}

	nextRetryAt := time.Now().Add(time.Duration(backoffMinutes[backoffIdx]) * time.Minute)

	query := `
		UPDATE webhook_queue
		SET retry_count = ?,
		    last_error = ?,
		    next_retry_at = ?,
		    processing_started_at = NULL
		WHERE id = ?
	`

	_, err := d.db.Exec(query, newRetryCount, errMsg, nextRetryAt.Unix(), id)
	if err != nil {
		return false, fmt.Errorf("failed to release webhook: %w", err)
	}

	return true, nil // Released for retry
}

// GetQueueLength returns the number of items in the webhook queue
func (d *DB) GetQueueLength() (int, error) {
	query := `SELECT COUNT(*) FROM webhook_queue`
	var count int

	err := d.db.QueryRow(query).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to get queue length: %w", err)
	}

	return count, nil
}

// GetReadyQueueLength returns the number of items ready to process
// Items are considered ready if:
// - next_retry_at is NULL or in the past
// - processing_started_at is NULL or stale (older than StaleLockTimeout)
func (d *DB) GetReadyQueueLength() (int, error) {
	now := time.Now()
	staleThreshold := now.Add(-StaleLockTimeout).Unix()

	query := `
		SELECT COUNT(*)
		FROM webhook_queue
		WHERE (next_retry_at IS NULL OR next_retry_at <= ?)
		  AND (processing_started_at IS NULL OR processing_started_at < ?)
	`
	var count int

	err := d.db.QueryRow(query, now.Unix(), staleThreshold).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to get ready queue length: %w", err)
	}

	return count, nil
}
