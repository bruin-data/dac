import { format as d3Format } from "d3-format";
import { timeFormat as d3TimeFormat } from "d3-time-format";
import type { AxisEncoding, ValueEncoding } from "../types/dashboard";

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

/**
 * Build a tick formatter from an encoding: d3-time-format for date axes,
 * d3-format otherwise. Falls back to the given formatter when no format is set.
 */
export function buildAxisFormatter(
  enc: Pick<AxisEncoding, "type" | "format"> | undefined,
  fallback: (val: unknown) => string,
): (val: unknown) => string {
  const fmt = enc?.format;
  if (!fmt) return fallback;

  if (enc?.type === "date") {
    const tf = d3TimeFormat(fmt);
    return (val) => {
      const d = val instanceof Date ? val : new Date(val as string);
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
 * Build a value formatter from an encoding, mirroring the cloud renderer:
 * d3-format for numbers, d3-time-format for dates, and an auto-compact
 * fallback when no format string is supplied.
 */
export function buildFormatter(enc?: ValueEncoding): (val: unknown) => string {
  return buildAxisFormatter(enc, (val) => {
    const num = Number(val);
    return val !== null && val !== "" && Number.isFinite(num) ? formatNumber(num) : String(val ?? "");
  });
}
