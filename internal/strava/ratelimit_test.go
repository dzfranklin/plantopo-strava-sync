package strava

import (
	"testing"
	"time"
)

func TestRateLimiterUpdate(t *testing.T) {
	rl := NewRateLimiter()

	// Update with real values
	rl.Update(200, 50, 2000, 500)

	status := rl.Status()

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

	// Check percentages
	if status.Usage15MinPct != 25.0 {
		t.Errorf("Expected usage15MinPct 25.0, got %f", status.Usage15MinPct)
	}
	if status.UsageDailyPct != 25.0 {
		t.Errorf("Expected usageDailyPct 25.0, got %f", status.UsageDailyPct)
	}
}

func TestRateLimiterDefaults(t *testing.T) {
	rl := NewRateLimiter()
	status := rl.Status()

	if status.Limit15Min != 200 {
		t.Errorf("Expected default limit15Min 200, got %d", status.Limit15Min)
	}
	if status.LimitDaily != 2000 {
		t.Errorf("Expected default limitDaily 2000, got %d", status.LimitDaily)
	}
	if status.Usage15Min != 0 {
		t.Errorf("Expected default usage15Min 0, got %d", status.Usage15Min)
	}
	if status.UsageDaily != 0 {
		t.Errorf("Expected default usageDaily 0, got %d", status.UsageDaily)
	}
}

func TestRateLimiterIsNearLimit(t *testing.T) {
	rl := NewRateLimiter()

	// Update with low usage
	rl.Update(200, 50, 2000, 500)
	if rl.IsNearLimit(80) {
		t.Error("Expected IsNearLimit(80) to be false at 25% usage")
	}

	// Update with high usage
	rl.Update(200, 180, 2000, 1800)
	if !rl.IsNearLimit(80) {
		t.Error("Expected IsNearLimit(80) to be true at 90% usage")
	}

	// Test daily limit threshold
	rl.Update(200, 50, 2000, 1900)
	if !rl.IsNearLimit(90) {
		t.Error("Expected IsNearLimit(90) to be true when daily at 95%")
	}
}

func TestRateLimiterLastUpdated(t *testing.T) {
	rl := NewRateLimiter()

	before := time.Now()
	time.Sleep(10 * time.Millisecond)
	rl.Update(200, 50, 2000, 500)
	time.Sleep(10 * time.Millisecond)
	after := time.Now()

	status := rl.Status()

	if status.LastUpdated.Before(before) || status.LastUpdated.After(after) {
		t.Error("LastUpdated timestamp not within expected range")
	}
}

func TestRateLimiterConcurrency(t *testing.T) {
	rl := NewRateLimiter()

	// Run concurrent updates and reads
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				rl.Update(200, j, 2000, j*10)
				_ = rl.Status()
				_ = rl.IsNearLimit(80)
			}
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	// Should complete without data races
}
