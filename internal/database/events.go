package database

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// EventType represents the type of event
type EventType string

const (
	EventTypeAthleteConnected EventType = "athlete_connected"
	EventTypeWebhook          EventType = "webhook"
)

// Event represents an event in the event stream
type Event struct {
	EventID        int64
	EventType      EventType
	AthleteID      int64
	ActivityID     *int64           // Nullable
	AthleteSummary json.RawMessage  // For athlete_connected events
	Activity       json.RawMessage  // For webhook events (detailed activity)
	WebhookEvent   json.RawMessage  // For webhook events (raw webhook data)
	CreatedAt      time.Time
}

// InsertAthleteConnectedEvent inserts an athlete_connected event
func (d *DB) InsertAthleteConnectedEvent(athleteID int64, athleteSummary json.RawMessage) (int64, error) {
	query := `
		INSERT INTO events (event_type, athlete_id, athlete_summary)
		VALUES (?, ?, ?)
	`

	result, err := d.db.Exec(query, EventTypeAthleteConnected, athleteID, athleteSummary)
	if err != nil {
		return 0, fmt.Errorf("failed to insert athlete_connected event: %w", err)
	}

	eventID, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("failed to get event_id: %w", err)
	}

	return eventID, nil
}

// InsertWebhookEvent inserts a webhook event with activity data
func (d *DB) InsertWebhookEvent(athleteID int64, activityID *int64, activity, webhookEvent json.RawMessage) (int64, error) {
	query := `
		INSERT INTO events (event_type, athlete_id, activity_id, activity, webhook_event)
		VALUES (?, ?, ?, ?, ?)
	`

	result, err := d.db.Exec(query, EventTypeWebhook, athleteID, activityID, activity, webhookEvent)
	if err != nil {
		return 0, fmt.Errorf("failed to insert webhook event: %w", err)
	}

	eventID, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("failed to get event_id: %w", err)
	}

	return eventID, nil
}

// GetEvents retrieves events with cursor-based pagination
// cursor: the last event_id seen (0 for first page)
// limit: maximum number of events to return
func (d *DB) GetEvents(cursor int64, limit int) ([]*Event, error) {
	query := `
		SELECT event_id, event_type, athlete_id, activity_id, athlete_summary, activity, webhook_event, created_at
		FROM events
		WHERE event_id > ?
		ORDER BY event_id ASC
		LIMIT ?
	`

	rows, err := d.db.Query(query, cursor, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query events: %w", err)
	}
	defer rows.Close()

	var events []*Event
	for rows.Next() {
		var event Event
		var activityID sql.NullInt64
		var athleteSummary, activity, webhookEvent sql.NullString
		var createdAt int64

		err := rows.Scan(
			&event.EventID,
			&event.EventType,
			&event.AthleteID,
			&activityID,
			&athleteSummary,
			&activity,
			&webhookEvent,
			&createdAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan event: %w", err)
		}

		if activityID.Valid {
			event.ActivityID = &activityID.Int64
		}
		if athleteSummary.Valid {
			event.AthleteSummary = json.RawMessage(athleteSummary.String)
		}
		if activity.Valid {
			event.Activity = json.RawMessage(activity.String)
		}
		if webhookEvent.Valid {
			event.WebhookEvent = json.RawMessage(webhookEvent.String)
		}
		event.CreatedAt = time.Unix(createdAt, 0)

		events = append(events, &event)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating events: %w", err)
	}

	return events, nil
}

// DeleteAthleteEvents deletes all events for an athlete except the deauthorization event
// This should be called when an athlete revokes access
func (d *DB) DeleteAthleteEvents(athleteID int64, exceptEventID int64) error {
	query := `
		DELETE FROM events
		WHERE athlete_id = ? AND event_id != ?
	`

	_, err := d.db.Exec(query, athleteID, exceptEventID)
	if err != nil {
		return fmt.Errorf("failed to delete athlete events: %w", err)
	}

	return nil
}
