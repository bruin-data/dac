import type { Filter, Query, Widget, WidgetData } from "../types/dashboard.ts";

export interface CrossFilterBinding {
  field: string;
  filter: Filter;
}

export interface CrossFilterSource {
  widgetId: string;
  value: unknown;
  keepScope: boolean;
}

export function crossFilterDefaultValue(filter: Filter, defaults: Record<string, unknown>): unknown {
  if (defaults[filter.name] != null) return defaults[filter.name];
  // The backend rejects a null date-range; an omitted key applies no range.
  return filter.type === "select" && filter.multiple ? [] : filter.type === "date-range" ? undefined : null;
}

const MIDNIGHT_UTC = /^(\d{4}-\d{2}-\d{2})T00:00:00(?:\.0+)?Z$/;

/**
 * Filter values compare by meaning: multi-selects as sets, YAML scalars (2024,
 * midnight-UTC dates) like their string form, and "" like no value.
 */
export function sameCrossFilterValue(a: unknown, b: unknown): boolean {
  const scalar = (value: unknown) => value == null || value === "" ? null
    : typeof value === "number" || typeof value === "boolean" ? String(value)
    : typeof value === "string" ? value.replace(MIDNIGHT_UTC, "$1") : value;
  const normalize = (value: unknown) => JSON.stringify(Array.isArray(value) ? value.map(scalar).sort() : scalar(value));
  return normalize(a) === normalize(b);
}

/** Bring a current or default value to the form a click produces, so they compare equal. */
function clickForm(filter: Filter, value: unknown): unknown {
  return crossFilterValue(filter, value) ?? value;
}

/** Only the originating multi-select widget keeps its pre-click query scope. */
export function crossFilterQueryFilters(widgetId: string, filters: Record<string, unknown> | undefined, sources?: Record<string, CrossFilterSource>) {
  const own = Object.entries(sources ?? {}).filter(([, source]) => source.widgetId === widgetId && source.keepScope);
  if (!own.length) return filters;
  return { ...filters, ...Object.fromEntries(own.map(([name, source]) => [name, source.value])) };
}

function templateStrings(value: unknown): string[] {
  if (typeof value === "string") return [value];
  if (Array.isArray(value)) return value.flatMap(templateStrings);
  if (value && typeof value === "object") return Object.values(value).flatMap(templateStrings);
  return [];
}

