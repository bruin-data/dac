# dac export

Export dashboards to external formats. Currently supports Google Slides.

## dac export slides

Export a dashboard to a Google Slides presentation. Queries are executed, charts are rendered as images, and everything is assembled into slides.

```shell
dac export slides [flags]
```

### Flags

| Flag | Alias | Type | Default | Description |
|------|-------|------|---------|-------------|
| `--dashboard` | `-n` | string | *required* | Dashboard name |
| `--credentials` | | string | | Path to Google OAuth `credentials.json` |
| `--filters` | | string | | JSON string with filter overrides |
| `--dir` | `-d` | string | `.` | Dashboard definitions directory |

### Authentication

The command authenticates with Google APIs in this order:

1. **gcloud Application Default Credentials** (recommended) — works if you've run:
   ```shell
   gcloud auth application-default login \
     --scopes=https://www.googleapis.com/auth/presentations,https://www.googleapis.com/auth/drive.file,https://www.googleapis.com/auth/cloud-platform
   ```
2. **Credentials file** — pass `--credentials path/to/credentials.json`, or place it at `~/.dac/credentials.json`

::: info Prerequisites
The Google Slides API and Google Drive API must be enabled on your GCP project:
```shell
gcloud services enable slides.googleapis.com drive.googleapis.com
```
:::

### Examples

```shell
# Export using gcloud ADC
dac export slides --dashboard "Sales Analytics"

# With explicit credentials and filter overrides
dac export slides \
  --dashboard "Sales Analytics" \
  --credentials ~/credentials.json \
  --filters '{"region": "Europe"}'
```

### Output

The command creates a new Google Slides presentation and prints the URL:

```
Created presentation: https://docs.google.com/presentation/d/1abc.../edit
```

### Slide Layout

- **Title slide** with dashboard name and description
- **One slide per row** in the dashboard
- **Metrics** rendered as text boxes with formatted values
- **Charts** rendered as PNG images via go-chart, uploaded to Drive, embedded in the slide, then cleaned up
- **Tables** rendered as native Slides tables (max 10 data rows)

### Supported Widget Types

| Widget | Rendering |
|--------|-----------|
| Metric | Formatted text box with the rendered metric value |
| Chart (line, area, bar, pie) | PNG image via go-chart |
| Table | Native Slides table |
| Text | Not currently exported |
