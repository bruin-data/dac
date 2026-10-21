import { useQuery, keepPreviousData } from "@tanstack/react-query";
import { useDataClient } from "../api/dataClient";
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
  const client = useDataClient();
  return useQuery<WidgetData>({
    queryKey: ["widget-data", dashboardName, widgetId, filters],
    queryFn: () => client.fetchWidgetData(dashboardName, widgetId, filters),
    enabled: enabled && !!dashboardName && !!widgetId,
    placeholderData: keepPreviousData,
  });
}
