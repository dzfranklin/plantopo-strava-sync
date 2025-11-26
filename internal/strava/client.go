package strava

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"plantopo-strava-sync/internal/config"
	"plantopo-strava-sync/internal/database"
	"plantopo-strava-sync/internal/metrics"
)

const (
	baseURL      = "https://www.strava.com/api/v3"
	tokenURL     = "https://www.strava.com/oauth/token"
	tokenBuffer  = 5 * time.Minute // Refresh tokens 5 minutes before expiry
)

// Client is the Strava API client
type Client struct {
	httpClient *http.Client
	config     *config.Config
	db         *database.DB
	rateLimits *RateLimits
	logger     *slog.Logger
	// Test overrides (empty in production)
	baseURL  string
	tokenURL string
}

// RateLimits tracks Strava API rate limits
// Strava uses two separate rate limit buckets:
// - Overall limits (all requests): 200/15min, 2000/day
// - Read limits (non-upload requests): 100/15min, 1000/day
type RateLimits struct {
	mu                sync.RWMutex
	// Overall limits (X-RateLimit-*)
	overallLimit15Min int
	overallUsage15Min int
	overallLimitDaily int
	overallUsageDaily int
	// Read limits (X-ReadRateLimit-*)
	readLimit15Min    int
	readUsage15Min    int
	readLimitDaily    int
	readUsageDaily    int
	lastUpdated       time.Time
}

// TokenResponse represents the response from Strava's token endpoint
type TokenResponse struct {
	AccessToken  string          `json:"access_token"`
	RefreshToken string          `json:"refresh_token"`
	ExpiresAt    int64           `json:"expires_at"`
	ExpiresIn    int             `json:"expires_in"`
	Athlete      json.RawMessage `json:"athlete"`
}

// NewClient creates a new Strava API client
func NewClient(cfg *config.Config, db *database.DB) *Client {
	return &Client{
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		config: cfg,
		db:     db,
		rateLimits: &RateLimits{
			// Default Strava limits (will be updated from API responses)
			overallLimit15Min: 200,
			overallLimitDaily: 2000,
			readLimit15Min:    100,
			readLimitDaily:    1000,
		},
		logger:   slog.Default(),
		baseURL:  baseURL,
		tokenURL: tokenURL,
	}
}

// SetBaseURL overrides the base URL (for testing)
func (c *Client) SetBaseURL(url string) {
	c.baseURL = url
}

// SetTokenURL overrides the token URL (for testing)
func (c *Client) SetTokenURL(url string) {
	c.tokenURL = url
}

// ExchangeCode exchanges an authorization code for access and refresh tokens
func (c *Client) ExchangeCode(code string, clientID string) (*TokenResponse, error) {
	start := time.Now()

	// Get client-specific credentials
	clientConfig, err := c.config.GetClient(clientID)
	if err != nil {
		return nil, fmt.Errorf("invalid client: %w", err)
	}

	data := url.Values{
		"client_id":     {clientConfig.ClientID},
		"client_secret": {clientConfig.ClientSecret},
		"code":          {code},
		"grant_type":    {"authorization_code"},
	}

	resp, err := c.httpClient.PostForm(c.tokenURL, data)
	if err != nil {
		duration := time.Since(start).Seconds()
		metrics.StravaAPIRequestsTotal.WithLabelValues(metrics.OpExchangeCode, "error").Inc()
		metrics.StravaAPIRequestDuration.WithLabelValues(metrics.OpExchangeCode, "error").Observe(duration)
		return nil, fmt.Errorf("failed to exchange code: %w", err)
	}
	defer resp.Body.Close()

	duration := time.Since(start).Seconds()
	statusCode := strconv.Itoa(resp.StatusCode)
	metrics.StravaAPIRequestsTotal.WithLabelValues(metrics.OpExchangeCode, statusCode).Inc()
	metrics.StravaAPIRequestDuration.WithLabelValues(metrics.OpExchangeCode, statusCode).Observe(duration)

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("token exchange failed with status %d: %s", resp.StatusCode, body)
	}

	var tokenResp TokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return nil, fmt.Errorf("failed to decode token response: %w", err)
	}

	return &tokenResp, nil
}

