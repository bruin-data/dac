import { Component, useEffect, useMemo, useRef, useState, type ReactNode } from 'react';
import { createRoot } from 'react-dom/client';
import { QueryClient, QueryClientProvider, useQueryClient } from '@tanstack/react-query';
import {
    BruinFilterBar, BruinRow, BruinWidgetContainer, BruinWidgetFrame,
    buildDefaultFilters, DataClientProvider, DataWidget, TabbedWidget, TemplateProvider,
    useFilterOptions, type Dashboard, type DataClient, type Widget,
} from '../src/lib';
import { darkTemplate, lightTemplate } from './theme';
import './dashboard.css';

// Mirrors Bruin Desktop's dashboard view (src/renderer/src/features/dashboards/dashboard-view.tsx and lib/dashboards.ts):
// the header, tabs, row layout and DataClient rules. Desktop-only parts (chat, widget tagging, Electron) are left out.
// When Desktop's view changes, update this copy too. Widget rendering itself comes from dac's own components (src/lib.ts).

// Lucide's RotateCw and ExternalLink, the icons Desktop's header uses.
const RefreshIcon = () => <svg viewBox="0 0 24 24" aria-hidden="true"><path d="M21 12a9 9 0 1 1-9-9c2.52 0 4.93 1 6.74 2.74L21 8" /><path d="M21 3v5h-5" /></svg>;
const OpenIcon = () => <svg viewBox="0 0 24 24" aria-hidden="true"><path d="M15 3h6v6" /><path d="M10 14 21 3" /><path d="M18 13v6a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2h6" /></svg>;

const relative = new Intl.RelativeTimeFormat(undefined, { numeric: 'auto' });
function relativeTime(iso: string): string {
    const seconds = (new Date(iso).getTime() - Date.now()) / 1000;
    if (Number.isNaN(seconds)) return '';
    for (const [unit, size] of [['year', 31536000], ['month', 2592000], ['week', 604800], ['day', 86400], ['hour', 3600], ['minute', 60]] as const) {
        if (Math.abs(seconds) >= size) return relative.format(Math.round(seconds / size), unit);
    }
    return 'just now';
}

type ToolResult = { isError?: boolean; content?: { type: string; text?: string }[]; structuredContent?: Record<string, unknown> };
type McpApp = {
    callServerTool(name: string, args: Record<string, unknown>): Promise<ToolResult>;
    onToolInput(callback: (input: { arguments?: Record<string, unknown> }) => void): void;
    onToolResult(callback: (result: ToolResult) => void): void;
    onHostContextChanged(callback: (context: { theme?: string }) => void): void;
    getHostContext(): { theme?: string } | null;
    openLink(url: string): Promise<unknown>;
    autoResize(): void;
};
declare global { interface Window { createMcpApp(setup: (app: McpApp) => void): Promise<void> } }
type Detail = {
    id: number; title: string; url: string; updated_at: string; revision: string; state_kind: 'draft' | 'published';
    has_draft: boolean; definition: Dashboard;
};

function payload(result: ToolResult) {
    const text = result.content?.find((item) => item.type === 'text')?.text;
    if (result.isError) throw new Error(text || 'Unable to load the dashboard.');
    return result.structuredContent ?? JSON.parse(text || '{}');
}

// Lucide's AlertTriangle, as in Desktop's widget-boundary.tsx.
const AlertIcon = () => <svg viewBox="0 0 24 24" aria-hidden="true" className="alert-icon"><path d="m21.73 18-8-14a2 2 0 0 0-3.48 0l-8 14A2 2 0 0 0 4 21h16a2 2 0 0 0 1.73-3" /><path d="M12 9v4" /><path d="M12 17h.01" /></svg>;

