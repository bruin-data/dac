---
name: create-dashboard
description: Create DAC dashboards by writing YAML or TSX dashboard definition files. Use when the user wants to create, modify, review, or understand DAC dashboards, widgets, filters, SQL queries, semantic models, or CLI validation workflows.
argument-hint: "[dashboard request]"
---

# Create Dashboard

Use this skill to create or modify DAC dashboard projects.

DAC projects define dashboards as code and run queries through Bruin connections. Dashboards can use direct SQL or the semantic layer. Semantic widgets reference models, dimensions, metrics, and segments; DAC compiles them to SQL in the backend.

## Validation Workflow — Read Before Editing

`dac check` runs every widget's query against the live database and scales with total widget count. Pick the cheapest tool that proves the change is correct:

| Change you just made | Run this | Why |
|---|---|---|
| UI-only — `chart` type, `col`, labels, `value` formatting, colors, text/markdown, divider/image, theme, row order | nothing | No query changed. `dac serve` live-reloads instantly in the browser. |
| One widget's SQL, `query:` ref, or column mapping | `dac query --dir ./dashboards --dashboard "X" --widget "Y"` | Executes only that one query. |
| Filters, named queries, metrics/dimensions wiring, `col` sums, new widget skeletons | `dac validate --dir ./dashboards` | Structural check, no SQL executed. |
| End-of-task sweep, or many widgets changed at once | `dac check --dir ./dashboards` | Full execution. Slow — only run once when you think you're done. |

Default to no command for UI tweaks, `dac query --widget` for per-widget SQL checks, and `dac check` for final sweeps.

## Workflow

Follow this order unless the user asks for something narrower:

1. Inspect the data before charting — row counts, nulls, distributions, duplicates, outliers, join behavior.
2. Choose YAML by default; use TSX only when the dashboard needs loops, variables, reusable components, or generated layouts.
3. Build the smallest dashboard that answers the question well.
4. Use the validation workflow above to choose the cheapest correct check: no command for UI-only edits, `dac validate` for structure, `dac query` for one widget, and `dac check` only for final sweeps or broad changes.
5. Serve locally with `dac serve --port 8321` and always give the user `http://localhost:8321`.
6. Visually review the rendered dashboard before calling it done.

## Core Dashboard Rules

### Start from the analytical question

- A dashboard should answer a question or test a hypothesis, not just restate raw data.
- Prefer 2 to 4 strong charts over many weak ones.
- Use a table instead of a chart when the dataset is small enough to read directly.
- Chart descriptions should quantify the finding when possible: correlation, ratio, change, rank, spread, or sample size.

### Match the chart type to the task

- Line for change over time, bar for ranked comparison, scatter for relationships, histogram for distributions, table for exact lookup.
- Do not use a trendy chart type when a simpler one answers the question more clearly.
- If the chart requires too much explanation to decode, choose a simpler encoding.

### Make each chart self-explanatory

Every analytical chart should include:

- A clear title stating what is shown: entity, metric, units, and time range where relevant.
- A short description stating the takeaway and its magnitude.
- A source and methodology note with limitations or caveats.
- Explicit units on axes, labels, metrics, or surrounding text.
- A visible legend or an explicit explanation of what each encoding means.

A strong default pattern is:

1. Text widget for title and description
2. Chart widget
3. Text widget for sources, tools, and limitations

Use this pattern whenever the chart itself cannot carry enough context cleanly.

## Truthful Visualization Defaults

- Bar and area charts should start the quantitative axis at zero.
- Do not use pie charts by default; use sorted bars instead.
- Do not use 3D charts.
- Do not use dual y-axes by default. Only use them when the comparison truly depends on shared time alignment and the scales are clearly labeled.
- Log scales must be labeled and justified in the title or description.
- Sort categorical bars by value unless there is a natural order such as time or ordered tiers.
- Do not make claims stronger than the data supports. Small samples and partial coverage must be called out in the description or limitations.
- Keep comparison charts on consistent scales when users are meant to compare magnitudes across panels or time windows.
- Show benchmarks and thresholds explicitly when they matter to interpretation, such as targets, averages, budgets, or 50% reference lines.
- Clearly distinguish actuals, forecasts, targets, and uncertainty ranges in labels and surrounding text.

## Accessibility And Readability

- Use a colorblind-safe palette by default.
- Never rely on color alone. Pair color with shape, stroke pattern, labels, or position.
- Multi-series charts need a legend or an explicit encoding key in surrounding text.
- Tooltips must remain interpretable: readable field names, sensible numeric formats, and units where needed.
- Avoid pre-truncating labels in SQL. Keep the full value available for tooltips.
- If labels, ticks, or legends collide, fix the layout or change the chart type. Do not ship cramped charts.
- If the data is dense or overplotted, aggregate, facet, bin, or reduce categories instead of shipping an unreadable chart.
- Prefer direct labels when there are only a few series and that makes the chart easier to read than a distant legend.

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

Use the validation workflow above when deciding between no command, `dac validate`, `dac query`, and `dac check`. Always surface `http://localhost:8321` to the user after starting the server.

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
        value:
          field: value
          type: number
          format: "$,.2f"
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
        value:
          field: revenue
          type: number
          format: "$,.0f"
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
        value={{ field: "revenue", type: "number", format: "$,.0f" }}
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

## Practical DAC Constraints

- Only use supported schema properties. Do not invent widget options and assume DAC will honor them.
- SQL output columns should be plain identifiers. Prefer `snake_case`.
- If a chart type has weak legend support, compensate with nearby text that explains the encodings.
- Choose chart types that DAC renders clearly instead of assuming chart-level customization exists.
- Keep semantic logic declarative; do not manually reimplement semantic metrics in TSX unless there is no alternative.

## Review Checklist

Before declaring the dashboard done:

1. `dac validate` passes.
2. `dac check` passes.
3. `dac serve --port 8321` is running and the user has the `http://localhost:8321` URL.
4. Chart titles state what is shown, not just the conclusion.
5. Descriptions state the takeaway with numbers when possible.
6. Sources, methodology, and limitations are present where needed.
7. Legends or encoding explanations are explicit.
8. Units are visible.
9. Labels, ticks, legends, and footnotes are readable with no collisions.
10. Tooltips show understandable names and formats.

If a chart is truthful but unreadable, it is not done. If it is attractive but misleading, it is wrong.
