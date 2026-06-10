import { format as d3Format } from "d3-format";
import { timeFormat as d3TimeFormat } from "d3-time-format";
import type { ValueEncoding } from "../types/dashboard";

/** Resolve the column name from a value channel that may be a bare string or an encoding object. */
export function valueField(value?: string | ValueEncoding): string | undefined {
  if (value == null) return undefined;
  return typeof value === "string" ? value : value.field;
}

/** Normalize a value channel into a ValueEncoding (bare strings become `{ field }`). */
export function valueEncoding(value?: string | ValueEncoding): ValueEncoding | undefined {
  if (value == null) return undefined;
  return typeof value === "string" ? { field: value } : value;
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
  const fmt = enc?.format;

  if (!fmt) {
    return (val) => {
      const num = Number(val);
      return val !== null && val !== "" && Number.isFinite(num) ? formatNumber(num) : String(val ?? "");
    };
  }

  if (enc?.type === "date") {
    const tf = d3TimeFormat(fmt);
    return (val) => {
      const d = val instanceof Date ? val : new Date(val as string);
      return isNaN(d.getTime()) ? String(val ?? "") : tf(d);
    };
  }

  try {
    const nf = d3Format(fmt);
    return (val) => {
      const num = Number(val);
      return Number.isFinite(num) ? nf(num) : String(val ?? "");
    };
  } catch {
    return (val) => String(val ?? "");
  }
}
