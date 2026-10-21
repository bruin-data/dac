import { createContext, useContext } from "react";
import type { Dashboard, DashboardSummary, WidgetData } from "../types/dashboard";
import { fetchFilterOptions, fetchWidgetData, getDashboard, listDashboards } from "./client";

/** Widget ids are `r{row}-w{widget}` (zero-based), plus `::{tab name}` for a tab of a tabs widget. */
export interface DataClient {
  listDashboards(): Promise<DashboardSummary[]>;
  getDashboard(name: string): Promise<Dashboard>;
  fetchWidgetData(
    dashboardName: string,
    widgetId: string,
    filters?: Record<string, unknown>,
  ): Promise<WidgetData>;
  fetchFilterOptions(
    dashboardName: string,
    filterName: string,
    filters?: Record<string, unknown>,
  ): Promise<string[]>;
}

export const httpDataClient: DataClient = {
  listDashboards,
  getDashboard,
  fetchWidgetData,
  fetchFilterOptions,
};

export const DataClientContext = createContext<DataClient>(httpDataClient);

export function useDataClient(): DataClient {
  return useContext(DataClientContext);
}
