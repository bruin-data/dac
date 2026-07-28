import { useState, useMemo, useCallback, useEffect, useRef } from "react";
import { useParams, useSearchParams } from "react-router-dom";
import { useDashboard } from "../hooks/useDashboard";
import { useWidgetQuery } from "../hooks/useWidgetQuery";
import { useTemplate } from "../themes/TemplateProvider";
import { resolvePreset } from "../themes/bruin/FilterBar";
import { fromParam, toParam, keepAllowedValues } from "../lib/filterParams";
import { YamlPanel } from "./YamlPanel";
import { ExportMenuButton, type ExportMenuItem } from "./ExportMenuButton";
import { fetchDashboardData } from "../api/client";
import { dashboardToCSV, downloadTextFile, slugify } from "../lib/csv";
import { exportElementAsPDF, exportElementAsPNG } from "../lib/renderExport";
import type { Filter, Widget, WidgetData } from "../types/dashboard";
import type { WidgetFrameProps } from "../types/template";

function buildDefaultFilters(dashboard: { filters?: Filter[] }): Record<string, unknown> {
  const defaults: Record<string, unknown> = {};
  if (dashboard.filters) {
    for (const f of dashboard.filters) {
      if (f.default !== undefined) {
        if (f.type === "date-range" && typeof f.default === "string") {
          const resolved = resolvePreset(f.default);
          defaults[f.name] = resolved ?? f.default;
        } else {
          defaults[f.name] = f.default;
        }
      }
    }
  }
  return defaults;
}

const isStaticMode = window.__DAC_STATIC__ !== undefined;
type DashboardExportFormat = "csv" | "png" | "pdf";

// Non-data widget types that don't need a query.
const STATIC_WIDGET_TYPES = new Set(["text", "divider", "image"]);

// Persist sidebar state across navigation (module-level, resets on page refresh).
let _yamlOpen = false;
let _yamlWidth = 420;

/**
 * DataWidget fetches data for a single widget via its own API call.
 * This allows tab-aware lazy loading and future viewport-based deferral.
 */
function DataWidget({
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
  // Static widgets (text, divider, image) don't need data.
  if (STATIC_WIDGET_TYPES.has(widget.type)) {
    return <WidgetFrame widget={widget} isLoading={false} />;
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
  return <WidgetFrame widget={widget} data={data} isLoading={isLoading || isPlaceholderData} />;
}

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

  // Last query string, so the reseed effect can skip our own writes.
  const lastSelfWrite = useRef<string | null>(null);

  const {
    DashboardLayout,
    WidgetFrame,
    FilterBar,
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

  const handleExport = useCallback(async (format: DashboardExportFormat) => {
    if (!dashboard) return;
    setExporting(format);
    try {
      const filename = `${slugify(dashboard.name)}.${format}`;
      if (format === "csv") {
        const widgetData = await fetchDashboardData(name || "", activeFilters ?? undefined);
        const sections: { name: string; data: WidgetData }[] = [];
        dashboard.rows.forEach((row, i) => {
          row.widgets.forEach((widget, j) => {
            if (widget.type === "text" || widget.type === "divider" || widget.type === "image") return;
            const data = widgetData[`r${i}-w${j}`];
            if (data && !data.error && data.rows && data.rows.length > 0) {
              sections.push({ name: widget.name, data });
            }
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

  const handleFilterChange = (filterName: string, value: unknown) => {
    // Accumulate locally; the projection effect writes the URL. Functional update
    // so batched changes in one tick don't clobber each other.
    setFilters((prev) => ({ ...(prev ?? activeFilters ?? {}), [filterName]: value }));
  };

  const filterBar = dashboard.filters ? (
    <FilterBar
      filters={dashboard.filters}
      values={activeFilters ?? {}}
      onChange={handleFilterChange}
    />
  ) : null;

  const exportItems: ExportMenuItem[] = [
    { key: "csv", label: "CSV", onSelect: () => handleExport("csv") },
    { key: "png", label: "PNG", onSelect: () => handleExport("png") },
    { key: "pdf", label: "PDF", onSelect: () => handleExport("pdf") },
  ];

  const headerActions = (
    <div data-dac-export-control className="flex items-center gap-1.5">
      <ExportMenuButton
        label={exporting ? "Exporting" : "Export"}
        ariaLabel="Export dashboard"
        title="Export dashboard"
        items={exportItems}
        disabled={exporting !== null}
      />
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
      <WidgetContainer key={id} col={col}>
        <DataWidget
          dashboardName={name || ""}
          widgetId={id}
          widget={widget}
          filters={activeFilters ?? undefined}
          WidgetFrame={WidgetFrame}
        />
      </WidgetContainer>
    );
  };

  return (
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
  );
}
