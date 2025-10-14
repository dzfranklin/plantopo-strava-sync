# plantopo-strava-sync

Syncs activity data from Strava to a SQLite database, maintaining a complete local copy of user activities.

## Overview

The main app (implemented elsewhere) has a "Connect with Strava" button that initiates an OAuth flow with redirect URL `https://connect-with-strava.plantopo.com/connect` requesting the `activity:read_all` scope.

On receiving the OAuth callback, this service:
1. Exchanges authorization code for access/refresh tokens
2. Inserts the user and tokens into the database
3. Kicks off a background sync to fetch all activities

The service consists of two components:
- **HTTP Server**: Handles OAuth callbacks, webhook events, and internal API endpoints
- **CLI Tool**: Manages webhook subscriptions (list, add, remove)

Build binaries to bin directory in project root

## Data Model

### Athletes

Athlete records represent Strava users who have authorized the application via OAuth.

**Core Fields**:
- `athlete_id` (INTEGER PRIMARY KEY): Strava's unique athlete identifier
- `access_token` (TEXT): OAuth access token for API requests (expires every 6 hours)
- `refresh_token` (TEXT): OAuth refresh token for obtaining new access tokens
- `expires_at` (INTEGER): Unix timestamp when access_token expires
- `scope` (TEXT): Granted OAuth scopes (e.g., "activity:read_all")

**State Tracking**:
- `authorized` (BOOLEAN): Whether athlete has authorized the app (false if deauthorized)
- `last_sync_at` (INTEGER): Unix timestamp of last successful full sync
- `sync_in_progress` (BOOLEAN): Whether a sync is currently running for this athlete
- `sync_error` (TEXT): Last sync error message, if any

**Metadata**:
- `created_at` (INTEGER): Unix timestamp when athlete first connected
- `updated_at` (INTEGER): Unix timestamp of last record update

**Optional Profile Data** (stored as JSON TEXT, cleared on deauthorization):
- `profile_json` (TEXT): Raw athlete profile data from Strava (name, photo, etc.)

**Token Refresh Logic**:
Before making any API request, check if `expires_at` is within 5 minutes of current time. If so, use `refresh_token` to obtain new credentials and update `access_token`, `refresh_token`, and `expires_at`.

**Deauthorization Handling**:
When athlete deauthorization webhook is received:
1. Set `authorized = false`
2. Clear `access_token` and `refresh_token`
3. Clear `profile_json`
4. Keep `athlete_id` for referential integrity with activities
5. Mark all activities as belonging to deauthorized athlete, clear personal data in activity records

### Activities

Activities are stored at three levels of detail, progressively fetched to balance completeness with API rate limits:

1. **Minimal** (from webhooks)
   - Source: Webhook event payloads
   - Contains: `object_id`, `aspect_type` (create/update/delete), `updates`, `owner_id`, `event_time`
   - Use: Real-time activity change notifications

2. **Summary** (from `/athlete/activities`)
   - Source: Activity list endpoint (paginated, specify page size of 200 for max efficiency)
   - Contains: Core activity metadata (name, type, distance, duration, start date, etc.)
   - Use: Complete activity inventory with basic stats

3. **Detailed** (from `/activities/{id}`)
   - Source: Individual activity detail endpoint
   - Contains: Full activity data including splits, laps, segment efforts, photos, etc.
   - Use: Complete activity records for analysis

**Goal**: Maintain full details for every activity, but prioritize having a complete list of activities before filling in full details.

## Sync Logic

### Full Sync
Iterates through paginated activity list from Strava and inserts/updates each in the database.

**Parameters**:
- `force` (boolean): If true, re-fetches summary data for existing activities and clears detail data to be refetched

**Triggers**:
- When a user completes OAuth connection (automatic): force=false
- Manual trigger via internal API endpoint `/api/sync/{athlete_id}`: force can be requested

**Process**:
1. Fetch `/athlete/activities` with pagination (use max page size allowed for efficiency)
2. For each activity: Insert or update summary data
3. If `force=true`: Clear existing detail data to force refetch

### Webhook Sync
Processes real-time activity events from Strava webhooks.

**Triggers**:
- Webhook POST to `/push` endpoint

**Process**:
1. Receive webhook event (create/update/delete)
2. Insert minimal metadata from event
3. Trigger detail fetch for the specific activity
4. For deletions: Mark activity as deleted in database, clear all personal info
5. For athlete deauthorizations: Mark all activities in database, mark athlete as deauthorized, clear all personal info

### Detail Fetch (Background Task)
Continuously fills in missing activity details, prioritizing newest activities.

**Triggers**:
- Timer-based (periodic background job)
- After webhook events
- After full sync completion

**Process**:
1. Query database for activities without detail data, sorted by date descending, with a reasonable limit
2. Fetch `/activities/{id}` for each
3. Respect rate limits with exponential backoff
4. Store complete JSON response
5. Repeat until query returns no results

## Stack

- **Go**: Primary language
- **SQLite**: Database (`modernc.org/sqlite` - pure Go, no CGo)
- **golang.org/x/oauth2**: OAuth 2.0 client
- **stdlib net/http**: HTTP server
- **stdlib log/slog**: Structured logging

