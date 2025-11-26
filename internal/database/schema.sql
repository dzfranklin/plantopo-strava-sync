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
    data TEXT NOT NULL, -- JSON blob containing webhook event data
    retry_count INTEGER NOT NULL DEFAULT 0,
    last_error TEXT,
    next_retry_at INTEGER, -- Unix timestamp, NULL = process immediately
    processing_started_at INTEGER -- Unix timestamp, NULL = not currently processing
);

-- Index for efficient retry scheduling and claiming
CREATE INDEX IF NOT EXISTS idx_webhook_queue_ready ON webhook_queue(next_retry_at, processing_started_at);

-- Sync jobs queue for background sync operations
-- Separate from webhook_queue to avoid mixing real webhooks with synthetic sync jobs
CREATE TABLE IF NOT EXISTS sync_jobs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    athlete_id INTEGER NOT NULL,
    job_type TEXT NOT NULL DEFAULT 'sync_all_activities', -- Job types: 'list_activities', 'sync_activity'
    activity_id INTEGER, -- For sync_activity jobs
    retry_count INTEGER NOT NULL DEFAULT 0,
    last_error TEXT,
    next_retry_at INTEGER, -- Unix timestamp, NULL = process immediately
    processing_started_at INTEGER, -- Unix timestamp, NULL = not currently processing
    created_at INTEGER NOT NULL DEFAULT (unixepoch()),
    FOREIGN KEY (athlete_id) REFERENCES athletes(athlete_id) ON DELETE CASCADE
);

-- Index for efficient retry scheduling and claiming
CREATE INDEX IF NOT EXISTS idx_sync_jobs_ready ON sync_jobs(next_retry_at, processing_started_at);

-- Index for athlete lookups
CREATE INDEX IF NOT EXISTS idx_sync_jobs_athlete_id ON sync_jobs(athlete_id);

-- Events table stores the event stream
-- Supports event types:
--   1. athlete_connected: When an athlete authorizes the app
--   2. webhook: Activity events from Strava webhooks (create/update/delete)
--   3. backfill: Historical activity from backfill sync
CREATE TABLE IF NOT EXISTS events (
    event_id INTEGER PRIMARY KEY AUTOINCREMENT,
    event_type TEXT NOT NULL CHECK(event_type IN ('athlete_connected', 'webhook', 'backfill')),
    athlete_id INTEGER NOT NULL,

    -- For webhook and backfill events with activities
    activity_id INTEGER,

    -- JSON data fields (nullable based on event type)
    athlete_summary TEXT, -- JSON: For athlete_connected events
    activity TEXT, -- JSON: For webhook and backfill events (detailed activity from API)
    webhook_event TEXT, -- JSON: For webhook events only (raw webhook data)

    created_at INTEGER NOT NULL DEFAULT (unixepoch()) -- Unix timestamp
);

-- Index for cursor-based pagination (events are ordered by event_id)
CREATE INDEX IF NOT EXISTS idx_events_event_id ON events(event_id);

-- Index for efficient athlete event lookups and deletion
CREATE INDEX IF NOT EXISTS idx_events_athlete_id ON events(athlete_id);

-- Index for webhook event queries by activity
CREATE INDEX IF NOT EXISTS idx_events_activity_id ON events(activity_id) WHERE activity_id IS NOT NULL;

-- Composite index for event type filtering with pagination
CREATE INDEX IF NOT EXISTS idx_events_type_id ON events(event_type, event_id);
