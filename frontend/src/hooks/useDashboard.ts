import { useQuery, keepPreviousData } from "@tanstack/react-query";
import { useDataClient } from "../api/dataClient";

export function useDashboardList() {
  const client = useDataClient();
  return useQuery({
    queryKey: ["dashboards"],
    queryFn: () => client.listDashboards(),
  });
}

export function useDashboard(name: string) {
  const client = useDataClient();
  return useQuery({
    queryKey: ["dashboard", name],
    queryFn: () => client.getDashboard(name),
    enabled: !!name,
    placeholderData: keepPreviousData,
  });
}
