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

	// Extract client_id from query parameter
	clientID := r.URL.Query().Get("client_id")
	if clientID == "" {
		h.logger.Warn("Missing client_id in webhook verification")
		http.Error(w, "Missing client_id parameter", http.StatusBadRequest)
		return
	}

	// Get client config
	clientConfig, err := h.config.GetClient(clientID)
	if err != nil {
		h.logger.Warn("Invalid client_id", "client_id", clientID)
		http.Error(w, "Invalid client_id", http.StatusBadRequest)
		return
	}

	// Extract verification parameters
	hubMode := r.URL.Query().Get("hub.mode")
	hubChallenge := r.URL.Query().Get("hub.challenge")
	hubVerifyToken := r.URL.Query().Get("hub.verify_token")

	h.logger.Info("Webhook verification request",
		"client_id", clientID,
		"hub.mode", hubMode,
		"hub.challenge", hubChallenge[:min(20, len(hubChallenge))],
	)

	// Validate against client-specific verify token
	if hubVerifyToken != clientConfig.VerifyToken {
		h.logger.Warn("Invalid verify token", "client_id", clientID)
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

	h.logger.Info("Webhook verification successful", "client_id", clientID)
}

// HandleEvent handles POST requests for webhook events
func (h *WebhookHandler) HandleEvent(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract client_id from query parameter
	clientID := r.URL.Query().Get("client_id")
	if clientID == "" {
		h.logger.Warn("Missing client_id in webhook event")
		http.Error(w, "Missing client_id parameter", http.StatusBadRequest)
		return
	}

	// Validate client exists
	if !h.config.HasClient(clientID) {
		h.logger.Warn("Invalid client_id", "client_id", clientID)
		http.Error(w, "Invalid client_id", http.StatusBadRequest)
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
		"client_id", clientID,
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

	h.logger.Info("Webhook enqueued successfully", "client_id", clientID)
}

// min returns the minimum of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
