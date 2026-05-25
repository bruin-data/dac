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
  theme?: string;
  refresh?: { interval: string };
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
  file?: string;
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
  file?: string;
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

  // Metric
  column?: string;
  prefix?: string;
  suffix?: string;
  format?: string;

  // Chart
  chart?: "line" | "bar" | "area" | "pie" | "scatter" | "bubble" | "combo" | "histogram" | "boxplot" | "funnel" | "sankey" | "heatmap" | "calendar" | "sparkline" | "waterfall" | "xmr" | "dumbbell" | "gauge" | "treemap" | "radar" | "candlestick";
  x?: string;
  y?: string[];
  label?: string;
  value?: string;
  stacked?: boolean;
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
