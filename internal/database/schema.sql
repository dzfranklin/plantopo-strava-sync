PRAGMA journal_mode=WAL;

-- Athletes table stores authentication tokens and athlete data
CREATE TABLE IF NOT EXISTS athletes (
    athlete_id INTEGER PRIMARY KEY,
    access_token TEXT NOT NULL,
    refresh_token TEXT NOT NULL,
    token_expires_at INTEGER NOT NULL, -- Unix timestamp
    athlete_summary TEXT NOT NULL, -- JSON blob of athlete summary from Strava
    created_at INTEGER NOT NULL DEFAULT (unixepoch()), -- Unix timestamp
    updated_at INTEGER NOT NULL DEFAULT (unixepoch()) -- Unix timestamp
);

-- Webhook queue for events pending hydration
-- Events do not appear in the events table until they have been hydrated
CREATE TABLE IF NOT EXISTS webhook_queue (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    data TEXT NOT NULL -- JSON blob containing webhook event data
);

-- Events table stores the event stream
-- Supports two event types:
--   1. athlete_connected: When an athlete authorizes the app
--   2. webhook: Activity updates from Strava webhooks
CREATE TABLE IF NOT EXISTS events (
    event_id INTEGER PRIMARY KEY AUTOINCREMENT,
    event_type TEXT NOT NULL CHECK(event_type IN ('athlete_connected', 'webhook')),
    athlete_id INTEGER NOT NULL,

    -- For webhook events with activities
    activity_id INTEGER,

    -- JSON data fields (nullable based on event type)
    athlete_summary TEXT, -- JSON: For athlete_connected events
    activity TEXT, -- JSON: For webhook events (detailed activity from API)
    webhook_event TEXT, -- JSON: For webhook events (raw webhook data)

    created_at INTEGER NOT NULL DEFAULT (unixepoch()), -- Unix timestamp

    FOREIGN KEY (athlete_id) REFERENCES athletes(athlete_id) ON DELETE CASCADE
);

-- Index for cursor-based pagination (events are ordered by event_id)
CREATE INDEX IF NOT EXISTS idx_events_event_id ON events(event_id);

-- Index for efficient athlete event lookups and deletion
CREATE INDEX IF NOT EXISTS idx_events_athlete_id ON events(athlete_id);

-- Index for webhook event queries by activity
CREATE INDEX IF NOT EXISTS idx_events_activity_id ON events(activity_id) WHERE activity_id IS NOT NULL;

-- Composite index for event type filtering with pagination
CREATE INDEX IF NOT EXISTS idx_events_type_id ON events(event_type, event_id);
