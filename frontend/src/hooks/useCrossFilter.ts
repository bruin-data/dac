import { createContext, useContext, useId, useMemo, useRef, type HTMLAttributes } from "react";
import type { Filter, Query, Widget, WidgetData } from "../types/dashboard";
import { crossFilterDateRange, crossFilterDefaultValue, queryFilterReferences, crossFilterQueryFilters, crossFilterValue, isCrossFilterSelected, nextCrossFilterValue, resolveCrossFilterBindings, sameCrossFilterValue, type CrossFilterSource } from "../lib/crossFilter";

export interface CrossFilterContextValue {
  filters: Filter[];
  queries?: Record<string, Query>;
  values: Record<string, unknown>;
  defaults: Record<string, unknown>;
  previewFilters?: string[];
  sources?: Record<string, CrossFilterSource>;
  /** `deselect` marks a click that removed the clicked value from the selection. */
  onChange: (name: string, value: unknown, widgetId?: string, deselect?: boolean) => void;
  reset?: (names?: string[]) => void;
}

export const CrossFilterContext = createContext<CrossFilterContextValue | null>(null);
// Stale results stay visible during a query, but must not emit selections.
export const CrossFilterReadyContext = createContext(true);
export const CrossFilterWidgetContext = createContext<string | undefined>(undefined);

export function useCrossFilters(widget: Widget, data?: WidgetData) {
  const context = useContext(CrossFilterContext);
  const ready = useContext(CrossFilterReadyContext);
  const fallbackId = useId();
  const widgetId = useContext(CrossFilterWidgetContext) ?? fallbackId;
  return useMemo(() => {
    if (!context) return [];
    const bindings = resolveCrossFilterBindings(widget, context.filters, data);
    const references = queryFilterReferences(widget.query ? context.queries?.[widget.query] : widget);
    const changedFilters = context.previewFilters?.filter((name) => references.has(name)) ?? [];
    // Preview only when every changed filter is a select dimension in this chart.
    // Changes to other dimensions can alter aggregates and must await the query.
    const preview = changedFilters.length > 0 && changedFilters.every((name) =>
      bindings.some((binding) => binding.filter.name === name && binding.filter.type === "select"));
    return bindings.map(({ field, filter }) => {
      const current = context.values[filter.name];
      const defaultValue = crossFilterDefaultValue(filter, context.defaults);
      const source = context.sources?.[filter.name];
      const isSource = widgetId !== undefined && source?.widgetId === widgetId;
      // Clicking a selection again, in any widget, restores the value that preceded the originating clicks.
      const restore = source ? source.value : defaultValue;
      return {
        field,
        filter,
        ready,
        changed: changedFilters.includes(filter.name),
        preview: preview && filter.type === "select",
        // An emptied multi-select or cleared input applies no filter, so nothing is selected to highlight.
        // A clicked value stays active even when it equals the default.
        active: !(Array.isArray(current) && current.length === 0) && current !== "" &&
          (!!source || !sameCrossFilterValue(current ?? defaultValue, defaultValue)),
        isSource,
        // The originating multi-select widget still queries its pre-click scope.
        keepsScope: isSource && source.keepScope,
        accepts: (raw: unknown) => crossFilterValue(filter, raw) !== undefined,
        canSelect: (raw: unknown) => ready && crossFilterValue(filter, raw) !== undefined,
        isSelected: (raw: unknown) => {
          const value = crossFilterValue(filter, raw);
          return value !== undefined && isCrossFilterSelected(filter, current, value);
        },
        /** `confirmed` applies a click made on ready data, even if a refetch has started since. */
        select: (raw: unknown, confirmed = false) => {
          const value = crossFilterValue(filter, raw);
          if ((ready || confirmed) && value !== undefined) context.onChange(filter.name, nextCrossFilterValue(filter, current, value, restore),
            widgetId, isCrossFilterSelected(filter, current, value));
        },
        /** `axis` (the x values) lets the range cover the whole last week/month bucket. */
        selectRange: (from: unknown, to: unknown, axis?: unknown[]) => {
          const selected = current as { start?: unknown; end?: unknown } | null;
          // Only this widget's own drag may supply a single-label bucket's end, so a repeat toggles it off.
          const range = crossFilterDateRange(from, to, axis, isSource ? selected : undefined);
          if (!ready || filter.type !== "date-range" || !range) return;
          const deselect = selected?.start === range.start && selected?.end === range.end;
          context.onChange(filter.name, deselect ? restore : range, widgetId, deselect);
        },
      };
    });
  }, [context, widget, data, ready, widgetId]);
}

/** Preserve the source query scope and reset only this widget's selections. */
export function useWidgetCrossFilter(widgetId: string, filters?: Record<string, unknown>) {
  const crossFilter = useContext(CrossFilterContext);
  const gesture = useRef<{ x: number; y: number; mark: boolean; dragged: boolean } | null>(null);
  const names = Object.entries(crossFilter?.sources ?? {})
    .filter(([, source]) => source.widgetId === widgetId).map(([name]) => name);
  const canReset = !!crossFilter?.reset && names.length > 0;
  const reset = () => crossFilter?.reset?.(names);
  const resetEvents: HTMLAttributes<HTMLDivElement> = {
    onPointerDownCapture: (event) => {
      gesture.current = event.button === 0 ? {
        x: event.clientX, y: event.clientY,
        mark: !!(event.target as Element).closest("[data-dac-mark]"), dragged: false,
      } : null;
    },
    onPointerMoveCapture: (event) => {
      const start = gesture.current;
      if (start && Math.hypot(event.clientX - start.x, event.clientY - start.y) >= 4) start.dragged = true;
    },
    onPointerUpCapture: (event) => {
      const start = gesture.current;
      if (start) {
        start.mark ||= !!(event.target as Element).closest("[data-dac-mark]");
        start.dragged ||= Math.hypot(event.clientX - start.x, event.clientY - start.y) >= 4;
      }
    },
    onPointerCancelCapture: () => { if (gesture.current) gesture.current.dragged = true; },
    onClick: (event) => {
      const start = gesture.current;
      gesture.current = null;
      if (!canReset || event.detail === 0 || start?.mark || start?.dragged) return;
      const target = event.target as Element;
      if (target.closest("[data-dac-mark], [data-dac-control]")) return;
      // A point may be replaced by its active dot before the click reaches us.
      // The pointer gesture above preserves that mark hit even if the click targets the SVG.
      if (target.closest("[data-dac-reset], [data-dac-plot]")) {
        reset();
        return;
      }
      const frame = event.currentTarget.querySelector("[data-dac-widget-frame]") ?? event.currentTarget;
      if (event.target !== frame && event.target !== event.currentTarget) return;
      if (frame.querySelector("[data-dac-plot]")) {
        reset();
        return;
      }
      const rect = frame.getBoundingClientRect();
      const edge = Math.min(event.clientX - rect.left, rect.right - event.clientX,
        event.clientY - rect.top, rect.bottom - event.clientY);
      if (edge >= 0 && edge <= 8) reset();
    },
  };
  return { queryFilters: crossFilterQueryFilters(widgetId, filters, crossFilter?.sources), resetEvents };
}
