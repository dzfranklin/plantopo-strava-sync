package database

import (
	"encoding/json"
	"fmt"
)

// WebhookQueueItem represents a webhook awaiting hydration
type WebhookQueueItem struct {
	ID   int64
	Data json.RawMessage
}

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

// DequeueWebhook retrieves and deletes the next webhook from the queue
// Returns nil if queue is empty
func (d *DB) DequeueWebhook() (*WebhookQueueItem, error) {
	tx, err := d.db.Begin()
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Get the oldest webhook
	query := `SELECT id, data FROM webhook_queue ORDER BY id ASC LIMIT 1`
	var item WebhookQueueItem

	err = tx.QueryRow(query).Scan(&item.ID, &item.Data)
	if err != nil {
		if err.Error() == "sql: no rows in result set" {
			return nil, nil // Queue is empty
		}
		return nil, fmt.Errorf("failed to query webhook queue: %w", err)
	}

	// Delete the webhook from the queue
	deleteQuery := `DELETE FROM webhook_queue WHERE id = ?`
	_, err = tx.Exec(deleteQuery, item.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to delete webhook from queue: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	return &item, nil
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
