---
name: create-dashboard
description: Create DAC dashboards by writing YAML or TSX dashboard definition files. Use when the user wants to create, modify, review, or understand DAC dashboards, widgets, filters, SQL queries, semantic models, or CLI validation workflows.
argument-hint: "[dashboard request]"
---

# Create Dashboard

Use this skill to create or modify DAC dashboard projects.

DAC projects define dashboards as code and run queries through Bruin connections. Dashboards can use direct SQL or the semantic layer. Semantic widgets reference models, dimensions, metrics, and segments; DAC compiles them to SQL in the backend.

## Project Layout

```text
my-dac-project/
  .bruin.yml
  dashboards/
    sales.yml
    sales.dashboard.tsx
    queries/
      revenue.sql
  semantic/
    sales.yml
  themes/
    brand.yml
```

Use `dashboards/` for dashboard files and `semantic/` for semantic model YAML files. Regular SQL dashboards do not need semantic models.

Dashboard files:

- `*.yml` and `*.yaml` are YAML dashboards.
- `*.dashboard.tsx` files are TSX dashboards.
- Other TSX files can be helpers, but are not auto-discovered as dashboards.

## Commands

```shell
dac init my-dashboards
dac validate --dir my-dashboards
dac check --dir my-dashboards
dac serve --dir my-dashboards --open
dac query --dir my-dashboards --dashboard "Sales" --widget "Revenue"
```

Use `dac validate` after editing structure and `dac check` when query execution should be verified.

## Connection Config

DAC reads Bruin connections from `.bruin.yml`.

```yaml
default_environment: default

environments:
  default:
    connections:
      duckdb:
        - name: local_duckdb
          path: data/analytics.duckdb
          read_only: true
```

Prefer `read_only: true` for DuckDB dashboards unless the project explicitly needs writes.

## YAML Dashboard

```yaml
name: Sales
description: Revenue and customer activity
connection: local_duckdb

filters:
  - name: region
    type: select
    default: All
    options:
      values: [All, North America, Europe, APAC]
  - name: date_range
    type: date-range
    default: last_30_days

rows:
  - widgets:
      - name: Revenue
        type: metric
        sql: |
          SELECT SUM(amount) AS value
          FROM sales
          WHERE created_at >= '{{ filters.date_range.start }}'
            AND created_at <= '{{ filters.date_range.end }}'
          {% if filters.region != 'All' %}
            AND region = '{{ filters.region }}'
          {% endif %}
        column: value
        prefix: "$"
        format: number
        col: 3
```

Widget types are `metric`, `chart`, `table`, `text`, `divider`, and `image`.

## Filters

Dashboard filters are UI controls. SQL dashboards use filter values through Jinja templates.

Supported filter types:

- `select`
- `date-range`
- `date`
- `number`
- `text`

Date range presets include `today`, `yesterday`, `last_7_days`, `last_30_days`, `last_90_days`, `this_month`, `last_month`, `this_quarter`, `this_year`, `year_to_date`, and `all_time`.

Select filters support `multiple: true` for multi-select. The value is a list — render with `join` in Jinja and guard the empty case:

```sql
{% if filters.status and filters.status | length > 0 %}
  AND status IN ('{{ filters.status | join("','") }}')
{% endif %}
```

## Named Queries

Use named queries when multiple widgets share the same SQL or semantic query.

```yaml
queries:
  revenue_by_region:
    sql: |
      SELECT region, SUM(amount) AS revenue
      FROM sales
      GROUP BY 1

rows:
  - widgets:
      - name: Revenue by Region
        type: chart
        chart: bar
        query: revenue_by_region
        x: region
        y: [revenue]
        col: 6
```

SQL files can be referenced with `file: queries/revenue.sql`, relative to the dashboard file.

## Semantic Models

Semantic models live in `semantic/*.yml`.

```yaml
name: sales
label: Sales
source:
  table: marts.sales

dimensions:
  - name: created_at
    type: time
    granularities:
      month: date_trunc('month', created_at)
  - name: region
    type: string
  - name: channel
    type: string

metrics:
  - name: revenue
    expression: sum(amount)
    format:
      type: currency
      currency: USD
      decimals: 0
  - name: orders
    expression: count(*)
  - name: average_order_value
    expression: "{revenue} / nullif({orders}, 0)"

segments:
  - name: online
    filter: "channel = 'online'"
```

Metrics are aggregate SQL expressions or expressions over other metrics using `{metric_name}` references. Dimensions are the only fields valid for semantic filters.

## Semantic Dashboard

```yaml
name: Semantic Sales
connection: local_duckdb
model: sales

filters:
  - name: region
    type: select
    default: North America
    options:
      values: [North America, Europe, APAC]

rows:
  - widgets:
      - name: Revenue
        type: metric
        metric: revenue
        filters:
          - dimension: region
            operator: equals
            value: "{{ filters.region }}"
        prefix: "$"
        format: number
        col: 3

      - name: Revenue by Month
        type: chart
        chart: area
        dimension: created_at
        granularity: month
        metrics: [revenue]
        sort:
          - name: created_at
            direction: asc
        col: 9
```

A widget can set `model` directly, or inherit the dashboard-level `model`. For multiple models, use a dashboard-level `models` map and reference the model alias on widgets or named queries.

Semantic filter operators include `equals`, `not_equals`, `gt`, `gte`, `lt`, `lte`, `in`, `not_in`, `between`, `is_null`, and `is_not_null`.

## TSX Dashboard

Use TSX when the dashboard needs variables, loops, reusable components, conditionals, or generated layouts.

```tsx
export default (
  <Dashboard name="Semantic Sales" connection="local_duckdb" model="sales">
    <Filter
      name="region"
      type="select"
      default="North America"
      options={{ values: ["North America", "Europe", "APAC"] }}
    />

    <Row>
      <Metric
        name="Revenue"
        metric="revenue"
        filters={[
          { dimension: "region", operator: "equals", value: "{{ filters.region }}" },
        ]}
        prefix="$"
        format="number"
        col={3}
      />
      <Chart
        name="Revenue by Month"
        chart="area"
        dimension="created_at"
        granularity="month"
        metrics={["revenue"]}
        sort={[{ name: "created_at", direction: "asc" }]}
        col={9}
      />
    </Row>
  </Dashboard>
)
```

TSX supports the same dashboard model as YAML. Keep semantic logic declarative; do not manually compile semantic metrics to SQL in TSX.

## Authoring Rules

- Keep dashboard files focused on presentation and query intent.
- Prefer semantic widgets when metrics or dimensions are reused.
- Use direct SQL for one-off custom queries or non-semantic dashboards.
- Validate both YAML and TSX dashboards after changes.
- Do not require semantic models for regular SQL dashboards.
- Do not put secrets in dashboard files; use Bruin connection config.
