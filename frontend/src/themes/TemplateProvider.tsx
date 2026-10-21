import { createContext, useContext, useMemo, type ReactNode } from "react";
import type { DashboardTemplate, TemplateComponents } from "../types/template";
import { bruinTemplate } from "./bruin/index";

const TemplateContext = createContext<DashboardTemplate>(bruinTemplate);

/**
 * Access the active template's color tokens.
 */
export function useTokens(): Record<string, string> {
  return useContext(TemplateContext).tokens;
}

/**
 * Access the active template's component set.
 */
export function useTemplate(): TemplateComponents {
  return useContext(TemplateContext).components;
}

/**
 * Access the full template object.
 */
export function useTemplateRaw(): DashboardTemplate {
  return useContext(TemplateContext);
}

export function TemplateProvider({
  template,
  className = "min-h-screen",
  children,
}: {
  template: DashboardTemplate;
  /** Sizing for the root element; embedded hosts can drop the full-screen default. */
  className?: string;
  children: ReactNode;
}) {
  const style = useMemo(() => {
    const vars: Record<string, string> = {
      fontFamily: '"Geist", system-ui, -apple-system, sans-serif',
    };
    for (const [key, value] of Object.entries(template.tokens)) {
      vars[`--dac-${key}`] = value;
    }
    return vars;
  }, [template]);

  return (
    <TemplateContext.Provider value={template}>
      <div
        style={style}
        className={`dac-root ${className} bg-[var(--dac-background)] text-[var(--dac-text-primary)] antialiased`}
      >
        {children}
      </div>
    </TemplateContext.Provider>
  );
}
