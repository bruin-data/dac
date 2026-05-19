import { useState, useMemo, useCallback, useEffect } from "react";
import { useParams, useLocation, useSearchParams } from "react-router-dom";
import { useQueryClient } from "@tanstack/react-query";
import { useDashboard } from "../hooks/useDashboard";
import { useWidgetQuery } from "../hooks/useWidgetQuery";
import { useTemplate } from "../themes/TemplateProvider";
import { resolvePreset } from "../themes/bruin/FilterBar";
import { saveDraft, discardDraft, createDraft } from "../api/client";
import { useDraftReload } from "../hooks/useDraftReload";
import { AgentChat } from "./AgentChat";
import { YamlPanel } from "./YamlPanel";
import type { Filter, Widget } from "../types/dashboard";
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

const isStaticMode = !!(window as any).__DAC_STATIC__;

// Non-data widget types that don't need a query.
const STATIC_WIDGET_TYPES = new Set(["text", "divider", "image"]);

// Persist sidebar state across navigation (module-level, resets on page refresh).
let _agentOpen = false;
let _yamlOpen = false;
let _agentWidth = 380;
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
  draftId,
}: {
  dashboardName: string;
  widgetId: string;
  widget: Widget;
  filters?: Record<string, unknown>;
  WidgetFrame: React.ComponentType<WidgetFrameProps>;
  draftId?: string;
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
      draftId={draftId}
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
  draftId,
}: {
  dashboardName: string;
  widgetId: string;
  widget: Widget;
  filters?: Record<string, unknown>;
  WidgetFrame: React.ComponentType<WidgetFrameProps>;
  draftId?: string;
}) {
  const { data, isLoading } = useWidgetQuery(dashboardName, widgetId, filters, true, draftId);
  return <WidgetFrame widget={widget} data={data} isLoading={isLoading} />;
}

