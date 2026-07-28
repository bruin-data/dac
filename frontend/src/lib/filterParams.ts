import type { Filter } from "../types/dashboard";
import { resolvePreset } from "../themes/bruin/FilterBar";

/**
 * Convert filter values to/from URL query-string params, so each filter is one
 * param keyed by its name. Multi-select uses repeated params (?x=a&x=b), read
 * and written in DashboardView so option values may safely contain commas.
 */

const ISO_DATE_RE = /^\d{4}-\d{2}-\d{2}$/;

// True only for a real calendar date in YYYY-MM-DD form. The regex alone checks
// shape, not validity, so this also rejects impossible dates like 2026-99-99 or
// 2026-02-30 (which would otherwise roll over).
function isRealDate(s: string): boolean {
  if (!ISO_DATE_RE.test(s)) return false;
  const d = new Date(`${s}T00:00:00Z`);
  return !Number.isNaN(d.getTime()) && d.toISOString().slice(0, 10) === s;
}

/** URL param string → typed filter value. Returns undefined to ignore the param. */
export function fromParam(filter: Filter, raw: string): unknown {
  switch (filter.type) {
    case "number": {
      if (raw === "") return undefined;
      const n = Number(raw);
      return Number.isFinite(n) ? n : undefined;
    }
    case "date": {
      const v = raw.trim();
      return isRealDate(v) ? v : undefined;
    }
    case "date-range": {
      const preset = resolvePreset(raw); // e.g. "last_30_days"
      if (preset) return preset;
      const [start = "", end = ""] = raw.split(/\.\.|,/).map((s) => s.trim()); // or "2025-01-01..2025-03-31"
      return isRealDate(start) && isRealDate(end) ? { start, end } : undefined;
    }
    case "select": {
      // Single-select only; multi-select is read from repeated params in
      // DashboardView. Only accept a value the filter actually offers.
      const allowed = filter.options?.values;
      return !allowed || allowed.includes(raw) ? raw : undefined;
    }
    default:
      return raw; // text
  }
}

/** Typed filter value → URL param string. Returns null to drop the param. */
export function toParam(filter: Filter, value: unknown): string | null {
  if (value == null || value === "") return null;
  if (filter.type === "date-range") {
    const v = value as { start?: string; end?: string };
    return v.start && v.end ? `${v.start}..${v.end}` : null;
  }
  return String(value);
}

/** Keep only multi-select values the filter offers (empty option list = accept as-is). */
export function keepAllowedValues(filter: Filter, vals: string[]): string[] {
  const allowed = filter.options?.values;
  return allowed ? vals.filter((v) => allowed.includes(v)) : vals;
}
