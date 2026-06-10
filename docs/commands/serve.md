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
| `--password` | | string | | Admin password for management API |

## Examples

```shell
# Start with defaults (port 8321, current directory)
dac serve

# Custom port, open browser
dac serve --port 3000 --open

# Dark theme, specific directory
dac serve --template bruin-dark --dir ./dashboards

# Enable admin API
dac serve --password my-secret
```

## Features

### Live Reload

The server watches the dashboard directory for file changes. When you save a YAML or TSX file, connected browsers refresh automatically via Server-Sent Events (SSE).

### Query Caching

Query results are cached with a 5-minute TTL. The cache is invalidated when dashboard files change. This means rapid page refreshes don't re-execute queries.

### Auto Port Increment

If the requested port is already in use, the server automatically tries the next port.

### Admin API

When `--password` is set, the server exposes admin endpoints for managing database connections via the browser UI. See the [API section](#api-endpoints) for details.

## API Endpoints

The server exposes a REST API used by the frontend:

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/v1/dashboards` | List all dashboards |
| `GET` | `/api/v1/dashboards/{name}` | Get dashboard definition |
| `GET` | `/api/v1/dashboards/{name}/raw` | Get raw YAML/TSX source |
| `POST` | `/api/v1/dashboards/{name}/widgets/{id}/query` | Execute a widget query |
| `POST` | `/api/v1/query` | Execute arbitrary SQL |
| `GET` | `/api/v1/themes` | List available themes |
| `GET` | `/api/v1/events` | SSE stream for live reload |
