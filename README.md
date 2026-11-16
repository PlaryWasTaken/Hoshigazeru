# Hoshigazeru

A Go server that tracks upcoming TV anime episodes from AniList and broadcasts release events over WebSocket. It also exposes the current anime list and the next scheduled release through HTTP.

There is no frontend, account system, or per-user watchlist. Every connected WebSocket client receives the same release feed.

## Run locally

Requires Go 1.22 or newer and network access to AniList.

```sh
git clone https://github.com/PlaryWasTaken/Hoshigazeru.git
cd Hoshigazeru
go mod download
go run .
```

The server listens on **http://localhost:8080**. To build a binary:

```sh
go build -o hoshigazeru .
```

Polling starts immediately and fetches all pages of TV anime with `RELEASING` or `NOT_YET_RELEASED` status. English titles are preferred, with romaji as a fallback.

| Setting | Default | Defined in |
| --- | --- | --- |
| HTTP address | `:8080` | `web/WebClient.go` |
| AniList refresh interval | 1 hour after each fetch completes | `main.go` |
| Release check interval | 15 seconds | `main.go` |
| AniList request timeout | 30 seconds per page | `AniList/polling.go` |
| WebSocket heartbeat interval | 5 seconds | `web/PingHandler.go` |

These settings are currently configured in code; there are no environment variables or command-line flags.

## Run with Docker

```sh
docker build -t hoshigazeru .
docker run --rm --name hoshigazeru -p 8080:8080 -v hoshigazeru-data:/data hoshigazeru
```

The named volume preserves the schedule cache between containers. The application runs from `/data` inside the container.

## HTTP API

| Method | Path | Response |
| --- | --- | --- |
| `GET` | `/animes` | JSON array of tracked anime; `[]` when no data is available |
| `GET` | `/animes/nextRelease` | The anime with the earliest future episode; `404` when none is known |
| WebSocket upgrade | `/releases` | Release events and text heartbeats |

The two HTTP endpoints return `405 Method Not Allowed` for other methods.

```sh
curl http://localhost:8080/animes
curl http://localhost:8080/animes/nextRelease
```

### Anime data

Field names are case-sensitive. An example item from `/animes`:

```json
{
  "Id": 123,
  "Title": "Example Anime",
  "Episodes": 12,
  "AiringSchedule": [
    {
      "AiringAt": 1893456000,
      "Episode": 1
    }
  ],
  "Description": "An example <b>description</b>.",
  "MarkdownDescription": "An example **description**.",
  "CoverImage": "https://example.com/cover.jpg",
  "ExternalLinks": [
    {
      "Type": "STREAMING",
      "Site": "Example",
      "Color": null,
      "Url": "https://example.com/watch",
      "Icon": null
    }
  ],
  "IsAdult": false
}
```

- `AiringAt` is a Unix timestamp in seconds.
- `Episodes`, `Description`, and `MarkdownDescription` can be `null`. Link colors and icons can also be `null`.
- `AiringSchedule` contains known upcoming episodes. Due episodes are removed during release checks.
- `MarkdownDescription` converts basic bold, italic, underline, and line-break tags; it is not a general HTML sanitizer.
- Disabled external links are omitted.
- `/animes/nextRelease` searches all known future schedule entries, without assuming that schedules are sorted. Its response uses the same anime object shape.

## WebSocket feed

Connect to `ws://localhost:8080/releases`. When a known episode becomes due, the server sends:

```json
{
  "media": {
    "Id": 123,
    "Title": "Example Anime",
    "Episodes": 12,
    "AiringSchedule": [],
    "Description": null,
    "MarkdownDescription": null,
    "CoverImage": "https://example.com/cover.jpg",
    "ExternalLinks": [],
    "IsAdult": false
  },
  "episode": {
    "AiringAt": 1893456000,
    "Episode": 1
  }
}
```

The released episode appears in `episode`; it has already been removed from `media.AiringSchedule`.

The server also sends the plain text message `ping` immediately after connection and every five seconds. Clients should respond with the plain text message `pong`. These are application messages, so clients must handle them separately from JSON events. After four consecutive missed heartbeat checks, the server closes the connection.

A browser client running from a same-origin page can use:

```js
const url = new URL("/releases", window.location.href);
url.protocol = url.protocol === "https:" ? "wss:" : "ws:";
const socket = new WebSocket(url);

socket.onmessage = ({ data }) => {
  if (data === "ping") {
    socket.send("pong");
    return;
  }

  const { media, episode } = JSON.parse(data);
  console.log(`${media.Title}: episode ${episode.Episode}`);
};
```

The default WebSocket origin policy accepts same-origin browser connections and clients without an `Origin` header. A browser frontend on a different origin requires an explicit server-side origin policy change. For HTTPS deployments, use `wss://` through your reverse proxy.

To disconnect, close the socket normally or send the text message `close`. Slow consumers are disconnected if their outgoing queue fills or a write times out.

## Cache and delivery behavior

The server writes `savedMedias.json` in its working directory after successful refreshes and after removing due episodes. Writes replace the previous cache through a temporary file. Cache failures are logged and leave the server running.

At startup, a valid cache is loaded before the first API refresh. Missing or invalid cache files do not prevent startup. Until data is available, `/animes` returns `[]` and `/animes/nextRelease` returns `404`. A failed API refresh keeps the previous in-memory snapshot.

Release events follow AniList's scheduled airing times. They do not confirm that an episode is available on a streaming service. Under normal operation, a known episode is detected on the next 15-second release check. New or changed schedules depend on the hourly refresh.

The feed is best effort: there is no replay, acknowledgement, or guaranteed ordering of release events. Episodes discovered as already due in the startup cache are processed before WebSocket clients can connect. Duplicate episodes from later refreshes are suppressed within the running process; the cache is a schedule snapshot, not a durable event log. Consumers that need reliable delivery should store and deduplicate events by anime ID and episode number.

## Development

```sh
go test ./...
go test -race ./...
go vet ./...
```

The race detector requires CGO and a compatible C compiler. Tests use local HTTP and WebSocket servers; they do not require AniList access.

| Directory | Responsibility |
| --- | --- |
| `AniList/` | GraphQL fetching, response conversion, and pagination |
| `Client/` | Schedule state, release detection, subscriptions, and disk cache |
| `web/` | HTTP routes, WebSocket delivery, and heartbeats |
| `main.go` | Default intervals and server startup |

## License

[Apache License 2.0](LICENSE).