## Architecture

### Deployment

Simple deployment to Ubuntu server accessible at `connect-with-strava.plantopo.com`.

**Infrastructure**:
- Caddy reverse proxy (handles TLS, proxies to app)
- Server access: `ssh app@pt0`
- App listens on `localhost:4101`

**Caddyfile**:
```
connect-with-strava.plantopo.com {
    reverse_proxy localhost:4101
}
```

### Data Storage Strategy

**Store raw JSON from Strava API with minimal parsing.**

Only extract fields needed for:
- Indexing (activity ID, athlete ID)
- Querying (timestamps, activity types)
- Sync state tracking (detail_fetched, last_updated)

Store complete JSON responses in TEXT columns for future-proofing and flexibility.

### Database Schema

**Tables**:
- `athletes`: User records with OAuth tokens
- `activities`: Activity records with sync state tracking
- `webhook_events`: Raw webhook event log

**Key Fields**:
- Token expiration tracking for refresh logic
- Sync state flags: `has_summary`, `has_details`, `deleted`
- Timestamps: `created_at`, `updated_at`, `last_synced_at`

### Testing

Prefer integration-style testing over mocking.

**Approach**:
- Use temporary SQLite databases (`:memory:` or temp files)
- Test against real database operations
- Mock only external HTTP calls to Strava when necessary
- Test OAuth flow, sync logic, and webhook handling end-to-end

## API Integration

### OAuth 2.0 Flow

**Authorization Request**:
```
GET https://www.strava.com/oauth/authorize
  ?client_id={CLIENT_ID}
  &redirect_uri=https://connect-with-strava.plantopo.com/connect
  &response_type=code
  &scope=activity:read_all
```

**Scopes**:
- `activity:read_all`: Read all activities (including private)
- Alternative: `activity:read` for public activities only

**Token Exchange**:
```
POST https://www.strava.com/api/v3/oauth/token
  client_id={CLIENT_ID}
  client_secret={CLIENT_SECRET}
  code={AUTHORIZATION_CODE}
  grant_type=authorization_code
```

**Response**:
- `access_token`: Valid for 6 hours
- `refresh_token`: Use to obtain new access token
- `expires_at`: Unix timestamp of token expiration

**Token Refresh** (required every 6 hours):
```
POST https://www.strava.com/api/v3/oauth/token
  client_id={CLIENT_ID}
  client_secret={CLIENT_SECRET}
  refresh_token={REFRESH_TOKEN}
  grant_type=refresh_token
```

### Webhook Subscription

The server expects a webhook subscription pointing to `https://connect-with-strava.plantopo.com/push`.

**Subscription Creation** (via CLI):
```bash
./cli webhook create \
  --callback-url https://connect-with-strava.plantopo.com/push
```

**Callback Validation**:
Strava validates the callback URL during subscription creation:
1. Strava sends GET request with `hub.mode`, `hub.verify_token`, `hub.challenge`
2. Server must respond within 2 seconds with 200 status
3. Response body: `{"hub.challenge": "{challenge_string}"}`

**Event Types**:
- `create`: New activity created
- `update`: Activity modified (title, type, privacy)
- `delete`: Activity deleted
- Athlete deauthorization events

**Event Payload**:
```json
{
  "object_type": "activity",
  "object_id": 12345,
  "aspect_type": "create",
  "updates": {},
  "owner_id": 67890,
  "subscription_id": 123,
  "event_time": 1234567890
}
```

### Key API Endpoints

**List Activities** (paginated):
```
GET /athlete/activities
  ?page={page}
  &per_page={per_page}
  &before={epoch}
  &after={epoch}
```
- Default: 30 activities per page
- Returns: Array of SummaryActivity objects

**Get Activity Details**:
```
GET /activities/{id}
```
- Returns: DetailedActivity object with full data

### Rate Limits

**Current Limits** (as of 2025):
- **200 requests per 15 minutes** (burst capacity)
- **2,000 requests per day** (sustained capacity)

**Design Target**: Support up to 100 users within these limits

**Rate Limit Handling**:
- Monitor rate limit headers: `X-RateLimit-Limit`, `X-RateLimit-Usage`
- Implement exponential backoff on 429 responses
- Queue detail fetches to stay within daily budget
- Prioritize webhook-triggered fetches over background tasks

**Rate Limit Headers**:
```
X-RateLimit-Limit: 200,2000
X-RateLimit-Usage: 50,1234
```
Format: `15min_used,daily_used` / `15min_limit,daily_limit`

### Strava API Pagination

Requests that return multiple items will be paginated to 30 items by default.
The page parameter can be used to specify further pages or offsets. The 
per_page may also be used for custom page sizes up to 200. Note that in 
certain cases, the number of items returned in the response may be lower 
than the requested page size, even when that page is not the last. If you 
need to fully go through the full set of results, prefer iterating until an 
empty page is returned.


## Configuration

All configuration via environment variables:

