import { useQuery, keepPreviousData } from "@tanstack/react-query";
import { fetchWidgetData } from "../api/client";
import type { WidgetData } from "../types/dashboard";

/**
 * Fetches data for a single widget via its own API call.
 * Uses React Query for caching, deduplication, and automatic refetch
 * when filters change.
 */
export function useWidgetQuery(
  dashboardName: string,
  widgetId: string,
  filters?: Record<string, unknown>,
  enabled = true,
) {
  return useQuery<WidgetData>({
    queryKey: ["widget-data", dashboardName, widgetId, filters],
    queryFn: () => fetchWidgetData(dashboardName, widgetId, filters),
    enabled: enabled && !!dashboardName && !!widgetId,
    placeholderData: keepPreviousData,
  });
}
