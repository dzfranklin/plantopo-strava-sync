package handlers

import (
	"fmt"
	"log/slog"
	"net/http"

	"plantopo-strava-sync/internal/config"
	"plantopo-strava-sync/internal/oauth"
)

// OAuthHandler handles OAuth flow endpoints
type OAuthHandler struct {
	oauthManager *oauth.Manager
	config       *config.Config
	logger       *slog.Logger
}

// NewOAuthHandler creates a new OAuth handler
func NewOAuthHandler(oauthManager *oauth.Manager, cfg *config.Config) *OAuthHandler {
	return &OAuthHandler{
		oauthManager: oauthManager,
		config:       cfg,
		logger:       slog.Default(),
	}
}

// HandleAuthStart initiates the OAuth flow by redirecting to Strava
func (h *OAuthHandler) HandleAuthStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract client_id from query parameter
	clientID := r.URL.Query().Get("client_id")
	if clientID == "" {
		clientID = h.config.GetDefaultClientID()
	}

	// Validate client_id
	if !h.config.HasClient(clientID) {
		h.logger.Warn("Invalid client_id", "client_id", clientID)
		http.Error(w, "Invalid client_id", http.StatusBadRequest)
		return
	}

	// Build redirect URI (same host/port as current request)
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	redirectURI := fmt.Sprintf("%s://%s/oauth-callback", scheme, r.Host)

	// Generate authorization URL with client ID
	authURL, state, err := h.oauthManager.GenerateAuthURL(redirectURI, clientID)
	if err != nil {
		h.logger.Error("Failed to generate auth URL", "error", err)
		http.Error(w, "Failed to start OAuth flow", http.StatusInternalServerError)
		return
	}

	h.logger.Info("Starting OAuth flow", "state", state, "redirect_uri", redirectURI, "client_id", clientID)

	// Redirect user to Strava authorization page
	http.Redirect(w, r, authURL, http.StatusTemporaryRedirect)
}

// HandleCallback processes the OAuth callback from Strava
func (h *OAuthHandler) HandleCallback(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract parameters from query string
	code := r.URL.Query().Get("code")
	state := r.URL.Query().Get("state")
	errorParam := r.URL.Query().Get("error")

	// Check for authorization denial
	if errorParam != "" {
		h.logger.Warn("OAuth authorization denied", "error", errorParam)
		http.Error(w, fmt.Sprintf("Authorization failed: %s", errorParam), http.StatusBadRequest)
		return
	}

	// Validate required parameters
	if code == "" || state == "" {
		h.logger.Warn("Missing OAuth parameters", "has_code", code != "", "has_state", state != "")
		http.Error(w, "Missing code or state parameter", http.StatusBadRequest)
		return
	}

	h.logger.Info("Processing OAuth callback", "code_length", len(code), "state", state)

	// Handle the callback (exchange code, store athlete, enqueue sync)
	athleteID, clientID, err := h.oauthManager.HandleCallback(code, state)
	if err != nil {
		h.logger.Error("Failed to handle OAuth callback", "error", err)

		// Provide user-friendly error message
		errorMsg := "Failed to complete authorization"
		if err.Error() == "invalid or expired state" {
			errorMsg = "Invalid or expired authorization request. Please try again."
		}

		http.Error(w, errorMsg, http.StatusBadRequest)
		return
	}

	h.logger.Info("OAuth flow completed successfully", "athlete_id", athleteID, "client_id", clientID)

	// Success! Return simple HTML page
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, `<!DOCTYPE html>
<html>
<head>
	<meta charset="utf-8">
	<title>Authorization Successful</title>
	<style>
		body {
			font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Helvetica, Arial, sans-serif;
			max-width: 600px;
			margin: 100px auto;
			padding: 20px;
			text-align: center;
		}
		h1 { color: #FC4C02; }
		p { color: #666; line-height: 1.6; }
		code {
			background: #f4f4f4;
			padding: 2px 6px;
			border-radius: 3px;
			font-family: monospace;
		}
	</style>
</head>
<body>
	<h1>✓ Authorization Successful</h1>
	<p>Your Strava account has been connected (Athlete ID: <code>%d</code>)</p>
	<p>Historical activities are now being synced in the background.</p>
	<p>You can close this window and return to your application.</p>
</body>
</html>`, athleteID)
}
