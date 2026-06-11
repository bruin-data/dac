# Widgets

Widgets are the visual building blocks of a dashboard. Each widget occupies a number of columns in a 12-column grid.

## Common Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | Yes | Display name |
| `type` | string | Yes | `metric`, `chart`, `table`, `text`, `image`, or `divider` |
| `col` | integer | No | Column span from 1 to 12 |
| `description` | string | No | Optional tooltip or subtitle |

Data-backed widgets (`metric`, `chart`, `table`) also need a query source: `query` (named query reference), `sql` (inline), `file` (external `.sql` path), or a semantic reference (`metric:` for metric widgets, `dimension:` + `metrics:` for charts). See [Queries & Templating](/dashboards/queries) for the full reference, including the `connection` override.

## Metric

Metric widgets display a single value. The value is configured with a `value`
encoding — which column to read, its type, and a [d3-format](https://d3js.org/d3-format)
string for display.

```yaml
- name: Total Revenue
  type: metric
  sql: SELECT SUM(amount) AS value FROM sales
  value:
    field: value
    type: number
    format: "$,.2f"
  col: 3
```

`field` is required; bare column names (`value: revenue`) are not valid. When
`format` is omitted the number is rendered with an auto-compact fallback (e.g. `1.2M`).

Metric widgets can also use semantic models:

```yaml
- name: Total Revenue
  type: metric
  model: sales
  metric: revenue
  filters:
    - dimension: created_at
      operator: between
      value:
        start: "{{ filters.date_range.start }}"
        end: "{{ filters.date_range.end }}"
  value:
    field: revenue
    type: number
    format: "$,.0f"
  col: 3
```

Semantic metric widgets inherit the dashboard-level `model` when `model` is omitted. See [Semantic Layer](/dashboards/semantic-layer) for model files, filters, segments, and aliases.

Metric-specific fields:

| Field | Type | Description |
|-------|------|-------------|
| `value` | object | Value encoding: `field` (required), `type`, and `format` |
| `value.field` | string | Result column to display |
| `value.type` | string | `number`, `date`, or `category` |
| `value.format` | string | [d3-format](https://d3js.org/d3-format) string (numbers, e.g. `"$,.2f"`, `".0%"`) or d3-time-format string (dates, e.g. `"%b %Y"`) |
| `metric` | string | Semantic metric name |

> **Note:** Currency and unit affixes live in the format string — `$` is a d3-format
> prefix (`"$,.2f"`) and `%` comes from the percent type (`".0%"` renders `0.12` as
> `12%`). There are no separate `prefix`/`suffix` fields.

## Chart

Charts visualize one or more series. The `chart` field selects the chart type and determines which fields are required.

### Chart types

| Chart | Required | Optional | Description |
|-------|----------|----------|-------------|
| `line` | `x`, `y` | `color` | Line chart |
| `bar` | `x`, `y` | `color`, `stacked`, `normalized`, `horizontal` | Bar chart |
| `area` | `x`, `y` | `color` | Area chart |
| `pie` | `label`, `value` | | Pie/donut chart |
| `scatter` | `x`, `y` | | Scatter plot |
| `bubble` | `x`, `y`, `size` | | Bubble chart |
| `combo` | `x`, `y` | `lines` | Mixed bar + line chart (y series in `lines` render as lines, the rest as bars) |
| `histogram` | `x` | `bins` | Histogram (client-side binning) |
| `boxplot` | `x`, `y` | | Box-and-whisker plot (client-side quartiles) |
| `funnel` | `label`, `value` | | Funnel chart |
| `sankey` | `source`, `target`, `value` | | Sankey/flow diagram |
| `heatmap` | `x`, `y`, `value` | | Grid heatmap |
| `calendar` | `x`, `value` | | GitHub-style calendar heatmap |
| `sparkline` | `x`, `y` | | Compact inline line (60px), no axes |
| `waterfall` | `x`, `y` | | Waterfall chart |
| `xmr` | `x`, `y` | `yMin`, `yMax` | Control chart with limits |
| `dumbbell` | `x`, `y` (2 columns) | | Horizontal range comparison |
| `gauge` | `value` | `target` | Semi-circular KPI-vs-target gauge (uses first row) |
| `treemap` | `label`, `value` | | Rectangular part-to-whole hierarchy |
| `radar` | `x`, `y` | | Multi-axis polar comparison |
| `candlestick` | `x`, `open`, `high`, `low`, `close` | | OHLC chart |

SQL-backed example:

```yaml
- name: Revenue Trend
  type: chart
  chart: area
  sql: |
    SELECT month, revenue
    FROM monthly_revenue
    ORDER BY month
  x: { field: month, type: date, format: "%b %Y" }
  y: { field: [revenue], type: number, title: Revenue, format: "$,.0f" }
  col: 8
```

### Axis encoding

`x` and `y` are encoding objects. `field` names the SQL column to plot (`y.field` accepts a list for multiple series); the remaining keys control how the axis renders:

| Key | Type | Description |
|-----|------|-------------|
| `field` | string \| string[] | SQL column(s) to plot. Required. |
| `type` | string | `number`, `date`, or `category`. Selects the axis scale and the format language (`date` uses d3-time-format, otherwise d3-format). |
| `title` | string | Human-readable axis label rendered next to the axis. |
| `format` | string | d3-format / d3-time-format string for tick labels, e.g. `"$,.0f"` → `$1,234`, `".0%"` → `12%`, `"%b %Y"` → `Jan 2024`. |

Without `format`, ticks fall back to automatic compact formatting. Bare column names (`x: month`, `y: [revenue]`) are not valid — always wrap the column in `{ field: ... }`.

### Series by category (color)

`color` names a category column that splits the single `y` series into one series
per distinct category value. The SQL returns long format — one row per x/category
pair — instead of pivoting with `CASE WHEN`:

```yaml
- name: Sales by Region
  type: chart
  chart: bar
  stacked: true                  # bar only; requires color
  color: { field: region }
  sql: |
    SELECT month, region, SUM(amount) AS revenue
    FROM sales GROUP BY 1, 2 ORDER BY 1, 2
  x: { field: month }
  y: { field: revenue }
  col: 6
```

Rules:

- `color` works on `bar`, `line`, and `area` charts and requires a single `y` field.
- `stacked: true` is bar-only and requires `color`. Listing multiple `y` columns renders **grouped** bars (or multiple lines/areas), never stacked.
- `normalized: true` shows each stacked bar as percentages of the row total; the y axis renders `%` automatically, so omit `y.format`.
- `horizontal: true` flips a bar chart: categories run down the vertical axis.

Semantic example:

```yaml
- name: Revenue Trend
  type: chart
  chart: area
  model: sales
  dimension: created_at
  granularity: month
  metrics: [revenue]
  sort:
    - name: created_at
      direction: asc
  col: 8
```

Common chart fields:

| Field | Type | Description |
|-------|------|-------------|
| `chart` | string | Chart type |
| `x` | object | X-axis encoding (`field`, `type`, `title`, `format`) |
| `y` | object | Y-axis encoding (`field` may list several series columns) |
| `label` | string | Label column for pie, funnel, and treemap charts |
| `value` | object | Value encoding (`{ field: ... }`) for pie, funnel, sankey, heatmap, calendar, treemap, and gauge charts |
| `source` | string | Source node column for sankey charts |
| `target` | string | Target/max column for gauge charts (also sankey target node) |
| `open` | string | Open column for candlestick charts |
| `high` | string | High column for candlestick charts |
| `low` | string | Low column for candlestick charts |
| `close` | string | Close column for candlestick charts |
| `color` | object | Category column (`{ field: ... }`) that splits the single `y` series into one series per category — bar, line, and area charts |
| `stacked` | boolean | Stack the color series (bar charts only; requires `color`) |
| `normalized` | boolean | Render stacked bars as percentages of the row total (requires `stacked`) |
| `horizontal` | boolean | Horizontal bars: categories on the vertical axis (bar charts only) |
| `size` | string | Bubble size column for bubble charts |
| `lines` | string[] | Which `y` series render as lines (rest as bars) for combo charts |
| `bins` | integer | Number of bins for histogram charts (default 10) |
| `yMin` | string | Lower control limit column for xmr charts |
| `yMax` | string | Upper control limit column for xmr charts |
| `dimension` | string | Semantic dimension name |
| `granularity` | string | Semantic time grain for `dimension` |
| `metrics` | string[] | Semantic metric names |
| `filters` | array | Semantic filters |
| `segments` | string[] | Semantic segments |
| `sort` | array | Sort instructions |
| `limit` | integer | Row limit |

Charts using `dimension`, `metrics`, `segments`, or semantic `filters` are compiled through the backend semantic layer instead of requiring hand-written SQL.

## Table

Tables display query results in a scrollable grid.

SQL-backed example:

```yaml
- name: Recent Orders
  type: table
  sql: |
    SELECT id, customer_name, amount, status, created_at
    FROM orders
    ORDER BY created_at DESC
    LIMIT 20
  columns:
    - name: id
      label: Order ID
    - name: amount
      label: Amount
      format: currency
```

Semantic example:

```yaml
- name: Sales Breakdown
  type: table
  model: sales
  dimensions:
    - name: region
    - name: channel
  metrics: [revenue, sales_count]
  sort:
    - name: revenue
      direction: desc
  columns:
    - name: region
      label: Region
    - name: revenue
      label: Revenue
      format: currency
```

Tables can mix semantic dimensions and metrics with explicit `columns` metadata for display labels and formatting.

Table column fields:

| Field | Type | Description |
|-------|------|-------------|
| `name` | string | Result column name (must match the SQL output) |
| `label` | string | Display header (defaults to `name`) |
| `format` | string | `number` or `currency` |

## Text

Text widgets render Markdown content.

```yaml
- name: Notes
  type: text
  col: 6
  content: |
    **Important:** Data refreshes daily at 06:00 UTC.
```

Supported markdown:

- Headers (`#` through `######`)
- Bold (`**text**`), italic (`*text*`), bold italic (`***text***`), strikethrough (`~~text~~`)
- Inline code (`` `code` ``)
- Links (`[text](url)`) and images (`![alt](src)`)
- Unordered lists (`-` or `*`) and ordered lists (`1.`)
- Blockquotes (`> text`)
- Horizontal rules (`---`, `***`, or `___`)

## Image

Image widgets render an image from a URL.

```yaml
- name: Logo
  type: image
  col: 3
  src: https://example.com/logo.png
  alt: Company Logo
```

Image-specific fields:

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `src` | string | Yes | Image URL |
| `alt` | string | No | Alt text for accessibility |

## Divider

Divider widgets add visual separation between sections.

```yaml
- name: Section Break
  type: divider
  col: 12
```
