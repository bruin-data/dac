import type { ComponentType, ReactNode } from "react";
import type { Dashboard, DashboardSummary, Filter, Widget, WidgetData } from "./dashboard";

/**
 * Props passed to individual widget renderers (Metric, Chart, Table).
 * Template authors implement components that accept these props.
 */
export interface WidgetDataProps {
  widget: Widget;
  data?: WidgetData;
}

/**
 * Props for the WidgetFrame — the container around each widget.
 * Handles the title, loading state, error display, and dispatches
 * to the correct widget renderer.
 */
export interface WidgetFrameProps {
  widget: Widget;
  data?: WidgetData;
  isLoading: boolean;
}

/**
 * Props for the filter bar.
 */
export interface FilterBarProps {
  filters: Filter[];
  values: Record<string, unknown>;
  onChange: (name: string, value: unknown) => void;
}

/**
 * Props for a grid row containing widgets.
 */
export interface RowProps {
  children: ReactNode;
  height?: number | string;
}

/**
 * Props for the column wrapper around each widget in a row.
 */
export interface WidgetContainerProps {
  col: number;
  children: ReactNode;
}

/**
 * Props for the dashboard list page.
 */
export interface DashboardListLayoutProps {
  dashboards: DashboardSummary[];
  adminEnabled?: boolean;
  onCreateClick?: () => void;
}

/**
 * Props for the dashboard detail page layout.
 */
export interface DashboardLayoutProps {
  dashboard: Dashboard;
  filterBar: ReactNode;
  headerActions?: ReactNode;
  children: ReactNode;
}

/**
 * The component set a template must provide.
 * Each component has a well-defined props contract.
 * Template authors implement these to create a fully custom UI.
 */
export interface TemplateComponents {
  /** Page layout for the dashboard detail view (header, filters, grid). */
  DashboardLayout: ComponentType<DashboardLayoutProps>;

  /** Page layout for the dashboard list/index. */
  DashboardListLayout: ComponentType<DashboardListLayoutProps>;

  /** Container around each widget — handles title, loading, error, and dispatches to widget type. */
  WidgetFrame: ComponentType<WidgetFrameProps>;

  /** Filter controls bar. */
  FilterBar: ComponentType<FilterBarProps>;

  /** Grid row. */
  Row: ComponentType<RowProps>;

  /** Column wrapper for a widget within a row. */
  WidgetContainer: ComponentType<WidgetContainerProps>;

  /** Single KPI number. */
  MetricWidget: ComponentType<WidgetDataProps>;

  /** Chart (line, bar, area, pie). */
  ChartWidget: ComponentType<WidgetDataProps>;

  /** Data table. */
  TableWidget: ComponentType<WidgetDataProps>;

  /** Static text/markdown block. */
  TextWidget: ComponentType<{ widget: Widget }>;
}

/**
 * A complete dashboard template — tokens + components.
 *
 * To create a custom template:
 * 1. Implement all components in TemplateComponents
 * 2. Define your color tokens
 * 3. Export a DashboardTemplate object
 *
 * Components receive data through props — they never fetch data themselves.
 * The framework handles data fetching, filter state, and routing.
 */
export interface DashboardTemplate {
  name: string;
  tokens: Record<string, string>;
  components: TemplateComponents;
}
