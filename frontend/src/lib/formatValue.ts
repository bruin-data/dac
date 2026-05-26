import { format as d3Format } from "d3-format";

export function formatValue(value: unknown, spec?: string): string {
  if (value === null || value === undefined) return "—";
  const num = Number(value);
  if (isNaN(num)) return String(value);
  if (!spec) return num.toLocaleString();
  try {
    return d3Format(spec)(num);
  } catch {
    return num.toLocaleString();
  }
}