**Required**:
- `STRAVA_CLIENT_ID`: OAuth client ID from Strava app settings
- `STRAVA_CLIENT_SECRET`: OAuth client secret
- `INTERNAL_API_KEY`: Bearer token for internal API endpoints

**Optional**:
- `PORT`: HTTP server port (default: 4101)
- `HOST`: Server bind address (default: localhost)
- `DATABASE_PATH`: SQLite database file path (default: ./data.db)
- `LOG_LEVEL`: Logging level (default: info)

See `.env.example` for complete reference.

## Internal API

Protected by bearer token authentication (`Authorization: Bearer {INTERNAL_API_KEY}`).

**Endpoints**:

```
POST /api/sync/{athlete_id}
  ?force=true|false
```
Trigger full sync for athlete.

```
GET /api/athletes/{athlete_id}
```
Get athlete info and sync status.

```
GET /api/activities/{athlete_id}
  ?page={page}
  &limit={limit}
```
List activities for athlete.

## Logging

Uses `log/slog` for structured logging.

**Logged Operations**:
- All HTTP requests to Strava API (method, URL, response status, duration)
- OAuth token exchanges and refreshes
- Webhook events received
- Sync operations (start, progress, completion, errors)
- Rate limit tracking

**Log Format**: JSON for production, text for development

**Example**:
```json
{
  "time": "2025-10-13T12:34:56Z",
  "level": "INFO",
  "msg": "strava_api_request",
  "method": "GET",
  "url": "/athlete/activities",
  "status": 200,
  "duration_ms": 245,
  "athlete_id": 12345
}
```

## Error Handling

**Strava API Errors**:
- `401 Unauthorized`: Token expired, trigger refresh
- `403 Forbidden`: Insufficient scope or deauthorized, mark athlete inactive
- `404 Not Found`: Activity deleted, mark as deleted in database
- `429 Too Many Requests`: Rate limited, exponential backoff
- `5xx Server Error`: Retry with exponential backoff

**Retry Strategy**:
- Initial retry delay: 1 second
- Exponential backoff: 2^n seconds (max 5 minutes)
- Max retries: 5 attempts
- Respect `Retry-After` header when present

## Resources

- [Strava API: Getting Started](https://developers.strava.com/docs/getting-started/)
- [Strava API: Reference Documentation](https://developers.strava.com/docs/reference/)
- [Strava API: Swagger/OpenAPI Spec](https://developers.strava.com/swagger/swagger.json)
- [Strava API: Webhooks Guide](https://developers.strava.com/docs/webhooks/)

## Implementation Notes

1. **Token Management**: Implement automatic token refresh before expiration (check `expires_at` field)
2. **Webhook Reliability**: Store raw webhook events before processing for debugging and replay
3. **Privacy Respect**: Honor activity privacy settings from webhook events
4. **Idempotency**: Handle duplicate webhook events gracefully (use event_time + object_id)
5. **Graceful Degradation**: Continue syncing other users if one user's sync fails
6. **Database Indexes**: Index on athlete_id, activity_id, start_date, has_details for efficient queries
7. **Monitoring**: Track sync completion rates, API error rates, and queue depths

## Implementation TODO

Write tests as you go

### Phase 1: Foundation
- [x] Set up core project structure (Go modules, directory layout)
- [x] Create database schema with athletes, activities, and webhook_events tables
- [x] Add database indexes (athlete_id, activity_id, start_date, has_details)
- [x] Implement configuration management (environment variables, .env loading)
- [x] Set up structured logging with slog (JSON format, log levels)
- [x] Create database repository layer (athletes, activities, events CRUD)

### Phase 2: Strava API Integration
- [ ] Implement Strava API client with rate limiting and token refresh
- [ ] Add automatic token refresh logic (check expires_at before API calls)
- [ ] Implement exponential backoff retry strategy for API errors

### Phase 3: OAuth & Sync
- [ ] Build OAuth callback handler (/connect endpoint with code exchange)
- [ ] Create full sync logic (paginated activity list fetching with per_page=200)

### Phase 4: Webhooks
- [ ] Implement webhook validation endpoint (GET /push with challenge response)
- [ ] Build webhook event processing (POST /push for create/update/delete/deauth)
- [ ] Create background detail fetch worker (prioritize newest, respect rate limits)
- [ ] Implement athlete deauthorization handling (clear tokens, mark activities)

### Phase 5: Internal API
- [ ] Build internal API with bearer token authentication middleware
- [ ] Add POST /api/sync/{athlete_id} endpoint with force parameter
- [ ] Add GET /api/athletes/{athlete_id} endpoint for status checking
- [ ] Add GET /api/activities/{athlete_id} endpoint with pagination

### Phase 6: CLI Tool
- [ ] Create CLI tool structure with webhook management commands
- [ ] Implement CLI webhook create/list/delete commands

### Phase 7: Server & Testing
- [ ] Create main HTTP server with graceful shutdown
- [ ] Write integration tests for OAuth flow
- [ ] Write integration tests for sync logic
- [ ] Write integration tests for webhook handling

### Phase 8: Deployment
- [ ] Create deployment scripts and systemd service file