/** Find template filter dependencies in SQL or semantic filters; ignore Jinja comments and quoted Jinja literals. */
export function queryFilterReferences(query?: Query | Widget): Set<string> {
  const references = new Set<string>();
  for (const source of templateStrings(query?.sql ? query.sql : query?.filters)) {
    // Jinja comments never render. SQL comments still do, so `-- {% if filters.x %}` is a dependency.
    const code = source.replace(/{#[\s\S]*?#}/g, "");
    const blocks = code.match(/{{[\s\S]*?}}|{%[\s\S]*?%}/g) ?? [];
    for (const block of blocks) {
      // Skip quoted Jinja literals; support both filters.region and filters['region'].
      const tokens = block.matchAll(/\bfilters\s*(?:\.\s*([A-Za-z_]\w*)|\[\s*(["'])(.*?)\2\s*\])|"(?:\\.|[^"\\])*"|'(?:\\.|[^'\\])*'/g);
      for (const token of tokens) {
        const name = token[1] ?? token[3];
        if (name) references.add(name);
      }
    }
  }
  return references;
}

/** Only directly encoded scalar fields can route Vega clicks; bins, time units and aggregates are derived. */
function vegaCrossFilterFields(spec: unknown): string[] {
  if (!spec || typeof spec !== "object") return [];
  if (Array.isArray(spec)) return spec.flatMap(vegaCrossFilterFields);
  const node = spec as Record<string, unknown>;
  const fields: string[] = [];
  if (node.encoding && typeof node.encoding === "object") {
    for (const encoding of Object.values(node.encoding)) {
      for (const definition of [encoding].flat()) {
        // Vega-Lite escapes literal dots and brackets (`customers\\.country`); result columns and datums don't.
        if (definition && typeof definition === "object" && typeof definition.field === "string" &&
          !definition.aggregate && !definition.bin && !definition.timeUnit) fields.push(definition.field.replace(/\\(.)/g, "$1"));
      }
    }
  }
  for (const key of ["layer", "hconcat", "vconcat", "concat", "spec"]) fields.push(...vegaCrossFilterFields(node[key]));
  return fields;
}

/** Any matching result dimension may initiate an existing dashboard filter. */
export function resolveCrossFilterBindings(widget: Widget, filters: Filter[], data?: WidgetData): CrossFilterBinding[] {
  const supported = widget.type === "table" || widget.type === "pivot_table" || (widget.type === "chart" &&
    ["bar", "line", "area", "pie", "scatter", "bubble", "combo", "boxplot", "funnel", "sankey", "heatmap", "calendar", "sparkline", "waterfall", "xmr", "dumbbell", "treemap", "radar", "candlestick", "forest", "vega-lite"].includes(widget.chart ?? ""));
  if (!supported) return [];
  const x = Array.isArray(widget.x?.field) ? widget.x.field[0] : widget.x?.field;
  const targets = filters.filter((filter) => filter.type !== "date-range" || (widget.type === "chart" &&
    ["line", "area", "calendar", "candlestick", "sparkline", "xmr"].includes(widget.chart ?? "")));
  let fields: Array<string | undefined>;
  if (widget.type === "table" || widget.type === "pivot_table") {
    fields = widget.pivot
      ? [...(widget.pivot.rows ?? []), ...(widget.pivot.columns ?? [])].map((dimension) => dimension.field)
      : widget.columns?.length
      ? widget.columns.filter((column) => !column.hidden && (!column.type || column.type === "text")).map((column) => column.name)
      : data?.columns?.map((column) => column.name) ?? [];
  } else {
    if (widget.chart === "vega-lite") {
      fields = vegaCrossFilterFields(widget.spec);
    } else if (["pie", "funnel", "treemap"].includes(widget.chart ?? "")) {
      fields = [widget.label || x || "label"];
    } else if (["bar", "line", "area"].includes(widget.chart ?? "")) {
      fields = [x, widget.color?.field];
    } else if (widget.chart === "sankey") {
      fields = [widget.source || "source", widget.target || "target"];
    } else if (["scatter", "bubble", "heatmap"].includes(widget.chart ?? "")) {
      const y = Array.isArray(widget.y?.field) ? widget.y.field[0] : widget.y?.field;
      fields = [x, y];
    } else {
      fields = [x];
    }
  }
  // Configured table columns fall back to a case-insensitive result match, like TableWidget does.
  const caseInsensitive = (widget.type === "table" || widget.type === "pivot_table") && !widget.pivot && !!widget.columns?.length;
  // Results without column metadata serialize `columns` as null.
  const resultFields = data ? new Set((data.columns ?? []).map((column) => caseInsensitive ? column.name.toLowerCase() : column.name)) : undefined;
  const names = [...new Set(fields)].filter((field): field is string => field !== undefined &&
    (!resultFields || resultFields.has(caseInsensitive ? field.toLowerCase() : field)));
  return targets.flatMap((filter) => {
    const matches = names.filter((field) => (filter.type !== "date-range" || field === x) &&
      (widget.crossFilter !== undefined
        ? widget.crossFilter.some((mapping) => mapping.filter === filter.name &&
          (caseInsensitive ? mapping.field.toLowerCase() === field.toLowerCase() : mapping.field === field))
        : field === filter.name));
    // Sankey nodes and composed Vega marks may expose different aliases for the same filter.
    return (widget.type === "chart" && !["vega-lite", "sankey"].includes(widget.chart ?? "")
      ? matches.slice(0, 1) : matches).map((field) => ({ field, filter }));
  });
}

/** Date axes can return DATE strings or ISO timestamps; filter bounds are calendar dates. */
function crossFilterDay(raw: unknown): string | undefined {
  if (typeof raw !== "string" || !/^\d{4}-\d{2}-\d{2}(?:T\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:\d{2}))?$/.test(raw)) return undefined;
  const value = raw.slice(0, 10);
  const date = new Date(`${value}T00:00:00Z`);
  return Number.isFinite(date.getTime()) && date.toISOString().slice(0, 10) === value &&
    Number.isFinite(Date.parse(raw)) ? value : undefined;
}

const addDays = (day: string, n: number) => new Date(Date.parse(`${day}T00:00:00Z`) + n * 86_400_000).toISOString().slice(0, 10);

const gcd = (a: number, b: number): number => b ? gcd(b, a % b) : a;

/** Extend the range to include the full final period when its size is identifiable. */
function bucketEnd(axis: unknown[], end: string, current?: { start?: unknown; end?: unknown } | null): string {
  const days = [...new Set(axis.map(crossFilterDay).filter((day): day is string => !!day))].sort();
  const i = days.indexOf(end);
  if (i < 0) return end;
  if (days.length < 2) {
    const currentEnd = crossFilterDay(current?.end);
    return crossFilterDay(current?.start) === end && currentEnd && currentEnd >= end ? currentEnd : end;
  }
  const next = i + 1 < days.length ? addDays(days[i + 1], -1) : undefined;
  let last: string | undefined;
  if (days.every((day) => day.endsWith("-01"))) {
    const index = (day: string) => +day.slice(0, 4) * 12 + +day.slice(5, 7) - 1;
    const step = days.slice(1).map((day, k) => index(day) - index(days[k])).reduce(gcd);
    const months = [3, 6, 12].includes(step) ? step : 1;
    const after = index(end) + months;
    last = addDays(new Date(Date.UTC(Math.floor(after / 12), after % 12, 1)).toISOString().slice(0, 10), -1);
  } else {
    const step = days.slice(1).map((day, k) => Math.round((Date.parse(day) - Date.parse(days[k])) / 86_400_000)).reduce(gcd);
    if (step === 7) last = addDays(end, 6);
  }
  if (!last) return end;
  return next && next < last ? next : last;
}

/** Order a dragged range; with the axis values, `end` covers its whole bucket. */
export function crossFilterDateRange(from: unknown, to: unknown, axis?: unknown[], current?: { start?: unknown; end?: unknown } | null): { start: string; end: string } | undefined {
  const a = crossFilterDay(from), b = crossFilterDay(to);
  if (!a || !b) return undefined;
  const [start, end] = a <= b ? [a, b] : [b, a];
  return { start, end: axis ? bucketEnd(axis, end, current) : end };
}

/** Convert result values to the target filter's wire type; ignore unusable marks. */
export function crossFilterValue(filter: Filter, raw: unknown): string | number | undefined {
  if (raw == null || !["string", "number", "boolean"].includes(typeof raw)) return undefined;
  if (filter.type === "number") {
    if (typeof raw === "boolean" || String(raw).trim() === "") return undefined;
    const n = Number(raw);
    return Number.isFinite(n) ? n : undefined;
  }
  if (typeof raw === "number" && !Number.isFinite(raw)) return undefined;
  let value = String(raw);
  // The SQL backend rejects these too. Don't offer marks that cannot be queried.
  if (!value || /['\\]/.test(value) || /\p{Cc}/u.test(value)) return undefined;
  if (filter.type === "date") {
    // DATE columns arrive as midnight UTC timestamps; keep the date part, like the backend's normalizeDate.
    value = value.replace(MIDNIGHT_UTC, "$1");
    if (!/^\d{4}-\d{2}-\d{2}$/.test(value)) return undefined;
    const date = new Date(`${value}T00:00:00Z`);
    return !Number.isNaN(date.getTime()) && date.toISOString().slice(0, 10) === value ? value : undefined;
  }
  if (filter.type === "date-range") return undefined;
  if (filter.type === "select" && filter.options?.values?.length && !filter.options.values.includes(value)) return undefined;
  return value;
}

export function isCrossFilterSelected(filter: Filter, current: unknown, value: string | number): boolean {
  return filter.type === "select" && filter.multiple
    ? Array.isArray(current) && current.some((v) => clickForm(filter, v) === value)
    : clickForm(filter, current) === value;
}

/** Pending charts show selected rows only when those rows can represent the click. */
export function previewCrossFilterRows<T extends Record<string, unknown>>(rows: T[], bindings: {
  field: string;
  ready: boolean;
  changed: boolean;
  preview: boolean;
  active: boolean;
  keepsScope: boolean;
  isSelected: (raw: unknown) => boolean;
}[]): T[] {
  // Filter-bar changes and clicks on unrelated dimensions keep the previous data.
  if (bindings.every((binding) => binding.ready) || !bindings.some((binding) => binding.changed)) return rows;
  // Keep-scope bindings will refetch all their values; narrowing them would hide series briefly.
  // Unchanged selections are already reflected in the rows, or never filtered them.
  const changed = bindings.filter((binding) => binding.changed && binding.preview && !binding.keepsScope);
  // A cleared selection widens the result, which the old rows cannot represent.
  if (!changed.length || changed.some((binding) => !binding.active)) return [];
  return rows.filter((row) => changed.every((binding) => binding.isSelected(row[binding.field])));
}

export function nextCrossFilterValue(filter: Filter, current: unknown, value: string | number, defaultValue: unknown): unknown {
  if (filter.type === "select" && filter.multiple) {
    const selected = Array.isArray(current) ? current.map((v) => clickForm(filter, v)) : [];
    return selected.includes(value) ? selected.filter((v) => v !== value) : [...selected, value];
  }
  return clickForm(filter, current) === value ? defaultValue : value;
}
