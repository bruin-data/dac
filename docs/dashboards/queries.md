# Queries & Templating

DAC supports both SQL queries and semantic queries. Widgets can reference named queries or define their query shape inline.

## Named SQL Queries

```yaml
queries:
  total_revenue:
    sql: |
      SELECT SUM(amount) AS value FROM sales
      WHERE created_at >= '{{ filters.date_range.start }}'
        AND created_at <= '{{ filters.date_range.end }}'

  revenue_by_month:
    sql: |
      SELECT DATE_TRUNC('month', created_at) AS month, SUM(amount) AS revenue
      FROM sales GROUP BY 1 ORDER BY 1

rows:
  - widgets:
      - name: Revenue
        type: metric
        query: total_revenue
        value: { field: value }
```

## Named Semantic Queries

```yaml
model: sales

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
      - name: Online Revenue by Region
        type: chart
        chart: bar
        query: online_by_region
```

Named semantic queries use the same model resolution rules as widgets. If `model` is omitted, DAC uses the dashboard-level `model`. If `models` aliases are defined, the query can use either a model name or an alias.

## Named Query Fields

### SQL Fields

| Field | Type | Description |
|-------|------|-------------|
| `sql` | string | Inline SQL |
| `file` | string | Path to a `.sql` file, relative to the dashboard |
| `connection` | string | Connection override |

### Semantic Fields

| Field | Type | Description |
|-------|------|-------------|
| `model` | string | Semantic model name. Falls back to dashboard `model` when omitted |
| `dimensions` | array | Dimensions to group by |
| `metrics` | string[] | Metrics to select |
| `filters` | array | Structured semantic filters |
| `segments` | string[] | Segment names to apply |
| `sort` | array | Sort instructions |
| `limit` | integer | Row limit |
| `connection` | string | Optional connection override |

Semantic query fields are compiled to SQL by the backend REST API when the widget is requested.

## Widget Query Sources

Every data widget needs a query. You can provide one in five ways:

1. `query`: reference a named query
2. `sql`: inline SQL
3. `file`: external `.sql` file
4. `metric`: semantic metric reference with a model context
5. semantic widget fields such as `dimension`, `dimensions`, `metrics`, `filters`, `segments`, `sort`, and `limit`

Examples:

```yaml
# Named query
- name: Revenue
  type: metric
  query: total_revenue
  value: { field: value }

# Inline SQL
- name: Revenue
  type: metric
  sql: SELECT SUM(amount) AS value FROM sales
  value: { field: value }

# Semantic metric widget
- name: Revenue
  type: metric
  model: sales
  metric: revenue

# Direct semantic chart
- name: Revenue by Month
  type: chart
  chart: area
  model: sales
  dimension: created_at
  granularity: month
  metrics: [revenue]
```

## Jinja Templating

SQL queries are processed through Jinja before execution. Semantic filter values are also templated before the backend compiles them to SQL.

### Variable Interpolation

```sql
SELECT * FROM orders
WHERE region = '{{ filters.region }}'
```

### Conditionals

```sql
SELECT * FROM orders
WHERE created_at >= '{{ filters.date_range.start }}'
  AND created_at <= '{{ filters.date_range.end }}'
{% if filters.region != 'All' %}
  AND region = '{{ filters.region }}'
{% endif %}
```

### Date Range Filters

Date range filters expose `start` and `end` as `YYYY-MM-DD` strings:

```sql
WHERE created_at >= '{{ filters.date_range.start }}'
  AND created_at < DATE '{{ filters.date_range.end }}' + INTERVAL 1 DAY
```

Plain date filters expose a single `YYYY-MM-DD` string, while number filters expose a numeric value:

```sql
WHERE order_date = DATE '{{ filters.as_of_date }}'
  AND order_value >= {{ filters.min_order_value }}
```

The same pattern works in semantic filters:

```yaml
filters:
  - dimension: created_at
    operator: between
    value:
      start: "{{ filters.date_range.start }}"
      end: "{{ filters.date_range.end }}"
```

Structured semantic filters target dimensions. Metric-specific predicates usually belong in the semantic model as a metric `filter` or as a reusable `segment`.

### Available Variables

| Variable | Source | Description |
|----------|--------|-------------|
| `filters.<name>` | Filter controls | Current filter value |
| `filters.<date_range>.start` | Date range filter | Start date |
| `filters.<date_range>.end` | Date range filter | End date |

## Connection Override

Any widget or named query can override the dashboard-level connection:

```yaml
connection: primary_db

rows:
  - widgets:
      - name: Analytics Events
        type: table
        sql: SELECT * FROM events
        connection: analytics_db
```