export function DashboardView() {
  const { name } = useParams<{ name: string }>();
  const location = useLocation();
  const [searchParams, setSearchParams] = useSearchParams();
  const queryClient = useQueryClient();
  const [draftId, _setDraftId] = useState<string | null>(() => searchParams.get("draft"));

  // Sync draftId to/from the URL.
  const setDraftId = useCallback((id: string | null) => {
    _setDraftId(id);
    setSearchParams((prev) => {
      if (id) {
        prev.set("draft", id);
      } else {
        prev.delete("draft");
      }
      return prev;
    }, { replace: true });
  }, [setSearchParams]);

  useDraftReload(draftId);
  const { data: dashboard, isLoading: dashLoading, error: dashError } = useDashboard(name || "", draftId ?? undefined);
  const [agentOpen, _setAgentOpen] = useState(() => !!draftId || !!(location.state as any)?.agentOpen || _agentOpen);
  const [yamlOpen, _setYamlOpen] = useState(_yamlOpen);
  const [agentWidth, _setAgentWidth] = useState(_agentWidth);
  const [yamlWidth, _setYamlWidth] = useState(_yamlWidth);

  const setAgentOpen = useCallback((v: boolean | ((prev: boolean) => boolean)) => {
    _setAgentOpen((prev) => {
      const next = typeof v === "function" ? v(prev) : v;
      _agentOpen = next;
      return next;
    });
  }, []);
  const setYamlOpen = useCallback((v: boolean | ((prev: boolean) => boolean)) => {
    _setYamlOpen((prev) => {
      const next = typeof v === "function" ? v(prev) : v;
      _yamlOpen = next;
      return next;
    });
  }, []);
  const setAgentWidth = useCallback((v: number | ((prev: number) => number)) => {
    _setAgentWidth((prev) => {
      const next = typeof v === "function" ? v(prev) : v;
      _agentWidth = next;
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

  const handleAgentResize = useCallback((delta: number) => {
    setAgentWidth((w: number) => Math.max(280, Math.min(600, w + delta)));
  }, [setAgentWidth]);
  const handleYamlResize = useCallback((delta: number) => {
    setYamlWidth((w: number) => Math.max(280, Math.min(800, w + delta)));
  }, [setYamlWidth]);
  const onResizeStart = useCallback(() => setIsResizing(true), []);
  const onResizeEnd = useCallback(() => setIsResizing(false), []);

  // Start editing: create a draft and open the agent sidebar.
  const handleStartEditing = useCallback(async () => {
    if (draftId || !name) return;
    const id = crypto.randomUUID().slice(0, 8);
    try {
      await createDraft(name, id);
      setDraftId(id);
      setAgentOpen(true);
    } catch (err) {
      console.error("Failed to create draft:", err);
    }
  }, [draftId, name, setAgentOpen]);

  // When the agent edits files, invalidate all queries so the draft preview updates.
  const handleAgentFileChange = useCallback(() => {
    queryClient.invalidateQueries();
  }, [queryClient]);

  // Save: apply draft to live, clear draft state.
  const handleSave = useCallback(async () => {
    if (!draftId) return;
    try {
      await saveDraft(draftId);
      setDraftId(null);
      // Invalidate live caches so the dashboard refreshes with the saved version.
      queryClient.invalidateQueries({ queryKey: ["dashboard", name] });
      queryClient.invalidateQueries({ predicate: (q) => q.queryKey[0] === "widget-data" && q.queryKey[1] === name });
    } catch (err) {
      console.error("Failed to save draft:", err);
    }
  }, [draftId, name, queryClient]);

  // Discard: remove draft, clear draft state.
  const handleDiscard = useCallback(async () => {
    if (!draftId) return;
    try {
      await discardDraft(draftId);
    } catch {
      // Ignore — draft may already be cleaned up.
    }
    setDraftId(null);
    // Re-fetch live dashboard.
    queryClient.invalidateQueries({ queryKey: ["dashboard", name] });
    queryClient.invalidateQueries({ predicate: (q) => q.queryKey[0] === "widget-data" && q.queryKey[1] === name });
  }, [draftId, name, queryClient]);

  const defaultFilters = useMemo(
    () => dashboard ? buildDefaultFilters(dashboard) : null,
    [dashboard],
  );

  const [filters, setFilters] = useState<Record<string, unknown> | null>(null);
  const activeFilters = filters ?? defaultFilters;

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

  // Reset local state when the dashboard definition changes (e.g. agent edits the file).
  // This avoids a full remount so AgentChat stays alive.
  useEffect(() => {
    setFilters(null);
    setActiveTab(null);
  }, [dashboard]);

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
    setFilters((prev) => ({ ...prev, [filterName]: value }));
  };

  const filterBar = dashboard.filters ? (
    <FilterBar
      filters={dashboard.filters}
      values={activeFilters ?? {}}
      onChange={handleFilterChange}
    />
  ) : null;

  const headerActions = isStaticMode ? null : (
    <div className="flex items-center gap-1.5">
      {draftId && (
        <>
          <button
            onClick={handleDiscard}
            className="inline-flex items-center gap-1.5 h-7 px-2 rounded-sm border border-[var(--dac-border)] bg-[var(--dac-background)] text-[var(--dac-text-muted)] hover:text-[var(--dac-error)] hover:border-[var(--dac-error)] text-[13px] transition-all duration-100"
            title="Discard draft changes"
          >
            Discard
          </button>
          <button
            onClick={handleSave}
            className="inline-flex items-center gap-1.5 h-7 px-2 rounded-sm border border-[var(--dac-accent)] bg-[var(--dac-accent)] text-white hover:opacity-90 text-[13px] transition-all duration-100"
            title="Save draft to live"
          >
            <svg width="13" height="13" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round">
              <path d="M12.5 2H3.5C2.67 2 2 2.67 2 3.5V12.5C2 13.33 2.67 14 3.5 14H12.5C13.33 14 14 13.33 14 12.5V5L11 2Z" />
              <path d="M11 2V5H14" />
              <path d="M5 10H11" />
              <path d="M5 12H8" />
            </svg>
            Save
          </button>
        </>
      )}
      <button
        onClick={draftId ? () => setAgentOpen(!agentOpen) : handleStartEditing}
        className={`inline-flex items-center gap-1.5 h-7 px-2 rounded-sm border text-[13px] transition-all duration-100 ${
          agentOpen
            ? "border-[var(--dac-accent)] text-[var(--dac-accent)] hover:bg-[color-mix(in_srgb,var(--dac-accent)_8%,transparent)]"
            : "border-[var(--dac-border)] bg-[var(--dac-background)] text-[var(--dac-text-secondary)] hover:text-[var(--dac-text-primary)] hover:border-[var(--dac-text-muted)] hover:bg-[var(--dac-surface-hover)]"
        }`}
        title="Edit with AI"
      >
        <svg width="13" height="13" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round">
          <path d="M11.5 1.5L14.5 4.5L5 14H2V11L11.5 1.5Z" />
        </svg>
        Edit
      </button>
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
    </div>
  );

  const gridColumns = `${agentOpen ? agentWidth : 0}px 1fr ${yamlOpen ? yamlWidth : 0}px`;

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
          draftId={draftId ?? undefined}
        />
      </WidgetContainer>
    );
  };

  return (
    <div
      className={`dac-layout h-screen overflow-hidden ${isResizing ? "dac-layout-resizing" : ""}`}
      style={{ gridTemplateColumns: gridColumns }}
    >
      <AgentChat
        dashboardName={name || ""}
        draftId={draftId ?? undefined}
        isOpen={agentOpen}
        onClose={() => setAgentOpen(false)}
        onResize={handleAgentResize}
        onResizeStart={onResizeStart}
        onResizeEnd={onResizeEnd}
        onFileChange={handleAgentFileChange}
      />
      <div className="overflow-y-auto min-w-0">
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
      <YamlPanel
        dashboardName={name || ""}
        fileType={dashboard.file_type}
        isOpen={yamlOpen}
        onClose={() => setYamlOpen(false)}
        onResize={handleYamlResize}
        onResizeStart={onResizeStart}
        onResizeEnd={onResizeEnd}
        draftId={draftId ?? undefined}
      />
    </div>
  );
}
