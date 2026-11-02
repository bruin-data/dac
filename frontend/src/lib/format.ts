import { format as d3Format } from "d3-format";
import { timeFormat as d3TimeFormat, timeParse as d3TimeParse } from "d3-time-format";
import type { AxisEncoding, DisplayFormat, SemanticFormat, ValueEncoding } from "../types/dashboard";

const parseDateOnly = d3TimeParse("%Y-%m-%d");
const formatDateOnly = d3TimeFormat("%Y-%m-%d");

/** Resolve the column name from a value encoding. */
export function valueField(value?: ValueEncoding): string | undefined {
  return value?.field;
}

/** Resolve the primary column from an axis encoding (first column when the field is a list). */
export function axisField(enc?: AxisEncoding): string | undefined {
  if (!enc) return undefined;
  return Array.isArray(enc.field) ? enc.field[0] : enc.field;
}

/** Resolve all series columns from an axis encoding. */
export function axisFields(enc?: AxisEncoding): string[] {
  if (!enc) return [];
  if (Array.isArray(enc.field)) return enc.field;
  return enc.field ? [enc.field] : [];
}

function dateForDisplay(val: unknown): Date {
  if (val instanceof Date) return val;
  if (typeof val === "string") {
    // Reject out-of-range values (e.g. "2025-13-01") that d3 silently rolls over.
    const date = parseDateOnly(val);
    if (date && formatDateOnly(date) === val) return date;
  }
  return new Date(val as string);
}

/**
 * Build a tick formatter from an encoding: d3-time-format for date axes,
 * d3-format or Intl for numbers. Falls back when no format is set.
 */
export function buildAxisFormatter(
  enc: Pick<AxisEncoding, "type" | "format"> & Partial<Pick<AxisEncoding, "field">> | undefined,
  fallback: (val: unknown) => string,
  field?: string,
  compact = false,
): (val: unknown) => string {
  const key = field ?? (Array.isArray(enc?.field) ? enc.field[0] : enc?.field);
  const fmt = formatForField(enc?.format, key);
  if (!fmt) return fallback;
  if (typeof fmt === "object") return buildSemanticFormatter(fmt, compact) ?? fallback;

  if (enc?.type === "date") {
    const tf = d3TimeFormat(fmt);
    return (val) => {
      const d = dateForDisplay(val);
      return isNaN(d.getTime()) ? fallback(val) : tf(d);
    };
  }

  try {
    const nf = d3Format(fmt);
    return (val) => {
      const num = Number(val);
      return Number.isFinite(num) ? nf(num) : fallback(val);
    };
  } catch {
    return fallback;
  }
}

/** Axis ticks keep object formats compact (`₺1.2M`); d3 strings apply as written. */
export function buildTickFormatter(
  enc: Parameters<typeof buildAxisFormatter>[0],
  fallback: (val: unknown) => string,
  field?: string,
): (val: unknown) => string {
  return buildAxisFormatter(enc, fallback, field, true);
}

/** Auto-compact fallback for numbers that have no explicit format. */
export function formatNumber(val: number): string {
  if (!Number.isFinite(val)) return String(val);
  const abs = Math.abs(val);
  if (abs >= 1_000_000_000) return `${trimZero(val / 1_000_000_000)}B`;
  if (abs >= 1_000_000) return `${trimZero(val / 1_000_000)}M`;
  if (abs >= 10_000) return `${trimZero(val / 1_000)}k`;
  if (Number.isInteger(val)) return val.toLocaleString();
  return String(parseFloat(val.toFixed(2)));
}

function trimZero(n: number): string {
  return n.toFixed(1).replace(/\.0$/, "");
}

/**
 * Build a value formatter shared by metric and chart encodings:
 * d3/Intl for numbers, d3-time-format for dates, otherwise auto-compact.
 */
export function buildFormatter(enc?: ValueEncoding): (val: unknown) => string {
  return buildAxisFormatter(enc, (val) => {
    const num = Number(val);
    return val !== null && val !== "" && Number.isFinite(num) ? formatNumber(num) : String(val ?? "");
  });
}

/** Warehouse aliases can differ from model names in case. */
export function formatForField(format: DisplayFormat | undefined, field?: string): string | SemanticFormat | undefined {
  if (!format || typeof format === "string" || !("fields" in format)) return format;
  return Object.entries(format.fields).find(([name]) => name.toLowerCase() === field?.toLowerCase())?.[1];
}

// Intl formatters are costly to build and Vega formats every label, so share them.
const semanticFormatters = new Map<string, ((val: unknown) => string) | undefined>();

/** Format semantic values in the viewer's locale without replacing their currency. */
export function buildSemanticFormatter(format?: SemanticFormat, compact = false): ((val: unknown) => string) | undefined {
  if (!format) return undefined;
  const key = JSON.stringify([format.type, format.currency, format.decimals, compact]);
  if (!semanticFormatters.has(key)) semanticFormatters.set(key, createSemanticFormatter(format, compact));
  return semanticFormatters.get(key);
}

function createSemanticFormatter(format: SemanticFormat, compact: boolean): ((val: unknown) => string) | undefined {
  const type = format.type ?? (format.currency ? "currency" : "decimal");
  if (!["number", "decimal", "currency", "percentage"].includes(type)) return undefined;
  const options: Intl.NumberFormatOptions = {
    style: type === "percentage" ? "percent" : type === "currency" && format.currency ? "currency" : "decimal",
    currency: type === "currency" ? format.currency : undefined,
    currencyDisplay: "narrowSymbol", // TRY displays ₺ outside the Turkish locale too.
    // `decimals` would round the shortened number (1.25M with 0 → 1M), so compact
    // ticks keep three significant digits instead.
    ...(compact
      ? { notation: "compact", maximumSignificantDigits: 3 }
      : { minimumFractionDigits: format.decimals, maximumFractionDigits: format.decimals }),
  };
  try {
    const formatter = new Intl.NumberFormat(undefined, options);
    return (val) => {
      const num = Number(val);
      return val != null && val !== "" && Number.isFinite(num) ? formatter.format(num) : String(val ?? "");
    };
  } catch {
    return (val) => val != null && val !== "" && options.style === "currency" ? `${format.currency} ${String(val)}` : String(val ?? "");
  }
}

/** Explicit table specs override defaults; the currency shorthand retains the model's code. */
export function buildTableNumberFormatter(value?: DisplayFormat, field?: string): ((val: unknown) => string) | undefined {
  const format = formatForField(value, field);
  if (!format || format === "currency" || format === "number") return undefined;
  return buildAxisFormatter({ format }, (val) => String(val ?? ""));
}
