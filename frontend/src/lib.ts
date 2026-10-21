// Package entry (`npm run build:lib`); the dac app starts from main.tsx.

export { DataClientProvider } from "./api/DataClientProvider";
export { httpDataClient, useDataClient, type DataClient } from "./api/dataClient";

export { TemplateProvider, useTemplate, useTokens, useTemplateRaw } from "./themes/TemplateProvider";
export { bruinTemplate, bruinDarkTemplate } from "./themes/bruin/index";
export { bruinLightTokens, bruinDarkTokens } from "./themes/bruin/tokens";
export { resolveTemplate } from "./themes/registry";

export { BruinFilterBar, resolvePreset } from "./themes/bruin/FilterBar";
export { BruinRow, BruinWidgetContainer } from "./themes/bruin/Row";
export { BruinWidgetFrame } from "./themes/bruin/WidgetFrame";
export { DataWidget, TabbedWidget } from "./components/DashboardWidget";
export { ChartWidget } from "./components/widgets/ChartWidget";
export { MetricWidget } from "./components/widgets/MetricWidget";
export { TableWidget } from "./components/widgets/TableWidget";
export { TextWidget } from "./components/widgets/TextWidget";

export { useDashboard, useDashboardList } from "./hooks/useDashboard";
export { useFilterOptions } from "./hooks/useFilterOptions";
export { useWidgetQuery } from "./hooks/useWidgetQuery";
export { buildDefaultFilters } from "./lib/filterParams";

export type * from "./types/dashboard";
export type * from "./types/template";
