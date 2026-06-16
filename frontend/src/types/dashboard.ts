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
  chart?: "line" | "bar" | "area" | "pie" | "scatter" | "bubble" | "combo" | "histogram" | "boxplot" | "funnel" | "sankey" | "heatmap" | "calendar" | "sparkline" | "waterfall" | "xmr" | "dumbbell" | "gauge" | "treemap" | "radar" | "candlestick";
  x?: AxisEncoding;
  y?: AxisEncoding;
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
  yMin?: string;       // xmr: min control limit column
  yMax?: string;       // xmr: max control limit column
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

export interface TableColumn {
  name: string;
  label?: string;
  format?: string;
}

/** Structured encoding for a chart axis. `y.field` may list several series columns. */
export interface AxisEncoding {
  field: string | string[];
  type?: "number" | "date" | "category";
  title?: string;  // human-readable axis label
  format?: string; // d3-format / d3-time-format string for tick labels
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
