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
| `line` | `x`, `y` | `color`, `y2`, `yMin`, `yMax` | Line chart. `yMin`/`yMax` shade a CI band behind the line |
| `bar` | `x`, `y` | `color`, `y2`, `stacked`, `normalized`, `horizontal`, `yMin`, `yMax` | Bar chart. `yMin`/`yMax` add error-bar caps |
| `area` | `x`, `y` | `color`, `y2`, `yMin`, `yMax` | Area chart. `yMin`/`yMax` shade a CI band |
| `pie` | `label`, `value` | | Pie/donut chart |
| `scatter` | `x`, `y` | | Scatter plot |
| `bubble` | `x`, `y`, `size` | | Bubble chart |
| `combo` | `x`, `y` | `lines`, `y2` | Mixed bar + line chart (y series in `lines` render as lines, the rest as bars) |
| `histogram` | `x` | `bins` | Histogram (client-side binning) |
| `boxplot` | `x`, `y` | | Box-and-whisker plot (client-side quartiles) |
| `funnel` | `label`, `value` | `horizontal` | Conversion funnel: one bar per stage with each stage's share of the top and step-to-step conversion. Rows render in query order — put the top of the funnel first. `horizontal: true` lays stages left-to-right |
| `sankey` | `source`, `target`, `value` | | Sankey/flow diagram |
| `heatmap` | `x`, `y`, `value` | `showValues`, `colorScale` | Grid heatmap. `showValues: true` prints each cell's value inside the cell; `colorScale` replaces the default blue buckets |
| `calendar` | `x`, `value` | | GitHub-style calendar heatmap |
| `sparkline` | `x`, `y` | | Compact inline line (60px), no axes |
| `waterfall` | `x`, `y` | | Waterfall chart |
| `xmr` | `x`, `y` | `yMin`, `yMax` | Control chart with limits |
| `dumbbell` | `x`, `y` (2 columns) | | Horizontal range comparison |
| `gauge` | `value` | `target` | Semi-circular KPI-vs-target gauge (uses first row) |
| `treemap` | `label`, `value` | | Rectangular part-to-whole hierarchy |
| `radar` | `x`, `y` | | Multi-axis polar comparison |
| `candlestick` | `x`, `open`, `high`, `low`, `close` | | OHLC chart |
| `forest` | `x`, `y` | `yMin`, `yMax`, `horizontal` | Point estimate + CI interval per category (grey when the interval spans 0). `horizontal: false` → vertical dot-and-whisker; multiple `y` = grouped |
| `vega-lite` | `spec` | | Advanced Vega-Lite visualization using DAC query data. Supports layers, facets, concatenation, transforms, and selections |

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
| `beginAtZero` | boolean | Anchor a line/area value axis at 0 so series scale honestly instead of auto-zooming. Opt-in (default auto-scales). |
| `markers` | boolean | Show point markers on sparse line/area series. Default `true`; set `false` to hide the dots. |
| `curve` | string | Chart-wide line interpolation: `smooth`, `straight`, or `stepline`. Use `straight` for period totals so the line doesn't imply movement between points. |
| `dash` | string | Chart-wide dash pattern every series inherits: `dotted`, `dashed`, or `long-dash` (omitted = solid). |

Per-series style overrides live in a **widget-level `series`** map (a sibling of `x`/`y`, not inside `y`), keyed by y-column: `series: { revenue: { color: "#EC4899", curve: straight, dash: dashed } }`. Each of `color`/`curve`/`dash` falls back to the chart-wide default (or palette for colour). Store only genuine differences.

Label/value charts (`pie`, `treemap`, `funnel`) have no y-column series — style their **slices** with a widget-level **`slices`** map keyed by the slice's data label: `slices: { Enterprise: { color: "#8B5CF6", label: "Enterprise (2026)" } }`. `color` overrides the palette; `label` renames the displayed slice. Both optional.

Without `format`, ticks fall back to automatic compact formatting. `beginAtZero`, `markers`, `curve`, and `dash` (on `y`) plus the widget-level `series` apply to line/area (and combo) charts. Bare column names (`x: month`, `y: [revenue]`) are not valid — always wrap the column in `{ field: ... }`.

