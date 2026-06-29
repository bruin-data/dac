import type { WidgetData } from "../types/dashboard";

/** Render a non-string scalar in a CSV-friendly way. */
function formatScalar(value: unknown): string {
  if (typeof value === "number") {
    return Number.isFinite(value) ? String(value) : "";
  }
  if (typeof value === "boolean") return value ? "true" : "false";
  if (value instanceof Date) return value.toISOString();
  try {
    return JSON.stringify(value);
  } catch {
    return String(value);
  }
}

/** Quote a cell per RFC 4180 when it contains a comma, quote, or newline. */
function escapeCell(value: unknown): string {
  if (value === null || value === undefined) return "";
  const s = typeof value === "string" ? value : formatScalar(value);
  if (/[",\n\r]/.test(s)) {
    return `"${s.replace(/"/g, '""')}"`;
  }
  return s;
}

/** Convert a single widget's result set to RFC 4180 CSV. */
export function widgetDataToCSV(data: WidgetData): string {
  const header = data.columns.map((c) => escapeCell(c.name)).join(",");
  const lines = [header];
  for (const row of data.rows) {
    lines.push(row.map(escapeCell).join(","));
  }
  return lines.join("\r\n");
}

export interface WidgetSection {
  name: string;
  data: WidgetData;
}

/**
 * Combine multiple widget result sets into one CSV. Each widget is a section
 * introduced by a `# <name>` divider line followed by its header and rows,
 * separated from the next section by a blank line.
 */
export function dashboardToCSV(sections: WidgetSection[]): string {
  const blocks: string[] = [];
  for (const section of sections) {
    blocks.push(escapeCell(`# ${section.name}`));
    blocks.push(widgetDataToCSV(section.data));
    blocks.push("");
  }
  return blocks.join("\r\n");
}

/** Trigger a browser download of a text file. */
export function downloadTextFile(filename: string, content: string, mimeType: string): void {
  const blob = new Blob([content], { type: mimeType });
  const url = URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = url;
  a.download = filename;
  document.body.appendChild(a);
  a.click();
  document.body.removeChild(a);
  URL.revokeObjectURL(url);
}

/** Sanitize a name into a filename-safe slug. */
export function slugify(name: string): string {
  const slug = name.trim().replace(/[^\w.-]+/g, "_").replace(/_+/g, "_");
  return slug || "export";
}
