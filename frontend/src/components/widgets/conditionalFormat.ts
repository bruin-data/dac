import type { CSSProperties } from "react";
import type { ColumnRef, FormatLayer, RuleValue } from "../../types/dashboard";

// Pure logic for table conditional formatting. A column's `format` is an ordered
// list of layers; the first layer that matches a cell wins. A layer with `if` is
// a condition; a layer without `if` always matches (a gradient or flat base —
// place it last). Colors resolve against the theme token map so named colors
// follow light/dark mode.

type RGB = [number, number, number];

interface ScaleStop {
  value: number;
  rgb: RGB;
}

export interface ResolvedScale {
  stops: ScaleStop[]; // sorted ascending by value
}

// Friendly color names → theme token key (the chart palette). Resolving through
// tokens gives automatic light/dark values; as table fills these are softened
// toward the surface (see `setFill`) so cells read gentler than solid chart marks.
const NAMED_TOKEN: Record<string, string> = {
  red: "chart-7",
  green: "chart-6",
  blue: "chart-8",
  indigo: "chart-1",
  cyan: "chart-2",
  purple: "chart-3",
  pink: "chart-4",
  amber: "chart-5",
  positive: "chart-6",
  negative: "chart-7",
  warning: "chart-5",
};

/** Resolve a color value (hex, named color, token, or raw CSS) to a CSS color string. */
export function resolveColor(value: string | undefined, tokens: Record<string, string>): string | undefined {
  if (!value) return undefined;
  if (value.startsWith("#")) return value;
  if (value === "white") return "#FFFFFF";
  if (value === "black") return "#000000";
  const named = NAMED_TOKEN[value];
  if (named && tokens[named]) return tokens[named];
  if (tokens[value]) return tokens[value]; // e.g. background, success, chart-3
  return value; // raw CSS name / var(...)
}

export function toNumber(value: unknown): number | null {
  if (value === null || value === undefined || value === "") return null;
  const n = Number(value);
  return isNaN(n) ? null : n;
}

/** True when this layer paints a gradient (an array backgroundColor). */
export function isGradient(layer: FormatLayer): boolean {
  return Array.isArray(layer.backgroundColor);
}

/** Build a gradient's stops for one layer, resolved against the column's values. */
export function resolveScale(
  layer: FormatLayer,
  values: number[],
  tokens: Record<string, string>,
): ResolvedScale | null {
  const names = Array.isArray(layer.backgroundColor) ? layer.backgroundColor : null;
  if (!names || names.length < 2 || values.length === 0) return null;

  const surface = surfaceRGB(tokens);
  const rgbs: RGB[] = names.map((n) => {
    const base = parseHex(resolveColor(n, tokens)) ?? [136, 136, 136];
    return n in NAMED_TOKEN ? soften(base, surface) : base;
  });
  const sorted = [...values].sort((a, b) => a - b);
  const lo = sorted[0];
  const hi = sorted[sorted.length - 1];

  const k = names.length;
  const anchors = resolveAnchors(layer.range, layer.unit, k, sorted, lo, hi);
  const stops = rgbs.map((rgb, i) => ({ value: anchors[i], rgb })).sort((a, b) => a.value - b.value);

  // Degenerate range (all anchors equal): paint every cell the top color.
  if (!(stops[stops.length - 1].value > stops[0].value)) {
    const rgb = stops[stops.length - 1].rgb;
    return { stops: [{ value: 0, rgb }, { value: 0, rgb }] };
  }
  return { stops };
}

/**
 * Turn a `range` + `unit` into `k` anchor values.
 * - omitted (or wrong length) → evenly spaced across the value range (auto min/max)
 * - unit `absolute` → raw values
 * - unit `percent` → percent of the min..max range
 * - unit `percentile` → percentile of the data
 */
function resolveAnchors(
  range: number[] | undefined,
  unit: string | undefined,
  k: number,
  sorted: number[],
  lo: number,
  hi: number,
): number[] {
  const even = () => Array.from({ length: k }, (_, i) => (k === 1 ? lo : lo + (i / (k - 1)) * (hi - lo)));
  if (!range || range.length !== k) return even();
  const u = unit || "absolute";
  if (u === "absolute") return range;
  return range.map((p) => (u === "percent" ? lo + (p / 100) * (hi - lo) : percentileValue(sorted, p)));
}

/** Linear-interpolated percentile of an ascending-sorted array (p in 0–100). */
function percentileValue(sorted: number[], p: number): number {
  if (sorted.length <= 1) return sorted[0] ?? 0;
  const rank = (Math.max(0, Math.min(100, p)) / 100) * (sorted.length - 1);
  const lo = Math.floor(rank);
  const hi = Math.ceil(rank);
  if (lo === hi) return sorted[lo];
  return sorted[lo] + (sorted[hi] - sorted[lo]) * (rank - lo);
}

