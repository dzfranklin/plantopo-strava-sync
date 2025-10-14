package strava

import (
	"log/slog"
	"os"
	"testing"
)

func TestNewClient(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	client := NewClient("test_client_id", "test_client_secret", logger)

	if client == nil {
		t.Fatal("Expected client to be created")
	}

	if client.clientID != "test_client_id" {
		t.Errorf("Expected clientID 'test_client_id', got %s", client.clientID)
	}

	if client.clientSecret != "test_client_secret" {
		t.Errorf("Expected clientSecret 'test_client_secret', got %s", client.clientSecret)
	}

	if client.httpClient == nil {
		t.Error("Expected httpClient to be initialized")
	}

	if client.rateLimiter == nil {
		t.Error("Expected rateLimiter to be initialized")
	}

	if client.logger == nil {
		t.Error("Expected logger to be set")
	}
}

func TestClientGetRateLimitStatus(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	client := NewClient("test_client_id", "test_client_secret", logger)

	// Update rate limiter
	client.rateLimiter.Update(200, 50, 2000, 500)

	status := client.GetRateLimitStatus()

	if status.Usage15Min != 50 {
		t.Errorf("Expected usage15Min 50, got %d", status.Usage15Min)
	}

	if status.UsageDaily != 500 {
		t.Errorf("Expected usageDaily 500, got %d", status.UsageDaily)
	}
}

func TestParseRateLimitHeaders(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	client := NewClient("test_client_id", "test_client_secret", logger)

	// Create mock headers
	headers := make(map[string][]string)
	headers["X-Ratelimit-Limit"] = []string{"200,2000"}
	headers["X-Ratelimit-Usage"] = []string{"50,500"}

	client.parseRateLimitHeaders(headers)

	status := client.GetRateLimitStatus()

	if status.Limit15Min != 200 {
		t.Errorf("Expected limit15Min 200, got %d", status.Limit15Min)
	}

	if status.Usage15Min != 50 {
		t.Errorf("Expected usage15Min 50, got %d", status.Usage15Min)
	}

	if status.LimitDaily != 2000 {
		t.Errorf("Expected limitDaily 2000, got %d", status.LimitDaily)
	}

	if status.UsageDaily != 500 {
		t.Errorf("Expected usageDaily 500, got %d", status.UsageDaily)
	}
}
