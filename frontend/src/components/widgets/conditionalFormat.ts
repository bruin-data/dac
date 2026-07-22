import type { CSSProperties } from "react";
import type { ColorScale, ColorStop, SingleColorRule } from "../../types/dashboard";

// Pure logic for table conditional formatting — colour scales (gradient) and
// single-colour rules (thresholds).

// Default three-color scale: muted red (low) → white (mid) → muted green (high).
// Going through white keeps mid-range cells calm; only the extremes get color.
const DEFAULT_MIN_COLOR: RGB = [230, 124, 115]; // #E67C73
const DEFAULT_MID_COLOR: RGB = [255, 255, 255]; // #FFFFFF
const DEFAULT_MAX_COLOR: RGB = [87, 187, 138]; // #57BB8A

export type RGB = [number, number, number];
type StopRole = "min" | "mid" | "max";

interface ScaleStop {
  value: number;
  rgb: RGB;
}

export interface ResolvedScale {
  stops: ScaleStop[]; // sorted ascending by value; length 2 or 3
}

export function toNumber(value: unknown): number | null {
  if (value === null || value === undefined || value === "") return null;
  const n = Number(value);
  return isNaN(n) ? null : n;
}

function normalizeStop(stop?: ColorStop | string): ColorStop | undefined {
  if (stop == null) return undefined;
  return typeof stop === "string" ? { color: stop } : stop;
}

/** Resolve a stop's anchor value against the sorted column values, per its type. */
function resolveAnchor(stop: ColorStop | undefined, role: StopRole, sorted: number[]): number {
  const lo = sorted[0];
  const hi = sorted[sorted.length - 1];
  const type = stop?.type ?? (role === "min" ? "min" : role === "max" ? "max" : "percentile");
  const pctDefault = role === "min" ? 0 : role === "max" ? 100 : 50;
  switch (type) {
    case "min":
      return lo;
    case "max":
      return hi;
    case "number":
      return stop?.value ?? (role === "max" ? hi : lo);
    case "percent":
      return lo + ((stop?.value ?? pctDefault) / 100) * (hi - lo);
    case "percentile":
      return percentileValue(sorted, stop?.value ?? pctDefault);
    default:
      return role === "max" ? hi : lo;
  }
}

/** Linear-interpolated percentile of an ascending-sorted array (p in 0–100). */
export function percentileValue(sorted: number[], p: number): number {
  if (sorted.length === 1) return sorted[0];
  const rank = (Math.max(0, Math.min(100, p)) / 100) * (sorted.length - 1);
  const lo = Math.floor(rank);
  const hi = Math.ceil(rank);
  if (lo === hi) return sorted[lo];
  return sorted[lo] + (sorted[hi] - sorted[lo]) * (rank - lo);
}

/** Build the concrete color stops for a column from its config and observed values. */
export function resolveScale(cs: ColorScale, values: number[]): ResolvedScale | null {
  if (values.length === 0) return null;

  const sorted = [...values].sort((a, b) => a - b);
  const minStop = normalizeStop(cs.min);
  const midStop = normalizeStop(cs.mid);
  const maxStop = normalizeStop(cs.max);

  const hasExplicitColor = Boolean(minStop?.color || midStop?.color || maxStop?.color);
  // The mid stop is active when it is given, or when nothing is configured at
  // all (the pure default is a three-color scale).
  const useMid = cs.mid != null || !hasExplicitColor;

  const minColor = parseHex(minStop?.color) ?? DEFAULT_MIN_COLOR;
  const maxColor = parseHex(maxStop?.color) ?? DEFAULT_MAX_COLOR;

  const stops: ScaleStop[] = [{ value: resolveAnchor(minStop, "min", sorted), rgb: minColor }];
  if (useMid) {
    stops.push({ value: resolveAnchor(midStop, "mid", sorted), rgb: parseHex(midStop?.color) ?? DEFAULT_MID_COLOR });
  }
  stops.push({ value: resolveAnchor(maxStop, "max", sorted), rgb: maxColor });
  stops.sort((a, b) => a.value - b.value);

  // Degenerate range (all anchors equal): paint every cell with the top color
  // rather than dividing by zero.
  if (!(stops[stops.length - 1].value > stops[0].value)) {
    const rgb = stops[stops.length - 1].rgb;
    return { stops: [{ value: 0, rgb }, { value: 0, rgb }] };
  }
  return { stops };
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

/** Turn a matched single-color rule into inline styles, merged onto `style`. */
export function applyRuleStyle(style: CSSProperties, rule: SingleColorRule): void {
  if (rule.background) style.backgroundColor = rule.background;
  if (rule.textColor) style.color = rule.textColor;
  if (rule.bold) style.fontWeight = 600;
  if (rule.italic) style.fontStyle = "italic";
  const decoration: string[] = [];
  if (rule.underline) decoration.push("underline");
  if (rule.strikethrough) decoration.push("line-through");
  if (decoration.length) style.textDecoration = decoration.join(" ");
}

const isEmpty = (v: unknown): boolean => v === null || v === undefined || v === "";

// Lowercased text for case-insensitive comparison; arrays/nullish become "".
const asText = (v: unknown): string => (v == null || Array.isArray(v) ? "" : String(v)).toLowerCase();

// Numeric comparison that only matches when both sides parse as numbers.
function numericCompare(cell: unknown, value: unknown, ok: (a: number, b: number) => boolean): boolean {
  const a = toNumber(cell);
  const b = toNumber(value);
  return a !== null && b !== null && ok(a, b);
}

// Numeric when both sides are numbers, otherwise a string comparison.
function valuesEqual(cell: unknown, value: unknown): boolean {
  const a = toNumber(cell);
  const b = toNumber(value);
  return a !== null && b !== null ? a === b : String(cell) === String(value);
}

// true/false when the [low, high] range is valid, null when it is malformed.
function betweenResult(cell: unknown, value: unknown): boolean | null {
  const pair = Array.isArray(value) ? value : [];
  const n = toNumber(cell);
  const lo = toNumber(pair[0]);
  const hi = toNumber(pair[1]);
  if (n === null || lo === null || hi === null) return null;
  return n >= Math.min(lo, hi) && n <= Math.max(lo, hi);
}

// Each operator is a predicate over (cell value, rule value). Empty cells are
// handled in matchRule before this table is consulted.
const CONDITIONS: Record<SingleColorRule["if"], (cell: unknown, value: unknown) => boolean> = {
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

/** Evaluate whether a cell value satisfies a single-color rule's condition. */
export function matchRule(rule: SingleColorRule, raw: unknown): boolean {
  if (rule.if === "is_empty") return isEmpty(raw);
  if (rule.if === "is_not_empty") return !isEmpty(raw);
  // Every other operator treats an empty cell as a non-match.
  if (isEmpty(raw)) return false;
  return CONDITIONS[rule.if]?.(raw, rule.value) ?? false;
}

function matchDate(op: "date_is" | "date_before" | "date_after", raw: unknown, value: unknown): boolean {
  const a = Date.parse(String(raw));
  const b = Date.parse(String(value));
  if (isNaN(a) || isNaN(b)) return false;
  // Compare exact instants when the target value carries a time component;
  // otherwise compare by calendar day.
  const hasTime = /[T\s]\d{1,2}:/.test(String(value));
  const x = hasTime ? a : Math.floor(a / 86400000);
  const y = hasTime ? b : Math.floor(b / 86400000);
  if (op === "date_is") return x === y;
  if (op === "date_before") return x < y;
  return x > y;
}
