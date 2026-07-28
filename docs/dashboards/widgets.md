# Widgets

Widgets are the visual building blocks of a dashboard. Each widget occupies a number of columns in a 12-column grid.

## Common Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | Yes | Display name |
| `type` | string | Yes | `metric`, `chart`, `table`, `text`, `image`, or `divider` |
| `col` | integer | No | Column span from 1 to 12 |
| `description` | string | No | Optional tooltip or subtitle |

Data-backed widgets (`metric`, `chart`, `table`) also need a query source: `query` (named query reference), `sql` (inline), `file` (external `.sql` path), a semantic reference (`metric:` for metric widgets, `dimension:` + `metrics:` for charts), or `data` (inline static values — see [Inline data](#inline-data)). See [Queries & Templating](/dashboards/queries) for the full reference, including the `connection` override.

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
| `format` | string \| object | Value display and conditional coloring, see below |

## Conditional formatting

A column's `format` handles the number format **and** coloring together. It is
either a **string** (shorthand for the number format) or an **object**. The keys
and options are below; a full worked example follows at the end.

```yaml
format: currency          # shorthand: just the number format

format:                   # object: number format + coloring
  number: "$,.2f"
  backgroundColor: [red, white, green]
```

**`format` keys**

| Key | What it does |
|-----|--------------|
| `number` | Value display: `currency`, `number`, or a d3-format string. |
| `backgroundColor` | **string** = flat fill for the whole column. **array** = gradient (at least 2 colors). |
| `domain` | Gradient anchors. A raw array `[-25, 0, 25]`, or `{ unit, anchors }` where `unit` is `value` (default) / `percent` / `percentile`. Omit for auto min/max; omit `anchors` to spread evenly (percentile puts the midpoint at the median). |
| `scheme` | A built-in gradient by name instead of listing colors. |
| `textColor` | Text color. |
| `bold` / `italic` / `underline` / `strikethrough` | Text styling (booleans). |
| `rules` | Conditional styling, evaluated in order, first match wins. |
| `like` | Copy another column's coloring, driven by that column's value. |

**Colors** (any color field)

| Kind | Values |
|------|--------|
| Named | `red` `green` `blue` `indigo` `cyan` `purple` `pink` `amber` |
| Fixed | `white` `black` |
| Aliases | `positive` (green), `negative` (red), `warning` (amber) |
| Hex | any `"#rrggbb"` |

Named colors match the chart palette and adapt to light/dark; `white` and `black` are fixed literals that stay the same in both themes.

**Built-in `scheme`s:** `red-white-green`, `green-white-red`, `red-amber-green`, `green-amber-red`, `white-green`, `white-red`, `white-blue`.

**Rule fields** (each rule = one condition + one or more styles)

| Field | What it does |
|-------|--------------|
| `if` | Operator (required). See the table below. |
| `value` | Scalar to compare against. Use `[low, high]` for between, `{ column: X }` for cross column, omit for empty checks. |
| `backgroundColor` / `textColor` | Cell colors. |
| `bold` / `italic` / `underline` / `strikethrough` | Text styling. At least one style is required. |

**Operators**

| Group | Operators |
|-------|-----------|
| Empty | `is_empty`, `is_not_empty` |
| Text (case insensitive) | `text_contains`, `text_does_not_contain`, `text_starts_with`, `text_ends_with`, `text_is_exactly` |
| Number | `greater_than`, `greater_than_or_equal`, `less_than`, `less_than_or_equal`, `is_equal_to`, `is_not_equal_to` |
| Range | `is_between`, `is_not_between` |
| Date | `date_is`, `date_before`, `date_after` (by day for a date, or exact instant if the value has a time) |

**Worked example** (a full dashboard exercising every feature above):

```yaml
name: Regions

rows:
  - widgets:
      - name: Regions
        type: table
        col: 12
        sql: SELECT region, revenue, growth, margin, score, status, actual, target, bonus, due FROM regions
        columns:
          - name: region
            format: { backgroundColor: "#F8FAFC", bold: true }          # flat fill + text style

          - name: revenue
            format:
              number: currency                                          # value display: currency
              scheme: red-white-green                                   # built-in gradient

          - name: growth
            format:
              number: number
              domain: [-25, 0, 25]                                      # raw anchors (0 = neutral middle)
              backgroundColor: [blue, white, amber]                     # custom gradient colors

          - name: margin
            format:
              number: ".0%"                                             # d3-format string
              scheme: red-amber-green                                   # built-in scheme
              domain: { unit: percentile }                              # anchor by percentile (midpoint = median)

          - name: score
            format:
              number: number
              rules:                                                    # numeric rules, first match wins
                - { if: greater_than_or_equal, value: 80, backgroundColor: green }    # scalar value
                - { if: is_between, value: [50, 79], backgroundColor: amber }          # [low, high] for between
                - { if: less_than, value: 50, textColor: red, strikethrough: true }    # text style

          - name: status
            format:
              rules:                                                    # text rules
                - { if: text_contains, value: urgent, backgroundColor: amber, bold: true }
                - { if: text_is_exactly, value: overdue, backgroundColor: red }
                - { if: is_empty, backgroundColor: "#F3F4F6", italic: true }           # no value for empty checks

          - name: actual
            format:
              number: number
              rules:
                - { if: less_than, value: { column: target }, backgroundColor: red }    # cross-column, same row
                - { if: greater_than, value: { column: target }, backgroundColor: green }

          - name: bonus
            format: { number: currency, like: score }                  # mirror score's colors, keep own number

          - name: due
            format:
              rules:                                                    # date rules
                - { if: date_before, value: "2026-07-22", textColor: red }
                - { if: date_after, value: "2026-07-22", textColor: green }
```

How the example reads, column by column:
- `region`: one flat color on every cell, bold text (a plain string fill plus a text style).
- `revenue`: currency, colored by the built-in `red-white-green` gradient.
- `growth`: a custom blue/white/amber gradient with the neutral point pinned to 0 via a raw `domain`.
- `margin`: a percentage, colored by the built-in `red-amber-green` scheme, anchored by `percentile` so the midpoint sits at the median.
- `score`: numeric rules covering two `value` forms, a scalar (`>= 80`) and a `[low, high]` range (`50 to 79`), plus a text style (`< 50` struck through in red).
- `status`: text rules checked in order, first match wins (contains "urgent", exactly "overdue", or empty).
- `actual`: cross-column rules, compared to `target` in the same row (explained next).
- `bonus`: `like: score`, so it takes score's colors per row but keeps its own currency display.
- `due`: date rules, before or after a cutoff day.

Cross column, step by step. Look at the `actual` rule:

```yaml
{ if: less_than, value: { column: target }, backgroundColor: red }
```

The cell being colored is `actual`. `value: { column: target }` means "compare it to the `target` column in the same row", not to a fixed number. So for every row independently: if that row's `actual` is less than that row's `target`, the `actual` cell turns red. Row 1 uses row 1's target, row 2 uses row 2's target, and so on. `target` can even be a column you do not display in the table, because its data is still available for the comparison. (Writing `value: 100` instead would compare against the constant 100.)

## Inline data

`metric`, `chart`, and `table` widgets can carry their data inline instead of a
query source. A widget with `data` renders without a connection or SQL — useful
for hardcoded numbers, sample dashboards, or building a dashboard before a
warehouse is connected. The encoding fields (`x`, `y`, `value`, `label`,
`columns`) reference the column names declared in `data.columns`.

```yaml
- name: Revenue by Quarter
  type: chart
  chart: bar
  col: 6
  data:
    columns: [quarter, revenue]
    rows:
      - [Q1, 12000]
      - [Q2, 15500]
      - [Q3, 14200]
      - [Q4, 18900]
  x: { field: quarter, type: category }
  y: { field: [revenue], type: number, format: "$,.0f" }
```

`data` fields:

| Field | Type | Description |
|-------|------|-------------|
| `columns` | list of strings | Column names; referenced by the encoding fields |
| `rows` | list of lists | One list per row, positional — each must have one value per column |

`data` is mutually exclusive with `sql` and `query`. It is not valid on `text`,
`image`, or `divider` widgets.

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

## Exports

The dashboard viewer can export widget data as CSV and rendered dashboard or widget views as PNG/PDF. In `dac serve`, exports use the currently applied filters. In static `dac build` dashboards, exports use the query results baked into the HTML at build time.

### Per-widget Exports

Chart and table widgets show an export menu in the widget header on hover or keyboard focus:

- **CSV** downloads that widget's result set as `<widget-name>.csv` (RFC 4180 CSV with quoted cells).
- **PNG** captures the rendered widget as `<widget-name>.png`.
- **PDF** captures the rendered widget as `<widget-name>.pdf`.

### Dashboard Exports

The dashboard header has an **Export** menu:

- **CSV** exports every data-bearing widget (metrics, charts, and tables) in one file. Each widget is written as a section introduced by a `# <widget-name>` divider line, followed by the column header and rows, with a blank line separating sections. The file is named `<dashboard-name>.csv`.
- **PNG** captures the rendered dashboard view as `<dashboard-name>.png`.
- **PDF** captures the rendered dashboard view as `<dashboard-name>.pdf`.

In `dac serve`, CSV exports reflect the active filter values at the time of the click. Static dashboard CSV exports use the baked query results. Text, divider, and image widgets are skipped in CSV exports. Widgets with query errors or no rows are omitted from the dashboard CSV.

PNG and PDF exports capture the visible rendered view, excluding viewer controls such as export buttons and YAML controls. For tabbed dashboards, they capture the currently active tab.
