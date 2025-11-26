package database

import (
	"fmt"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"plantopo-strava-sync/internal/metrics"
)

// SyncJob represents a sync job awaiting processing
type SyncJob struct {
	ID                  int64
	AthleteID           int64
	JobType             string
	RetryCount          int
	LastError           *string
	NextRetryAt         *time.Time
	ProcessingStartedAt *time.Time
	CreatedAt           time.Time
}

// EnqueueSyncJob adds a sync job to the processing queue
func (d *DB) EnqueueSyncJob(athleteID int64, jobType string) (int64, error) {
	timer := prometheus.NewTimer(metrics.DBOperationDuration.WithLabelValues(metrics.DBOpEnqueueSyncJob))
	defer timer.ObserveDuration()

	query := `INSERT INTO sync_jobs (athlete_id, job_type) VALUES (?, ?)`

	result, err := d.db.Exec(query, athleteID, jobType)
	if err != nil {
		metrics.DBOperationErrorsTotal.WithLabelValues(metrics.DBOpEnqueueSyncJob).Inc()
		return 0, fmt.Errorf("failed to enqueue sync job: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		metrics.DBOperationErrorsTotal.WithLabelValues(metrics.DBOpEnqueueSyncJob).Inc()
		return 0, fmt.Errorf("failed to get sync job id: %w", err)
	}

	// Record successful enqueue
	metrics.QueueEnqueueTotal.WithLabelValues(metrics.QueueTypeSyncJob).Inc()

	return id, nil
}

// ClaimSyncJob claims the next ready sync job for processing
// Marks it as processing and returns it. Returns nil if no items are ready.
// Items are considered ready if:
// - next_retry_at is NULL or in the past
// - processing_started_at is NULL or stale (older than StaleLockTimeout)
// Uses UPDATE to atomically claim the job, preventing race conditions
func (d *DB) ClaimSyncJob() (*SyncJob, error) {
	timer := prometheus.NewTimer(metrics.DBOperationDuration.WithLabelValues(metrics.DBOpClaimSyncJob))
	defer timer.ObserveDuration()

	now := time.Now()
	staleThreshold := now.Add(-StaleLockTimeout).Unix()

	// Atomically claim the oldest ready sync job by updating it first
	// This prevents race conditions between concurrent workers
	updateQuery := `
		UPDATE sync_jobs
		SET processing_started_at = ?
		WHERE id = (
			SELECT id
			FROM sync_jobs
			WHERE (next_retry_at IS NULL OR next_retry_at <= ?)
			  AND (processing_started_at IS NULL OR processing_started_at < ?)
			ORDER BY id ASC
			LIMIT 1
		)
		RETURNING id, athlete_id, job_type, retry_count, last_error, next_retry_at, created_at
	`

	var job SyncJob
	var lastError *string
	var nextRetryAt *int64
	var createdAt int64

	err := d.db.QueryRow(updateQuery, now.Unix(), now.Unix(), staleThreshold).Scan(
		&job.ID,
		&job.AthleteID,
		&job.JobType,
		&job.RetryCount,
		&lastError,
		&nextRetryAt,
		&createdAt,
	)
	if err != nil {
		if err.Error() == "sql: no rows in result set" {
			return nil, nil // No items ready
		}
		metrics.DBOperationErrorsTotal.WithLabelValues(metrics.DBOpClaimSyncJob).Inc()
		return nil, fmt.Errorf("failed to claim sync job: %w", err)
	}

	job.LastError = lastError
	if nextRetryAt != nil {
		t := time.Unix(*nextRetryAt, 0)
		job.NextRetryAt = &t
	}
	job.ProcessingStartedAt = &now
	job.CreatedAt = time.Unix(createdAt, 0)

	return &job, nil
}

// DeleteSyncJob deletes a processed sync job from the queue
func (d *DB) DeleteSyncJob(id int64) error {
	timer := prometheus.NewTimer(metrics.DBOperationDuration.WithLabelValues(metrics.DBOpDeleteSyncJob))
	defer timer.ObserveDuration()

	query := `DELETE FROM sync_jobs WHERE id = ?`

	_, err := d.db.Exec(query, id)
	if err != nil {
		metrics.DBOperationErrorsTotal.WithLabelValues(metrics.DBOpDeleteSyncJob).Inc()
		return fmt.Errorf("failed to delete sync job: %w", err)
	}

	return nil
}

// ReleaseSyncJob releases a failed sync job back to the queue with retry tracking
// Uses exponential backoff: 1min, 5min, 15min, 30min, 1hr, etc.
// Returns true if the job was released, false if it was dropped due to max retries
func (d *DB) ReleaseSyncJob(id int64, retryCount int, errMsg string) (bool, error) {
	timer := prometheus.NewTimer(metrics.DBOperationDuration.WithLabelValues(metrics.DBOpReleaseSyncJob))
	defer timer.ObserveDuration()

	newRetryCount := retryCount + 1

	// Drop job if it has exceeded max retries
	if newRetryCount > MaxRetries {
		err := d.DeleteSyncJob(id)
		if err != nil {
			return false, fmt.Errorf("failed to drop sync job after max retries: %w", err)
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
		UPDATE sync_jobs
		SET retry_count = ?,
		    last_error = ?,
		    next_retry_at = ?,
		    processing_started_at = NULL
		WHERE id = ?
	`

	_, err := d.db.Exec(query, newRetryCount, errMsg, nextRetryAt.Unix(), id)
	if err != nil {
		metrics.DBOperationErrorsTotal.WithLabelValues(metrics.DBOpReleaseSyncJob).Inc()
		return false, fmt.Errorf("failed to release sync job: %w", err)
	}

	return true, nil // Released for retry
}

// GetSyncJobQueueLength returns the number of sync jobs in the queue
func (d *DB) GetSyncJobQueueLength() (int, error) {
	query := `SELECT COUNT(*) FROM sync_jobs`
	var count int

	err := d.db.QueryRow(query).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to get sync job queue length: %w", err)
	}

	return count, nil
}

// GetReadySyncJobQueueLength returns the number of sync jobs ready to process
// Items are considered ready if:
// - next_retry_at is NULL or in the past
// - processing_started_at is NULL or stale (older than StaleLockTimeout)
func (d *DB) GetReadySyncJobQueueLength() (int, error) {
	now := time.Now()
	staleThreshold := now.Add(-StaleLockTimeout).Unix()

	query := `
		SELECT COUNT(*)
		FROM sync_jobs
		WHERE (next_retry_at IS NULL OR next_retry_at <= ?)
		  AND (processing_started_at IS NULL OR processing_started_at < ?)
	`
	var count int

	err := d.db.QueryRow(query, now.Unix(), staleThreshold).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to get ready sync job queue length: %w", err)
	}

	return count, nil
}

// GetProcessingSyncJobQueueLength returns the number of sync jobs currently being processed
// Items are considered processing if they have a recent processing_started_at timestamp
func (d *DB) GetProcessingSyncJobQueueLength() (int, error) {
	now := time.Now()
	staleThreshold := now.Add(-StaleLockTimeout).Unix()

	query := `
		SELECT COUNT(*)
		FROM sync_jobs
		WHERE processing_started_at IS NOT NULL
		  AND processing_started_at >= ?
	`
	var count int

	err := d.db.QueryRow(query, staleThreshold).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to get processing sync job queue length: %w", err)
	}

	return count, nil
}