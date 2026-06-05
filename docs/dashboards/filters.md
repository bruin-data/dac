# Filters

Filters add interactive controls to dashboards. Users can change filter values in the browser, and all queries that reference those filters re-execute automatically.

## Filter Types

DAC supports three filter types:

### Select

A dropdown with predefined or query-driven options.

```yaml
filters:
  - name: region
    type: select
    default: "All"
    options:
      values: ["All", "North America", "Europe", "APAC"]
```

Multi-select:

```yaml
  - name: status
    type: select
    multiple: true
    default: ["active", "pending"]
    options:
      values: ["active", "pending", "completed", "cancelled"]
```

Query-driven options:

```yaml
  - name: customer
    type: select
    options:
      query: SELECT DISTINCT customer_name FROM orders ORDER BY 1
      connection: my_db
```

### Date Range

A date picker with start and end dates. Supports presets for common ranges.

```yaml
  - name: date_range
    type: date-range
    default: last_30_days
```

With specific presets:

```yaml
  - name: date_range
    type: date-range
    default: last_90_days
    options:
      presets:
        - today
        - last_7_days
        - last_30_days
        - last_90_days
        - this_year
```

### Text

A free-form text input.

```yaml
  - name: search
    type: text
    default: ""
```

## Available Date Presets

| Preset | Description |
|--------|-------------|
| `today` | Current day |
| `yesterday` | Previous day |
| `last_7_days` | Past 7 days including today |
| `last_30_days` | Past 30 days including today |
| `last_90_days` | Past 90 days including today |
| `this_month` | First to last day of current month |
| `last_month` | First to last day of previous month |
| `this_quarter` | First to last day of current quarter |
| `this_year` | January 1 to December 31 of current year |
| `year_to_date` | January 1 to today |
| `all_time` | 1970-01-01 to 2099-12-31 |

## Using Filters in Queries

Filter values are injected into SQL via [Jinja templating](/dashboards/queries). Access them with `filters.<filter_name>`:

### Select Filters

```sql
SELECT * FROM orders
WHERE region = '{{ filters.region }}'
```

With an "All" option:

```sql
SELECT * FROM orders
{% if filters.region != 'All' %}
WHERE region = '{{ filters.region }}'
{% endif %}
```

Multi-select (`multiple: true`) — the value is a list, render it with `join`:

```sql
SELECT * FROM orders
{% if filters.status and filters.status | length > 0 %}
WHERE status IN ('{{ filters.status | join("','") }}')
{% endif %}
```

### Date Range Filters

Date range filters provide `.start` and `.end` properties:

```sql
SELECT * FROM orders
WHERE created_at >= '{{ filters.date_range.start }}'
  AND created_at <= '{{ filters.date_range.end }}'
```

### Text Filters

```sql
SELECT * FROM orders
WHERE customer_name LIKE '%{{ filters.search }}%'
```

## Filter Fields Reference

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | Yes | Filter identifier, used in `filters.<name>` |
| `type` | string | Yes | `select`, `date-range`, or `text` |
| `multiple` | bool | No | Allow multiple selections (select only) |
| `default` | any | No | Initial value. String preset for date-range, array for multi-select |
| `options` | object | No | Filter options configuration |
| `options.values` | string[] | No | Static list of options (select) |
| `options.query` | string | No | SQL to populate options (select) |
| `options.connection` | string | No | Connection for the options query |
| `options.presets` | string[] | No | Which date presets to show (date-range) |
