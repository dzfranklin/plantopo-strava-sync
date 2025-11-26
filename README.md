# plantopo-strava-sync

Syncs activity data from Strava to a SQLite database and provides an event
stream. Hosted at connect-with-strava.plantopo.com

To manage Strava webhook subscriptions the server binary can also be executed
with `plantopo-strava-sync --list-strava-subscriptions`,
`plantopo-strava-sync --delete-strava-subscription <id>`, and
`plantopo-strava-sync --create-strava-subscription <callback_url>`.

See .env.example for configuration.

## API

### `/oauth-callback`

OAuth callback provided to Strava as `redirect_uri`. Handles setting up the
user.

### `/webhook-callback/{client}`

Webhook callback registered with Strava. The `{client}` path parameter specifies
which Strava client (e.g., `primary` or `secondary`) the webhook is for. The
callback must be registered using the configured `VERIFY_TOKEN` for that client
(as `--create-strava-subscription` does).

Example URLs:
- `/webhook-callback/primary`
- `/webhook-callback/secondary`

### `/events`

Log of events received.

When an athlete revokes access all existing events with their athlete_id are
deleted. The athlete delete event is retained.

Events do not appear until they have been hydrated.

Authorization: Provide the header `Authorization: <INTERNAL_API_KEY>`

Parameters:
- cursor (int, optional): The ID of the last event seen.
- long_poll (bool, option): If true then the server may wait to reply until
  events are available. After a period the server will timeout and reply with
  an empty events array

Response
```json5
{
  "events": [
    {
      "event_id": 1,
      "event_type": "athlete_connected",
      "athlete_id": 134815,
      "athlete_summary": {
        // The summary athlete representation provided by Strava on
        // authentication <https://developers.strava.com/docs/authentication/#token-exchange>
      }
    },
    {
      "event_id": 2,
      "event_type": "webhook",
      "activity_id": 1360128428,
      "athlete_id": 134815,
      "activity": {
        // Provided if: object_type activity and aspect_type create or update
        // From https://www.strava.com/api/v3/activities/{id}
        "id": 12345678987654321,
        "resource_state": 3,
        "name": "Happy Friday",
        "distance": 28099,
        // ...
      },
      "event": {
        // The data from the Strava webhook event
        // See <https://developers.strava.com/docs/webhooks/>
        "aspect_type": "update",
        "event_time": 1516126040,
        "object_id": 1360128428,
        "object_type": "activity",
        "owner_id": 134815,
        "subscription_id": 120475,
        "updates": {
          "title": "Messy"
        }
      }
    }
  ]
}
```

## Resources

- https://developers.strava.com/docs/getting-started/
- https://developers.strava.com/docs/webhooks/
- https://developers.strava.com/docs/authentication/
- https://developers.strava.com/docs/reference/#api-Activities-getActivityById
