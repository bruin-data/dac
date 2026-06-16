import { createContext } from "react";

export const RowHeightContext = createContext<number | undefined>(undefined);

export function parseRowHeight(h: number | string | undefined): number | undefined {
  if (h === undefined) return undefined;
  if (typeof h === "number") return h;
  const m = h.match(/^(\d+(?:\.\d+)?)(px)?$/);
  if (m) return parseFloat(m[1]);
  return undefined;
}
