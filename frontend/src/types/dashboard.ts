export interface DashboardSummary {
  name: string;
  description?: string;
  connection?: string;
  widget_count?: number;
  filter_count?: number;
  row_count?: number;
}

export interface Dashboard {
  name: string;
  description?: string;
  connection?: string;
  model?: string;
  models?: Record<string, string>;
  filters?: Filter[];
  queries?: Record<string, Query>;
  rows: Row[];
  file_type?: "yaml" | "tsx";
}

export interface Filter {
  name: string;
  type: "date" | "date-range" | "number" | "select" | "text";
  multiple?: boolean;
  default?: unknown;
  options?: {
    values?: string[];
    query?: string;
    connection?: string;
    presets?: string[];
  };
}

export interface Query {
  sql?: string;
  connection?: string;
  model?: string;
  dimensions?: SemanticDimensionRef[];
  metrics?: string[];
  filters?: SemanticQueryFilter[];
  segments?: string[];
  sort?: SemanticSort[];
  limit?: number;
}

export interface Row {
  tab?: string;
  height?: number | string;
  widgets: Widget[];
}

export interface Widget {
  name: string;
  description?: string;
  type: "metric" | "chart" | "table" | "text" | "divider" | "image";
  col?: number;

  // Query source
  query?: string;
  sql?: string;
  connection?: string;
  model?: string;

  // Declarative metric/dimensional
  metric?: string;
  dimension?: string;
  granularity?: string;
  dimensions?: SemanticDimensionRef[];
  metrics?: string[];
  filters?: SemanticQueryFilter[];
  segments?: string[];
  sort?: SemanticSort[];

  // Chart
  chart?: "line" | "bar" | "area" | "pie" | "scatter" | "bubble" | "combo" | "histogram" | "boxplot" | "funnel" | "sankey" | "heatmap" | "calendar" | "sparkline" | "waterfall" | "xmr" | "dumbbell" | "gauge" | "treemap" | "radar" | "candlestick" | "forest";
  x?: AxisEncoding;
  y?: AxisEncoding;
  // Optional second value axis on the right (line/area/bar/combo). A y-column
  // listed in y2.field plots against the right axis; all others stay on the left.
  y2?: AxisEncoding;
  label?: string;
  // metric: the value + formatting; pie/funnel/heatmap/calendar/treemap/gauge: value column
  value?: ValueEncoding;
  // bar/line/area: split the single y series by a category column (long-format SQL)
  color?: ColorEncoding;
  stacked?: boolean;    // bar only; requires color
  normalized?: boolean; // stacked bars as percentages
  horizontal?: boolean; // bar only
  size?: string;       // bubble: size dimension
  source?: string;     // sankey: source column
  target?: string;     // sankey: target column / gauge: target (max) column
  bins?: number;       // histogram: number of bins
  lines?: string[];    // combo: which y series are lines
  series?: Record<string, SeriesStyle>; // per-series line style overrides, keyed by y-column
  // xmr control limit column; line/bar/forest CI bound — a column name, or a
  // per-series map { series column: bound column } for multi-line CI bands.
  yMin?: string | Record<string, string>;
  yMax?: string | Record<string, string>;
  refLines?: RefLine[];  // reference guide lines (axis + value + optional label)
  refBands?: RefBand[];  // shaded reference bands (axis + from/to + optional label)
  open?: string;       // candlestick: open column
  high?: string;       // candlestick: high column
  low?: string;        // candlestick: low column
  close?: string;      // candlestick: close column

  // Table
  columns?: TableColumn[];

  // Text
  content?: string;

  // Image
  src?: string;
  alt?: string;
}

export interface RefLine {
  axis: "x" | "y";
  value: number;
  label?: string;
  color?: string;
}

export interface RefBand {
  axis: "x" | "y";
  from: number;
  to: number;
  label?: string;
  color?: string;
}

