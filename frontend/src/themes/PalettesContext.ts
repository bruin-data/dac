import { createContext, useContext } from "react";

/** Dashboard-level custom palettes (name -> gradient colors), referenced by a
 * column's format.scheme. Empty by default so widgets work without a provider. */
export const PalettesContext = createContext<Record<string, string[]>>({});

export function usePalettes(): Record<string, string[]> {
  return useContext(PalettesContext);
}
