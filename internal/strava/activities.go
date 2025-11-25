package strava

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
)

// ActivitySummary represents a summary of an activity from list endpoints
type ActivitySummary struct {
	ID int64 `json:"id"`
}

// GetActivity fetches detailed activity data for a specific activity
func (c *Client) GetActivity(athleteID int64, activityID int64) (json.RawMessage, error) {
	path := fmt.Sprintf("/activities/%d", activityID)

	respBody, err := c.doRequest("GET", path, athleteID, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get activity %d: %w", activityID, err)
	}

	return json.RawMessage(respBody), nil
}

// ListActivities fetches a list of activities for an athlete with pagination
// Returns activity IDs and whether there are more pages available
func (c *Client) ListActivities(athleteID int64, page, perPage int) ([]int64, bool, error) {
	if page < 1 {
		page = 1
	}
	if perPage < 1 || perPage > 200 {
		perPage = 200 // Strava max
	}

	params := url.Values{
		"page":     {strconv.Itoa(page)},
		"per_page": {strconv.Itoa(perPage)},
	}

	path := "/athlete/activities?" + params.Encode()

	respBody, err := c.doRequest("GET", path, athleteID, nil)
	if err != nil {
		return nil, false, fmt.Errorf("failed to list activities: %w", err)
	}

	var activities []ActivitySummary
	if err := json.Unmarshal(respBody, &activities); err != nil {
		return nil, false, fmt.Errorf("failed to unmarshal activities: %w", err)
	}

	// Extract activity IDs
	activityIDs := make([]int64, len(activities))
	for i, activity := range activities {
		activityIDs[i] = activity.ID
	}

	// If we got a full page, there might be more
	hasMore := len(activities) == perPage

	return activityIDs, hasMore, nil
}
