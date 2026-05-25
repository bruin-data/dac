import type { RowProps, WidgetContainerProps } from "../../types/template";
import { RowHeightContext, parseRowHeight } from "../RowContext";

export function BruinRow({ children, height }: RowProps) {
  const style = height !== undefined
    ? { height: typeof height === "number" ? `${height}px` : height }
    : undefined;
  return (
    <div className="grid grid-cols-1 sm:grid-cols-12 gap-4" style={style}>
      <RowHeightContext.Provider value={parseRowHeight(height)}>
        {children}
      </RowHeightContext.Provider>
    </div>
  );
}

export function BruinWidgetContainer({ col, children }: WidgetContainerProps) {
  return (
    <div
      className="col-span-1 min-h-0"
      style={{ gridColumn: `span ${col}` }}
    >
      {children}
    </div>
  );
}