### Second value axis (`y2`)

`y2` adds a right-hand value axis so two series on different scales share one chart without the smaller one being squashed flat — e.g. revenue in dollars against conversion rate in percent. A y-column plots against the right axis when it is listed in `y2.field`; every other series stays on the left `y` axis. `y2` is a full axis encoding, so it takes the same `title`, `format`, `beginAtZero`, `curve`, and `dash` keys as `y`, and each axis's ticks and tooltip values format independently.

`y2` is supported on `line`, `area`, `bar`, and `combo` charts. A column must belong to exactly one axis (no overlap between `y.field` and `y2.field`), `y2.type` must be `number`, and `y2` cannot be combined with `stacked`, `horizontal` bars, or `color` (a category split — list the right-axis columns in `y2.field` instead).

Axis assignment (`y` vs `y2`) and shape (bar vs line, via `lines`) are independent — the classic combo puts revenue bars on the left and a conversion-rate line on the right:

```yaml
- name: Revenue vs Conversion
  type: chart
  chart: combo
  lines: [conversion_rate]        # this series draws as a line, the rest as bars
  sql: |
    SELECT month, revenue, conversion_rate
    FROM monthly ORDER BY month
  x:  { field: month }
  y:  { field: [revenue],         title: Revenue,    format: "$,.0f", beginAtZero: true }
  y2: { field: [conversion_rate], title: Conversion, format: ".1%" }
  col: 6
```

