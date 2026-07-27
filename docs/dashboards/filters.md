# Filters

Filters add interactive controls to dashboards. Users can change filter values in the browser, and all queries that reference those filters re-execute automatically.

## Filter Types

DAC supports five filter types:

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

### Date

A single plain date input. The current value is a `YYYY-MM-DD` string.

```yaml
  - name: as_of_date
    type: date
    default: "2025-01-31"
```

### Number

A plain numerical input. Numeric values are sent to queries as numbers.

```yaml
  - name: min_order_value
    type: number
    default: 100
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

## Sharing Filter State via URL

Filter values are reflected in the page's URL query string. Each filter is one query parameter keyed by its `name`:

```
/dashboards/sales?region=Europe&date_range=2025-01-01..2025-03-31
```

- **Multi-select** values are comma-separated: `?region=Europe,APAC`.
- **Date-range** values use `start..end`: `?date_range=2025-01-01..2025-03-31`.
- Opening a link applies the URL values on load; queries run with them immediately.

A few behaviors worth noting:

- **Defaults stay out of the URL.** The URL only captures values the viewer actively changes, keeping links clean. A link with no parameter for a filter uses that filter's current `default` from the YAML.
- **Invalid values are ignored.** URL values are validated before use, so a filter falls back to its default when the value doesn't fit its type. For `select` with a static `options.values` list, values that aren't real options are discarded (per-value for multi-select; if none are valid, the default is used). `date` and `date-range` values must be `YYYY-MM-DD` dates (`date-range` written as `start..end`); anything else is rejected. `number` values must parse as a number.

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

### Date Filters

```sql
SELECT * FROM orders
WHERE created_at::date = DATE '{{ filters.as_of_date }}'
```

### Number Filters

```sql
SELECT * FROM orders
WHERE order_value >= {{ filters.min_order_value }}
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
| `type` | string | Yes | `select`, `date-range`, `date`, `number`, or `text` |
| `multiple` | bool | No | Allow multiple selections (select only) |
| `default` | any | No | Initial value. String preset for date-range, `YYYY-MM-DD` string for date, number for number, array for multi-select |
| `options` | object | No | Filter options configuration |
| `options.values` | string[] | No | Static list of options (select) |
| `options.query` | string | No | SQL to populate options (select) |
| `options.connection` | string | No | Connection for the options query |
| `options.presets` | string[] | No | Which date presets to show (date-range) |