/** Interpolate the background/text colors for a single cell value. */
export function cellColor(scale: ResolvedScale, value: number | null): { background: string; text: string } | undefined {
  if (value === null) return undefined;

  const { stops } = scale;
  const clamped = Math.max(stops[0].value, Math.min(stops[stops.length - 1].value, value));

  let rgb = stops[stops.length - 1].rgb;
  for (let i = 0; i < stops.length - 1; i++) {
    const a = stops[i];
    const b = stops[i + 1];
    if (clamped <= b.value) {
      const span = b.value - a.value;
      const t = span === 0 ? 0 : (clamped - a.value) / span;
      rgb = lerpRGB(a.rgb, b.rgb, t);
      break;
    }
  }

  return {
    background: `rgb(${rgb[0]}, ${rgb[1]}, ${rgb[2]})`,
    text: rgbLuminance(rgb) > 0.4 ? "#0A0D14" : "#FFFFFF",
  };
}

/**
 * Compute the inline style for one cell: the first matching layer wins. `scales`
 * is aligned to `layers` (one resolved gradient scale per layer, or null).
 * `lookup` returns another column's value in the same row for cross-column rules.
 */
export function cellStyle(
  layers: FormatLayer[] | undefined,
  scales: (ResolvedScale | null)[],
  raw: unknown,
  tokens: Record<string, string>,
  lookup: (column: string) => unknown,
): CSSProperties {
  const style: CSSProperties = {};
  if (!layers) return style;

  for (let i = 0; i < layers.length; i++) {
    const layer = layers[i];
    if (!matchLayer(layer, raw, lookup)) continue;

    // Matched — apply this layer and stop (first match wins).
    if (Array.isArray(layer.backgroundColor)) {
      const scale = scales[i];
      const fill = scale ? cellColor(scale, toNumber(raw)) : undefined;
      if (fill) {
        style.backgroundColor = fill.background;
        if (!layer.textColor) style.color = fill.text;
      }
    } else if (typeof layer.backgroundColor === "string") {
      setFill(style, layer.backgroundColor, tokens);
    }
    if (layer.textColor) style.color = resolveColor(layer.textColor, tokens);
    applyTextStyles(style, layer);
    return style;
  }
  return style;
}

/** A layer matches a cell when it has no `if` (base) or its condition holds. */
function matchLayer(layer: FormatLayer, raw: unknown, lookup: (column: string) => unknown): boolean {
  if (!layer.if) return true;
  if (layer.if === "is_empty") return isEmpty(raw);
  if (layer.if === "is_not_empty") return !isEmpty(raw);
  if (isEmpty(raw)) return false; // every other operator: empty is a non-match
  const value = resolveRuleValue(layer.value, lookup);
  return CONDITIONS[layer.if]?.(raw, value) ?? false;
}

function applyTextStyles(style: CSSProperties, s: FormatLayer): void {
  if (s.bold) style.fontWeight = 600;
  if (s.italic) style.fontStyle = "italic";
  const decoration: string[] = [];
  if (s.underline) decoration.push("underline");
  if (s.strikethrough) decoration.push("line-through");
  if (decoration.length) style.textDecoration = decoration.join(" ");
}

/**
 * Paint a background fill. Semantic palette colors (red/green/amber/…) are
 * softened toward the theme surface so the theme's own text color stays legible;
 * explicit hex/white/raw fills are left exactly as given. Text color is never
 * forced here — it follows the theme, or an explicit `textColor`.
 */
function setFill(style: CSSProperties, name: string, tokens: Record<string, string>): void {
  const resolved = resolveColor(name, tokens);
  if (!resolved) return;
  const rgb = name in NAMED_TOKEN ? parseHex(resolved) : null;
  if (!rgb) {
    style.backgroundColor = resolved;
    return;
  }
  const s = soften(rgb, surfaceRGB(tokens));
  style.backgroundColor = `rgb(${s[0]}, ${s[1]}, ${s[2]})`;
}

/** Mix a color toward another by `ratio` (0 = unchanged, 1 = fully the target). */
function soften(rgb: RGB, toward: RGB, ratio = 0.32): RGB {
  return [
    Math.round(rgb[0] + (toward[0] - rgb[0]) * ratio),
    Math.round(rgb[1] + (toward[1] - rgb[1]) * ratio),
    Math.round(rgb[2] + (toward[2] - rgb[2]) * ratio),
  ];
}

/** The theme surface color as RGB, so softening is light/dark aware. Falls back to white. */
function surfaceRGB(tokens: Record<string, string>): RGB {
  return parseHex(resolveColor("background", tokens)) ?? [255, 255, 255];
}

