# plantopo-strava-sync

Syncs activity data from Strava to a SQLite database, maintaining a complete local copy of user activities.

## Project Structure

```
.
├── cmd/
│   ├── server/          # HTTP server for OAuth callbacks and webhooks
│   └── cli/             # CLI tool for webhook management
├── internal/
│   ├── api/             # Internal API handlers
│   ├── config/          # Configuration management
│   ├── database/        # Database layer (SQLite)
│   ├── ratelimit/       # Rate limiting logic
│   ├── strava/          # Strava API client
│   ├── sync/            # Activity sync logic
│   └── webhook/         # Webhook event handling
├── .env.example         # Example environment configuration
├── CLAUDE.md            # Detailed implementation guide
├── go.mod               # Go module definition
└── README.md            # This file
```

## Building

```bash
# Build server
go build -o bin/server ./cmd/server

# Build CLI tool
go build -o bin/cli ./cmd/cli
```

## Running

```bash
# Start the server
./bin/server

# Use CLI tool
./bin/cli webhook list
```

## Configuration

See `.env.example` for required environment variables and `CLAUDE.md` for detailed implementation documentation.

## Development

For complete architecture details, API integration guide, and implementation plan, see [CLAUDE.md](./CLAUDE.md).
