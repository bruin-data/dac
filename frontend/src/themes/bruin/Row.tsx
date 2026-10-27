import { useSyncExternalStore, type CSSProperties } from "react";
import type { RowProps, WidgetContainerProps } from "../../types/template";
import { RowHeightContext, parseRowHeight } from "../RowContext";

const STACKED_QUERY = "(width < 40rem)";

function subscribeToStacking(callback: () => void) {
  const query = window.matchMedia(STACKED_QUERY);
  query.addEventListener("change", callback);
  return () => query.removeEventListener("change", callback);
}

function isStacked() {
  return window.matchMedia(STACKED_QUERY).matches;
}

function serverStacking() {
  return false;
}

const cssHeight = (height: number | string) => typeof height === "number" ? `${height}px` : height;

export function BruinRow({ children, height }: RowProps) {
  const stacked = useSyncExternalStore(subscribeToStacking, isStacked, serverStacking);
  const style = height !== undefined
    ? { "--dac-row-height": cssHeight(height) } as CSSProperties
    : undefined;
  return (
    <div className="dac-row grid grid-cols-1 sm:grid-cols-12 gap-4" style={style}>
      <RowHeightContext.Provider value={stacked ? undefined : parseRowHeight(height)}>
        {children}
      </RowHeightContext.Provider>
    </div>
  );
}

export function BruinWidgetContainer({ col, height, children }: WidgetContainerProps) {
  const stacked = useSyncExternalStore(subscribeToStacking, isStacked, serverStacking);
  const style = { "--dac-widget-col": col, ...(height !== undefined ? { "--dac-widget-height": cssHeight(height) } : {}) };
  const content = (
    <div
      className="dac-widget-container min-h-0 min-w-0"
      data-height={height !== undefined ? "" : undefined}
      style={style as CSSProperties}
    >
      {children}
    </div>
  );
  if (height === undefined) return content;
  // The widget's own height replaces the row height its charts size against.
  return (
    <RowHeightContext.Provider value={stacked ? undefined : parseRowHeight(height)}>
      {content}
    </RowHeightContext.Provider>
  );
}
