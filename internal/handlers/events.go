package handlers

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"plantopo-strava-sync/internal/config"
	"plantopo-strava-sync/internal/database"
)

// EventsHandler handles the events stream endpoint
type EventsHandler struct {
	db           *database.DB
	config       *config.Config
	logger       *slog.Logger
	pollInterval time.Duration
	pollTimeout  time.Duration
}

// NewEventsHandler creates a new events handler
func NewEventsHandler(db *database.DB, cfg *config.Config) *EventsHandler {
	return &EventsHandler{
		db:           db,
		config:       cfg,
		logger:       slog.Default(),
		pollInterval: 500 * time.Millisecond,
		pollTimeout:  30 * time.Second,
	}
}

// HandleEvents handles GET /events with optional long-polling
// Query parameters:
//   - cursor: Last event_id seen (default: 0)
//   - limit: Maximum events to return (default: 100, max: 1000)
//   - long_poll: Enable long-polling (default: false)
//
// Authentication: Requires Authorization header
func (h *EventsHandler) HandleEvents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Verify authentication - check Authorization header
	authHeader := r.Header.Get("Authorization")
	if authHeader != "Bearer "+h.config.InternalAPIKey {
		h.logger.Warn("Unauthorized events request", "has_auth", authHeader != "")
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Parse query parameters
	query := r.URL.Query()

	cursorStr := query.Get("cursor")
	cursor := int64(0)
	if cursorStr != "" {
		var err error
		cursor, err = strconv.ParseInt(cursorStr, 10, 64)
		if err != nil {
			http.Error(w, "Invalid cursor parameter", http.StatusBadRequest)
			return
		}
	}

	limitStr := query.Get("limit")
	limit := 100
	if limitStr != "" {
		var err error
		limit, err = strconv.Atoi(limitStr)
		if err != nil {
			http.Error(w, "Invalid limit parameter", http.StatusBadRequest)
			return
		}
		if limit < 1 || limit > 1000 {
			http.Error(w, "Limit must be between 1 and 1000", http.StatusBadRequest)
			return
		}
	}

	// Parse long_poll parameter (default: false)
	longPoll := false
	if query.Has("long_poll") && query.Get("long_poll") == "" {
		longPoll = true
	} else if longPollStr := query.Get("long_poll"); longPollStr != "" {
		longPoll = longPollStr == "true" || longPollStr == "1"
	}

	h.logger.Info("Events request", "cursor", cursor, "limit", limit, "long_poll", longPoll)

	// Get events (with or without long-polling)
	var events []*database.Event
	if longPoll {
		events = h.longPollEvents(cursor, limit)
	} else {
		var err error
		events, err = h.db.GetEvents(cursor, limit)
		if err != nil {
			h.logger.Error("Failed to get events", "error", err)
			events = []*database.Event{}
		}
	}

	if events == nil {
		events = []*database.Event{}
	}

	// Return events as JSON
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		"events": events,
		"cursor": h.getLatestCursor(events, cursor),
	}); err != nil {
		h.logger.Error("Failed to encode events response", "error", err)
	}
}

// longPollEvents polls for events until some are available or timeout occurs
func (h *EventsHandler) longPollEvents(cursor int64, limit int) []*database.Event {
	deadline := time.Now().Add(h.pollTimeout)

	for {
		// Try to get events
		events, err := h.db.GetEvents(cursor, limit)
		if err != nil {
			h.logger.Error("Failed to get events", "error", err, "cursor", cursor)
			return []*database.Event{} // Return empty on error
		}

		// If we have events, return them
		if len(events) > 0 {
			h.logger.Info("Returning events", "count", len(events), "cursor", cursor)
			return events
		}

		// Check if we've exceeded the timeout
		if time.Now().After(deadline) {
			h.logger.Info("Long-poll timeout, returning empty", "cursor", cursor)
			return []*database.Event{}
		}

		// Wait before next poll
		time.Sleep(h.pollInterval)
	}
}

// getLatestCursor returns the latest event_id from the events list, or the original cursor
func (h *EventsHandler) getLatestCursor(events []*database.Event, currentCursor int64) int64 {
	if len(events) == 0 {
		return currentCursor
	}
	return events[len(events)-1].EventID
}
