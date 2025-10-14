package database

import (
	"database/sql"
	"fmt"
	"time"
)

// Activity represents a Strava activity
type Activity struct {
	ID            int64
	AthleteID     int64
	HasSummary    bool
	HasDetails    bool
	Deleted       bool
	SummaryJSON   *string
	DetailsJSON   *string
	StartDate     *int64
	ActivityType  *string
	CreatedAt     int64
	UpdatedAt     int64
	LastSyncedAt  *int64
}

// CreateActivity inserts a new activity into the database
func (db *DB) CreateActivity(a *Activity) error {
	now := time.Now().Unix()
	a.CreatedAt = now
	a.UpdatedAt = now

	_, err := db.conn.Exec(`
		INSERT INTO activities (
			id, athlete_id, has_summary, has_details, deleted,
			summary_json, details_json, start_date, activity_type,
			created_at, updated_at, last_synced_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, a.ID, a.AthleteID, a.HasSummary, a.HasDetails, a.Deleted,
		a.SummaryJSON, a.DetailsJSON, a.StartDate, a.ActivityType,
		a.CreatedAt, a.UpdatedAt, a.LastSyncedAt)

	if err != nil {
		return fmt.Errorf("failed to create activity: %w", err)
	}
	return nil
}

// GetActivity retrieves an activity by ID
func (db *DB) GetActivity(activityID int64) (*Activity, error) {
	var a Activity
	err := db.conn.QueryRow(`
		SELECT id, athlete_id, has_summary, has_details, deleted,
		       summary_json, details_json, start_date, activity_type,
		       created_at, updated_at, last_synced_at
		FROM activities WHERE id = ?
	`, activityID).Scan(
		&a.ID, &a.AthleteID, &a.HasSummary, &a.HasDetails, &a.Deleted,
		&a.SummaryJSON, &a.DetailsJSON, &a.StartDate, &a.ActivityType,
		&a.CreatedAt, &a.UpdatedAt, &a.LastSyncedAt,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get activity: %w", err)
	}
	return &a, nil
}

// UpsertActivitySummary inserts or updates activity summary data
func (db *DB) UpsertActivitySummary(activityID, athleteID int64, summaryJSON string, startDate *int64, activityType *string) error {
	now := time.Now().Unix()

	_, err := db.conn.Exec(`
		INSERT INTO activities (
			id, athlete_id, has_summary, has_details, deleted,
			summary_json, start_date, activity_type,
			created_at, updated_at, last_synced_at
		) VALUES (?, ?, 1, 0, 0, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			summary_json = excluded.summary_json,
			start_date = excluded.start_date,
			activity_type = excluded.activity_type,
			has_summary = 1,
			updated_at = excluded.updated_at,
			last_synced_at = excluded.last_synced_at
	`, activityID, athleteID, summaryJSON, startDate, activityType, now, now, now)

	if err != nil {
		return fmt.Errorf("failed to upsert activity summary: %w", err)
	}
	return nil
}

// UpdateActivityDetails updates activity details data
func (db *DB) UpdateActivityDetails(activityID int64, detailsJSON string) error {
	now := time.Now().Unix()

	result, err := db.conn.Exec(`
		UPDATE activities
		SET details_json = ?, has_details = 1, updated_at = ?, last_synced_at = ?
		WHERE id = ?
	`, detailsJSON, now, now, activityID)

	if err != nil {
		return fmt.Errorf("failed to update activity details: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("activity not found")
	}

	return nil
}

// MarkActivityDeleted marks an activity as deleted
func (db *DB) MarkActivityDeleted(activityID int64) error {
	result, err := db.conn.Exec(`
		UPDATE activities
		SET deleted = 1, summary_json = NULL, details_json = NULL, updated_at = ?
		WHERE id = ?
	`, time.Now().Unix(), activityID)

	if err != nil {
		return fmt.Errorf("failed to mark activity deleted: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("activity not found")
	}

	return nil
}

// ListActivitiesByAthlete returns activities for an athlete with pagination
func (db *DB) ListActivitiesByAthlete(athleteID int64, offset, limit int, includeDeleted bool) ([]*Activity, error) {
	query := `
		SELECT id, athlete_id, has_summary, has_details, deleted,
		       summary_json, details_json, start_date, activity_type,
		       created_at, updated_at, last_synced_at
		FROM activities
		WHERE athlete_id = ?
	`
	if !includeDeleted {
		query += " AND deleted = 0"
	}
	query += " ORDER BY start_date DESC"
	if limit > 0 {
		query += fmt.Sprintf(" LIMIT %d OFFSET %d", limit, offset)
	}

	rows, err := db.conn.Query(query, athleteID)
	if err != nil {
		return nil, fmt.Errorf("failed to list activities: %w", err)
	}
	defer rows.Close()

	var activities []*Activity
	for rows.Next() {
		var a Activity
		err := rows.Scan(
			&a.ID, &a.AthleteID, &a.HasSummary, &a.HasDetails, &a.Deleted,
			&a.SummaryJSON, &a.DetailsJSON, &a.StartDate, &a.ActivityType,
			&a.CreatedAt, &a.UpdatedAt, &a.LastSyncedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan activity: %w", err)
		}
		activities = append(activities, &a)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating activities: %w", err)
	}

	return activities, nil
}

// ListActivitiesNeedingDetails returns activities that need detail fetching with pagination
func (db *DB) ListActivitiesNeedingDetails(offset, limit int) ([]*Activity, error) {
	query := `
		SELECT id, athlete_id, has_summary, has_details, deleted,
		       summary_json, details_json, start_date, activity_type,
		       created_at, updated_at, last_synced_at
		FROM activities
		WHERE has_details = 0 AND deleted = 0
		ORDER BY start_date DESC
	`
	if limit > 0 {
		query += fmt.Sprintf(" LIMIT %d OFFSET %d", limit, offset)
	}

	rows, err := db.conn.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to list activities needing details: %w", err)
	}
	defer rows.Close()

	var activities []*Activity
	for rows.Next() {
		var a Activity
		err := rows.Scan(
			&a.ID, &a.AthleteID, &a.HasSummary, &a.HasDetails, &a.Deleted,
			&a.SummaryJSON, &a.DetailsJSON, &a.StartDate, &a.ActivityType,
			&a.CreatedAt, &a.UpdatedAt, &a.LastSyncedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan activity: %w", err)
		}
		activities = append(activities, &a)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating activities: %w", err)
	}

	return activities, nil
}