export interface TableColumn {
  name: string;
  label?: string;
  /** Value display: `currency`, `number`, or a d3-format spec. */
  number?: string;
  /** Mirror another column's coloring, driven by that column's per-row value. */
  like?: string;
  /** Keep the column in the result (for cross-column rules / `like`) but don't render it. */
  hidden?: boolean;
  /** Text alignment override — applies to the header and body cells. */
  align?: 'left' | 'center' | 'right';
  /** Ordered style layers; the first layer that matches a cell wins. */
  format?: FormatLayer[];
}

export type FormatOperator =
  | "is_empty"
  | "is_not_empty"
  | "text_contains"
  | "text_does_not_contain"
  | "text_starts_with"
  | "text_ends_with"
  | "text_is_exactly"
  | "date_is"
  | "date_before"
  | "date_after"
  | "greater_than"
  | "greater_than_or_equal"
  | "less_than"
  | "less_than_or_equal"
  | "is_equal_to"
  | "is_not_equal_to"
  | "is_between"
  | "is_not_between";

/** Reference to another column in the same row, used as a rule comparison value. */
export interface ColumnRef {
  column: string;
}

export type RuleValue = number | string | ColumnRef | Array<number | string | ColumnRef>;

/**
 * One entry in a column's `format` list. The first layer that matches a cell wins.
 *
 * - A layer with `if` is a **condition** (applies to matching cells).
 * - A layer without `if` always matches — a **gradient** (`backgroundColor` array,
 *   optional `range`/`unit`) or a **flat fill** (`backgroundColor` string). Place
 *   it last as the fallback base.
 *
 * `backgroundColor`: string = flat/single fill · array = gradient.
 * `range` + `unit` (`absolute` | `percent` | `percentile`) pin a gradient's
 * anchors; omit `range` for an auto min/max gradient.
 */
export interface FormatLayer {
  if?: FormatOperator;
  value?: RuleValue;
  backgroundColor?: string | string[];
  range?: number[];
  unit?: "absolute" | "percent" | "percentile";
  textColor?: string;
  bold?: boolean;
  italic?: boolean;
  underline?: boolean;
  strikethrough?: boolean;
}

/** Structured encoding for a chart axis. `y.field` may list several series columns. */
export interface AxisEncoding {
  field: string | string[];
  type?: "number" | "date" | "category";
  title?: string;  // human-readable axis label
  format?: string; // d3-format / d3-time-format string for tick labels
  beginAtZero?: boolean; // anchor a line/area value axis at 0
  markers?: boolean; // show point markers on line/area (default true)
  curve?: "smooth" | "straight" | "stepline"; // chart-wide line interpolation
  dash?: "solid" | "dotted" | "dashed" | "long-dash"; // chart-wide dash pattern (omitted = solid)
}

/** Per-series style override, grouped under widget-level `series` (keyed by y-column). */
export interface SeriesStyle {
  color?: string; // #hex; unset uses the palette
  curve?: "smooth" | "straight" | "stepline"; // falls back to y.curve
  dash?: "solid" | "dotted" | "dashed" | "long-dash"; // falls back to y.dash
}

/** Encoding for the color channel: the category column that splits y into series. */
export interface ColorEncoding {
  field: string;
}

/** Structured encoding for a widget's value channel (metric value, pie/funnel/gauge value column). */
export interface ValueEncoding {
  field: string;
  type?: "number" | "date" | "category";
  format?: string; // d3-format / d3-time-format string, e.g. "$,.2f", ".0%", "%b %Y"
}

export interface SemanticDimensionRef {
  name: string;
  granularity?: string;
}

export interface SemanticQueryFilter {
  dimension?: string;
  operator?: "equals" | "not_equals" | "gt" | "gte" | "lt" | "lte" | "in" | "not_in" | "between" | "is_null" | "is_not_null";
  value?: unknown;
  expression?: string;
}

export interface SemanticSort {
  name: string;
  direction?: "asc" | "desc";
}

export interface WidgetData {
  columns: { name: string; type?: string }[];
  rows: unknown[][];
  query?: string;
  error?: string;
}

export interface BatchDataResponse {
  widgets: Record<string, WidgetData>;
}
