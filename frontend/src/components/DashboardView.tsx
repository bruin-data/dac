import { useState, useMemo, useCallback, useEffect, useRef } from "react";
import { useParams, useSearchParams } from "react-router-dom";
import { useDashboard } from "../hooks/useDashboard";
import { useFilterOptions } from "../hooks/useFilterOptions";
import { useTemplate } from "../themes/TemplateProvider";
import { buildDefaultFilters, fromParam, toParam, keepAllowedValues } from "../lib/filterParams";
import { DataWidget, TabbedWidget } from "./DashboardWidget";
import { CrossFilterContext } from "../hooks/useCrossFilter";
import { crossFilterDefaultValue, crossFilterQueryFilters, sameCrossFilterValue, type CrossFilterSource } from "../lib/crossFilter";
import { YamlPanel } from "./YamlPanel";
import { ExportMenuButton, type ExportMenuItem } from "./ExportMenuButton";
import { fetchDashboardData, fetchWidgetData } from "../api/client";
import { dashboardToCSV, downloadTextFile, slugify } from "../lib/csv";
import { exportElementAsPDF, exportElementAsPNG } from "../lib/renderExport";
import type { Widget, WidgetData } from "../types/dashboard";

const isStaticMode = window.__DAC_STATIC__ !== undefined;
const EMPTY_FILTERS: Record<string, unknown> = {};

interface CrossFilterSelection {
  base: Record<string, unknown>;
  changes: Record<string, unknown>;
  sources: Record<string, CrossFilterSource>;
  previewFilters: string[];
}

/** A selection is current while the filter values still equal its base plus its changes. */
function isCurrentSelection(state: CrossFilterSelection | null, values: Record<string, unknown>): state is CrossFilterSelection {
  return !!state && JSON.stringify({ ...state.base, ...state.changes }) === JSON.stringify(values);
}
type DashboardExportFormat = "csv" | "png" | "pdf";

// Persist sidebar state across navigation (module-level, resets on page refresh).
let _yamlOpen = false;
let _yamlWidth = 420;

