import { createContext } from "react";

export const RowHeightContext = createContext<number | undefined>(undefined);

export function parseRowHeight(h: number | string | undefined): number | undefined {
  if (typeof h === "number") return h;
  if (typeof h !== "string") return undefined;
  const m = h.match(/^(\d+(?:\.\d+)?)(px)?$/);
  if (m) return parseFloat(m[1]);
  return undefined;
}
