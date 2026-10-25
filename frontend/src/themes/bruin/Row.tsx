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

export function BruinRow({ children, height }: RowProps) {
  const stacked = useSyncExternalStore(subscribeToStacking, isStacked, serverStacking);
  const style = height !== undefined
    ? { "--dac-row-height": typeof height === "number" ? `${height}px` : height } as CSSProperties
    : undefined;
  return (
    <div className="dac-row grid grid-cols-1 sm:grid-cols-12 gap-4" style={style}>
      <RowHeightContext.Provider value={stacked ? undefined : parseRowHeight(height)}>
        {children}
      </RowHeightContext.Provider>
    </div>
  );
}

export function BruinWidgetContainer({ col, children }: WidgetContainerProps) {
  return (
    <div
      className="dac-widget-container min-h-0 min-w-0"
      style={{ "--dac-widget-col": col } as CSSProperties}
    >
      {children}
    </div>
  );
}