export function DashboardView() {
  const { name } = useParams<{ name: string }>();
  const dashboardExportRef = useRef<HTMLDivElement>(null);

  const { data: dashboard, isLoading: dashLoading, error: dashError } = useDashboard(name || "");
  const [yamlOpen, _setYamlOpen] = useState(_yamlOpen);
  const [yamlWidth, _setYamlWidth] = useState(_yamlWidth);

  const setYamlOpen = useCallback((v: boolean | ((prev: boolean) => boolean)) => {
    _setYamlOpen((prev) => {
      const next = typeof v === "function" ? v(prev) : v;
      _yamlOpen = next;
      return next;
    });
  }, []);
  const setYamlWidth = useCallback((v: number | ((prev: number) => number)) => {
    _setYamlWidth((prev) => {
      const next = typeof v === "function" ? v(prev) : v;
      _yamlWidth = next;
      return next;
    });
  }, []);
  const [isResizing, setIsResizing] = useState(false);
  const [exporting, setExporting] = useState<DashboardExportFormat | null>(null);

  const handleYamlResize = useCallback((delta: number) => {
    setYamlWidth((w: number) => Math.max(280, Math.min(800, w + delta)));
  }, [setYamlWidth]);
  const onResizeStart = useCallback(() => setIsResizing(true), []);
  const onResizeEnd = useCallback(() => setIsResizing(false), []);

  const [searchParams, setSearchParams] = useSearchParams();

  const baseFilters = useMemo(() => {
    if (!dashboard) return null;
    // Start from the dashboard defaults, then apply any values from the URL.
    const values = buildDefaultFilters(dashboard);
    for (const f of dashboard.filters ?? []) {
      // Multi-select uses repeated params (?x=a&x=b) so values may contain commas.
      if (f.type === "select" && f.multiple) {
        const raws = searchParams.getAll(f.name).map((s) => s.trim()).filter(Boolean);
        if (!raws.length) continue;
        const kept = keepAllowedValues(f, raws);
        if (kept.length) values[f.name] = kept;
        continue;
      }
      const raw = searchParams.get(f.name);
      if (raw == null) continue;
      const parsed = fromParam(f, raw);
      if (parsed !== undefined) values[f.name] = parsed;
    }
    return values;
  }, [dashboard, searchParams]);

  const [filters, setFilters] = useState<Record<string, unknown> | null>(null);
  const activeFilters = filters ?? baseFilters;
  const allFilters = useFilterOptions(dashboard, activeFilters ?? undefined);

  const values = activeFilters ?? EMPTY_FILTERS;
  const defaults = useMemo(() => dashboard ? buildDefaultFilters(dashboard) : EMPTY_FILTERS, [dashboard]);
  const selectionKey = !dashLoading && !dashError ? dashboard : undefined;
  const [selection, setSelection] = useState<CrossFilterSelection | null>(null);
  const [observedValues, setObservedValues] = useState(values);
  const [observedResetKey, setObservedResetKey] = useState(selectionKey);
  const current = isCurrentSelection(selection, values) ? selection : null;

  // External values or a reloaded definition invalidate old click origins.
  if (observedResetKey !== selectionKey) {
    setObservedResetKey(selectionKey);
    setSelection(null);
  }
  if (observedValues !== values) {
    setObservedValues(values);
    if (selection && !current) setSelection(null);
  }

  // Stable callbacks and context value, so cross-filter bindings only recompute when filter state changes.
  const handleFilterChange = useCallback((name: string, next: unknown, widgetId?: string, deselect?: boolean) => {
    // Several dimensions selected in one click share the same base state.
    setSelection((previous) => {
      const sameBase = previous?.base === values;
      const sources = previous && (sameBase || isCurrentSelection(previous, values)) ? { ...previous.sources } : {};
      const filter = (dashboard?.filters ?? []).find((filter) => filter.name === name);
      const baseline = name in sources ? sources[name].value : values[name];
      // Deselecting back to the default is a clear; selecting a value that equals the default still owns the change.
      const cleared = filter && ((Array.isArray(next) && next.length === 0) ||
        sameCrossFilterValue(next, baseline) ||
        (deselect && sameCrossFilterValue(next, crossFilterDefaultValue(filter, defaults))));
      if (widgetId && filter && !cleared) {
        const keepScope = filter.type === "select" && !!filter.multiple;
        // Clicks in other widgets edit a multi-select; its origin keeps the pre-click scope and reset.
        const owner = keepScope && sources[name] ? sources[name].widgetId : widgetId;
        sources[name] = { widgetId: owner, value: baseline, keepScope };
      } else {
        delete sources[name];
      }
      return {
        base: values,
        changes: { ...(sameBase ? previous.changes : {}), [name]: next },
        sources,
        previewFilters: [...(sameBase ? previous.previewFilters : []), ...(widgetId ? [name] : [])],
      };
    });
    setFilters((previous) => ({ ...(previous ?? values), [name]: next }));
  }, [dashboard, values, defaults]);
  const resetFilters = useCallback((names?: string[]) => {
    for (const filter of dashboard?.filters ?? []) {
      if (names && !names.includes(filter.name)) continue;
      // Widget resets restore the pre-click scope; a full reset uses defaults.
      const source = names && current?.sources[filter.name];
      const next = source ? source.value : defaults[filter.name];
      handleFilterChange(filter.name, next ?? crossFilterDefaultValue(filter, defaults));
    }
  }, [dashboard, current, defaults, handleFilterChange]);
  const crossFilterContext = useMemo(() => isStaticMode || !dashboard ? null : {
    filters: dashboard.filters ?? [], queries: dashboard.queries, values, defaults,
    onChange: handleFilterChange, reset: resetFilters,
    previewFilters: current?.previewFilters, sources: current?.sources,
  }, [dashboard, values, defaults, handleFilterChange, resetFilters, current]);

  // Last query string, so the reseed effect can skip our own writes.
  const lastSelfWrite = useRef<string | null>(null);

  const {
    DashboardLayout,
    FilterBar,
    WidgetFrame,
    Row,
    WidgetContainer,
  } = useTemplate();

  // ─── Tab support ───
  // Hooks must be called before any early returns (Rules of Hooks).
  const tabNames = useMemo(() => {
    if (!dashboard) return [];
    const seen = new Set<string>();
    const names: string[] = [];
    for (const row of dashboard.rows) {
      if (row.tab && !seen.has(row.tab)) {
        seen.add(row.tab);
        names.push(row.tab);
      }
    }
    return names;
  }, [dashboard]);

  const [activeTab, setActiveTab] = useState<string | null>(null);

  // Reset local overrides when the dashboard definition changes (e.g. file edited on disk).
  useEffect(() => {
    setFilters(null);
    setActiveTab(null);
  }, [dashboard]);

  // Mirror all filters to the URL in one write (order-stable, concurrency-safe).
  useEffect(() => {
    if (filters == null || !dashboard) return;
    const next = new URLSearchParams(searchParams);
    for (const f of dashboard.filters ?? []) next.delete(f.name);
    for (const f of dashboard.filters ?? []) {
      const v = filters[f.name];
      if (f.type === "select" && f.multiple) {
        if (Array.isArray(v)) {
          for (const x of v) if (x != null && x !== "") next.append(f.name, String(x));
        }
      } else {
        const param = toParam(f, v);
        if (param != null) next.set(f.name, param);
      }
    }
    // Record even when unchanged, so a later nav isn't taken for a self-write.
    const s = next.toString();
    lastSelfWrite.current = s;
    if (s !== searchParams.toString()) setSearchParams(next, { replace: true });
    // eslint-disable-next-line react-hooks/exhaustive-deps -- run only on filters change
  }, [filters]);

  // External URL change (e.g. a shared link) re-seeds from the URL; our own writes are skipped.
  useEffect(() => {
    if (searchParams.toString() === lastSelfWrite.current) return;
    setFilters(null);
  }, [searchParams]);

  const handleExport = useCallback(async (format: DashboardExportFormat, sources?: Record<string, CrossFilterSource>) => {
    if (!dashboard) return;
    setExporting(format);
    try {
      const filename = `${slugify(dashboard.name)}.${format}`;
      if (format === "csv") {
        const widgetData = await fetchDashboardData(name || "", activeFilters ?? undefined);
        // A multi-select click origin keeps its pre-click scope on screen; export what it shows.
        const origins = [...new Set(Object.values(sources ?? {}).filter((source) => source.keepScope).map((source) => source.widgetId))];
        await Promise.all(origins.map(async (widgetId) => {
          widgetData[widgetId] = await fetchWidgetData(name || "", widgetId, crossFilterQueryFilters(widgetId, activeFilters ?? undefined, sources));
        }));
        const sections: { name: string; data: WidgetData }[] = [];
        const pushSection = (label: string, data: WidgetData | undefined) => {
          if (data && !data.error && data.rows && data.rows.length > 0) {
            sections.push({ name: label, data });
          }
        };
        dashboard.rows.forEach((row, i) => {
          row.widgets.forEach((widget, j) => {
            if (widget.type === "text" || widget.type === "divider") return;
            // Tabbed widgets keep their data per tab (`::{name}`), so export one
            // section per tab rather than a missing base id.
            if (widget.tabs && widget.tabs.length) {
              widget.tabs.forEach((tab) => {
                const label = [widget.name, tab.name].filter(Boolean).join(" — ");
                pushSection(label, widgetData[`r${i}-w${j}::${tab.name}`]);
              });
              return;
            }
            pushSection(widget.name, widgetData[`r${i}-w${j}`]);
          });
        });
        if (sections.length === 0) return;
        const csv = dashboardToCSV(sections);
        downloadTextFile(filename, csv, "text/csv;charset=utf-8");
        return;
      }

      const element = dashboardExportRef.current;
      if (!element) return;
      if (format === "png") {
        await exportElementAsPNG(element, filename);
      } else {
        await exportElementAsPDF(element, filename);
      }
    } catch (err) {
      console.error("Dashboard export failed", err);
    } finally {
      setExporting(null);
    }
  }, [dashboard, name, activeFilters]);

  if (dashLoading) {
    return (
      <div className="max-w-[1400px] mx-auto px-4 sm:px-6 py-6 sm:py-8">
        <div className="skeleton h-3 w-16 mb-6" />
        <div className="skeleton h-6 w-48 mb-1.5" />
        <div className="skeleton h-3 w-72" />
      </div>
    );
  }

  if (dashError || !dashboard) {
    return (
      <div className="max-w-[1400px] mx-auto px-4 sm:px-6 py-6 sm:py-8">
        <div className="text-[13px] font-mono text-[var(--dac-error)]">
          {dashError?.message || "Dashboard not found"}
        </div>
      </div>
    );
  }

  const hasTabs = tabNames.length > 0;
  const currentTab = activeTab ?? (hasTabs ? tabNames[0] : null);

  // Filters with no `tab` (or an unmatched one — the validator rejects those) go
  // in the global bar; a tab-scoped filter shows only on its tab.
  const globalFilters = allFilters.filter((f) => !f.tab || !tabNames.includes(f.tab));
  const tabFilters = currentTab ? allFilters.filter((f) => f.tab === currentTab) : [];

  const filterBar = globalFilters.length ? (
    <FilterBar
      filters={globalFilters}
      values={activeFilters ?? {}}
      onChange={handleFilterChange}
    />
  ) : null;

  const filtersChanged = Object.keys(current?.sources ?? {}).length > 0 || (dashboard.filters ?? []).some((filter) => {
    const initial = crossFilterDefaultValue(filter, defaults);
    return !sameCrossFilterValue(values[filter.name] ?? initial, initial);
  });
  const exportItems: ExportMenuItem[] = (["csv", "png", "pdf"] as const).map((format) => ({
    key: format, label: format.toUpperCase(), onSelect: () => handleExport(format, current?.sources),
  }));

  const headerActions = (
    <div data-dac-export-control className="flex items-center gap-1.5">
      {!isStaticMode && filtersChanged && (
        <button type="button" onClick={() => resetFilters()} aria-label="Reset all filters"
          className="inline-flex items-center h-7 px-2 rounded-sm border border-[var(--dac-border)] text-[13px] text-[var(--dac-text-secondary)] hover:text-[var(--dac-text-primary)] hover:bg-[var(--dac-surface-hover)]">
          Reset filters
        </button>
      )}
      <ExportMenuButton label={exporting ? "Exporting" : "Export"} ariaLabel="Export dashboard"
        title="Export dashboard" items={exportItems} disabled={exporting !== null} />
      {!isStaticMode && (
        <button
          onClick={() => setYamlOpen(!yamlOpen)}
          className={`inline-flex items-center gap-1.5 h-7 px-2 rounded-sm border text-[13px] transition-all duration-100 ${
            yamlOpen
              ? "border-[var(--dac-accent)] text-[var(--dac-accent)] hover:bg-[color-mix(in_srgb,var(--dac-accent)_8%,transparent)]"
              : "border-[var(--dac-border)] bg-[var(--dac-background)] text-[var(--dac-text-secondary)] hover:text-[var(--dac-text-primary)] hover:border-[var(--dac-text-muted)] hover:bg-[var(--dac-surface-hover)]"
          }`}
          title="View YAML"
        >
          <svg width="13" height="13" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round">
            <path d="M5.5 4L2 8L5.5 12" />
            <path d="M10.5 4L14 8L10.5 12" />
          </svg>
        </button>
      )}
    </div>
  );

  const gridColumns = `1fr ${yamlOpen ? yamlWidth : 0}px`;

  const renderWidget = (widget: Widget, rowIdx: number, widgetIdx: number, totalInRow: number) => {
    const id = `r${rowIdx}-w${widgetIdx}`;
    const col = widget.col || Math.floor(12 / totalInRow);
    return (
      <WidgetContainer key={id} col={col} height={widget.height}>
        {widget.tabs && widget.tabs.length > 0 ? (
          <TabbedWidget
            dashboardName={name || ""}
            baseId={id}
            widget={widget}
            filters={activeFilters ?? undefined}
            WidgetFrame={WidgetFrame}
          />
        ) : (
          <DataWidget
            dashboardName={name || ""}
            widgetId={id}
            widget={widget}
            filters={activeFilters ?? undefined}
            WidgetFrame={WidgetFrame}
          />
        )}
      </WidgetContainer>
    );
  };

  return (
    <CrossFilterContext.Provider key={dashboard.name} value={crossFilterContext}>
    <div
      className={`dac-layout h-screen overflow-hidden ${isResizing ? "dac-layout-resizing" : ""}`}
      style={{ gridTemplateColumns: gridColumns }}
    >
      <div className="overflow-y-auto min-w-0">
        <div ref={dashboardExportRef}>
          <DashboardLayout dashboard={dashboard} filterBar={filterBar} headerActions={headerActions}>
            {/* Rows without a tab — always visible */}
            {dashboard.rows.map((row, rowIdx) => {
              if (row.tab) return null;
              return (
                <div
                  key={rowIdx}
                  className="animate-in"
                  style={{ animationDelay: `${50 + rowIdx * 30}ms` }}
                >
                  <Row height={row.height}>
                    {row.widgets.map((widget, widgetIdx) =>
                      renderWidget(widget, rowIdx, widgetIdx, row.widgets.length),
                    )}
                  </Row>
                </div>
              );
            })}

            {/* Tab bar + tab content */}
            {hasTabs && (
              <>
                <div className="flex overflow-x-auto scrollbar-hide border-b border-[var(--dac-border)]">
                  {tabNames.map((tab) => (
                    <button
                      key={tab}
                      onClick={() => setActiveTab(tab)}
                      className={`shrink-0 px-4 py-2 text-[13px] font-medium transition-colors duration-100 border-b-2 -mb-px ${
                        currentTab === tab
                          ? "border-[var(--dac-accent)] text-[var(--dac-text-primary)]"
                          : "border-transparent text-[var(--dac-text-muted)] hover:text-[var(--dac-text-secondary)]"
                      }`}
                    >
                      {tab}
                    </button>
                  ))}
                </div>

                {tabFilters.length > 0 && (
                  <div className="mt-4">
                    <FilterBar
                      filters={tabFilters}
                      values={activeFilters ?? {}}
                      onChange={handleFilterChange}
                    />
                  </div>
                )}

                {dashboard.rows.map((row, rowIdx) => {
                  if (row.tab !== currentTab) return null;
                  return (
                    <div
                      key={rowIdx}
                      className="animate-in"
                      style={{ animationDelay: `${50 + rowIdx * 30}ms` }}
                    >
                      <Row height={row.height}>
                        {row.widgets.map((widget, widgetIdx) =>
                          renderWidget(widget, rowIdx, widgetIdx, row.widgets.length),
                        )}
                      </Row>
                    </div>
                  );
                })}
              </>
            )}
          </DashboardLayout>
        </div>
      </div>
      <YamlPanel
        dashboardName={name || ""}
        fileType={dashboard.file_type}
        isOpen={yamlOpen}
        onClose={() => setYamlOpen(false)}
        onResize={handleYamlResize}
        onResizeStart={onResizeStart}
        onResizeEnd={onResizeEnd}
      />
    </div>
    </CrossFilterContext.Provider>
  );
}