// refreshToken refreshes an athlete's access token
func (c *Client) refreshToken(athlete *database.Athlete) error {
	start := time.Now()
	c.logger.Info("Refreshing access token", "athlete_id", athlete.AthleteID, "client_id", athlete.ClientID)

	// Get client config from athlete's stored client_id
	clientConfig, err := c.config.GetClient(athlete.ClientID)
	if err != nil {
		return fmt.Errorf("invalid client for athlete: %w", err)
	}

	data := url.Values{
		"client_id":     {clientConfig.ClientID},
		"client_secret": {clientConfig.ClientSecret},
		"refresh_token": {athlete.RefreshToken},
		"grant_type":    {"refresh_token"},
	}

	resp, err := c.httpClient.PostForm(c.tokenURL, data)
	if err != nil {
		duration := time.Since(start).Seconds()
		metrics.StravaAPIRequestsTotal.WithLabelValues(metrics.OpRefreshToken, "error").Inc()
		metrics.StravaAPIRequestDuration.WithLabelValues(metrics.OpRefreshToken, "error").Observe(duration)
		return fmt.Errorf("failed to refresh token: %w", err)
	}
	defer resp.Body.Close()

	duration := time.Since(start).Seconds()
	statusCode := strconv.Itoa(resp.StatusCode)
	metrics.StravaAPIRequestsTotal.WithLabelValues(metrics.OpRefreshToken, statusCode).Inc()
	metrics.StravaAPIRequestDuration.WithLabelValues(metrics.OpRefreshToken, statusCode).Observe(duration)

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("token refresh failed with status %d: %s", resp.StatusCode, body)
	}

	var tokenResp TokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return fmt.Errorf("failed to decode refresh response: %w", err)
	}

	// Update athlete with new tokens
	athlete.AccessToken = tokenResp.AccessToken
	athlete.RefreshToken = tokenResp.RefreshToken
	athlete.TokenExpiresAt = time.Unix(tokenResp.ExpiresAt, 0)
	athlete.UpdatedAt = time.Now()

	if err := c.db.UpsertAthlete(athlete); err != nil {
		return fmt.Errorf("failed to update athlete tokens: %w", err)
	}

	c.logger.Info("Token refreshed successfully", "athlete_id", athlete.AthleteID, "expires_at", athlete.TokenExpiresAt)

	return nil
}

// ensureValidToken ensures the athlete has a valid access token, refreshing if necessary
func (c *Client) ensureValidToken(athleteID int64) (*database.Athlete, error) {
	athlete, err := c.db.GetAthlete(athleteID)
	if err != nil {
		return nil, fmt.Errorf("failed to get athlete: %w", err)
	}

	if athlete == nil {
		return nil, fmt.Errorf("athlete %d not found", athleteID)
	}

	// Check if token needs refresh (expires within 5 minutes)
	if time.Now().Add(tokenBuffer).After(athlete.TokenExpiresAt) {
		if err := c.refreshToken(athlete); err != nil {
			return nil, fmt.Errorf("failed to refresh token: %w", err)
		}
	}

	return athlete, nil
}

// doRequest performs an authenticated request to the Strava API
func (c *Client) doRequest(method, path string, athleteID int64, body io.Reader, operation string) ([]byte, error) {
	start := time.Now()

	athlete, err := c.ensureValidToken(athleteID)
	if err != nil {
		return nil, err
	}

	reqURL := c.baseURL + path
	req, err := http.NewRequest(method, reqURL, body)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+athlete.AccessToken)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		duration := time.Since(start).Seconds()
		metrics.StravaAPIRequestsTotal.WithLabelValues(operation, "error").Inc()
		metrics.StravaAPIRequestDuration.WithLabelValues(operation, "error").Observe(duration)
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	// Update rate limits from response headers
	c.updateRateLimits(resp)

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	// Record metrics
	duration := time.Since(start).Seconds()
	statusCode := strconv.Itoa(resp.StatusCode)
	metrics.StravaAPIRequestsTotal.WithLabelValues(operation, statusCode).Inc()
	metrics.StravaAPIRequestDuration.WithLabelValues(operation, statusCode).Observe(duration)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, &HTTPError{
			StatusCode: resp.StatusCode,
			Body:       string(respBody),
		}
	}

	return respBody, nil
}

