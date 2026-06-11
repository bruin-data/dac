---
name: create-dashboard
description: Create dac dashboards by writing YAML or TSX definition files. Use when the user wants to create, modify, or understand dashboard files, widget configuration, filters, query templating, or CLI usage. TSX dashboards enable loops, variables, custom components, and data-driven layouts impossible in YAML.
argument-hint: "[description of the dashboard to create]"
---

# Create Dashboard

Create dac dashboards by writing YAML or TSX files. This skill covers both formats, widget types, filters, query templating, and project setup.

**When to use YAML vs TSX:**
- **YAML** — straightforward dashboards with static layouts. Simpler syntax, no programming needed.
- **TSX** — dashboards that need loops, variables, custom reusable components, conditional logic, or data-driven layouts that adapt to the database contents at load time.

Both formats produce identical Dashboard structs and coexist in the same directory.

When invoked, use `$ARGUMENTS` as the description of what dashboard to create.

---

## Validation Workflow — Read Before Editing

`dac check` runs **every widget's query** against the live database and scales with total widget count. Running it after every edit is the single biggest waste of time when iterating on a dashboard. Pick the cheapest tool that proves the change is correct:

| Change you just made | Run this | Why |
|---|---|---|
| UI-only — `chart` type, `col`, labels, `value` formatting, colors, text/markdown, divider/image, theme, row order | **nothing** | No query changed. `dac serve` live-reloads instantly in the browser. |
| One widget's SQL, `query:` ref, or column mapping | `dac query --dir ./dashboards --dashboard "X" --widget "Y"` | Executes only that one query, returns rows. ~1 query of latency. |
| Filters, named queries, metrics/dimensions wiring, `col` sums, new widget skeletons | `dac validate --dir ./dashboards` | Structural check, no SQL executed. Sub-second. |
| End-of-task sweep, or many widgets changed at once | `dac check --dir ./dashboards` | Full execution. Slow — only run once when you think you're done. |

**Rules:**
1. Default to **no command** for UI tweaks. The live reload in `dac serve` is the feedback loop, not the CLI.
2. Never run `dac check` and then re-run individual widgets — pick one. Per-edit = `dac query --widget`; end-of-task = `dac check`.
3. If you changed N widgets and N ≥ ~half the dashboard, `dac check` is fine. Otherwise `dac query --widget` per change is cheaper.
4. `dac validate` is cheap (<1s) but only catches structure, never SQL errors. Don't rely on it for SQL edits.

---

## Project Structure

```
my-project/
  .bruin.yml                    # Database connections (required for queries)
  dashboards/
    my-dashboard.yml            # Any *.yml file = a dashboard
    another.yml
    explorer.dashboard.tsx      # Any *.dashboard.tsx file = a TSX dashboard
    dynamic-report.dashboard.tsx
    lib/
      kpi.tsx                   # Shared TSX helpers (not auto-discovered)
    queries/
      my_query.sql              # SQL files for TSX include() and dac query -f
  themes/
    corporate.yml               # Optional custom themes (token overrides)
```

- Any `*.yml`/`*.yaml` file in the dashboard directory is auto-discovered as a YAML dashboard.
- Any `*.dashboard.tsx` file is auto-discovered as a TSX dashboard.
- Files in `lib/` or without the `.dashboard.tsx` suffix are NOT auto-discovered (use `require()` to import them).
- Files starting with `.` are ignored (e.g. `.bruin.yml`).
- SQL files in `queries/` are for TSX `include()` and `dac query -f` — YAML widgets always inline their SQL (or reference a named query).

---

## .bruin.yml — Connection Config

The `.bruin.yml` file defines database connections. It must exist somewhere above or in the dashboard directory. The CLI auto-discovers it by walking up the directory tree.

```yaml
default_environment: default
environments:
  default:
    connections:
      duckdb:
        - name: my_duckdb
          path: /absolute/path/to/data.db
          read_only: true       # recommended for DuckDB to avoid lock issues

      postgres:
        - name: my_postgres
          host: localhost
          port: 5432
          database: analytics
          username: user
          password: pass

      # Other connection types supported by bruin CLI
```

Queries are executed via `bruin query` under the hood. Any connection type supported by bruin works.

---

## Dashboard YAML Schema

```yaml
# Required
name: My Dashboard                    # Display name, also used as URL slug
rows: []                              # At least one row required

# Optional
description: A description            # Shown on the dashboard list page
connection: my_duckdb                 # Default connection for all queries

# Optional: interactive filters
filters: []

# Optional: named queries (reusable across widgets)
queries: {}

# Optional: declarative data source (enables metrics & dimensions)
source:
  table: my_schema.my_table
  date_column: created_at             # For automatic date range filtering
  date_format: "%Y%m%d"              # strftime format if date is stored as string
  connection: my_postgres             # Overrides dashboard-level connection

# Optional: reusable metric definitions
metrics: {}

# Optional: reusable dimension definitions
dimensions: {}
```

---

## Filters

Filters create interactive controls in the UI. Filter values are injected into SQL queries via Jinja templating.

```yaml
filters:
  # Select dropdown
  - name: region
    type: select
    default: "All"                     # Initial value
    multiple: false                    # true for multi-select
    options:
      values: ["All", "North America", "Europe", "APAC"]   # Static options
      # OR dynamic options from a query:
      # query: SELECT DISTINCT region FROM dim_regions ORDER BY region
      # connection: my_postgres        # Optional connection override

  # Date range picker (with preset default)
  - name: date_range
    type: date-range
    default: last_30_days              # Preset name OR explicit {start, end}
    # default:                         # Explicit dates also work:
    #   start: "2025-01-01"
    #   end: "2025-12-31"
    options:
      presets:                         # Optional: control which presets appear
        - last_7_days
        - last_30_days
        - last_90_days
        - this_month
        - this_year

  # Free text input
  - name: search
    type: text
    default: ""

  # Plain date input
  - name: as_of_date
    type: date
    default: "2025-01-01"

  # Plain numeric input
  - name: min_revenue
    type: number
    default: 1000
```

