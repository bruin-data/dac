import { useQuery, keepPreviousData } from "@tanstack/react-query";
import { dataClientKey, useDataClient } from "../api/dataClient";

export function useDashboardList() {
  const client = useDataClient();
  return useQuery({
    queryKey: ["dashboards", dataClientKey(client)],
    queryFn: () => client.listDashboards(),
  });
}

export function useDashboard(name: string) {
  const client = useDataClient();
  return useQuery({
    queryKey: ["dashboard", name, dataClientKey(client)],
    queryFn: () => client.getDashboard(name),
    enabled: !!name,
    placeholderData: keepPreviousData,
  });
}