// updateRateLimits parses and updates rate limit information from response headers
// Strava provides two separate headers:
// - X-RateLimit-Limit/Usage: Overall limits (200/15min, 2000/day)
// - X-ReadRateLimit-Limit/Usage: Read-only limits (100/15min, 1000/day)
// Format for each: "15min_value,daily_value"
func (c *Client) updateRateLimits(resp *http.Response) {
	// Parse overall limits
	overallUsageHeader := resp.Header.Get("X-RateLimit-Usage")
	overallLimitHeader := resp.Header.Get("X-RateLimit-Limit")

	// Parse read limits
	readUsageHeader := resp.Header.Get("X-ReadRateLimit-Usage")
	readLimitHeader := resp.Header.Get("X-ReadRateLimit-Limit")

	c.rateLimits.mu.Lock()
	defer c.rateLimits.mu.Unlock()

	// Update overall limits if present
	if overallUsageHeader != "" && overallLimitHeader != "" {
		usageParts := strings.Split(overallUsageHeader, ",")
		limitParts := strings.Split(overallLimitHeader, ",")

		if len(usageParts) == 2 && len(limitParts) == 2 {
			c.rateLimits.overallUsage15Min, _ = strconv.Atoi(usageParts[0])
			c.rateLimits.overallUsageDaily, _ = strconv.Atoi(usageParts[1])
			c.rateLimits.overallLimit15Min, _ = strconv.Atoi(limitParts[0])
			c.rateLimits.overallLimitDaily, _ = strconv.Atoi(limitParts[1])
		}
	}

	// Update read limits if present
	if readUsageHeader != "" && readLimitHeader != "" {
		usageParts := strings.Split(readUsageHeader, ",")
		limitParts := strings.Split(readLimitHeader, ",")

		if len(usageParts) == 2 && len(limitParts) == 2 {
			c.rateLimits.readUsage15Min, _ = strconv.Atoi(usageParts[0])
			c.rateLimits.readUsageDaily, _ = strconv.Atoi(usageParts[1])
			c.rateLimits.readLimit15Min, _ = strconv.Atoi(limitParts[0])
			c.rateLimits.readLimitDaily, _ = strconv.Atoi(limitParts[1])
		}
	}

	c.rateLimits.lastUpdated = time.Now()

	// Update Prometheus gauges
	metrics.StravaRateLimitUsage.WithLabelValues(metrics.RateLimitOverall15Min, metrics.BucketLimit).Set(float64(c.rateLimits.overallLimit15Min))
	metrics.StravaRateLimitUsage.WithLabelValues(metrics.RateLimitOverall15Min, metrics.BucketUsage).Set(float64(c.rateLimits.overallUsage15Min))
	metrics.StravaRateLimitUsage.WithLabelValues(metrics.RateLimitOverallDaily, metrics.BucketLimit).Set(float64(c.rateLimits.overallLimitDaily))
	metrics.StravaRateLimitUsage.WithLabelValues(metrics.RateLimitOverallDaily, metrics.BucketUsage).Set(float64(c.rateLimits.overallUsageDaily))
	metrics.StravaRateLimitUsage.WithLabelValues(metrics.RateLimitRead15Min, metrics.BucketLimit).Set(float64(c.rateLimits.readLimit15Min))
	metrics.StravaRateLimitUsage.WithLabelValues(metrics.RateLimitRead15Min, metrics.BucketUsage).Set(float64(c.rateLimits.readUsage15Min))
	metrics.StravaRateLimitUsage.WithLabelValues(metrics.RateLimitReadDaily, metrics.BucketLimit).Set(float64(c.rateLimits.readLimitDaily))
	metrics.StravaRateLimitUsage.WithLabelValues(metrics.RateLimitReadDaily, metrics.BucketUsage).Set(float64(c.rateLimits.readUsageDaily))

	// Calculate usage percentages
	overallPct15Min := float64(c.rateLimits.overallUsage15Min) / float64(c.rateLimits.overallLimit15Min) * 100
	overallPctDaily := float64(c.rateLimits.overallUsageDaily) / float64(c.rateLimits.overallLimitDaily) * 100
	readPct15Min := float64(c.rateLimits.readUsage15Min) / float64(c.rateLimits.readLimit15Min) * 100
	readPctDaily := float64(c.rateLimits.readUsageDaily) / float64(c.rateLimits.readLimitDaily) * 100

	// Log warnings at thresholds
	highUsage := overallPct15Min >= 90 || overallPctDaily >= 90 || readPct15Min >= 90 || readPctDaily >= 90
	approachingLimit := overallPct15Min >= 80 || overallPctDaily >= 80 || readPct15Min >= 80 || readPctDaily >= 80

	if highUsage {
		c.logger.Warn("High rate limit usage",
			"overall_15min", fmt.Sprintf("%d/%d (%.1f%%)", c.rateLimits.overallUsage15Min, c.rateLimits.overallLimit15Min, overallPct15Min),
			"overall_daily", fmt.Sprintf("%d/%d (%.1f%%)", c.rateLimits.overallUsageDaily, c.rateLimits.overallLimitDaily, overallPctDaily),
			"read_15min", fmt.Sprintf("%d/%d (%.1f%%)", c.rateLimits.readUsage15Min, c.rateLimits.readLimit15Min, readPct15Min),
			"read_daily", fmt.Sprintf("%d/%d (%.1f%%)", c.rateLimits.readUsageDaily, c.rateLimits.readLimitDaily, readPctDaily),
		)
	} else if approachingLimit {
		c.logger.Info("Approaching rate limit",
			"overall_15min_pct", fmt.Sprintf("%.1f%%", overallPct15Min),
			"overall_daily_pct", fmt.Sprintf("%.1f%%", overallPctDaily),
			"read_15min_pct", fmt.Sprintf("%.1f%%", readPct15Min),
			"read_daily_pct", fmt.Sprintf("%.1f%%", readPctDaily),
		)
	}
}

