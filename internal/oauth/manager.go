package oauth

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/url"
	"sync"
	"time"

	"plantopo-strava-sync/internal/config"
	"plantopo-strava-sync/internal/database"
	"plantopo-strava-sync/internal/strava"
)

const (
	authorizationURL = "https://www.strava.com/oauth/authorize"
	scope            = "activity:read_all" // Read all activities including private ones
)

// Manager handles OAuth 2.0 flow with Strava
type Manager struct {
	config       *config.Config
	db           *database.DB
	stravaClient *strava.Client
	logger       *slog.Logger
	states       *stateStore // CSRF protection
}

// stateEntry holds OAuth state information including client ID
type stateEntry struct {
	clientID string
	expiry   time.Time
}

// stateStore tracks valid OAuth states for CSRF protection
type stateStore struct {
	mu     sync.RWMutex
	states map[string]*stateEntry
}

// NewManager creates a new OAuth manager
func NewManager(cfg *config.Config, db *database.DB, stravaClient *strava.Client) *Manager {
	mgr := &Manager{
		config:       cfg,
		db:           db,
		stravaClient: stravaClient,
		logger:       slog.Default(),
		states: &stateStore{
			states: make(map[string]*stateEntry),
		},
	}

	// Start background cleanup of expired states
	go mgr.cleanupStates()

	return mgr
}

// GenerateAuthURL generates a Strava authorization URL with CSRF protection
func (m *Manager) GenerateAuthURL(redirectURI, clientID string) (string, string, error) {
	// Get client config
	clientConfig, err := m.config.GetClient(clientID)
	if err != nil {
		return "", "", fmt.Errorf("invalid client: %w", err)
	}

	// Generate random state for CSRF protection
	state, err := generateRandomState()
	if err != nil {
		return "", "", fmt.Errorf("failed to generate state: %w", err)
	}

	// Store state with expiration (10 minutes) and client ID
	m.states.mu.Lock()
	m.states.states[state] = &stateEntry{
		clientID: clientID,
		expiry:   time.Now().Add(10 * time.Minute),
	}
	m.states.mu.Unlock()

	// Build authorization URL using client-specific credentials
	params := url.Values{
		"client_id":     {clientConfig.ClientID},
		"redirect_uri":  {redirectURI},
		"response_type": {"code"},
		"scope":         {scope},
		"state":         {state},
	}

	authURL := fmt.Sprintf("%s?%s", authorizationURL, params.Encode())

	m.logger.Info("Generated auth URL", "state", state, "client_id", clientID)

	return authURL, state, nil
}

// HandleCallback processes the OAuth callback
// Returns the athlete ID and client ID on success
func (m *Manager) HandleCallback(code, state string) (int64, string, error) {
	// Validate state and get client ID
	clientID, valid := m.validateState(state)
	if !valid {
		return 0, "", fmt.Errorf("invalid or expired state")
	}

	m.logger.Info("Handling OAuth callback", "code_length", len(code), "client_id", clientID)

	// Exchange code for tokens using client-specific credentials
	tokenResp, err := m.stravaClient.ExchangeCode(code, clientID)
	if err != nil {
		return 0, "", fmt.Errorf("failed to exchange code: %w", err)
	}

	// Extract athlete ID from response
	var athleteData struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal(tokenResp.Athlete, &athleteData); err != nil {
		return 0, "", fmt.Errorf("failed to parse athlete data: %w", err)
	}

	athleteID := athleteData.ID

	m.logger.Info("Exchanged code for tokens", "athlete_id", athleteID, "client_id", clientID)

	// Create/update athlete record with client ID
	athlete := &database.Athlete{
		AthleteID:      athleteID,
		ClientID:       clientID,
		AccessToken:    tokenResp.AccessToken,
		RefreshToken:   tokenResp.RefreshToken,
		TokenExpiresAt: time.Unix(tokenResp.ExpiresAt, 0),
		AthleteSummary: tokenResp.Athlete,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}

	if err := m.db.UpsertAthlete(athlete); err != nil {
		return 0, "", fmt.Errorf("failed to upsert athlete: %w", err)
	}

	m.logger.Info("Stored athlete record", "athlete_id", athleteID, "client_id", clientID)

	// Insert athlete_connected event
	eventID, err := m.db.InsertAthleteConnectedEvent(athleteID, tokenResp.Athlete)
	if err != nil {
		return 0, "", fmt.Errorf("failed to insert athlete_connected event: %w", err)
	}

	m.logger.Info("Inserted athlete_connected event", "athlete_id", athleteID, "event_id", eventID)

	// Enqueue sync job to trigger historical activity listing
	if _, err := m.db.EnqueueSyncJob(athleteID, "list_activities"); err != nil {
		m.logger.Error("Failed to enqueue sync job", "error", err, "athlete_id", athleteID)
		// Don't fail the OAuth flow if sync enqueueing fails
	} else {
		m.logger.Info("Enqueued sync job", "athlete_id", athleteID, "job_type", "list_activities")
	}

	return athleteID, clientID, nil
}

// validateState checks if a state is valid and removes it (one-time use)
// Returns the client ID and whether the state is valid
func (m *Manager) validateState(state string) (string, bool) {
	m.states.mu.Lock()
	defer m.states.mu.Unlock()

	entry, exists := m.states.states[state]
	if !exists {
		return "", false
	}

	// Check if expired
	if time.Now().After(entry.expiry) {
		delete(m.states.states, state)
		return "", false
	}

	// Remove state after use (one-time use)
	clientID := entry.clientID
	delete(m.states.states, state)

	return clientID, true
}

// cleanupStates removes expired states every minute
func (m *Manager) cleanupStates() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		m.states.mu.Lock()
		now := time.Now()
		for state, entry := range m.states.states {
			if now.After(entry.expiry) {
				delete(m.states.states, state)
			}
		}
		m.states.mu.Unlock()
	}
}

// generateRandomState generates a cryptographically secure random state
func generateRandomState() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b), nil
}
