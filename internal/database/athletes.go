package database

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"plantopo-strava-sync/internal/metrics"
)

// Athlete represents an athlete's authentication data in the database
type Athlete struct {
	AthleteID      int64
	AccessToken    string
	RefreshToken   string
	TokenExpiresAt time.Time
	AthleteSummary json.RawMessage // JSON blob from Strava
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// UpsertAthlete inserts or updates an athlete's data
func (d *DB) UpsertAthlete(athlete *Athlete) error {
	timer := prometheus.NewTimer(metrics.DBOperationDuration.WithLabelValues(metrics.DBOpUpsertAthlete))
	defer timer.ObserveDuration()

	query := `
		INSERT INTO athletes (athlete_id, access_token, refresh_token, token_expires_at, athlete_summary, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(athlete_id) DO UPDATE SET
			access_token = excluded.access_token,
			refresh_token = excluded.refresh_token,
			token_expires_at = excluded.token_expires_at,
			athlete_summary = excluded.athlete_summary,
			updated_at = excluded.updated_at
	`

	_, err := d.db.Exec(query,
		athlete.AthleteID,
		athlete.AccessToken,
		athlete.RefreshToken,
		athlete.TokenExpiresAt.Unix(),
		athlete.AthleteSummary,
		athlete.CreatedAt.Unix(),
		athlete.UpdatedAt.Unix(),
	)

	if err != nil {
		metrics.DBOperationErrorsTotal.WithLabelValues(metrics.DBOpUpsertAthlete).Inc()
		return fmt.Errorf("failed to upsert athlete: %w", err)
	}

	return nil
}

// GetAthlete retrieves an athlete by ID
func (d *DB) GetAthlete(athleteID int64) (*Athlete, error) {
	timer := prometheus.NewTimer(metrics.DBOperationDuration.WithLabelValues(metrics.DBOpGetAthlete))
	defer timer.ObserveDuration()

	query := `
		SELECT athlete_id, access_token, refresh_token, token_expires_at, athlete_summary, created_at, updated_at
		FROM athletes
		WHERE athlete_id = ?
	`

	var athlete Athlete
	var expiresAt, createdAt, updatedAt int64

	err := d.db.QueryRow(query, athleteID).Scan(
		&athlete.AthleteID,
		&athlete.AccessToken,
		&athlete.RefreshToken,
		&expiresAt,
		&athlete.AthleteSummary,
		&createdAt,
		&updatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, nil // Athlete not found
	}
	if err != nil {
		metrics.DBOperationErrorsTotal.WithLabelValues(metrics.DBOpGetAthlete).Inc()
		return nil, fmt.Errorf("failed to get athlete: %w", err)
	}

	athlete.TokenExpiresAt = time.Unix(expiresAt, 0)
	athlete.CreatedAt = time.Unix(createdAt, 0)
	athlete.UpdatedAt = time.Unix(updatedAt, 0)

	return &athlete, nil
}

// DeleteAthlete deletes an athlete record
// Note: This does not delete their events - use DeleteAthleteEvents separately if needed
func (d *DB) DeleteAthlete(athleteID int64) error {
	query := `DELETE FROM athletes WHERE athlete_id = ?`

	_, err := d.db.Exec(query, athleteID)
	if err != nil {
		return fmt.Errorf("failed to delete athlete: %w", err)
	}

	return nil
}
