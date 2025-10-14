package database

// Schema contains all SQL statements for creating tables and indexes
const Schema = `
-- Athletes table: Stores Strava users who have authorized the application
CREATE TABLE IF NOT EXISTS athletes (
    athlete_id INTEGER PRIMARY KEY,

    -- OAuth tokens
    access_token TEXT NOT NULL,
    refresh_token TEXT NOT NULL,
    expires_at INTEGER NOT NULL,
    scope TEXT NOT NULL,

    -- State tracking
    authorized BOOLEAN NOT NULL DEFAULT 1,
    last_sync_at INTEGER,
    sync_in_progress BOOLEAN NOT NULL DEFAULT 0,
    sync_error TEXT,

    -- Metadata
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,

    -- Optional profile data (JSON)
    profile_json TEXT
);

-- Activities table: Stores activity data at varying levels of detail
CREATE TABLE IF NOT EXISTS activities (
    id INTEGER PRIMARY KEY,  -- Strava activity ID
    athlete_id INTEGER NOT NULL,

    -- Sync state flags
    has_summary BOOLEAN NOT NULL DEFAULT 0,
    has_details BOOLEAN NOT NULL DEFAULT 0,
    deleted BOOLEAN NOT NULL DEFAULT 0,

    -- Activity data (stored as JSON)
    summary_json TEXT,
    details_json TEXT,

    -- Extracted fields for querying and indexing
    start_date INTEGER,  -- Unix timestamp
    activity_type TEXT,  -- e.g., "Run", "Ride", "Swim"

    -- Metadata
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    last_synced_at INTEGER,

    FOREIGN KEY (athlete_id) REFERENCES athletes(athlete_id) ON DELETE CASCADE
);

-- Webhook events table: Raw log of all webhook events received
CREATE TABLE IF NOT EXISTS webhook_events (
    id INTEGER PRIMARY KEY AUTOINCREMENT,

    -- Event metadata
    object_type TEXT NOT NULL,
    object_id INTEGER NOT NULL,
    aspect_type TEXT NOT NULL,
    owner_id INTEGER NOT NULL,
    subscription_id INTEGER NOT NULL,
    event_time INTEGER NOT NULL,

    -- Event data
    updates TEXT,  -- JSON object
    raw_json TEXT NOT NULL,  -- Complete event payload

    -- Processing state
    processed BOOLEAN NOT NULL DEFAULT 0,
    processed_at INTEGER,
    error TEXT,

    -- Metadata
    created_at INTEGER NOT NULL
);

-- Indexes for athletes table
CREATE INDEX IF NOT EXISTS idx_athletes_authorized ON athletes(authorized);
CREATE INDEX IF NOT EXISTS idx_athletes_sync_in_progress ON athletes(sync_in_progress);

-- Indexes for activities table
CREATE INDEX IF NOT EXISTS idx_activities_athlete_id ON activities(athlete_id);
CREATE INDEX IF NOT EXISTS idx_activities_start_date ON activities(start_date DESC);
CREATE INDEX IF NOT EXISTS idx_activities_has_details ON activities(has_details);
CREATE INDEX IF NOT EXISTS idx_activities_deleted ON activities(deleted);
CREATE INDEX IF NOT EXISTS idx_activities_athlete_start ON activities(athlete_id, start_date DESC);
CREATE INDEX IF NOT EXISTS idx_activities_athlete_needs_details ON activities(athlete_id, has_details) WHERE has_details = 0 AND deleted = 0;

-- Indexes for webhook_events table
CREATE INDEX IF NOT EXISTS idx_webhook_events_processed ON webhook_events(processed);
CREATE INDEX IF NOT EXISTS idx_webhook_events_object ON webhook_events(object_type, object_id);
CREATE INDEX IF NOT EXISTS idx_webhook_events_owner ON webhook_events(owner_id);
CREATE INDEX IF NOT EXISTS idx_webhook_events_event_time ON webhook_events(event_time DESC);

-- Unique constraint to prevent duplicate webhook events
CREATE UNIQUE INDEX IF NOT EXISTS idx_webhook_events_unique ON webhook_events(event_time, object_id, aspect_type);
`
