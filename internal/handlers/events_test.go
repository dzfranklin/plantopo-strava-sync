package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"plantopo-strava-sync/internal/config"
	"plantopo-strava-sync/internal/database"
)

func setupEventsHandlerTest(t *testing.T) (*EventsHandler, *database.DB) {
	dbPath := t.TempDir() + "/test.db"
	db, err := database.Open(dbPath)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}

	cfg := &config.Config{
		InternalAPIKey: "test_api_key",
	}

	handler := NewEventsHandler(db, cfg)
	// Speed up tests by reducing poll interval and timeout
	handler.pollInterval = 10 * time.Millisecond
	handler.pollTimeout = 100 * time.Millisecond

	return handler, db
}

func TestHandleEvents_Success(t *testing.T) {
	handler, db := setupEventsHandlerTest(t)
	defer db.Close()

	// Insert test events
	athleteID := int64(12345)
	_, err := db.InsertAthleteConnectedEvent(athleteID, json.RawMessage(`{"id": 12345}`))
	if err != nil {
		t.Fatalf("Failed to insert event: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/events?cursor=0&limit=10", nil)
	req.Header.Set("Authorization", "Bearer test_api_key")
	w := httptest.NewRecorder()

	handler.HandleEvents(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var response map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	events, ok := response["events"].([]interface{})
	if !ok {
		t.Fatal("Expected events array in response")
	}

	if len(events) != 1 {
		t.Errorf("Expected 1 event, got %d", len(events))
	}

	cursor, ok := response["cursor"].(float64)
	if !ok {
		t.Fatal("Expected cursor in response")
	}

	if cursor != 1 {
		t.Errorf("Expected cursor 1, got %v", cursor)
	}
}

func TestHandleEvents_Unauthorized(t *testing.T) {
	handler, db := setupEventsHandlerTest(t)
	defer db.Close()

	tests := []struct {
		name   string
		apiKey string
	}{
		{"no key", ""},
		{"wrong key", "wrong_key"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/events", nil)
			if tt.apiKey != "" {
				req.Header.Set("Authorization", tt.apiKey)
			}
			w := httptest.NewRecorder()

			handler.HandleEvents(w, req)

			if w.Code != http.StatusUnauthorized {
				t.Errorf("Expected status 401, got %d", w.Code)
			}
		})
	}
}

func TestHandleEvents_WrongMethod(t *testing.T) {
	handler, db := setupEventsHandlerTest(t)
	defer db.Close()

	req := httptest.NewRequest(http.MethodPost, "/events", nil)
	req.Header.Set("Authorization", "Bearer test_api_key")
	w := httptest.NewRecorder()

	handler.HandleEvents(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("Expected status 405, got %d", w.Code)
	}
}

func TestHandleEvents_InvalidCursor(t *testing.T) {
	handler, db := setupEventsHandlerTest(t)
	defer db.Close()

	req := httptest.NewRequest(http.MethodGet, "/events?cursor=invalid", nil)
	req.Header.Set("Authorization", "Bearer test_api_key")
	w := httptest.NewRecorder()

	handler.HandleEvents(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", w.Code)
	}
}

func TestHandleEvents_InvalidLimit(t *testing.T) {
	handler, db := setupEventsHandlerTest(t)
	defer db.Close()

	tests := []struct {
		name  string
		limit string
	}{
		{"non-numeric", "invalid"},
		{"too small", "0"},
		{"too large", "1001"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/events?limit="+tt.limit, nil)
			req.Header.Set("Authorization", "Bearer test_api_key")
			w := httptest.NewRecorder()

			handler.HandleEvents(w, req)

			if w.Code != http.StatusBadRequest {
				t.Errorf("Expected status 400, got %d", w.Code)
			}
		})
	}
}

func TestHandleEvents_EmptyWithTimeout(t *testing.T) {
	handler, db := setupEventsHandlerTest(t)
	defer db.Close()

	req := httptest.NewRequest(http.MethodGet, "/events?cursor=0&long_poll=true", nil)
	req.Header.Set("Authorization", "Bearer test_api_key")
	w := httptest.NewRecorder()

	start := time.Now()
	handler.HandleEvents(w, req)
	elapsed := time.Since(start)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	// Should have waited at least pollTimeout
	if elapsed < handler.pollTimeout {
		t.Errorf("Expected to wait at least %v, waited %v", handler.pollTimeout, elapsed)
	}

	var response map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	events, ok := response["events"].([]interface{})
	if !ok {
		t.Fatal("Expected events array in response")
	}

	if len(events) != 0 {
		t.Errorf("Expected 0 events, got %d", len(events))
	}

	cursor, ok := response["cursor"].(float64)
	if !ok {
		t.Fatal("Expected cursor in response")
	}

	if cursor != 0 {
		t.Errorf("Expected cursor 0 (unchanged), got %v", cursor)
	}
}

func TestHandleEvents_LongPolling(t *testing.T) {
	handler, db := setupEventsHandlerTest(t)
	defer db.Close()

	// Start request in goroutine
	req := httptest.NewRequest(http.MethodGet, "/events?cursor=0&long_poll=true", nil)
	req.Header.Set("Authorization", "Bearer test_api_key")
	w := httptest.NewRecorder()

	done := make(chan bool)
	go func() {
		handler.HandleEvents(w, req)
		done <- true
	}()

	// Insert event after a brief delay
	time.Sleep(50 * time.Millisecond)
	athleteID := int64(12345)
	_, err := db.InsertAthleteConnectedEvent(athleteID, json.RawMessage(`{"id": 12345}`))
	if err != nil {
		t.Fatalf("Failed to insert event: %v", err)
	}

	// Wait for response
	select {
	case <-done:
		// Success
	case <-time.After(1 * time.Second):
		t.Fatal("Request timed out")
	}

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var response map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	events, ok := response["events"].([]interface{})
	if !ok {
		t.Fatal("Expected events array in response")
	}

	if len(events) != 1 {
		t.Errorf("Expected 1 event (found via long-poll), got %d", len(events))
	}
}

func TestHandleEvents_LongPollDisabled(t *testing.T) {
	handler, db := setupEventsHandlerTest(t)
	defer db.Close()

	// Insert test event
	athleteID := int64(12345)
	_, err := db.InsertAthleteConnectedEvent(athleteID, json.RawMessage(`{"id": 12345}`))
	if err != nil {
		t.Fatalf("Failed to insert event: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/events?long_poll=false", nil)
	req.Header.Set("Authorization", "Bearer test_api_key")
	w := httptest.NewRecorder()

	start := time.Now()
	handler.HandleEvents(w, req)
	elapsed := time.Since(start)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	// Should return immediately without waiting
	if elapsed > 100*time.Millisecond {
		t.Errorf("Expected immediate response with long_poll=false, took %v", elapsed)
	}

	var response map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	events, ok := response["events"].([]interface{})
	if !ok {
		t.Fatal("Expected events array in response")
	}

	if len(events) != 1 {
		t.Errorf("Expected 1 event, got %d", len(events))
	}
}
