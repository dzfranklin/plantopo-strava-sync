package strava

import (
	"testing"
)

func TestParseActivitiesSummary(t *testing.T) {
	data := []byte(`[
		{
			"id": 12345,
			"name": "Morning Run",
			"type": "Run",
			"start_date": "2025-10-14T06:00:00Z"
		},
		{
			"id": 67890,
			"name": "Evening Ride",
			"type": "Ride",
			"start_date": "2025-10-14T18:00:00Z"
		}
	]`)

	activities, err := ParseActivitiesSummary(data)
	if err != nil {
		t.Fatalf("Failed to parse activities: %v", err)
	}

	if len(activities) != 2 {
		t.Errorf("Expected 2 activities, got %d", len(activities))
	}

	if activities[0].ID != 12345 {
		t.Errorf("Expected ID 12345, got %d", activities[0].ID)
	}
	if activities[0].Name != "Morning Run" {
		t.Errorf("Expected name 'Morning Run', got %s", activities[0].Name)
	}
	if activities[0].Type != "Run" {
		t.Errorf("Expected type 'Run', got %s", activities[0].Type)
	}

	if activities[1].ID != 67890 {
		t.Errorf("Expected ID 67890, got %d", activities[1].ID)
	}
}

func TestParseActivitiesSummaryEmpty(t *testing.T) {
	data := []byte(`[]`)

	activities, err := ParseActivitiesSummary(data)
	if err != nil {
		t.Fatalf("Failed to parse empty activities: %v", err)
	}

	if len(activities) != 0 {
		t.Errorf("Expected 0 activities, got %d", len(activities))
	}
}

func TestParseActivitiesSummaryInvalid(t *testing.T) {
	data := []byte(`{"invalid": "json"}`)

	_, err := ParseActivitiesSummary(data)
	if err == nil {
		t.Error("Expected error for invalid JSON, got nil")
	}
}

func TestParseActivity(t *testing.T) {
	data := []byte(`{
		"id": 98765,
		"name": "Test Activity",
		"type": "Run",
		"distance": 5000
	}`)

	id, err := ParseActivity(data)
	if err != nil {
		t.Fatalf("Failed to parse activity: %v", err)
	}

	if id != 98765 {
		t.Errorf("Expected ID 98765, got %d", id)
	}
}

func TestParseActivityInvalid(t *testing.T) {
	data := []byte(`invalid json`)

	_, err := ParseActivity(data)
	if err == nil {
		t.Error("Expected error for invalid JSON, got nil")
	}
}

func TestParseActivityMissingID(t *testing.T) {
	data := []byte(`{"name": "Test Activity"}`)

	id, err := ParseActivity(data)
	if err != nil {
		t.Fatalf("Failed to parse activity: %v", err)
	}

	if id != 0 {
		t.Errorf("Expected ID 0 for missing ID, got %d", id)
	}
}
