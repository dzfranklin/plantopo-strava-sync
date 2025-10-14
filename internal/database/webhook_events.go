package database

import (
	"database/sql"
	"fmt"
	"time"
)

// WebhookEvent represents a Strava webhook event
type WebhookEvent struct {
	ID             int64
	ObjectType     string
	ObjectID       int64
	AspectType     string
	OwnerID        int64
	SubscriptionID int64
	EventTime      int64
	Updates        *string
	RawJSON        string
	Processed      bool
	ProcessedAt    *int64
	Error          *string
	CreatedAt      int64
}

// CreateWebhookEvent inserts a new webhook event into the database
func (db *DB) CreateWebhookEvent(e *WebhookEvent) error {
	e.CreatedAt = time.Now().Unix()

	result, err := db.conn.Exec(`
		INSERT INTO webhook_events (
			object_type, object_id, aspect_type, owner_id, subscription_id,
			event_time, updates, raw_json, processed, processed_at, error, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, e.ObjectType, e.ObjectID, e.AspectType, e.OwnerID, e.SubscriptionID,
		e.EventTime, e.Updates, e.RawJSON, e.Processed, e.ProcessedAt, e.Error, e.CreatedAt)

	if err != nil {
		return fmt.Errorf("failed to create webhook event: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("failed to get last insert id: %w", err)
	}
	e.ID = id

	return nil
}

// GetWebhookEvent retrieves a webhook event by ID
func (db *DB) GetWebhookEvent(eventID int64) (*WebhookEvent, error) {
	var e WebhookEvent
	err := db.conn.QueryRow(`
		SELECT id, object_type, object_id, aspect_type, owner_id, subscription_id,
		       event_time, updates, raw_json, processed, processed_at, error, created_at
		FROM webhook_events WHERE id = ?
	`, eventID).Scan(
		&e.ID, &e.ObjectType, &e.ObjectID, &e.AspectType, &e.OwnerID, &e.SubscriptionID,
		&e.EventTime, &e.Updates, &e.RawJSON, &e.Processed, &e.ProcessedAt, &e.Error, &e.CreatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get webhook event: %w", err)
	}
	return &e, nil
}

// MarkWebhookEventProcessed marks a webhook event as processed
func (db *DB) MarkWebhookEventProcessed(eventID int64, eventError *string) error {
	now := time.Now().Unix()

	result, err := db.conn.Exec(`
		UPDATE webhook_events
		SET processed = 1, processed_at = ?, error = ?
		WHERE id = ?
	`, now, eventError, eventID)

	if err != nil {
		return fmt.Errorf("failed to mark webhook event processed: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("webhook event not found")
	}

	return nil
}

// ListUnprocessedWebhookEvents returns unprocessed webhook events with pagination
func (db *DB) ListUnprocessedWebhookEvents(offset, limit int) ([]*WebhookEvent, error) {
	query := `
		SELECT id, object_type, object_id, aspect_type, owner_id, subscription_id,
		       event_time, updates, raw_json, processed, processed_at, error, created_at
		FROM webhook_events
		WHERE processed = 0
		ORDER BY event_time ASC
	`
	if limit > 0 {
		query += fmt.Sprintf(" LIMIT %d OFFSET %d", limit, offset)
	}

	rows, err := db.conn.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to list unprocessed webhook events: %w", err)
	}
	defer rows.Close()

	var events []*WebhookEvent
	for rows.Next() {
		var e WebhookEvent
		err := rows.Scan(
			&e.ID, &e.ObjectType, &e.ObjectID, &e.AspectType, &e.OwnerID, &e.SubscriptionID,
			&e.EventTime, &e.Updates, &e.RawJSON, &e.Processed, &e.ProcessedAt, &e.Error, &e.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan webhook event: %w", err)
		}
		events = append(events, &e)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating webhook events: %w", err)
	}

	return events, nil
}

// ListWebhookEventsByAthlete returns webhook events for an athlete with pagination
func (db *DB) ListWebhookEventsByAthlete(athleteID int64, offset, limit int) ([]*WebhookEvent, error) {
	query := `
		SELECT id, object_type, object_id, aspect_type, owner_id, subscription_id,
		       event_time, updates, raw_json, processed, processed_at, error, created_at
		FROM webhook_events
		WHERE owner_id = ?
		ORDER BY event_time DESC
	`
	if limit > 0 {
		query += fmt.Sprintf(" LIMIT %d OFFSET %d", limit, offset)
	}

	rows, err := db.conn.Query(query, athleteID)
	if err != nil {
		return nil, fmt.Errorf("failed to list webhook events by athlete: %w", err)
	}
	defer rows.Close()

	var events []*WebhookEvent
	for rows.Next() {
		var e WebhookEvent
		err := rows.Scan(
			&e.ID, &e.ObjectType, &e.ObjectID, &e.AspectType, &e.OwnerID, &e.SubscriptionID,
			&e.EventTime, &e.Updates, &e.RawJSON, &e.Processed, &e.ProcessedAt, &e.Error, &e.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan webhook event: %w", err)
		}
		events = append(events, &e)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating webhook events: %w", err)
	}

	return events, nil
}
