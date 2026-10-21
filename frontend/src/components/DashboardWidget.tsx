import { useState } from "react";
import { useWidgetQuery } from "../hooks/useWidgetQuery";
import type { Widget } from "../types/dashboard";
import type { WidgetFrameProps } from "../types/template";

// Non-data widget types that don't need a query.
const STATIC_WIDGET_TYPES = new Set(["text", "divider"]);

/**
 * DataWidget fetches data for a single widget via its own API call.
 * This allows tab-aware lazy loading and future viewport-based deferral.
 */
export function DataWidget({
  dashboardName,
  widgetId,
  widget,
  filters,
  WidgetFrame,
}: {
  dashboardName: string;
  widgetId: string;
  widget: Widget;
  filters?: Record<string, unknown>;
  WidgetFrame: React.ComponentType<WidgetFrameProps>;
}) {
  // Static widgets (text, divider) don't need data.
  if (STATIC_WIDGET_TYPES.has(widget.type)) {
    return <WidgetFrame key={`${dashboardName}:${widgetId}`} widget={widget} isLoading={false} />;
  }

  return (
    <DataWidgetInner
      dashboardName={dashboardName}
      widgetId={widgetId}
      widget={widget}
      filters={filters}
      WidgetFrame={WidgetFrame}
    />
  );
}

/** Inner component — calls the hook unconditionally (Rules of Hooks). */
function DataWidgetInner({
  dashboardName,
  widgetId,
  widget,
  filters,
  WidgetFrame,
}: {
  dashboardName: string;
  widgetId: string;
  widget: Widget;
  filters?: Record<string, unknown>;
  WidgetFrame: React.ComponentType<WidgetFrameProps>;
}) {
  const { data, isLoading, isPlaceholderData } = useWidgetQuery(dashboardName, widgetId, filters, true);
  return <WidgetFrame key={`${dashboardName}:${widgetId}`} widget={widget} data={data} isLoading={isLoading || isPlaceholderData} />;
}

/**
 * TabbedWidget renders a `type: tabs` widget: each tab is a complete widget,
 * switched in place and rendered as a normal DataWidget under the per-tab id
 * `${baseId}::${tab name}`. Tabs inherit nothing from the container.
 */
export function TabbedWidget({
  dashboardName,
  baseId,
  widget,
  filters,
  WidgetFrame,
}: {
  dashboardName: string;
  baseId: string;
  widget: Widget;
  filters?: Record<string, unknown>;
  WidgetFrame: React.ComponentType<WidgetFrameProps>;
}) {
  const [activeIndex, setActiveIndex] = useState(0);
  const tabs = widget.tabs ?? [];
  const active = Math.min(activeIndex, tabs.length - 1);
  const tab = tabs[active];
  // The frame shows the container's title and description; the tab label is in the bar.
  const mergedTab: Widget = {
    ...tab,
    name: widget.name,
    description: widget.description,
    // Downloads are named per tab so two tabs' exports don't share a file name.
    exportName: [widget.name, tab.name].filter(Boolean).join(" - "),
    tabs: undefined,
  };

  return (
    <div className="flex flex-col h-full">
      <div className="flex overflow-x-auto scrollbar-hide border-b border-[var(--dac-border)]">
        {tabs.map((t, k) => (
          <button
            key={k}
            onClick={() => setActiveIndex(k)}
            className={`shrink-0 px-3 py-1.5 text-[13px] font-medium transition-colors duration-100 border-b-2 -mb-px ${
              k === active
                ? "border-[var(--dac-accent)] text-[var(--dac-text-primary)]"
                : "border-transparent text-[var(--dac-text-muted)] hover:text-[var(--dac-text-secondary)]"
            }`}
          >
            {t.name || `Tab ${k + 1}`}
            {t.description && (
              // A tab's own description, as a small info tooltip.
              <span title={t.description} aria-label={t.description} className="ml-1 inline-flex align-middle opacity-60">
                <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                  <circle cx="12" cy="12" r="10" />
                  <path d="M12 16v-4M12 8h.01" />
                </svg>
              </span>
            )}
          </button>
        ))}
      </div>
      <div className="flex-1 min-h-0">
        {/* Keyed by tab so switching never shows the previous tab's data or state. */}
        <DataWidget
          key={tab.name}
          dashboardName={dashboardName}
          widgetId={`${baseId}::${tab.name}`}
          widget={mergedTab}
          filters={filters}
          WidgetFrame={WidgetFrame}
        />
      </div>
    </div>
  );
}
