package database

import (
	"database/sql"
	"fmt"
	"time"
)

// Athlete represents a Strava athlete/user
type Athlete struct {
	AthleteID     int64
	AccessToken   string
	RefreshToken  string
	ExpiresAt     int64
	Scope         string
	Authorized    bool
	LastSyncAt    *int64
	SyncInProgress bool
	SyncError     *string
	CreatedAt     int64
	UpdatedAt     int64
	ProfileJSON   *string
}

// CreateAthlete inserts a new athlete into the database
func (db *DB) CreateAthlete(a *Athlete) error {
	now := time.Now().Unix()
	a.CreatedAt = now
	a.UpdatedAt = now

	_, err := db.conn.Exec(`
		INSERT INTO athletes (
			athlete_id, access_token, refresh_token, expires_at, scope,
			authorized, last_sync_at, sync_in_progress, sync_error,
			created_at, updated_at, profile_json
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, a.AthleteID, a.AccessToken, a.RefreshToken, a.ExpiresAt, a.Scope,
		a.Authorized, a.LastSyncAt, a.SyncInProgress, a.SyncError,
		a.CreatedAt, a.UpdatedAt, a.ProfileJSON)

	if err != nil {
		return fmt.Errorf("failed to create athlete: %w", err)
	}
	return nil
}

// GetAthlete retrieves an athlete by ID
func (db *DB) GetAthlete(athleteID int64) (*Athlete, error) {
	var a Athlete
	err := db.conn.QueryRow(`
		SELECT athlete_id, access_token, refresh_token, expires_at, scope,
		       authorized, last_sync_at, sync_in_progress, sync_error,
		       created_at, updated_at, profile_json
		FROM athletes WHERE athlete_id = ?
	`, athleteID).Scan(
		&a.AthleteID, &a.AccessToken, &a.RefreshToken, &a.ExpiresAt, &a.Scope,
		&a.Authorized, &a.LastSyncAt, &a.SyncInProgress, &a.SyncError,
		&a.CreatedAt, &a.UpdatedAt, &a.ProfileJSON,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get athlete: %w", err)
	}
	return &a, nil
}

// UpdateAthleteTokens updates an athlete's OAuth tokens
func (db *DB) UpdateAthleteTokens(athleteID int64, accessToken, refreshToken string, expiresAt int64) error {
	result, err := db.conn.Exec(`
		UPDATE athletes
		SET access_token = ?, refresh_token = ?, expires_at = ?, updated_at = ?
		WHERE athlete_id = ?
	`, accessToken, refreshToken, expiresAt, time.Now().Unix(), athleteID)

	if err != nil {
		return fmt.Errorf("failed to update athlete tokens: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("athlete not found")
	}

	return nil
}

// UpdateAthleteSyncState updates an athlete's sync state
func (db *DB) UpdateAthleteSyncState(athleteID int64, inProgress bool, syncError *string, lastSyncAt *int64) error {
	result, err := db.conn.Exec(`
		UPDATE athletes
		SET sync_in_progress = ?, sync_error = ?, last_sync_at = ?, updated_at = ?
		WHERE athlete_id = ?
	`, inProgress, syncError, lastSyncAt, time.Now().Unix(), athleteID)

	if err != nil {
		return fmt.Errorf("failed to update athlete sync state: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("athlete not found")
	}

	return nil
}

// DeauthorizeAthlete marks an athlete as deauthorized and clears sensitive data
func (db *DB) DeauthorizeAthlete(athleteID int64) error {
	result, err := db.conn.Exec(`
		UPDATE athletes
		SET authorized = 0, access_token = '', refresh_token = '', profile_json = NULL, updated_at = ?
		WHERE athlete_id = ?
	`, time.Now().Unix(), athleteID)

	if err != nil {
		return fmt.Errorf("failed to deauthorize athlete: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("athlete not found")
	}

	return nil
}

// ListAthletes returns athletes with pagination, optionally filtered by authorization status
func (db *DB) ListAthletes(authorizedOnly bool, offset, limit int) ([]*Athlete, error) {
	query := `
		SELECT athlete_id, access_token, refresh_token, expires_at, scope,
		       authorized, last_sync_at, sync_in_progress, sync_error,
		       created_at, updated_at, profile_json
		FROM athletes
	`
	if authorizedOnly {
		query += " WHERE authorized = 1"
	}
	query += " ORDER BY created_at DESC"
	if limit > 0 {
		query += fmt.Sprintf(" LIMIT %d OFFSET %d", limit, offset)
	}

	rows, err := db.conn.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to list athletes: %w", err)
	}
	defer rows.Close()

	var athletes []*Athlete
	for rows.Next() {
		var a Athlete
		err := rows.Scan(
			&a.AthleteID, &a.AccessToken, &a.RefreshToken, &a.ExpiresAt, &a.Scope,
			&a.Authorized, &a.LastSyncAt, &a.SyncInProgress, &a.SyncError,
			&a.CreatedAt, &a.UpdatedAt, &a.ProfileJSON,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan athlete: %w", err)
		}
		athletes = append(athletes, &a)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating athletes: %w", err)
	}

	return athletes, nil
}
