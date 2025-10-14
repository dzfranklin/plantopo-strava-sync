package strava

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	baseURL       = "https://www.strava.com/api/v3"
	tokenURL      = "https://www.strava.com/oauth/token"
	maxRetries    = 5
	initialDelay  = 1 * time.Second
	maxDelay      = 5 * time.Minute
	tokenBuffer   = 5 * time.Minute // Refresh tokens 5 minutes before expiry
)

// Client is a Strava API client
type Client struct {
	httpClient   *http.Client
	clientID     string
	clientSecret string
	logger       *slog.Logger
	rateLimiter  *RateLimiter
}

// TokenRefresher is the interface for refreshing athlete tokens
type TokenRefresher interface {
	UpdateAthleteTokens(athleteID int64, accessToken, refreshToken string, expiresAt int64) error
}

// NewClient creates a new Strava API client
func NewClient(clientID, clientSecret string, logger *slog.Logger) *Client {
	return &Client{
		httpClient:   &http.Client{Timeout: 30 * time.Second},
		clientID:     clientID,
		clientSecret: clientSecret,
		logger:       logger,
		rateLimiter:  NewRateLimiter(),
	}
}

// TokenResponse represents the response from a token exchange or refresh
type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresAt    int64  `json:"expires_at"`
	ExpiresIn    int    `json:"expires_in"`
}

// ExchangeToken exchanges an authorization code for access and refresh tokens
func (c *Client) ExchangeToken(ctx context.Context, code string) (*TokenResponse, error) {
	data := map[string]string{
		"client_id":     c.clientID,
		"client_secret": c.clientSecret,
		"code":          code,
		"grant_type":    "authorization_code",
	}

	body, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", tokenURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	start := time.Now()
	resp, err := c.httpClient.Do(req)
	duration := time.Since(start)

	if err != nil {
		c.logger.Error("token exchange failed", "error", err, "duration_ms", duration.Milliseconds())
		return nil, fmt.Errorf("token exchange failed: %w", err)
	}
	defer resp.Body.Close()

	c.logger.Info("token_exchange", "status", resp.StatusCode, "duration_ms", duration.Milliseconds())

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("token exchange failed with status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var tokenResp TokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &tokenResp, nil
}

// RefreshToken refreshes an access token using a refresh token
func (c *Client) RefreshToken(ctx context.Context, refreshToken string) (*TokenResponse, error) {
	data := map[string]string{
		"client_id":     c.clientID,
		"client_secret": c.clientSecret,
		"refresh_token": refreshToken,
		"grant_type":    "refresh_token",
	}

	body, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", tokenURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	start := time.Now()
	resp, err := c.httpClient.Do(req)
	duration := time.Since(start)

	if err != nil {
		c.logger.Error("token refresh failed", "error", err, "duration_ms", duration.Milliseconds())
		return nil, fmt.Errorf("token refresh failed: %w", err)
	}
	defer resp.Body.Close()

	c.logger.Info("token_refresh", "status", resp.StatusCode, "duration_ms", duration.Milliseconds())

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("token refresh failed with status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var tokenResp TokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &tokenResp, nil
}

// doRequest performs an HTTP request with automatic token refresh and retries
func (c *Client) doRequest(ctx context.Context, method, path, accessToken string, athleteID int64, expiresAt int64, refreshToken string, refresher TokenRefresher) (*http.Response, error) {
	// Check if token needs refresh
	if time.Now().Unix()+int64(tokenBuffer.Seconds()) >= expiresAt && refresher != nil {
		c.logger.Info("refreshing token", "athlete_id", athleteID)
		tokenResp, err := c.RefreshToken(ctx, refreshToken)
		if err != nil {
			return nil, fmt.Errorf("failed to refresh token: %w", err)
		}

		// Update tokens in database
		if err := refresher.UpdateAthleteTokens(athleteID, tokenResp.AccessToken, tokenResp.RefreshToken, tokenResp.ExpiresAt); err != nil {
			c.logger.Error("failed to update tokens", "athlete_id", athleteID, "error", err)
			return nil, fmt.Errorf("failed to update tokens: %w", err)
		}

		accessToken = tokenResp.AccessToken
	}

	var lastErr error
	delay := initialDelay

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			c.logger.Info("retrying request", "attempt", attempt, "delay_ms", delay.Milliseconds())
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(delay):
			}
			delay = min(delay*2, maxDelay)
		}

		req, err := http.NewRequestWithContext(ctx, method, baseURL+path, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to create request: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+accessToken)

		start := time.Now()
		resp, err := c.httpClient.Do(req)
		duration := time.Since(start)

		if err != nil {
			lastErr = err
			c.logger.Error("request failed", "method", method, "path", path, "error", err, "attempt", attempt)
			continue
		}

		// Parse rate limit headers
		c.parseRateLimitHeaders(resp.Header)

		c.logger.Info("strava_api_request", "method", method, "path", path, "status", resp.StatusCode, "duration_ms", duration.Milliseconds(), "athlete_id", athleteID)

		// Handle different status codes
		switch {
		case resp.StatusCode == http.StatusOK:
			return resp, nil
		case resp.StatusCode == http.StatusTooManyRequests:
			resp.Body.Close()
			retryAfter := c.parseRetryAfter(resp.Header)
			if retryAfter > 0 {
				delay = retryAfter
			}
			lastErr = fmt.Errorf("rate limited (429)")
			continue
		case resp.StatusCode >= 500:
			resp.Body.Close()
			lastErr = fmt.Errorf("server error (%d)", resp.StatusCode)
			continue
		case resp.StatusCode == http.StatusUnauthorized:
			resp.Body.Close()
			return nil, fmt.Errorf("unauthorized (401) - token may be invalid")
		case resp.StatusCode == http.StatusForbidden:
			resp.Body.Close()
			return nil, fmt.Errorf("forbidden (403) - insufficient permissions")
		case resp.StatusCode == http.StatusNotFound:
			resp.Body.Close()
			return nil, fmt.Errorf("not found (404)")
		default:
			bodyBytes, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			return nil, fmt.Errorf("request failed with status %d: %s", resp.StatusCode, string(bodyBytes))
		}
	}

	return nil, fmt.Errorf("max retries exceeded: %w", lastErr)
}

