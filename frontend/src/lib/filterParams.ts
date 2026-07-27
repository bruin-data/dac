import type { Filter } from "../types/dashboard";
import { resolvePreset } from "../themes/bruin/FilterBar";

/**
 * Convert filter values to/from URL query-string params,
 * so each filter is one param keyed by its name.
 */

const ISO_DATE_RE = /^\d{4}-\d{2}-\d{2}$/;

/** URL param string → typed filter value. Returns undefined to ignore the param. */
export function fromParam(filter: Filter, raw: string): unknown {
  switch (filter.type) {
    case "number": {
      const n = Number(raw);
      return raw !== "" && !Number.isNaN(n) ? n : undefined;
    }
    case "date": {
      const v = raw.trim();
      return ISO_DATE_RE.test(v) ? v : undefined;
    }
    case "date-range": {
      const preset = resolvePreset(raw); // e.g. "last_30_days"
      if (preset) return preset;
      const [start = "", end = ""] = raw.split(/\.\.|,/).map((s) => s.trim()); // or "2025-01-01..2025-03-31"
      return ISO_DATE_RE.test(start) && ISO_DATE_RE.test(end) ? { start, end } : undefined;
    }
    case "select": {
      // Only accept values that are actually offered as options. This ignores
      // anything injected into the URL that isn't a real choice.
      const allowed = filter.options?.values;
      if (filter.multiple) {
        const vals = raw.split(",").map((s) => s.trim()).filter(Boolean);
        if (!allowed) return vals;
        const filtered = vals.filter((v) => allowed.includes(v));
        return filtered.length ? filtered : undefined;
      }
      return !allowed || allowed.includes(raw) ? raw : undefined;
    }
    default:
      return raw; // text
  }
}

/** Typed filter value → URL param string. Returns null to drop the param. */
export function toParam(filter: Filter, value: unknown): string | null {
  if (value == null || value === "") return null;
  if (Array.isArray(value)) return value.length ? value.map(String).join(",") : null;
  if (filter.type === "date-range") {
    const v = value as { start?: string; end?: string };
    return v.start && v.end ? `${v.start}..${v.end}` : null;
  }
  return String(value);
}
