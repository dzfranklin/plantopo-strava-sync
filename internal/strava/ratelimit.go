package strava

import (
	"sync"
	"time"
)

// RateLimiter tracks Strava API rate limits
type RateLimiter struct {
	mu           sync.RWMutex
	limit15Min   int
	usage15Min   int
	limitDaily   int
	usageDaily   int
	lastUpdated  time.Time
}

// RateLimitStatus represents the current rate limit status
type RateLimitStatus struct {
	Limit15Min      int
	Usage15Min      int
	LimitDaily      int
	UsageDaily      int
	Usage15MinPct   float64
	UsageDailyPct   float64
	LastUpdated     time.Time
}

// NewRateLimiter creates a new rate limiter
func NewRateLimiter() *RateLimiter {
	return &RateLimiter{
		// Default Strava limits
		limit15Min: 200,
		limitDaily: 2000,
	}
}

// Update updates the rate limit information
func (rl *RateLimiter) Update(limit15Min, usage15Min, limitDaily, usageDaily int) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	rl.limit15Min = limit15Min
	rl.usage15Min = usage15Min
	rl.limitDaily = limitDaily
	rl.usageDaily = usageDaily
	rl.lastUpdated = time.Now()
}

// Status returns the current rate limit status
func (rl *RateLimiter) Status() RateLimitStatus {
	rl.mu.RLock()
	defer rl.mu.RUnlock()

	usage15MinPct := 0.0
	if rl.limit15Min > 0 {
		usage15MinPct = float64(rl.usage15Min) / float64(rl.limit15Min) * 100
	}

	usageDailyPct := 0.0
	if rl.limitDaily > 0 {
		usageDailyPct = float64(rl.usageDaily) / float64(rl.limitDaily) * 100
	}

	return RateLimitStatus{
		Limit15Min:    rl.limit15Min,
		Usage15Min:    rl.usage15Min,
		LimitDaily:    rl.limitDaily,
		UsageDaily:    rl.usageDaily,
		Usage15MinPct: usage15MinPct,
		UsageDailyPct: usageDailyPct,
		LastUpdated:   rl.lastUpdated,
	}
}

// IsNearLimit returns true if we're approaching rate limits
func (rl *RateLimiter) IsNearLimit(threshold float64) bool {
	status := rl.Status()
	return status.Usage15MinPct >= threshold || status.UsageDailyPct >= threshold
}
