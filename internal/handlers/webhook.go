package handlers

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"

	"plantopo-strava-sync/internal/config"
	"plantopo-strava-sync/internal/database"
)

// WebhookHandler handles Strava webhook callbacks
type WebhookHandler struct {
	db     *database.DB
	config *config.Config
	logger *slog.Logger
}

// NewWebhookHandler creates a new webhook handler
func NewWebhookHandler(db *database.DB, cfg *config.Config) *WebhookHandler {
	return &WebhookHandler{
		db:     db,
		config: cfg,
		logger: slog.Default(),
	}
}

// HandleVerification handles GET requests for subscription verification
func (h *WebhookHandler) HandleVerification(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract verification parameters
	hubMode := r.URL.Query().Get("hub.mode")
	hubChallenge := r.URL.Query().Get("hub.challenge")
	hubVerifyToken := r.URL.Query().Get("hub.verify_token")

	h.logger.Info("Webhook verification request",
		"hub.mode", hubMode,
		"hub.challenge", hubChallenge[:min(20, len(hubChallenge))],
	)

	// Validate verify token
	if hubVerifyToken != h.config.StravaVerifyToken {
		h.logger.Warn("Invalid verify token")
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	// Respond with challenge
	response := map[string]string{
		"hub.challenge": hubChallenge,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	if err := json.NewEncoder(w).Encode(response); err != nil {
		h.logger.Error("Failed to encode challenge response", "error", err)
	}

	h.logger.Info("Webhook verification successful")
}

// HandleEvent handles POST requests for webhook events
func (h *WebhookHandler) HandleEvent(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Read the entire request body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		h.logger.Error("Failed to read webhook body", "error", err)
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	// Validate it's valid JSON
	var webhookData map[string]interface{}
	if err := json.Unmarshal(body, &webhookData); err != nil {
		h.logger.Error("Invalid JSON in webhook body", "error", err)
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	h.logger.Info("Received webhook event",
		"object_type", webhookData["object_type"],
		"object_id", webhookData["object_id"],
		"aspect_type", webhookData["aspect_type"],
		"owner_id", webhookData["owner_id"],
	)

	// Enqueue webhook for async processing
	if _, err := h.db.EnqueueWebhook(json.RawMessage(body)); err != nil {
		h.logger.Error("Failed to enqueue webhook", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// Respond immediately (async processing)
	w.WriteHeader(http.StatusOK)

	h.logger.Info("Webhook enqueued successfully")
}

// min returns the minimum of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