**Filter types:** `select`, `date-range`, `date`, `number`, `text`

**Date range presets:** `today`, `yesterday`, `last_7_days`, `last_30_days`, `last_90_days`, `this_month`, `last_month`, `this_quarter`, `this_year`, `year_to_date`, `all_time`. If `options.presets` is omitted, a default set is shown. Users can always pick "Custom range" for arbitrary dates.

---

## Named Queries

Define reusable queries that multiple widgets can reference:

```yaml
queries:
  total_revenue:
    sql: |
      SELECT SUM(amount) as value FROM sales
      WHERE created_at >= '{{ filters.date_range.start }}'

  revenue_by_month:
    sql: |
      SELECT DATE_TRUNC('month', created_at) AS month, SUM(amount) AS revenue
      FROM sales GROUP BY 1 ORDER BY 1
    connection: my_postgres                  # Optional connection override
```

---

## Declarative Source, Metrics & Dimensions

Instead of writing repetitive SQL for every widget, you can define a **source table**, **metrics**, and **dimensions** at the top level. The tool auto-generates optimized SQL — multiple scalar metrics are merged into a single query, and dimensional charts get automatic GROUP BY queries.

### Source

Defines the base table all metrics and dimensions query against.

```yaml
source:
  table: my_schema.events                # REQUIRED: table name (supports Jinja)
  date_column: event_date                # Optional: enables automatic date range filtering
  date_format: "%Y%m%d"                  # Optional: strftime format if date is stored as string
  connection: my_postgres                # Optional: overrides dashboard-level connection
```

The `table` field supports Jinja templating for dynamic table selection:

```yaml
source:
  table: "`project.{% if filters.env == 'prod' %}prod_dataset{% else %}dev_dataset{% endif %}.events`"
```

### Metrics

Metrics define reusable aggregate calculations. Two types:

**Aggregate metrics** — map to SQL aggregation functions:

```yaml
metrics:
  page_views:
    aggregate: count                     # count, count_distinct, sum, avg, min, max
    # column not needed for count

  users:
    aggregate: count_distinct
    column: user_id                      # REQUIRED for all aggregates except count

  revenue:
    aggregate: sum
    column: amount

  high_value_orders:
    aggregate: count
    filter:                              # Optional: conditional aggregation
      status: completed
      amount_gt: 100                     # Generates: status = 'completed' AND amount_gt = '100'
```

**Expression metrics** — computed from other metrics (no SQL, evaluated client-side for scalars or inlined as SQL for dimensional queries):

```yaml
metrics:
  pages_per_session:
    expression: page_views / sessions    # Arithmetic using other metric names

  conversion_rate:
    expression: conversions / visits * 100
```

Supported aggregates: `count`, `count_distinct`, `sum`, `avg`, `min`, `max`.

Expression operators: `+`, `-`, `*`, `/`, parentheses. Division is automatically wrapped with `NULLIF(..., 0)` for safety.

### Dimensions

Dimensions define GROUP BY columns for chart widgets:

```yaml
dimensions:
  daily:
    column: event_date
    type: date                           # "date" = chronological ORDER BY ASC

  country:
    column: geo.country                  # Dotted paths work (aliased as "country")

  event:
    column: event_name                   # No type = ORDER BY metric DESC (top-N)
```

- `type: date` dimensions sort chronologically (ASC).
- Other dimensions sort by the first metric descending (top-N pattern).
- Dotted column names (e.g. `geo.country`) are auto-aliased to the last segment.

---

## Rows and Grid

Dashboards use a **12-column grid**. Each row contains widgets whose `col` values should sum to 12 (or less). If `col` is omitted, widgets share space equally.

```yaml
rows:
  - widgets:
      - name: Widget A
        col: 8                # Takes 8/12 columns
        # ...
      - name: Widget B
        col: 4                # Takes 4/12 columns
        # ...

  - widgets:
      - name: Full Width
        col: 12               # Full width
        # ...
```

### Row Height

Each row accepts an optional `height` to override its rendered height. Useful when you want charts in a row to be taller (or shorter) than the default.

- Number → pixels (e.g. `height: 480`). Charts inside the row expand to fill it.
- String pixel value → e.g. `"480px"`. Same behavior as a number.
- Other CSS strings → e.g. `"60vh"`, `"32rem"`. The row container takes that height, but charts fall back to their default 240px (use a pixel value if you want the chart to grow).
- Omitted → default height; widgets use their built-in sizing.

```yaml
rows:
  - height: 480               # Tall row — charts fill the extra vertical space
    widgets:
      - name: Revenue Trend
        type: chart
        chart: area
        col: 8
        # ...
      - name: By Region
        type: chart
        chart: pie
        col: 4
        # ...

  - widgets:                  # No height → default
      - name: Recent Orders
        type: table
        col: 12
        # ...
```

---

## Widget Types

Every widget (except `text`, `divider`, and `image`) needs a query source. **Priority order:**
1. `query: <name>` — reference a named query from the `queries:` map
2. `sql: |` — inline SQL

(In TSX, `include("path/to/query.sql")` reads a .sql file into an inline `sql` string at load time.)

### Metric Widget

Single KPI number card. Two modes: **declarative** (using top-level metrics) or **query-based** (using SQL).

**Declarative mode** — reference a top-level metric by name. No SQL needed:

```yaml
- name: Page Views
  type: metric
  metric: page_views              # References a metric from the metrics: map
  value:
    field: value                  # Result column (auto-aliased to "value" for metric refs)
    type: number
    format: ",.0f"                # Optional: d3-format string
  col: 3
```

All metric-ref widgets sharing the same dashboard are merged into a **single SQL query** for efficiency. Expression metrics (e.g. `pages_per_session`) are evaluated client-side from the query results.

