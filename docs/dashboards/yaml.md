# YAML Format

YAML is the primary format for DAC dashboards. It is declarative, easy to review, and works well for most dashboards.

## File Naming

Place dashboard YAML files in the project's `dashboards/` directory. Any `.yml` or `.yaml` file there is treated as a dashboard definition.

## Top-Level Fields

```yaml
name: Sales Analytics              # required
description: Revenue tracking      # optional
connection: local_duckdb           # optional default connection
model: sales                       # optional default semantic model
theme: bruin-dark                  # optional

refresh:
  interval: 5m

filters:
  - name: region
    type: select
    options:
      values: [North America, Europe, APAC]

queries:
  revenue_sql:
    sql: SELECT SUM(amount) AS value FROM sales
  online_by_region:
    dimensions:
      - name: region
    metrics: [revenue]
    segments: [online]

rows:
  - widgets: [...]
```

## Minimal Example

```yaml
name: Hello World
connection: my_db

rows:
  - widgets:
      - name: Row Count
        type: metric
        sql: SELECT COUNT(*) AS value FROM my_table
        value:
          field: value
          type: number
          format: ",.0f"
```

## Semantic Example

Semantic models are defined separately in `semantic/*.yml`. Dashboards reference them by model name:

```yaml
name: Semantic Sales Example
connection: local_duckdb
model: sales

filters:
  - name: region
    type: select
    default: North America
    options:
      values: [North America, Europe, APAC]
  - name: date_range
    type: date-range
    default: all_time

queries:
  online_by_region:
    dimensions:
      - name: region
    metrics: [revenue]
    segments: [online]
    sort:
      - name: revenue
        direction: desc
    limit: 8

rows:
  - widgets:
      - name: Revenue
        type: metric
        metric: revenue
        filters:
          - dimension: region
            operator: equals
            value: "{{ filters.region }}"
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

      - name: Revenue Trend
        type: chart
        chart: area
        dimension: created_at
        granularity: month
        metrics: [revenue]
        sort:
          - name: created_at
            direction: asc
        col: 9

  - widgets:
      - name: Sales Breakdown
        type: table
        dimensions:
          - name: region
          - name: channel
        metrics: [revenue, sales_count]
        limit: 20
```

The backend REST API renders semantic filter templates and compiles the semantic query to SQL at request time.

For a complete runnable project, see `examples/semantic-yaml`.

## Top-Level Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `schema` | string | No | Optional schema ID. Defaults to `https://getbruin.com/schemas/dac/dashboard/v1` when omitted |
| `name` | string | Yes | Dashboard display name |
| `description` | string | No | Optional subtitle |
| `connection` | string | No | Default connection for widgets and named queries |
| `model` | string | No | Default semantic model for semantic widgets and named semantic queries |
| `models` | map | No | Optional aliases that map dashboard names to semantic model names |
| `theme` | string | No | Theme name or theme file path |
| `refresh` | object | No | Auto-refresh configuration |
| `filters` | array | No | Interactive filter controls |
| `queries` | map | No | Named SQL or semantic queries |
| `rows` | array | Yes | Dashboard layout rows |

## Query Sources

Widgets can get their data from:

1. `query`: reference a named query
2. `sql`: inline SQL
3. `file`: external `.sql` file relative to the dashboard
4. `metric`: semantic metric reference with a model context
5. semantic widget fields: `dimension`, `dimensions`, `metrics`, `filters`, `segments`, `sort`, `limit`

### Named Query Reference

```yaml
queries:
  total_revenue:
    sql: SELECT SUM(amount) AS value FROM sales

rows:
  - widgets:
      - name: Revenue
        type: metric
        query: total_revenue
        value: value
```

### Inline SQL

```yaml
- name: Revenue
  type: metric
  sql: SELECT SUM(amount) AS value FROM sales
  value: value
```

### External SQL File

```yaml
- name: Revenue
  type: metric
  file: queries/revenue.sql
  value: value
```

### Direct Semantic Query

```yaml
- name: Revenue Trend
  type: chart
  chart: area
  model: sales
  dimension: created_at
  granularity: month
  metrics: [revenue]
```

If the dashboard sets a top-level `model`, widgets can omit `model`.

Use `models` when a dashboard needs stable aliases:

```yaml
model: sales_model

models:
  sales_model: sales
```

## Connection Override

Any widget or named query can override the dashboard default:

```yaml
connection: primary_db

queries:
  warehouse_summary:
    sql: SELECT * FROM warehouse.summary
    connection: warehouse

rows:
  - widgets:
      - name: Events
        type: table
        sql: SELECT * FROM events
        connection: analytics_db
```
