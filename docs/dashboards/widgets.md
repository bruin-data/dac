# Widgets

Widgets are the visual building blocks of a dashboard. Each widget occupies a number of columns in a 12-column grid.

## Common Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | Yes | Display name |
| `type` | string | Yes | `metric`, `chart`, `table`, `text`, `image`, or `divider` |
| `col` | integer | No | Column span from 1 to 12 |
| `description` | string | No | Optional tooltip or subtitle |

## Metric

Metric widgets display a single value.

```yaml
- name: Total Revenue
  type: metric
  sql: SELECT SUM(amount) AS value FROM sales
  column: value
  prefix: "$"
  format: number
  col: 3
```

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
  prefix: "$"
  format: number
  col: 3
```

Semantic metric widgets inherit the dashboard-level `model` when `model` is omitted. See [Semantic Layer](/dashboards/semantic-layer) for model files, filters, segments, and aliases.

Metric-specific fields:

| Field | Type | Description |
|-------|------|-------------|
| `column` | string | Result column to display for SQL-backed metrics |
| `metric` | string | Semantic metric name |
| `prefix` | string | Text before the value |
| `suffix` | string | Text after the value |
| `format` | string | `number`, `currency`, or `percent` |

## Chart

Charts visualize one or more series. DAC supports line, bar, area, pie, scatter, bubble, combo, histogram, boxplot, funnel, sankey, heatmap, calendar, sparkline, waterfall, XMR, dumbbell, gauge, treemap, radar, and candlestick charts.

SQL-backed example:

```yaml
- name: Revenue Trend
  type: chart
  chart: area
  sql: |
    SELECT month, revenue
    FROM monthly_revenue
    ORDER BY month
  x: month
  y: [revenue]
  col: 8
```

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
| `x` | string | X-axis column for SQL-backed charts |
| `y` | string[] | Y-axis columns for SQL-backed charts |
| `label` | string | Label column for pie, funnel, and treemap charts |
| `value` | string | Value column for pie, funnel, heatmap, calendar, treemap, and gauge charts |
| `target` | string | Target/max column for gauge charts (also sankey target node) |
| `open` | string | Open column for candlestick charts |
| `high` | string | High column for candlestick charts |
| `low` | string | Low column for candlestick charts |
| `close` | string | Close column for candlestick charts |
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

## Text

Text widgets render Markdown content.

```yaml
- name: Notes
  type: text
  col: 6
  content: |
    **Important:** Data refreshes daily at 06:00 UTC.
```

## Image

Image widgets render an image from a URL.

```yaml
- name: Logo
  type: image
  col: 3
  src: https://example.com/logo.png
  alt: Company Logo
```

## Divider

Divider widgets add visual separation between sections.

```yaml
- name: Section Break
  type: divider
  col: 12
```
