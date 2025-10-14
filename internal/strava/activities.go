package strava

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
)

// GetActivities fetches a page of activities for an athlete
func (c *Client) GetActivities(ctx context.Context, accessToken string, athleteID int64, expiresAt int64, refreshToken string, refresher TokenRefresher, page, perPage int) ([]byte, error) {
	path := fmt.Sprintf("/athlete/activities?page=%d&per_page=%d", page, perPage)

	resp, err := c.doRequest(ctx, "GET", path, accessToken, athleteID, expiresAt, refreshToken, refresher)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	return body, nil
}

// GetActivity fetches detailed information for a single activity
func (c *Client) GetActivity(ctx context.Context, activityID int64, accessToken string, athleteID int64, expiresAt int64, refreshToken string, refresher TokenRefresher) ([]byte, error) {
	path := fmt.Sprintf("/activities/%d", activityID)

	resp, err := c.doRequest(ctx, "GET", path, accessToken, athleteID, expiresAt, refreshToken, refresher)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	return body, nil
}

// ParseActivitiesSummary parses a list of activities and returns basic info
func ParseActivitiesSummary(data []byte) ([]ActivitySummary, error) {
	var activities []ActivitySummary
	if err := json.Unmarshal(data, &activities); err != nil {
		return nil, fmt.Errorf("failed to parse activities: %w", err)
	}
	return activities, nil
}

// ActivitySummary represents basic activity information
type ActivitySummary struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Type      string `json:"type"`
	StartDate string `json:"start_date"`
}

// ParseActivity extracts the activity ID from detailed activity data
func ParseActivity(data []byte) (int64, error) {
	var activity struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal(data, &activity); err != nil {
		return 0, fmt.Errorf("failed to parse activity: %w", err)
	}
	return activity.ID, nil
}