// GetRateLimits returns current rate limit information
// Returns: overallUsage15Min, overallLimit15Min, overallUsageDaily, overallLimitDaily,
//          readUsage15Min, readLimit15Min, readUsageDaily, readLimitDaily
func (c *Client) GetRateLimits() (overallUsage15Min, overallLimit15Min, overallUsageDaily, overallLimitDaily,
	readUsage15Min, readLimit15Min, readUsageDaily, readLimitDaily int) {
	c.rateLimits.mu.RLock()
	defer c.rateLimits.mu.RUnlock()

	return c.rateLimits.overallUsage15Min, c.rateLimits.overallLimit15Min,
		c.rateLimits.overallUsageDaily, c.rateLimits.overallLimitDaily,
		c.rateLimits.readUsage15Min, c.rateLimits.readLimit15Min,
		c.rateLimits.readUsageDaily, c.rateLimits.readLimitDaily
}

// HTTPError represents an HTTP error from the Strava API
type HTTPError struct {
	StatusCode int
	Body       string
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("HTTP %d: %s", e.StatusCode, e.Body)
}

// IsNotFound returns true if the error is a 404
func IsNotFound(err error) bool {
	httpErr, ok := err.(*HTTPError)
	return ok && httpErr.StatusCode == http.StatusNotFound
}

// IsUnauthorized returns true if the error is a 401
func IsUnauthorized(err error) bool {
	httpErr, ok := err.(*HTTPError)
	return ok && httpErr.StatusCode == http.StatusUnauthorized
}

// IsTooManyRequests returns true if the error is a 429 (rate limit)
func IsTooManyRequests(err error) bool {
	httpErr, ok := err.(*HTTPError)
	return ok && httpErr.StatusCode == http.StatusTooManyRequests
}
