package database

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"plantopo-strava-sync/internal/metrics"
)

type CircuitBreakerState struct {
	ID                   int64
	State                string // closed, open, half_open
	OpenedAt             *time.Time
	ClosesAt             *time.Time
	Last429At            *time.Time
	Remaining15Min       *int
	RemainingDaily       *int
	ConsecutiveSuccesses int
	UpdatedAt            time.Time
}

func (d *DB) GetCircuitBreakerState() (*CircuitBreakerState, error) {
	timer := prometheus.NewTimer(metrics.DBOperationDuration.WithLabelValues(metrics.DBOpGetCircuitBreakerState))
	defer timer.ObserveDuration()

	query := `
		SELECT id, state, opened_at, closes_at, last_429_at,
		       remaining_15min, remaining_daily, consecutive_successes, updated_at
		FROM rate_limit_circuit_breaker
		WHERE id = 1
	`

	var state CircuitBreakerState
	var openedAt, closesAt, last429At, updatedAt *int64

	err := d.db.QueryRow(query).Scan(
		&state.ID, &state.State,
		&openedAt, &closesAt, &last429At,
		&state.Remaining15Min, &state.RemainingDaily,
		&state.ConsecutiveSuccesses, &updatedAt,
	)

	if err == sql.ErrNoRows {
		return &CircuitBreakerState{State: "closed", UpdatedAt: time.Now()}, nil
	}
	if err != nil {
		metrics.DBOperationErrorsTotal.WithLabelValues(metrics.DBOpGetCircuitBreakerState).Inc()
		return nil, fmt.Errorf("failed to get circuit breaker state: %w", err)
	}

	// Convert Unix timestamps
	if openedAt != nil {
		t := time.Unix(*openedAt, 0)
		state.OpenedAt = &t
	}
	if closesAt != nil {
		t := time.Unix(*closesAt, 0)
		state.ClosesAt = &t
	}
	if last429At != nil {
		t := time.Unix(*last429At, 0)
		state.Last429At = &t
	}
	if updatedAt != nil {
		state.UpdatedAt = time.Unix(*updatedAt, 0)
	}

	return &state, nil
}

func (d *DB) OpenCircuitBreaker(remaining15min, remainingDaily int, cooldown time.Duration) error {
	timer := prometheus.NewTimer(metrics.DBOperationDuration.WithLabelValues(metrics.DBOpOpenCircuitBreaker))
	defer timer.ObserveDuration()

	now := time.Now()
	closesAt := now.Add(cooldown)

	query := `
		UPDATE rate_limit_circuit_breaker
		SET state = 'open',
		    opened_at = ?,
		    closes_at = ?,
		    last_429_at = ?,
		    remaining_15min = ?,
		    remaining_daily = ?,
		    consecutive_successes = 0,
		    updated_at = ?
		WHERE id = 1
	`

	_, err := d.db.Exec(query,
		now.Unix(), closesAt.Unix(), now.Unix(),
		remaining15min, remainingDaily, now.Unix(),
	)

	if err != nil {
		metrics.DBOperationErrorsTotal.WithLabelValues(metrics.DBOpOpenCircuitBreaker).Inc()
		return fmt.Errorf("failed to open circuit breaker: %w", err)
	}

	return nil
}

func (d *DB) TransitionCircuitBreakerToHalfOpen() error {
	timer := prometheus.NewTimer(metrics.DBOperationDuration.WithLabelValues(metrics.DBOpTransitionCircuitBreaker))
	defer timer.ObserveDuration()

	query := `
		UPDATE rate_limit_circuit_breaker
		SET state = 'half_open',
		    consecutive_successes = 0,
		    updated_at = ?
		WHERE id = 1 AND state = 'open'
	`

	_, err := d.db.Exec(query, time.Now().Unix())
	if err != nil {
		metrics.DBOperationErrorsTotal.WithLabelValues(metrics.DBOpTransitionCircuitBreaker).Inc()
	}
	return err
}

func (d *DB) TransitionCircuitBreakerToClosed() error {
	timer := prometheus.NewTimer(metrics.DBOperationDuration.WithLabelValues(metrics.DBOpTransitionCircuitBreaker))
	defer timer.ObserveDuration()

	query := `
		UPDATE rate_limit_circuit_breaker
		SET state = 'closed',
		    opened_at = NULL,
		    closes_at = NULL,
		    consecutive_successes = 0,
		    updated_at = ?
		WHERE id = 1
	`

	_, err := d.db.Exec(query, time.Now().Unix())
	if err != nil {
		metrics.DBOperationErrorsTotal.WithLabelValues(metrics.DBOpTransitionCircuitBreaker).Inc()
	}
	return err
}

func (d *DB) IncrementCircuitBreakerSuccesses() error {
	query := `
		UPDATE rate_limit_circuit_breaker
		SET consecutive_successes = consecutive_successes + 1,
		    updated_at = ?
		WHERE id = 1 AND state = 'half_open'
	`

	_, err := d.db.Exec(query, time.Now().Unix())
	return err
}
