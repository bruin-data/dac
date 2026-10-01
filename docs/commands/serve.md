# dac serve

Start a development server with live reload.

```shell
dac serve [flags]
```

## Flags

| Flag | Alias | Type | Default | Description |
|------|-------|------|---------|-------------|
| `--port` | `-p` | int | `8321` | Port to listen on |
| `--dir` | `-d` | string | `.` | Dashboard definitions directory |
| `--template` | `-t` | string | `bruin` | Theme name or path to YAML file |
| `--host` | | string | `localhost` | Host to bind to |
| `--open` | | bool | `false` | Open browser automatically |
| `--password` | | string | | Password required for dashboard viewing and all APIs; also enables admin endpoints |

## Examples

```shell
# Start with defaults (port 8321, current directory)
dac serve

# Custom port, open browser
dac serve --port 3000 --open

# Dark theme, specific directory
dac serve --template bruin-dark --dir ./dashboards

# Require authentication for the entire server
dac serve --password my-secret
```

## Features

### Live Reload

The server watches the dashboard directory for file changes. When you save a YAML or TSX file, connected browsers refresh automatically via Server-Sent Events (SSE).

### Query Caching

Query results are cached with a 5-minute TTL. The cache is invalidated when dashboard files change. This means rapid page refreshes don't re-execute queries.

### Auto Port Increment

If the requested port is already in use, the server automatically tries the next port.

### Authentication

When `--password` is set, **the entire server requires authentication**, including the dashboard list, definitions, raw source, widget queries, batch and streaming data, live reload, themes, and frontend files. Missing or incorrect credentials return HTTP 401 before dashboards are loaded or queries execute.

Browsers show an HTTP Basic login prompt: enter any username and the configured password. The browser sends credentials on subsequent dashboard and live-reload requests. API clients can use HTTP Basic or an explicit bearer token:

```shell
curl -H "Authorization: Bearer $DAC_PASSWORD" http://localhost:8321/api/v1/dashboards
```

Embedded frontends on another origin, such as the Bruin VS Code webview, cannot send these credentials, so run the server they connect to without `--password`.

Without `--password`, dashboard viewing and dashboard queries remain public. Use HTTPS when exposing a password-protected server beyond localhost. Dashboard connections should use dedicated read-only database credentials.

### Admin API

When `--password` is set, the server also exposes admin endpoints for managing database connections. Admin operations and `POST /api/v1/query` require an explicit `Authorization: Bearer <password>` header; browser Basic credentials alone do not authorize them. This prevents ambient browser credentials from authorizing cross-site management writes. The existing management UI asks for the password and sends that header.

**Migration:** `--password` previously protected only management endpoints. It now protects dashboard viewing as well. Arbitrary SQL through `POST /api/v1/query` is disabled (HTTP 403) when no password is configured; authenticated administrators can still use it. Dashboard queries continue to execute SQL defined by the dashboard author.

## API Endpoints

The server exposes a REST API used by the frontend:

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/v1/dashboards` | List all dashboards |
| `GET` | `/api/v1/dashboards/{name}` | Get dashboard definition |
| `GET` | `/api/v1/dashboards/{name}/raw` | Get raw YAML/TSX source |
| `POST` | `/api/v1/dashboards/{name}/widgets/{id}/query` | Execute a widget query |
| `POST` | `/api/v1/dashboards/{name}/data` | Execute all widget queries |
| `POST` | `/api/v1/dashboards/{name}/stream` | Stream widget query results |
| `POST` | `/api/v1/query` | Execute arbitrary SQL (explicit admin bearer token required) |
| `GET` | `/api/v1/themes` | List available themes |
| `GET` | `/api/v1/events` | SSE stream for live reload |
