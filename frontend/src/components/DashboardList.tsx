import { useEffect } from "react";
import { useNavigate } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";
import { useDashboardList } from "../hooks/useDashboard";
import { useTemplate } from "../themes/TemplateProvider";
import { fetchConfig } from "../api/client";

export function DashboardList() {
  const { data: dashboards, isLoading, error } = useDashboardList();
  const { data: config } = useQuery({
    queryKey: ["config"],
    queryFn: fetchConfig,
    staleTime: Infinity,
  });
  const { DashboardListLayout } = useTemplate();
  const navigate = useNavigate();

  // Auto-redirect to the dashboard if there's only one.
  useEffect(() => {
    if (dashboards && dashboards.length === 1) {
      navigate(`/d/${encodeURIComponent(dashboards[0].name)}`, { replace: true });
    }
  }, [dashboards, navigate]);

  if (isLoading) {
    return (
      <div className="max-w-[860px] mx-auto px-4 sm:px-6 pt-16 sm:pt-24 pb-8">
        <div className="skeleton h-7 w-40 mb-8" />
        <div className="skeleton h-8 w-full mb-4 rounded" />
        <div className="space-y-2">
          <div className="skeleton h-12 w-full" />
          <div className="skeleton h-12 w-full" />
          <div className="skeleton h-12 w-3/4" />
        </div>
      </div>
    );
  }

  if (error) {
    return (
      <div className="max-w-[860px] mx-auto px-4 sm:px-6 pt-16 sm:pt-24 pb-8">
        <div className="text-[13px] font-mono text-[var(--dac-error)]">{error.message}</div>
      </div>
    );
  }

  // If there's only one dashboard, we'll redirect — don't flash the list.
  if (dashboards && dashboards.length === 1) {
    return null;
  }

  return (
    <div className="h-screen overflow-y-auto">
      <DashboardListLayout
        dashboards={dashboards ?? []}
        adminEnabled={config?.admin_enabled}
      />
    </div>
  );
}
