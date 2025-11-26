package metrics

import (
	"context"
	"log/slog"
	"time"
)

// DB interface for queue depth queries
type DB interface {
	GetQueueLength() (int, error)
	GetReadyQueueLength() (int, error)
	GetProcessingWebhookQueueLength() (int, error)
	GetSyncJobQueueLength() (int, error)
	GetReadySyncJobQueueLength() (int, error)
	GetProcessingSyncJobQueueLength() (int, error)
}

// StartQueueDepthCollector starts a background goroutine that periodically
// collects queue depth metrics from the database
func StartQueueDepthCollector(ctx context.Context, db DB, interval time.Duration) {
	logger := slog.Default()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// Collect once immediately
	collectQueueDepths(db, logger)

	for {
		select {
		case <-ctx.Done():
			logger.Info("Queue depth collector stopping")
			return
		case <-ticker.C:
			collectQueueDepths(db, logger)
		}
	}
}

func collectQueueDepths(db DB, logger *slog.Logger) {
	// Webhook queue metrics
	if total, err := db.GetQueueLength(); err != nil {
		logger.Error("Failed to get webhook queue length", "error", err)
	} else {
		QueueDepthTotal.WithLabelValues(QueueTypeWebhook).Set(float64(total))
	}

	if ready, err := db.GetReadyQueueLength(); err != nil {
		logger.Error("Failed to get ready webhook queue length", "error", err)
	} else {
		QueueDepthReady.WithLabelValues(QueueTypeWebhook).Set(float64(ready))
	}

	if processing, err := db.GetProcessingWebhookQueueLength(); err != nil {
		logger.Error("Failed to get processing webhook queue length", "error", err)
	} else {
		QueueDepthProcessing.WithLabelValues(QueueTypeWebhook).Set(float64(processing))
	}

	// Sync job queue metrics
	if total, err := db.GetSyncJobQueueLength(); err != nil {
		logger.Error("Failed to get sync job queue length", "error", err)
	} else {
		QueueDepthTotal.WithLabelValues(QueueTypeSyncJob).Set(float64(total))
	}

	if ready, err := db.GetReadySyncJobQueueLength(); err != nil {
		logger.Error("Failed to get ready sync job queue length", "error", err)
	} else {
		QueueDepthReady.WithLabelValues(QueueTypeSyncJob).Set(float64(ready))
	}

	if processing, err := db.GetProcessingSyncJobQueueLength(); err != nil {
		logger.Error("Failed to get processing sync job queue length", "error", err)
	} else {
		QueueDepthProcessing.WithLabelValues(QueueTypeSyncJob).Set(float64(processing))
	}
}