/** One widget the components can't draw (e.g. a malformed definition) mustn't take the view down. */
class WidgetBoundary extends Component<{ name: string; children: ReactNode }, { failed: boolean }> {
    state = { failed: false };
    static getDerivedStateFromError() { return { failed: true }; }
    componentDidCatch(error: unknown) { console.error(`Widget "${this.props.name}" failed to render:`, error); }
    render() {
        if (!this.state.failed) return this.props.children;
        return <div role="alert" className="widget-boundary">
            <p><AlertIcon />{this.props.name || 'This widget'} can’t be shown</p>
            <p className="meta">Its definition doesn’t match what the app can draw.</p>
        </div>;
    }
}

function Loading() {
    return <div aria-busy="true" className="loading">
        <div className="bar title" /><div className="bar line" />
        <div className="cards">{[0, 1, 2, 3].map((index) => <div key={index} />)}</div>
        <div className="block" />
    </div>;
}

function Centered({ children }: { children: ReactNode }) {
    return <div className="centered">{children}</div>;
}

/** Like Desktop, a versioned name keeps an edit from reusing another widget's cached data. */
function dacName(id: number, revision: string): string {
    return `${id}@${revision}`;
}

function DashboardView({ app }: { app: McpApp }) {
    const [detail, setDetail] = useState<Detail>();
    const [error, setError] = useState('');
    // The tool call itself failed (e.g. a confirmation it needs): shown as is, with nothing to retry.
    const [failed, setFailed] = useState('');
    const [dark, setDark] = useState(app.getHostContext()?.theme === 'dark');
    const selection = useRef<{ id: number; state?: unknown } | undefined>(undefined);
    const input = useRef<Record<string, unknown>>({});
    const sequence = useRef(0);
    // A background refetch keeps the current dashboard on screen and leaves errors to the caller.
    const load = async (background = false): Promise<Detail | undefined> => {
        if (!selection.current) return undefined;
        const current = ++sequence.current;
        if (!background) {
            setError('');
            setDetail(undefined);
        }
        try {
            const result = payload(await app.callServerTool('dashboard-preview-tool', {
                dashboard_id: selection.current.id, action: 'definition',
                ...(selection.current.state ? { state: selection.current.state } : {}),
            }));
            // Like React Query in Desktop: an unchanged definition keeps its object, so the data client
            // and widget cache keys stay the same and a refresh doesn't query every widget twice.
            if (current === sequence.current) setDetail((shown) => shown?.id === result.id && shown?.revision === result.revision ? shown : result);
            return result;
        } catch (e) {
            if (background) throw e;
            if (current === sequence.current) setError((e as Error).message);
        }
    };
    useEffect(() => {
        // Like Desktop: a link never navigates the dashboard frame; http(s) ones open through the host.
        document.addEventListener('click', (event) => {
            const link = (event.target as Element | null)?.closest?.('a[href]') as HTMLAnchorElement | null;
            if (!link) return;
            event.preventDefault();
            if (/^https?:\/\//i.test(link.getAttribute('href') ?? '')) void app.openLink(link.href);
        }, true);
        app.onToolInput((event) => { input.current = event.arguments ?? {}; });
        app.onToolResult((result) => {
            try {
                const body = payload(result);
                if (!Number.isInteger(body.id)) return;
                setFailed('');
                selection.current = { id: body.id, state: input.current.state === 'draft' || input.current.state === 'published' ? input.current.state : undefined };
                void load();
            } catch (e) {
                selection.current = undefined;
                setFailed((e as Error).message);
            }
        });
        app.onHostContextChanged((context) => setDark(context.theme === 'dark'));
        app.autoResize();
    }, [app]);

    let body: ReactNode;
    // Like Desktop: only a first load can fail visibly; a deleted dashboard says so.
    if (failed) {
        body = <Centered><p>{failed}</p></Centered>;
    } else if (error) {
        const gone = /\b404\b|not found/i.test(error);
        body = <Centered>
            <p>{gone ? 'This dashboard was deleted.' : 'Couldn’t load this dashboard.'}</p>
            {!gone && <button className="action" onClick={() => void load()}>Try again</button>}
        </Centered>;
    } else if (!detail) {
        body = <Loading />;
    } else {
        body = <WidgetBoundary key={dacName(detail.id, detail.revision)} name={detail.title}>
            <Loaded detail={detail} app={app} refetch={() => load(true)} />
        </WidgetBoundary>;
    }
    return <TemplateProvider template={dark ? darkTemplate : lightTemplate} className="dash-root">{body}</TemplateProvider>;
}

function Loaded({ detail, app, refetch }: { detail: Detail; app: McpApp; refetch: () => Promise<Detail | undefined> }) {
    const queryClient = useQueryClient();
    const dac = useMemo(() => ({ ...detail.definition, name: dacName(detail.id, detail.revision) }), [detail]);
    const [refreshing, setRefreshing] = useState(false);
    // Like Desktop: keep what's on screen and spin the icon while the definition and data reload.
    const refresh = async () => {
        setRefreshing(true);
        try {
            // The definition first: if it changed, the widgets remount under its new revision anyway,
            // so only an unchanged one needs its data invalidated.
            if ((await refetch())?.revision !== detail.revision) return;
            await Promise.all([
                queryClient.invalidateQueries({ queryKey: ['widget-data', dac.name] }),
                queryClient.invalidateQueries({ queryKey: ['filterOptions', dac.name] }),
            ]);
        } catch {
            // The widgets keep what they showed; the button just stops spinning.
        } finally {
            setRefreshing(false);
        }
    };
    // Desktop's dashboardDataClient, reading through the app-only preview tool instead of Electron.
    const dashboardDataClient = useMemo<DataClient>(() => {
        const read = async (action: string, extra = {}) => payload(await app.callServerTool('dashboard-preview-tool', {
            dashboard_id: detail.id, state: detail.state_kind, revision: detail.revision, action, ...extra,
        }));
        return {
            listDashboards: async () => [],
            getDashboard: async () => dac,
            fetchWidgetData: async (_name, widgetId, filters) => {
                try { return await read('widget', { widget_id: widgetId, filters: filters ?? {} }); }
                catch (error) { return { columns: [], rows: [], error: (error as Error).message }; }
            },
            fetchFilterOptions: async (_name, filterName, filters) => read('filter-options', { filter_name: filterName, filters: filters ?? {} }),
        };
    }, [app, dac, detail]);
    return (
        <DataClientProvider client={dashboardDataClient}>
            <div className="dash-page">
                <header>
                    <div className="title">
                        <h1>
                            <span>{detail.title || detail.definition.name}</span>
                            {detail.state_kind === 'draft' && <span className="pill">Unpublished changes</span>}
                        </h1>
                        {detail.definition.description && <p>{detail.definition.description}</p>}
                    </div>
                    <div className="actions">
                        {detail.updated_at && <span className="meta" title={new Date(detail.updated_at).toLocaleString()}>Edited {relativeTime(detail.updated_at)}</span>}
                        <button className={refreshing ? 'icon spinning' : 'icon'} onClick={() => void refresh()} disabled={refreshing} aria-label="Refresh data" title="Refresh data"><RefreshIcon /></button>
                        <button className="icon" onClick={() => void app.openLink(detail.url)} aria-label="Open in Bruin Cloud" title="Open in Bruin Cloud"><OpenIcon /></button>
                    </div>
                </header>
                <Rows dac={dac} />
            </div>
        </DataClientProvider>
    );
}

/** Desktop's filters, tabs and rows; split from Loaded only because useFilterOptions needs the DataClientProvider above it. */
function Rows({ dac }: { dac: Dashboard }) {
    const [filters, setFilters] = useState<Record<string, unknown>>(() => buildDefaultFilters(dac));
    // Typing in a text or number filter would otherwise re-run every widget per keystroke.
    const typing = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);
    // Edits wait here together, so changing one filter never drops another one still waiting.
    const pending = useRef<Record<string, unknown>>({});
    useEffect(() => () => clearTimeout(typing.current), []);
    const onFilterChange = (name: string, value: unknown) => {
        const type = dac.filters?.find((filter) => filter.name === name)?.type;
        pending.current[name] = value;
        const apply = () => {
            const edits = pending.current;
            pending.current = {};
            setFilters((current) => ({ ...current, ...edits }));
        };
        clearTimeout(typing.current);
        if (type === 'text' || type === 'number') typing.current = setTimeout(apply, 400);
        else apply();
    };
    const allFilters = useFilterOptions(dac, filters);
    const tabNames = [...new Set(dac.rows.flatMap((row) => row.tab ? [row.tab] : []))];
    const [activeTab, setActiveTab] = useState<string | null>(null);
    const currentTab = activeTab && tabNames.includes(activeTab) ? activeTab : (tabNames[0] ?? null);
    if (dac.rows.length === 0) {
        return <div className="empty"><p>This dashboard is empty.</p><p className="meta">Ask Claude to add some widgets to it.</p></div>;
    }
    const globalFilters = allFilters.filter((filter) => !filter.tab || !tabNames.includes(filter.tab));
    const tabFilters = currentTab ? allFilters.filter((filter) => filter.tab === currentTab) : [];
    const visibleRows = dac.rows.map((row, index) => ({ row, index })).filter(({ row }) => !row.tab || row.tab === currentTab);
    return <>
        {globalFilters.length > 0 && <div className="dash-filters"><BruinFilterBar filters={globalFilters} values={filters} onChange={onFilterChange} /></div>}
        {tabNames.length > 0 && <div className="tabs" role="tablist" aria-label="Dashboard sections">
            {tabNames.map((tab) => <button key={tab} role="tab" aria-selected={tab === currentTab} onClick={() => setActiveTab(tab)}>{tab}</button>)}
        </div>}
        {tabFilters.length > 0 && <div className="dash-filters"><BruinFilterBar filters={tabFilters} values={filters} onChange={onFilterChange} /></div>}
        <div className="space-y-4">
            {visibleRows.map(({ row, index }) => <div key={index} className="dash-row" style={{ '--row-h': rowHeight(row.height) } as Record<string, string>}>
                <BruinRow height={row.height}>
                    {row.widgets.map((widget, widgetIndex) => {
                        const widgetId = `r${index}-w${widgetIndex}`;
                        return <BruinWidgetContainer key={widgetId} col={widget.col ? Number(widget.col) : Math.floor(12 / row.widgets.length)}>
                            <WidgetShell id={widgetId} dashboardName={dac.name} widget={widget} filters={filters} />
                        </BruinWidgetContainer>;
                    })}
                </BruinRow>
            </div>)}
        </div>
    </>;
}

function rowHeight(height: number | string | undefined): string {
    if (typeof height === 'number') return `${height}px`;
    return height ?? 'auto';
}

function WidgetShell({ id, dashboardName, widget, filters }: {
    id: string; dashboardName: string; widget: Widget; filters: Record<string, unknown>;
}) {
    return <div data-widget-id={id} data-type={widget.type} className="dash-widget relative h-full">
        <WidgetBoundary name={widget.name}>
            {widget.tabs?.length
                ? <TabbedWidget dashboardName={dashboardName} baseId={id} widget={widget} filters={filters} WidgetFrame={BruinWidgetFrame} />
                : <DataWidget dashboardName={dashboardName} widgetId={id} widget={widget} filters={filters} WidgetFrame={BruinWidgetFrame} />}
        </WidgetBoundary>
    </div>;
}

window.createMcpApp((app) => {
    const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false, refetchOnWindowFocus: false } } });
    createRoot(document.getElementById('root')!).render(<QueryClientProvider client={queryClient}><DashboardView app={app} /></QueryClientProvider>);
}).catch(() => { document.getElementById('root')!.textContent = 'Unable to connect to the dashboard host. Open the dashboard link in Bruin Cloud.'; });