The `value` encoding controls how the number is displayed:

- `value.field` — REQUIRED: which result column holds the number.
- `value.type` — `number`, `date`, or `category`.
- `value.format` — optional [d3-format](https://github.com/d3/d3-format) string. Currency and percent are expressed in the format string itself — there are no `prefix`/`suffix` fields. Examples: `",.0f"` (integer with thousands separators), `"$,.2f"` (currency), `".1%"` (percent).

`value` is always an object; `field` is required, `type` and `format` are optional.

**Query-based mode** — provide SQL directly:

```yaml
- name: Total Revenue
  type: metric
  query: total_revenue          # or sql:
  value:
    field: value                # REQUIRED: which result column to display
    type: number                # number | date | category
    format: "$,.2f"             # Optional: d3-format string (currency, percent, etc.)
  col: 3
```

When no formatting is needed, only `field` is required:

```yaml
- name: Total Orders
  type: metric
  sql: SELECT COUNT(*) as total FROM orders
  value: { field: total }
  col: 3
```

The SQL must return at least one row. The value from `value.field` in the first row is displayed.

### Chart Widget

Visualizations using Recharts. **17 chart types** available. Two modes: **dimensional** (using top-level dimensions + metrics) or **query-based** (using SQL with x/y columns).

#### Dimensional Charts (no SQL needed)

Reference top-level dimensions and metrics. SQL is auto-generated with GROUP BY, ORDER BY, and optional LIMIT:

```yaml
- name: Daily Traffic
  type: chart
  chart: area                       # line | bar | area (or any x/y chart type)
  dimension: daily                  # References a dimension from dimensions: map
  metrics: [page_views, users]      # References metrics from metrics: map
  col: 8

- name: Top Countries
  type: chart
  chart: bar
  dimension: country                # Non-date dimension = sorted by first metric DESC
  metrics: [users]
  limit: 8                          # Optional: limit number of results
  col: 4

- name: Pages/Session Trend
  type: chart
  chart: line
  dimension: daily
  metrics: [pages_per_session]      # Expression metrics work too — inlined as SQL
  col: 4
```

- Date dimensions (`type: date`) sort chronologically (ASC).
- Other dimensions sort by the first metric descending (top-N).
- Expression metrics are automatically inlined as SQL with `NULLIF` division safety.
- The `x` and `y` fields are auto-set by the loader — no need to specify them.

#### Query-Based Charts (SQL mode)

`x` and `y` are encoding objects. `field` is required and names the SQL column to plot (`y.field` accepts a list for multiple series). Optional keys control rendering:

- `type` — `number` | `date` | `category`. Picks the axis scale and format language (`date` → d3-time-format, otherwise d3-format).
- `title` — human-readable axis label.
- `format` — d3-format / d3-time-format string for tick labels: `"$,.0f"` → `$1,234`, `".0%"` → `12%`, `"%b %Y"` → `Jan 2024`.

Bare column names (`x: month`, `y: [revenue]`) are invalid — always wrap in `{ field: ... }`.

#### Line / Bar / Area

```yaml
- name: Revenue Trend
  type: chart
  chart: line                   # line | bar | area
  sql: |
    SELECT month, revenue, target FROM monthly_data ORDER BY month
  x: { field: month, type: date, format: "%b %Y" }    # REQUIRED: x encoding
  y: { field: [revenue, target], format: "$,.0f" }    # REQUIRED: y encoding (field may be a list)
  col: 8
```

#### Series by Category (color) / Stacked / Horizontal Bars

`color` splits the single `y` series into one series per distinct value of a category column. The SQL returns **long format** — one row per x/category pair — no `CASE WHEN` pivoting:

```yaml
- name: Sales by Region
  type: chart
  chart: bar
  stacked: true                 # bar only; REQUIRES color
  color: { field: region }      # one stacked series per region
  sql: |
    SELECT month, region, SUM(amount) AS revenue
    FROM sales GROUP BY 1, 2 ORDER BY 1, 2
  x: { field: month }
  y: { field: revenue }         # single column — color does the splitting
  col: 6
```

- `color` works on `bar`, `line`, and `area` charts and requires a single `y` field.
- `stacked: true` is bar-only and requires `color`. Multiple bare `y` columns render as **grouped** bars, never stacked.
- `normalized: true` (with `stacked`) shows each bar as percentages of the row total; omit `y.format`, values display as `%` automatically.
- `horizontal: true` (bar only) flips the chart: categories on the vertical axis, values on the horizontal.

#### Pie

```yaml
- name: Revenue by Region
  type: chart
  chart: pie
  sql: |
    SELECT region, SUM(amount) as total FROM sales GROUP BY 1
  label: region                 # REQUIRED: category column
  value: { field: total }                  # REQUIRED: numeric column
  col: 4
```

#### Scatter

```yaml
- name: Price vs Quantity
  type: chart
  chart: scatter
  sql: SELECT price, quantity FROM orders
  x: { field: price }
  y: { field: [quantity] }
  col: 6
```

X axis auto-detects numeric vs category data.

#### Bubble

```yaml
- name: Sales Bubble
  type: chart
  chart: bubble
  sql: SELECT region, revenue, profit, order_count FROM summary
  x: { field: region }                     # X axis
  y: { field: [revenue] }                  # Y axis
  size: order_count             # REQUIRED: bubble size column
  col: 6
```

#### Combo (mixed bar + line)

```yaml
- name: Revenue vs Growth
  type: chart
  chart: combo
  sql: SELECT month, revenue, growth_pct FROM monthly
  x: { field: month }
  y: { field: [revenue, growth_pct] }
  lines: [growth_pct]           # Which y series render as lines (rest are bars)
  col: 8
```

#### Histogram

```yaml
- name: Order Distribution
  type: chart
  chart: histogram
  sql: SELECT amount FROM orders
  x: { field: amount }                     # REQUIRED: column to bin
  bins: 20                      # Optional: number of bins (default: 10)
  col: 6
```

Client-side binning of raw data values.

#### Boxplot

```yaml
- name: Amount by Status
  type: chart
  chart: boxplot
  sql: SELECT status, amount FROM orders
  x: { field: status }                     # Category column
  y: { field: [amount] }                   # Numeric column
  col: 6
```

Client-side quartile computation from raw data rows.

#### Funnel

```yaml
- name: Conversion Funnel
  type: chart
  chart: funnel
  sql: SELECT stage, count FROM funnel_data ORDER BY count DESC
  label: stage                  # REQUIRED: category column
  value: { field: count }                  # REQUIRED: numeric column
  col: 6
```

#### Sankey

```yaml
- name: Flow Diagram
  type: chart
  chart: sankey
  sql: SELECT source_stage, target_stage, flow_count FROM flows
  source: source_stage          # REQUIRED: source node column
  target: target_stage          # REQUIRED: target node column
  value: { field: flow_count }             # REQUIRED: flow weight column
  col: 8
```

#### Heatmap

```yaml
- name: Activity Heatmap
  type: chart
  chart: heatmap
  sql: SELECT day_of_week, hour, event_count FROM activity
  x: { field: hour }                       # REQUIRED: X axis column
  y: { field: [day_of_week] }              # REQUIRED: Y axis column (array with 1 element)
  value: { field: event_count }            # REQUIRED: intensity column
  col: 8
```

Custom SVG rendering with hover tooltips.

#### Calendar

```yaml
- name: Daily Revenue
  type: chart
  chart: calendar
  sql: SELECT date, revenue FROM daily_sales
  x: { field: date }                       # REQUIRED: date column (YYYY-MM-DD)
  value: { field: revenue }                # REQUIRED: intensity column
  col: 12
```

GitHub-style calendar heatmap, custom SVG.

#### Sparkline

```yaml
- name: Revenue Sparkline
  type: chart
  chart: sparkline
  sql: SELECT month, revenue FROM monthly ORDER BY month
  x: { field: month }
  y: { field: [revenue] }
  col: 3
```

Compact line chart (60px height), no axes or labels. Great for KPI rows.

#### Waterfall

```yaml
- name: P&L Waterfall
  type: chart
  chart: waterfall
  sql: |
    SELECT category, amount FROM pnl
    ORDER BY CASE category
      WHEN 'Revenue' THEN 1
      WHEN 'COGS' THEN 2
      WHEN 'OpEx' THEN 3
      WHEN 'Net' THEN 4
    END
  x: { field: category }
  y: { field: [amount] }
  col: 8
```

Positive values shown in one color, negative in another. Bars float to show cumulative effect.

#### XMR (Control Chart)

```yaml
- name: Process Control
  type: chart
  chart: xmr
  sql: SELECT date, value, mean, ucl, lcl FROM process_data
  x: { field: date }
  y: { field: [value, mean] }              # First = data line, second = center line (dashed)
  yMin: lcl                     # Lower control limit (dashed)
  yMax: ucl                     # Upper control limit (dashed)
  col: 8
```

#### Dumbbell

```yaml
- name: H1 vs H2 Revenue
  type: chart
  chart: dumbbell
  sql: SELECT region, h1_revenue, h2_revenue FROM comparison
  x: { field: region }                     # Category column (vertical axis)
  y: { field: [h1_revenue, h2_revenue] }   # Two numeric columns (start and end points)
  col: 6
```

Horizontal chart showing range between two values per category.

#### Gauge

```yaml
- name: Revenue vs Target
  type: chart
  chart: gauge
  sql: SELECT current_revenue, revenue_target FROM kpi
  value: { field: current_revenue }        # REQUIRED: current value column (first row)
  target: revenue_target        # Optional: target/max column (default 100)
  col: 3
```

Semi-circular progress gauge for KPI-vs-target. Reads the first row.

#### Treemap

```yaml
- name: Revenue by Category
  type: chart
  chart: treemap
  sql: SELECT category, revenue FROM sales
  label: category               # REQUIRED: label column
  value: { field: revenue }                # REQUIRED: size column
  col: 6
```

Rectangular hierarchy showing part-to-whole proportions. Use instead of pie when slices exceed ~7.

#### Radar

```yaml
- name: Product Scorecard
  type: chart
  chart: radar
  sql: SELECT attribute, product_a, product_b FROM scorecard
  x: { field: attribute }                  # REQUIRED: axis category column
  y: { field: [product_a, product_b] }     # REQUIRED: one or more series to compare
  col: 6
```

Multi-axis comparison across a small number of entities.

#### Candlestick

```yaml
- name: Daily Price
  type: chart
  chart: candlestick
  sql: SELECT date, open, high, low, close FROM ohlc ORDER BY date
  x: { field: date }                       # REQUIRED: time column
  open: open                    # REQUIRED
  high: high                    # REQUIRED
  low: low                      # REQUIRED
  close: close                  # REQUIRED
  col: 12
```

OHLC chart for financial/pricing data. Green when close ≥ open, red otherwise.

### Table Widget

Data table with optional column configuration.

```yaml
- name: Recent Orders
  type: table
  sql: |
    SELECT id, customer_name, amount, status, created_at
    FROM orders ORDER BY created_at DESC LIMIT 25
  columns:                      # Optional: customize column display
    - name: customer_name       # Must match SQL column name
      label: Customer           # Display header
    - name: amount
      label: Amount
      format: currency          # "currency" adds $ prefix, "number" for locale formatting
    - name: created_at
      label: Date               # ISO dates auto-format to readable strings
  col: 12
```

If `columns` is omitted, all result columns are shown with their SQL names as headers.

### Text Widget

Static content with markdown formatting. No query needed.

```yaml
- name: Notes
  type: text
  content: |
    # Section Header

    **Important:** Revenue figures are updated daily.

    Data source: Snowflake `analytics.sales`

    - Bullet point one
    - Bullet point two

    1. Ordered item
    2. Another item

    > This is a blockquote for callouts

    Visit [our docs](https://example.com) for details.

    ---

    *Italic text*, **bold text**, ~~strikethrough~~, and `inline code`.
  col: 12
```

**Supported markdown syntax:**
- Headers: `#` through `######`
- Bold: `**text**` or `__text__`
- Italic: `*text*` or `_text_`
- Bold italic: `***text***`
- Strikethrough: `~~text~~`
- Inline code: `` `code` ``
- Links: `[text](url)`
- Images: `![alt](src)`
- Unordered lists: `- item` or `* item`
- Ordered lists: `1. item`
- Blockquotes: `> text`
- Horizontal rules: `---`, `***`, or `___`

### Divider Widget

A visual horizontal separator line. No query or content needed.

```yaml
- name: separator
  type: divider
  col: 12
```

Use dividers to visually separate sections within a dashboard.

### Image Widget

Displays an image from a URL. No query needed.

```yaml
- name: Company Logo
  type: image
  src: https://example.com/logo.png    # REQUIRED: image URL
  alt: Company logo                     # Optional: alt text
  col: 4
```

---

## Query Templating (Jinja)

SQL queries support Jinja syntax for filter variable substitution. Filter values are available under `filters.<name>`.

**Variable interpolation:**
```sql
WHERE created_at >= '{{ filters.date_range.start }}'
  AND created_at <= '{{ filters.date_range.end }}'
```

**Conditionals:**
```sql
{% if filters.region != 'All' %}
  AND region = '{{ filters.region }}'
{% endif %}
```

**Accessing nested values (date-range):**
```sql
{{ filters.date_range.start }}
{{ filters.date_range.end }}
```

**Accessing simple values (select, date, number, text):**
```sql
{{ filters.region }}
{{ filters.as_of_date }}
{{ filters.min_revenue }}
{{ filters.search }}
```

**Multi-select values (`multiple: true`)** — value is a list, render with `join` and guard the empty case:
```sql
{% if filters.status and filters.status | length > 0 %}
  AND status IN ('{{ filters.status | join("','") }}')
{% endif %}
```

---

## TSX Dashboards (Code-Based)

TSX dashboards use JSX syntax that maps directly to the same widget types as YAML. The file is transpiled with esbuild and executed with goja at load time.

### Basic TSX Dashboard

```tsx
// sales.dashboard.tsx
export default (
  <Dashboard name="Sales Analytics" connection="local_duckdb">
    <Filter name="region" type="select" default="All"
      options={{ values: ["All", "NA", "EU", "APAC"] }} />
    <Filter name="date_range" type="date-range" default="last_30_days" />

    <Row>
      <Metric name="Revenue" col={3}
        sql="SELECT SUM(amount) as value FROM sales"
        value={{ field: "value", type: "number", format: "$,.2f" }} />
      <Metric name="Orders" col={3}
        sql="SELECT COUNT(*) as value FROM orders"
        value={{ field: "value", type: "number", format: ",.0f" }} />
    </Row>

    <Row>
      <Chart name="Trend" chart="area" col={8}
        sql="SELECT month, revenue FROM monthly ORDER BY 1"
        x={{ field: "month" }} y={{ field: ["revenue"] }} />
      <Chart name="By Region" chart="pie" col={4}
        sql="SELECT region, SUM(amount) as total FROM sales GROUP BY 1"
        label="region" value={{ field: "total" }} />
    </Row>

    <Row>
      <Table name="Recent Orders" col={12}
        sql="SELECT * FROM orders ORDER BY created_at DESC LIMIT 20" />
    </Row>
  </Dashboard>
)
```

### JSX Tag Reference

Every YAML widget type has a corresponding JSX tag. Props map directly to YAML fields:

| JSX Tag | YAML `type:` | Props |
|---------|-------------|-------|
| `<Dashboard>` | (root) | `name`, `connection`, `description`, `theme`, `refresh` |
| `<Row>` | (row) | `height` |
| `<Filter>` | (filter) | `name`, `type`, `default`, `multiple`, `options` |
| `<Query>` | (named query) | `name`, `sql`, `file`, `connection` |
| `<Semantic>` | (semantic layer) | `source`, `metrics`, `dimensions` |
| `<Metric>` | `metric` | `name`, `col`, `sql`, `query`, `value`, `metric` |
| `<Chart>` | `chart` | `name`, `col`, `chart`, `sql`, `x`, `y`, `label`, `value`, `color`, `stacked`, `normalized`, `horizontal`, `dimension`, `metrics`, `limit`, etc. |
| `<Table>` | `table` | `name`, `col`, `sql`, `query`, `columns` |
| `<Text>` | `text` | `name`, `col`, `content` |
| `<Divider>` | `divider` | `name`, `col` |
| `<Image>` | `image` | `name`, `col`, `src`, `alt` |

### Custom Components

Define reusable widget patterns as functions — impossible in YAML:

```tsx
function KPI({ name, sql, format = ",.0f", ...rest }) {
  return <Metric name={name} sql={sql} value={{ field: "value", type: "number", format }} {...rest} />
}

export default (
  <Dashboard name="Sales" connection="duckdb">
    <Row>
      <KPI name="Revenue" sql="SELECT SUM(amount) as value FROM sales" format="$,.0f" col={4} />
      <KPI name="Orders" sql="SELECT COUNT(*) as value FROM orders" col={4} />
    </Row>
  </Dashboard>
)
```

### Loops and Variables

Generate widgets programmatically — impossible in YAML:

```tsx
const regions = ["NA", "EU", "APAC"]

export default (
  <Dashboard name="Sales" connection="duckdb">
    <Row>
      {regions.map(r =>
        <Metric name={`${r} Revenue`} col={4}
          sql={`SELECT SUM(amount) as value FROM sales WHERE region = '${r}'`}
          value={{ field: "value", type: "number", format: "$,.2f" }} />
      )}
    </Row>
  </Dashboard>
)
```

### Data-Driven Dashboards with `query()`

`query(connection, sql)` executes SQL at dashboard load time and returns `{ columns, rows }`. Use it to build dashboards that adapt to the database:

```tsx
// Discover regions and statuses from the database at load time
const regions = query("duckdb", "SELECT DISTINCT region FROM sales ORDER BY 1")
const statuses = query("duckdb", "SELECT DISTINCT status FROM orders ORDER BY 1")
const tables = query("duckdb",
  "SELECT table_name FROM information_schema.tables WHERE table_schema = 'main' ORDER BY 1"
)

function KPI({ name, sql, format = ",.0f", ...rest }) {
  return <Metric name={name} sql={sql} value={{ field: "value", type: "number", format }} {...rest} />
}

export default (
  <Dashboard name="Sales (TSX)" connection="duckdb">
    {/* Filter options built from live data */}
    <Filter name="region" type="select" default="All"
      options={{ values: ["All", ...regions.rows.map(r => r[0])] }} />

    {/* Auto-generated per-region KPIs — adapts when data changes */}
    <Row>
      {regions.rows.map(([region]) => (
        <KPI name={region} format="$,.0f"
          col={Math.floor(12 / regions.rows.length)}
          sql={`SELECT SUM(amount) as value FROM sales WHERE region = '${region}'`} />
      ))}
    </Row>

    {/* Long-format SQL + color — the engine splits revenue into one series per region */}
    <Row>
      <Chart name="Revenue by Region" chart="bar" stacked={true} color={{ field: "region" }} col={8}
        sql={`
          SELECT STRFTIME(DATE_TRUNC('month', created_at), '%Y-%m') AS month,
            region, SUM(amount) AS revenue
          FROM sales GROUP BY 1, 2 ORDER BY 1, 2
        `}
        x={{ field: "month" }}
        y={{ field: "revenue" }} />
    </Row>

    {/* Auto-generated table preview for every table in the DB */}
    {tables.rows.map(([name]) => (
      <Row>
        <Table name={name} col={12}
          sql={`SELECT * FROM "${name}" ORDER BY created_at DESC LIMIT 10`} />
      </Row>
    ))}
  </Dashboard>
)
```

**Offline behavior:** When no backend is available (e.g. `dac validate`), `query()` returns `{ columns: [], rows: [] }` so the file still loads — data-driven sections produce zero widgets.

### Two-Phase Templating

TSX dashboards support both JS template literals (resolved at load time) and Jinja markers (resolved at query time per request):

```tsx
<Chart name="Revenue" chart="line"
  sql={`SELECT month, SUM(amount) as rev
    FROM sales
    WHERE region = '{{ filters.region }}'
    AND created_at >= '{{ filters.date_range.start }}'
    GROUP BY 1 ORDER BY 1`}
  x={{ field: "month" }} y={{ field: ["rev"] }} />
```

- **`${...}`** (JS template literal) — resolved when goja runs the script at load time
- **`{{ ... }}`** (Jinja) — preserved in the SQL string, resolved per request with filter values

### `include()` — Read SQL Files

```tsx
const sql = include("queries/recent_orders.sql")

export default (
  <Dashboard name="Orders" connection="duckdb">
    <Row>
      <Table name="Recent" sql={sql} col={12} />
    </Row>
  </Dashboard>
)
```

### `require()` — Import Shared Modules

Import shared `.tsx`, `.js`, or `.json` files using CommonJS `require()`:

```tsx
// lib/kpi.tsx
function KPI({ name, sql, format = ",.0f", ...rest }) {
  return <Metric name={name} sql={sql} value={{ field: "value", type: "number", format }} {...rest} />
}
module.exports = { KPI }

// sales.dashboard.tsx
const { KPI } = require("./lib/kpi")

export default (
  <Dashboard name="Sales" connection="duckdb">
    <Row>
      <KPI name="Revenue" sql="..." format="$,.0f" col={4} />
    </Row>
  </Dashboard>
)
```

- Paths resolve relative to the importing file
- `.tsx`/`.ts`/`.jsx` files are auto-transpiled
- Extension auto-resolution: `require("./lib/kpi")` tries `.tsx`, `.ts`, `.jsx`, `.js`, `.json`
- Module cache: each file is executed once

### Semantic Layer in TSX

```tsx
<Dashboard name="Google Analytics" connection="gcp-default">
  <Semantic
    source={{ table: "events", dateColumn: "event_date", dateFormat: "%Y%m%d" }}
    metrics={{
      page_views: { aggregate: "count", filter: { event_name: "page_view" } },
      users: { aggregate: "count_distinct", column: "user_id" },
      pages_per_session: { expression: "page_views / sessions" },
    }}
    dimensions={{
      daily: { column: "event_date", type: "date" },
      country: { column: "geo.country" },
    }}
  />

  <Row>
    <Metric name="Page Views" metric="page_views" col={3} />
    <Metric name="Users" metric="users" col={3} />
  </Row>

  <Row>
    <Chart name="Daily Traffic" chart="area" col={8}
      dimension="daily" metrics={["page_views", "users"]} />
  </Row>
</Dashboard>
```

### TypeScript IDE Support

Reference `dac.d.ts` (shipped at the repo root) for autocomplete and type checking:

```tsx
/// <reference path="../../dac.d.ts" />

export default (
  <Dashboard name="My Dashboard" connection="duckdb">
    {/* Full autocomplete for all tags, props, and globals */}
  </Dashboard>
)
```

---

## CLI Commands

### `dac serve` — Start dev server

```bash
dac serve --dir ./dashboards
dac serve --dir ./dashboards --port 9000 --template bruin-dark
dac serve --dir ./dashboards --template ./themes/corporate.yml
```

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--port` | `-p` | `8321` | Port (auto-increments if taken) |
| `--dir` | `-d` | `.` | Dashboard directory |
| `--template` | `-t` | `bruin` | Template: `bruin`, `bruin-dark`, or path to `.yml` |
| `--host` | | `localhost` | Bind host |
| `--open` | | `false` | Open browser |

### `dac validate` — Validate YAML structure

```bash
dac validate --dir ./dashboards
```

Checks YAML syntax, required fields, column sums, and query references. Does not execute queries.

### `dac check` — Deep validation (YAML + execute all queries)

```bash
dac check --dir ./dashboards
```

Goes beyond `validate`: parses YAML, resolves all query references, applies default filter values, executes every query, and reports results with row/column counts and timing. Catches SQL errors, missing tables, bad column names.

> **Cost warning:** runtime scales linearly with the total widget count across all dashboards in `--dir`. Reserve it for end-of-task sweeps. For per-edit checks, use `dac query --dashboard X --widget Y` to run a single widget instead. See the [Validation Workflow](#validation-workflow--read-before-editing) section at the top.

### `dac query` — Run a SQL query

```bash
# Inline SQL
dac query -c local_duckdb "SELECT * FROM sales LIMIT 5"

# From a .sql file
dac query -c local_duckdb -f queries/my_query.sql

# Run a specific widget's query (resolves named refs, applies default filters)
dac query -d ./dashboards --dashboard "Sales Analytics" --widget "Total Revenue"

# Output formats: table (default), json, csv
dac query -c local_duckdb "SELECT 1" -o json
```

| Flag | Short | Description |
|------|-------|-------------|
| `--connection` | `-c` | Connection name |
| `--file` | `-f` | Path to `.sql` file |
| `--dashboard` | | Dashboard name (with `--widget`) |
| `--widget` | `-w` | Widget name (with `--dashboard`) |
| `--output` | `-o` | Output format: `table`, `json`, `csv` |
| `--dir` | `-d` | Dashboard directory (for `--dashboard`) |

### `dac ls` — List dashboards

```bash
dac ls --dir ./dashboards
```

Shows all discovered dashboards with widget count, filter count, and connection.

### `dac connections` — Test connections

```bash
dac connections --dir ./dashboards
```

Tests each connection in `.bruin.yml` by running `SELECT 1`. Reports connection status.

### Global flags

| Flag | Short | Description |
|------|-------|-------------|
| `--config` | `-c` | Path to `.bruin.yml` (default: auto-discover) |
| `--environment` | `-e` | Target environment name |
| `--debug` | | Enable debug logging |

---

## Custom Themes

Create a `themes/*.yml` file in the dashboard directory for token overrides:

```yaml
# themes/corporate.yml
name: corporate
extends: bruin                        # Inherit from built-in template
tokens:
  background: "#FAFAFA"
  surface: "#FFFFFF"
  border: "#E5E7EB"
  text-primary: "#111827"
  accent: "#0052CC"
  chart-1: "#0052CC"
  chart-2: "#00B8D9"
  chart-3: "#8B5CF6"
  # Missing tokens fall back to the base template
```

**Available tokens:** `background`, `surface`, `surface-hover`, `border`, `text-primary`, `text-secondary`, `text-muted`, `accent`, `accent-hover`, `accent-subtle`, `success`, `warning`, `error`, `chart-1` through `chart-8`.

---

## Complete Examples

### Declarative Dashboard (source + metrics + dimensions)

Best for dashboards with KPI cards and standard charts over a single source table. No SQL needed for most widgets.

```yaml
name: Web Analytics
description: Traffic and engagement metrics
connection: gcp-default

filters:
  - name: date_range
    type: date-range
    default:
      start: "2025-01-01"
      end: "2025-12-31"

source:
  table: analytics.events
  date_column: event_date

dimensions:
  daily:
    column: event_date
    type: date

  country:
    column: geo.country

metrics:
  page_views:
    aggregate: count
    filter:
      event_name: page_view

  users:
    aggregate: count_distinct
    column: user_id

  sessions:
    aggregate: count
    filter:
      event_name: session_start

  pages_per_session:
    expression: page_views / sessions

rows:
  # KPI row — all 4 metrics execute as a single SQL query
  - widgets:
      - name: Page Views
        type: metric
        metric: page_views
        value:
          field: value
          type: number
          format: ",.0f"
        col: 3
      - name: Users
        type: metric
        metric: users
        value:
          field: value
          type: number
          format: ",.0f"
        col: 3
      - name: Sessions
        type: metric
        metric: sessions
        value:
          field: value
          type: number
          format: ",.0f"
        col: 3
      - name: Pages / Session
        type: metric
        metric: pages_per_session
        value:
          field: value
          type: number
          format: ".2f"
        col: 3

  # Dimensional charts — SQL auto-generated from source + metrics + dimensions
  - widgets:
      - name: Daily Traffic
        type: chart
        chart: area
        dimension: daily
        metrics: [page_views, users]
        col: 8
      - name: Pages/Session Trend
        type: chart
        chart: line
        dimension: daily
        metrics: [pages_per_session]
        col: 4

  - widgets:
      - name: Top Countries
        type: chart
        chart: bar
        dimension: country
        metrics: [users]
        limit: 8
        col: 6

      # You can still use raw SQL alongside declarative widgets
      - name: Top Pages
        type: table
        col: 6
        sql: |
          SELECT page_title as page, COUNT(*) as views
          FROM analytics.events
          WHERE event_name = 'page_view'
            AND event_date >= '{{ filters.date_range.start }}'
          GROUP BY 1 ORDER BY 2 DESC LIMIT 10
        columns:
          - name: page
            label: Page
          - name: views
            label: Views
            format: number
```

### Query-Based Dashboard (SQL mode)

Best for complex queries, JOINs, custom transformations, or multi-source dashboards.

```yaml
name: Sales Analytics
description: Real-time sales performance
connection: local_duckdb

filters:
  - name: region
    type: select
    default: "All"
    options:
      values: ["All", "North America", "Europe", "APAC"]
  - name: date_range
    type: date-range
    default: this_year

queries:
  total_revenue:
    sql: |
      SELECT SUM(amount) as value FROM sales
      WHERE created_at >= '{{ filters.date_range.start }}'
        AND created_at <= '{{ filters.date_range.end }}'
      {% if filters.region != 'All' %}
        AND region = '{{ filters.region }}'
      {% endif %}
  revenue_by_month:
    sql: |
      SELECT DATE_TRUNC('month', created_at) AS month, SUM(amount) AS revenue
      FROM sales
      WHERE created_at >= '{{ filters.date_range.start }}'
        AND created_at <= '{{ filters.date_range.end }}'
      GROUP BY 1 ORDER BY 1

rows:
  - widgets:
      - name: Total Revenue
        type: metric
        query: total_revenue
        value:
          field: value
          type: number
          format: "$,.2f"
        col: 4
      - name: Total Orders
        type: metric
        col: 4
        sql: SELECT COUNT(*) as total FROM orders
        value:
          field: total
          type: number
          format: ",.0f"
      - name: Avg Order
        type: metric
        col: 4
        sql: SELECT ROUND(AVG(amount), 2) as avg FROM orders
        value:
          field: avg
          type: number
          format: "$,.2f"

  - widgets:
      - name: Revenue Trend
        type: chart
        chart: area
        query: revenue_by_month
        x: { field: month }
        y: { field: [revenue] }
        col: 8
      - name: By Region
        type: chart
        chart: pie
        col: 4
        sql: |
          SELECT region, SUM(amount) as total
          FROM sales GROUP BY 1
        label: region
        value: { field: total }

  - widgets:
      - name: Recent Orders
        type: table
        col: 12
        sql: |
          SELECT id, customer_name, amount, status, created_at
          FROM orders ORDER BY created_at DESC LIMIT 20
        columns:
          - name: id
            label: Order ID
          - name: customer_name
            label: Customer
          - name: amount
            label: Amount
            format: currency
          - name: status
            label: Status
          - name: created_at
            label: Date
```

---

## Widget Type Reference

| Type | Required Fields | Query Source | Description |
|------|----------------|--------------|-------------|
| `metric` | `metric:` ref OR `value` + query | Declarative or SQL | Single KPI number card |
| `chart` | `dimension` + `metrics` OR `chart` + x/y + query | Declarative or SQL | Visualization (21 chart types) |
| `table` | — | SQL | Data table with optional column config |
| `text` | `content` | None | Markdown/text content |
| `divider` | — | None | Horizontal separator line |
| `image` | `src` | None | Image from URL |

### Chart Types

| Chart | Required | Optional | Description |
|-------|----------|----------|-------------|
| `line` | `x`, `y` | `color` | Line chart |
| `bar` | `x`, `y` | `color`, `stacked`, `normalized`, `horizontal` | Bar chart |
| `area` | `x`, `y` | `color` | Area chart |
| `pie` | `label`, `value` | | Pie/donut chart |
| `scatter` | `x`, `y` | | Scatter plot |
| `bubble` | `x`, `y`, `size` | | Bubble chart |
| `combo` | `x`, `y`, `lines` | | Mixed bar + line chart |
| `histogram` | `x` | `bins` | Histogram (client-side binning) |
| `boxplot` | `x`, `y` | | Box-and-whisker plot (client-side quartiles) |
| `funnel` | `label`, `value` | | Funnel chart |
| `sankey` | `source`, `target`, `value` | | Sankey/flow diagram |
| `heatmap` | `x`, `y`, `value` | | Grid heatmap |
| `calendar` | `x`, `value` | | Calendar heatmap (GitHub-style) |
| `sparkline` | `x`, `y` | | Compact inline line (60px) |
| `waterfall` | `x`, `y` | | Waterfall chart |
| `xmr` | `x`, `y` | `yMin`, `yMax` | Control chart with limits |
| `dumbbell` | `x`, `y` (2 fields) | | Horizontal range comparison |
| `gauge` | `value` | `target` | Semi-circular KPI-vs-target gauge (uses first row) |
| `treemap` | `label`, `value` | | Rectangular part-to-whole hierarchy |
| `radar` | `x`, `y` | | Polar/spider chart for multi-metric comparison |
| `candlestick` | `x`, `open`, `high`, `low`, `close` | | OHLC chart |

---

## Validation Rules

- `name` is required on the dashboard and every widget.
- At least one row is required; each row needs at least one widget.
- `col` must be 1-12; total per row must not exceed 12.
- Every widget that requires data needs a query source (`query`, `sql`, `file`) OR a declarative reference (`metric:` for metric widgets, `dimension:` + `metrics:` for chart widgets).
- `metric` widgets require either `metric: <name>` (declarative) or `value` + query source (SQL mode).
- `chart` widgets require `chart` type plus either `dimension` + `metrics` (declarative) or chart-specific fields (SQL mode).
- `text` widgets require `content`.
- `image` widgets require `src`.
- `divider` widgets have no required fields.
- Filter types must be one of: `select`, `date-range`, `date`, `number`, `text`.
- Named query references (`query: name`) must exist in the `queries:` map.
- `source` is required when `metrics` or `dimensions` are defined; `source.table` is required.
- Each metric must have either `aggregate` or `expression` (not both).
- Valid aggregates: `count`, `count_distinct`, `sum`, `avg`, `min`, `max`.
- Non-count aggregates require `column`.
- Expression metrics can only reference other defined metrics.
- Dimensions require `column`; `type` must be `"date"` or omitted.
- `metric:` refs must reference a metric in the `metrics:` map.
- `dimension:` refs must reference a dimension in the `dimensions:` map.
- `metrics:` list refs on chart widgets must all exist in the `metrics:` map.

Run `dac validate` for structure checks, or `dac check` to also execute all queries and verify they return data.
