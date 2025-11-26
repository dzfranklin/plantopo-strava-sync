package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"plantopo-strava-sync/internal/config"
	"plantopo-strava-sync/internal/database"
)

func setupWebhookTest(t *testing.T) (*WebhookHandler, *database.DB) {
	dbPath := t.TempDir() + "/test.db"
	db, err := database.Open(dbPath)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}

	cfg := &config.Config{
		StravaClients: map[string]*config.StravaClientConfig{
			"primary": {
				ClientID:     "test_client_id",
				ClientSecret: "test_secret",
				VerifyToken:  "test_verify_token",
			},
		},
	}

	handler := NewWebhookHandler(db, cfg)

	return handler, db
}

// newRequestWithClient creates a test request with client ID in context
func newRequestWithClient(method, path string, body io.Reader, client string) *http.Request {
	req := httptest.NewRequest(method, path, body)
	ctx := context.WithValue(req.Context(), "client", client)
	return req.WithContext(ctx)
}

func TestHandleVerification_Success(t *testing.T) {
	handler, db := setupWebhookTest(t)
	defer db.Close()

	req := newRequestWithClient(http.MethodGet, "/webhook-callback/primary?hub.mode=subscribe&hub.challenge=test_challenge&hub.verify_token=test_verify_token", nil, "primary")
	w := httptest.NewRecorder()

	handler.HandleVerification(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var response map[string]string
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if response["hub.challenge"] != "test_challenge" {
		t.Errorf("Expected challenge 'test_challenge', got '%s'", response["hub.challenge"])
	}
}

func TestHandleVerification_InvalidToken(t *testing.T) {
	handler, db := setupWebhookTest(t)
	defer db.Close()

	req := newRequestWithClient(http.MethodGet, "/webhook-callback/primary?hub.mode=subscribe&hub.challenge=test_challenge&hub.verify_token=wrong_token", nil, "primary")
	w := httptest.NewRecorder()

	handler.HandleVerification(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("Expected status 403, got %d", w.Code)
	}
}

func TestHandleVerification_WrongMethod(t *testing.T) {
	handler, db := setupWebhookTest(t)
	defer db.Close()

	req := newRequestWithClient(http.MethodPost, "/webhook-callback/primary", nil, "primary")
	w := httptest.NewRecorder()

	handler.HandleVerification(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("Expected status 405, got %d", w.Code)
	}
}

func TestHandleEvent_Success(t *testing.T) {
	handler, db := setupWebhookTest(t)
	defer db.Close()

	webhookData := map[string]interface{}{
		"object_type": "activity",
		"object_id":   1234567890,
		"aspect_type": "create",
		"owner_id":    98765,
		"event_time":  1234567890,
	}

	body, _ := json.Marshal(webhookData)
	req := newRequestWithClient(http.MethodPost, "/webhook-callback/primary", bytes.NewReader(body), "primary")
	w := httptest.NewRecorder()

	handler.HandleEvent(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	// Verify webhook was enqueued
	length, err := db.GetQueueLength()
	if err != nil {
		t.Fatalf("Failed to get queue length: %v", err)
	}

	if length != 1 {
		t.Errorf("Expected queue length 1, got %d", length)
	}

	// Verify the data in the queue
	item, err := db.ClaimWebhook()
	if err != nil {
		t.Fatalf("Failed to claim webhook: %v", err)
	}

	if item == nil {
		t.Fatal("Expected webhook item, got nil")
	}

	var queuedData map[string]interface{}
	if err := json.Unmarshal(item.Data, &queuedData); err != nil {
		t.Fatalf("Failed to unmarshal queued data: %v", err)
	}

	if queuedData["object_type"] != "activity" {
		t.Errorf("Expected object_type 'activity', got '%v'", queuedData["object_type"])
	}
}

func TestHandleEvent_InvalidJSON(t *testing.T) {
	handler, db := setupWebhookTest(t)
	defer db.Close()

	req := newRequestWithClient(http.MethodPost, "/webhook-callback/primary", bytes.NewReader([]byte("invalid json")), "primary")
	w := httptest.NewRecorder()

	handler.HandleEvent(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", w.Code)
	}

	// Verify nothing was enqueued
	length, err := db.GetQueueLength()
	if err != nil {
		t.Fatalf("Failed to get queue length: %v", err)
	}

	if length != 0 {
		t.Errorf("Expected queue length 0, got %d", length)
	}
}

func TestHandleEvent_WrongMethod(t *testing.T) {
	handler, db := setupWebhookTest(t)
	defer db.Close()

	req := newRequestWithClient(http.MethodGet, "/webhook-callback/primary", nil, "primary")
	w := httptest.NewRecorder()

	handler.HandleEvent(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("Expected status 405, got %d", w.Code)
	}
}