// --- color helpers ---

function lerpRGB(a: RGB, b: RGB, t: number): RGB {
  return [
    Math.round(a[0] + (b[0] - a[0]) * t),
    Math.round(a[1] + (b[1] - a[1]) * t),
    Math.round(a[2] + (b[2] - a[2]) * t),
  ];
}

function parseHex(hex?: string): RGB | null {
  if (!hex) return null;
  const match = hex.trim().match(/^#?([a-f\d]{2})([a-f\d]{2})([a-f\d]{2})$/i);
  if (!match) return null;
  return [parseInt(match[1], 16), parseInt(match[2], 16), parseInt(match[3], 16)];
}

function rgbLuminance([r, g, b]: RGB): number {
  const channels = [r, g, b].map((c) => {
    const v = c / 255;
    return v <= 0.03928 ? v / 12.92 : ((v + 0.055) / 1.055) ** 2.4;
  });
  return 0.2126 * channels[0] + 0.7152 * channels[1] + 0.0722 * channels[2];
}

// --- condition matching ---

const isEmpty = (v: unknown): boolean => v === null || v === undefined || v === "";
const asText = (v: unknown): string => (v == null || Array.isArray(v) ? "" : String(v)).toLowerCase();

function numericCompare(cell: unknown, value: unknown, ok: (a: number, b: number) => boolean): boolean {
  const a = toNumber(cell);
  const b = toNumber(value);
  return a !== null && b !== null && ok(a, b);
}

function valuesEqual(cell: unknown, value: unknown): boolean {
  const a = toNumber(cell);
  const b = toNumber(value);
  return a !== null && b !== null ? a === b : String(cell) === String(value);
}

function betweenResult(cell: unknown, value: unknown): boolean | null {
  const pair = Array.isArray(value) ? value : [];
  const n = toNumber(cell);
  const lo = toNumber(pair[0]);
  const hi = toNumber(pair[1]);
  if (n === null || lo === null || hi === null) return null;
  return n >= Math.min(lo, hi) && n <= Math.max(lo, hi);
}

const CONDITIONS: Record<string, (cell: unknown, value: unknown) => boolean> = {
  is_empty: (c) => isEmpty(c),
  is_not_empty: (c) => !isEmpty(c),
  text_contains: (c, v) => asText(c).includes(asText(v)),
  text_does_not_contain: (c, v) => !asText(c).includes(asText(v)),
  text_starts_with: (c, v) => asText(c).startsWith(asText(v)),
  text_ends_with: (c, v) => asText(c).endsWith(asText(v)),
  text_is_exactly: (c, v) => asText(c) === asText(v),
  date_is: (c, v) => matchDate("date_is", c, v),
  date_before: (c, v) => matchDate("date_before", c, v),
  date_after: (c, v) => matchDate("date_after", c, v),
  greater_than: (c, v) => numericCompare(c, v, (a, b) => a > b),
  greater_than_or_equal: (c, v) => numericCompare(c, v, (a, b) => a >= b),
  less_than: (c, v) => numericCompare(c, v, (a, b) => a < b),
  less_than_or_equal: (c, v) => numericCompare(c, v, (a, b) => a <= b),
  is_equal_to: (c, v) => valuesEqual(c, v),
  is_not_equal_to: (c, v) => !valuesEqual(c, v),
  is_between: (c, v) => betweenResult(c, v) === true,
  is_not_between: (c, v) => betweenResult(c, v) === false,
};

/** Resolve `{ column: X }` references (scalar or inside a list) to same-row values. */
function resolveRuleValue(value: RuleValue | undefined, lookup: (column: string) => unknown): unknown {
  if (Array.isArray(value)) return value.map((v) => resolveOne(v, lookup));
  return resolveOne(value, lookup);
}

function resolveOne(v: unknown, lookup: (column: string) => unknown): unknown {
  if (v !== null && typeof v === "object" && !Array.isArray(v) && "column" in (v as object)) {
    return lookup((v as ColumnRef).column);
  }
  return v;
}

function matchDate(op: "date_is" | "date_before" | "date_after", raw: unknown, value: unknown): boolean {
  const a = Date.parse(String(raw));
  const b = Date.parse(String(value));
  if (isNaN(a) || isNaN(b)) return false;
  // Exact instant when the value carries a time, otherwise by calendar day.
  const hasTime = /[T\s]\d{1,2}:/.test(String(value));
  const x = hasTime ? a : Math.floor(a / 86400000);
  const y = hasTime ? b : Math.floor(b / 86400000);
  if (op === "date_is") return x === y;
  if (op === "date_before") return x < y;
  return x > y;
}