// parseRateLimitHeaders extracts and updates rate limit information from response headers
func (c *Client) parseRateLimitHeaders(headers http.Header) {
	limitHeader := headers.Get("X-RateLimit-Limit")
	usageHeader := headers.Get("X-RateLimit-Usage")

	if limitHeader != "" && usageHeader != "" {
		limits := strings.Split(limitHeader, ",")
		usages := strings.Split(usageHeader, ",")

		if len(limits) == 2 && len(usages) == 2 {
			limit15, _ := strconv.Atoi(strings.TrimSpace(limits[0]))
			limitDaily, _ := strconv.Atoi(strings.TrimSpace(limits[1]))
			usage15, _ := strconv.Atoi(strings.TrimSpace(usages[0]))
			usageDaily, _ := strconv.Atoi(strings.TrimSpace(usages[1]))

			c.rateLimiter.Update(limit15, usage15, limitDaily, usageDaily)

			c.logger.Debug("rate_limit",
				"limit_15min", limit15,
				"usage_15min", usage15,
				"limit_daily", limitDaily,
				"usage_daily", usageDaily,
				"usage_15min_pct", float64(usage15)/float64(limit15)*100,
				"usage_daily_pct", float64(usageDaily)/float64(limitDaily)*100,
			)
		}
	}
}

// parseRetryAfter extracts retry delay from Retry-After header
func (c *Client) parseRetryAfter(headers http.Header) time.Duration {
	retryAfter := headers.Get("Retry-After")
	if retryAfter == "" {
		return 0
	}

	seconds, err := strconv.Atoi(retryAfter)
	if err != nil {
		return 0
	}

	return time.Duration(seconds) * time.Second
}

// GetRateLimitStatus returns the current rate limit status
func (c *Client) GetRateLimitStatus() RateLimitStatus {
	return c.rateLimiter.Status()
}

func min(a, b time.Duration) time.Duration {
	if a < b {
		return a
	}
	return b
}
