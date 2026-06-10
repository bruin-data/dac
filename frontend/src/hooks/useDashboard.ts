import { useQuery, keepPreviousData } from "@tanstack/react-query";
import { listDashboards, getDashboard } from "../api/client";

export function useDashboardList() {
  return useQuery({
    queryKey: ["dashboards"],
    queryFn: listDashboards,
  });
}

export function useDashboard(name: string) {
  return useQuery({
    queryKey: ["dashboard", name],
    queryFn: () => getDashboard(name),
    enabled: !!name,
    placeholderData: keepPreviousData,
  });
}
