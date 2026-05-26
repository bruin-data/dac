import type { Widget } from "../types/dashboard";

export function valueField(w: Widget): string | undefined {
  if (typeof w.value === "string") return w.value;
  return w.value?.field;
}

export function valueFormat(w: Widget): string | undefined {
  if (typeof w.value === "string") return undefined;
  return w.value?.format;
}