The left `y` axis title renders above the plot (as for single-axis charts); the `y2` title renders rotated alongside the right axis. Left/right gridlines are not tick-aligned.

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
| `y` | object | Y-axis encoding (`field` may list several series columns; supports `beginAtZero`, `markers`, `curve`, `dash`, and per-series `series` overrides for line/area — see [Axis encoding](#axis-encoding)) |
| `label` | string | Label column for pie, funnel, and treemap charts |
| `value` | object | Value encoding (`{ field: ... }`) for pie, funnel, sankey, heatmap, calendar, treemap, and gauge charts |
| `slices` | object | Per-slice style overrides for pie/treemap/funnel, keyed by slice label: `{ Enterprise: { color: "#8B5CF6", label: "Enterprise (2026)" } }` |
| `source` | string | Source node column for sankey charts |
| `target` | string | Target/max column for gauge charts (also sankey target node) |
| `open` | string | Open column for candlestick charts |
| `high` | string | High column for candlestick charts |
| `low` | string | Low column for candlestick charts |
| `close` | string | Close column for candlestick charts |
| `color` | object | Category column (`{ field: ... }`) that splits the single `y` series into one series per category — bar, line, and area charts |
| `stacked` | boolean | Stack the color series (bar charts only; requires `color`) |
| `normalized` | boolean | Render stacked bars as percentages of the row total (requires `stacked`) |
| `horizontal` | boolean | Horizontal layout: bar charts put categories on the vertical axis; funnel charts lay stages left-to-right; forest charts are horizontal by default (`false` = vertical) |
| `size` | string | Bubble size column for bubble charts |
| `colorScale` | object | Heatmap color ramp: `backgroundColor` (2+ colors, low→high) plus optional `range` anchors and `unit` — the same keys a table gradient uses, resolved by the same code. Scaled across every cell in the grid, unlike a table gradient which is scaled per column. Omit for the default blue buckets |
| `showValues` | boolean | Print each cell's value inside the cell (heatmap charts only). A number too wide for its cell is omitted — the shade still carries the magnitude and the tooltip has the exact value |
| `lines` | string[] | Which `y` series render as lines (rest as bars) for combo charts |
| `series` | object | Per-series line style overrides keyed by y-column: `{ revenue: { color, curve, dash } }` — line/area/combo. Each key falls back to the chart-wide `y.curve`/`y.dash`/palette |
| `bins` | integer | Number of bins for histogram charts (default 10) |
| `yMin` | string \| object | CI lower bound: xmr control limit, line/area CI band, bar error-bar cap, forest interval. A column name, or a per-series map `{ seriesColumn: boundColumn }` for multi-line/grouped charts |
| `yMax` | string \| object | CI upper bound (see `yMin`) |
| `refLines` | object[] | Reference guide lines: `[{ axis: x\|y, value, label?, color? }]` — dashed line (e.g. a 0 no-effect mark). line/area/bar/forest |
| `refBands` | object[] | Shaded reference bands: `[{ axis: x\|y, from, to, label?, color? }]` — a translucent range (e.g. a ±MDE band). line/area/bar/forest |
| `spec` | object | Vega-Lite specification for `chart: vega-lite`. DAC data is injected as the named `dac` dataset |
| `dimension` | string | Semantic dimension name |
| `granularity` | string | Semantic time grain for `dimension` |
| `metrics` | string[] | Semantic metric names |
| `filters` | array | Semantic filters |
| `segments` | string[] | Semantic segments |
| `sort` | array | Sort instructions |
| `limit` | integer | Row limit |

Charts using `dimension`, `metrics`, `segments`, or semantic `filters` are compiled through the backend semantic layer instead of requiring hand-written SQL.

### Heatmap color scale

By default a heatmap paints four blue buckets (Low → Very High) scaled across the
whole grid. `colorScale` replaces them with a continuous ramp described in the
same vocabulary as a table gradient:

```yaml
- name: Revenue vs Channel Average
  type: chart
  chart: heatmap
  x: { field: channel, type: category }
  y: { field: [region] }
  value: { field: delta, format: "$,.0f" }
  showValues: true
  colorScale:
    backgroundColor: [red, white, green]   # low → high, 2 or more colors
    range: [-500, 0, 500]                  # one anchor per color; omit for auto min/max
    unit: absolute                         # absolute (default) | percent | percentile
```

Pinning `range` does two things worth knowing: it puts the neutral color exactly
on zero (so the sign is readable at a glance), and it keeps the colors stable as
filters change — without it the ramp rescales to whatever the current result set
contains, so two filtered views are not comparable.

The default buckets round near-equal values to the same shade; a `colorScale`
interpolates, so small differences stay visible.

Colors accept the same names as table formatting (`red`, `green`, `blue`,
`indigo`, `cyan`, `purple`, `pink`, `amber`, plus `white`/`black` and the
`positive`/`negative`/`warning` aliases) or hex. Cell-value ink is derived from
each cell's fill, so `showValues` stays readable on any ramp.

### Vega-Lite charts

Use `chart: vega-lite` when a visualization needs composition beyond DAC's built-in chart options, such as layered marks, independent scales, faceting, concatenation, or Vega-Lite transforms. Built-in charts remain the concise default for standard visualizations.

DAC executes the widget's `sql`, named `query`, semantic query, or inline `data` exactly like any other chart. The result is converted to records and made available to the Vega-Lite specification as the named `dac` dataset:

```yaml
- name: Revenue with confidence interval
  type: chart
  chart: vega-lite
  col: 12
  sql: |
    SELECT month, revenue, lower_ci, upper_ci
    FROM monthly_revenue ORDER BY month
  spec:
    data: { name: dac }
    encoding:
      x: { field: month, type: temporal }
    layer:
      - mark: { type: area, opacity: 0.14 }
        encoding:
          y: { field: lower_ci, type: quantitative, title: Revenue }
          y2: { field: upper_ci }
      - mark: { type: line, strokeWidth: 2 }
        encoding:
          y: { field: revenue, type: quantitative }
      - mark: { type: point, filled: true }
        encoding:
          y: { field: revenue, type: quantitative }
```

`spec.data` may be omitted; DAC inserts `{ name: dac }` automatically. If present, it must use that name. `data.url` and `datasets.dac` are rejected: load primary chart data through DAC so filters, validation, CSV export, and static builds continue to work consistently. Other inline named datasets may be included for small supporting values.

For a single or layered view, DAC supplies responsive width, row-derived height, and fit autosizing when the spec does not set them. Explicit `width`, `height`, `autosize`, and `config` values win. DAC also supplies theme-aware axes, legends, tooltips, typography, and the `chart-1` through `chart-8` categorical palette; values in `spec.config` override those defaults.

PNG and PDF exports wait for Vega-Lite's asynchronous render to finish before capturing the widget or dashboard.

See `examples/basic-yaml/dashboards/vega-lite.yml` for a Sankey-style customer journey built from curved weighted ribbons, layered confidence intervals, independent dual axes, a lollipop chart, and a SQL-backed revenue heatmap with centered, contrast-aware value labels.

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
| `align` | string | Text alignment override: `left`, `center`, or `right`. Applies to the column header and its body cells. Use it to right-align a text value like `£177K` that isn't detected as numeric. |
| `hidden` | boolean | Keep the column in the result but don't render it, see [Hidden columns](#hidden-columns) |
| `format` | string \| object | Value display and conditional coloring, see below |

## Conditional formatting

A column sets value display with `number` and coloring with `format`. `number`
is `currency`, `number`, or a d3-format string. `format` is an **ordered list of
layers**, and for each cell the **first layer that matches wins**.

`format` is polymorphic for backward compatibility: a scalar string (e.g.
`format: currency`) is the older value-display shorthand and is treated as an
alias for `number`; a list is the conditional-format layers. Prefer `number`
in new dashboards, and use a list `format` for coloring (the two can coexist).

A layer's `if` is optional: with `if`, the layer styles only the cells that match
the condition; without `if`, the layer styles every cell.

```yaml
- name: score
  number: number
  format:
    - { if: greater_than_or_equal, value: 80, backgroundColor: green }   # matches only score >= 80
    - { backgroundColor: [red, white, green] }                           # no if → every other cell, so last
```

**Column keys**

| Key | What it does |
|-----|--------------|
| `number` | Value display: `currency`, `number`, or a d3-format string. |
| `align` | Text alignment: `left`, `center`, or `right` (aligns the header and body cells). |
| `like` | Mirror another column's coloring, driven by that column's per-row value (keeps own `number`). |
| `hidden` | Keep the column in the result (so it can drive cross-column rules or be a `like` source) but don't render it. |
| `format` | Ordered list of style layers; first match wins. |

**Layer keys** (the `Group` column shows which keys belong together):

| Group | Key | What it does |
|-------|-----|--------------|
| Condition | `if` | The operator (see the table below). Omit to apply the layer to every cell. |
| Condition | `value` | What `if` compares against: a scalar, `[low, high]` for between, `{ column: X }` for cross-column, omitted for empty checks. |
| Fill | `backgroundColor` | A **string** (flat fill) or a **list** of colors (gradient, at least 2). |
| Fill | `range` + `unit` | Gradient only. `range` is a list of anchor numbers; `unit` is `absolute`, `percent`, or `percentile`. Omit `range` for an auto min/max gradient. |
| Text | `textColor` | Text color. |
| Text | `bold` / `italic` / `underline` / `strikethrough` | Text styling (booleans). |

Each layer is a YAML object, so these two forms are identical — use whichever reads better:

```yaml
- { backgroundColor: [red, white, green], range: [-25, 0, 25], unit: absolute }   # compact (flow)

- backgroundColor: [red, white, green]                                            # block
  range: [-25, 0, 25]
  unit: absolute
```

**Colors** (any color field)

| Kind | Values |
|------|--------|
| Named | `red` `green` `blue` `indigo` `cyan` `purple` `pink` `amber` |
| Fixed | `white` `black` |
| Aliases | `positive` (green), `negative` (red), `warning` (amber) |
| Hex | any `"#rrggbb"` |

Named colors match the chart palette and adapt to light/dark; `white` and `black` are fixed literals that stay the same in both themes. Named fills are softened toward the theme surface so the theme text stays legible.

**Operators** (used by a layer's `if`)

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
            format:
              - { backgroundColor: "#F8FAFC", bold: true }              # flat fill + text style

          - name: revenue
            number: currency                                            # value display: currency
            format:
              - { backgroundColor: [red, white, green] }                # gradient, auto min→max

          - name: growth
            number: number
            format:                                                     # compact form (0 = neutral middle)
              - { backgroundColor: [blue, white, amber], range: [-25, 0, 25], unit: absolute }

          - name: margin
            number: ".0%"                                               # d3-format string; same layer, block form
            format:
              - backgroundColor: [red, amber, green]
                range: [0, 50, 100]
                unit: percentile                                        # anchor by percentile (midpoint = median)

          - name: score
            number: number
            format:                                                     # conditions, first match wins
              - { if: greater_than_or_equal, value: 80, backgroundColor: green, textColor: white, bold: true }   # fill + text styles together
              - { if: is_between, value: [50, 79], backgroundColor: amber }         # [low, high] for between
              - { if: less_than, value: 50, backgroundColor: "#FEE2E2", textColor: red, strikethrough: true }    # fill + text style

          - name: status
            format:                                                     # text conditions
              - { if: text_contains, value: urgent, backgroundColor: amber, bold: true }
              - { if: text_is_exactly, value: overdue, backgroundColor: red }
              - { if: is_empty, backgroundColor: "#F3F4F6", italic: true }          # no value for empty checks

          - name: actual
            number: number
            format:
              - { if: less_than, value: { column: target }, backgroundColor: red }   # cross-column, same row
              - { if: greater_than, value: { column: target }, backgroundColor: green }

          - name: target
            hidden: true                                                # in the result for the rule above, not rendered

          - name: bonus
            number: currency
            like: score                                                 # mirror score's colors, keep own number

          - name: due
            format:                                                     # date conditions (fill + text)
              - { if: date_before, value: "2026-07-22", backgroundColor: red, textColor: white, bold: true }
              - { if: date_after, value: "2026-07-22", backgroundColor: green, textColor: white }

          - name: health
            number: number
            format:                                                     # order matters: this wins over the layer below
              - { if: is_equal_to, value: 0, backgroundColor: red, bold: true }
              - { backgroundColor: [red, white, green], range: [0, 50, 100], unit: percent }  
```

**Order matters.** Layers are checked top to bottom and the first match wins, so
list the specific conditions first and any catch-all (a gradient or flat fill with
no `if`) last. `growth` is written in the compact one-line form and `margin` as an
indented block; both are the same layer shape.

How the example reads, column by column:
- `region`: one flat color on every cell, bold text (a single layer, string fill plus a text style).
- `revenue`: currency, colored by an auto min→max gradient.
- `growth`: a blue/white/amber gradient with the neutral point pinned to 0 via `range` + `unit: absolute`.
- `margin`: a percentage, colored by a gradient anchored by `percentile` so the midpoint sits at the median.
- `score`: numeric conditions covering two `value` forms, a scalar (`>= 80`) and a `[low, high]` range (`50 to 79`); each layer combines a `backgroundColor` fill with text styles (`< 50` gets a light-red fill plus red struck-through text).
- `status`: text conditions checked in order, first match wins (contains "urgent", exactly "overdue", or empty).
- `actual`: cross-column conditions, compared to `target` in the same row (explained next).
- `target`: `hidden: true`, so it stays in the result to drive `actual`'s comparison but is not rendered as its own column.
- `bonus`: `like: score`, so it takes score's colors per row but keeps its own currency display.
- `due`: date conditions, before or after a cutoff day.
- `health`: a condition (`= 0` turns red) checked first, then a `percent`-range gradient with no `if` last, for every other value.

Cross column, step by step. Look at the `actual` rule:

```yaml
{ if: less_than, value: { column: target }, backgroundColor: red }
```

The cell being colored is `actual`. `value: { column: target }` means "compare it to the `target` column in the same row", not to a fixed number. So for every row independently: if that row's `actual` is less than that row's `target`, the `actual` cell turns red. Row 1 uses row 1's target, row 2 uses row 2's target, and so on. `target` is marked `hidden: true` here, so it drives the comparison without taking up a column of its own. (Writing `value: 100` instead would compare against the constant 100.)

## Hidden columns

A column with `hidden: true` stays in the query result but is not rendered. It is optional: coloring reads a column whether or not it's shown. Hide only to drop a column from the display.

```yaml
columns:
  - name: actual
    number: number
    format:
      - { if: less_than, value: { column: target }, backgroundColor: red }
  - name: target
    hidden: true      # feeds the rule above, never rendered
```

Hidden columns are still in the underlying data, so they appear in CSV exports. `hidden` has no effect on non-table widgets.

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
