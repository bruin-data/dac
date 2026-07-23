import type { ReactElement } from "react";
import { BrowserRouter, HashRouter, Navigate, Routes, Route } from "react-router-dom";
import { QueryClient, QueryClientProvider, useQuery } from "@tanstack/react-query";
import { TemplateProvider } from "./themes/TemplateProvider";
import { resolveTemplate } from "./themes/registry";
import { fetchConfig, getStaticPayload } from "./api/client";
import { DashboardList } from "./components/DashboardList";
import { DashboardView } from "./components/DashboardView";
import { Admin } from "./components/Admin";
import { useLiveReload } from "./hooks/useLiveReload";

const staticPayload = getStaticPayload();
// HashRouter for embed/static mode (no server-side routing).
const isEmbedded = !!window.__DAC_API_BASE__;
const Router = staticPayload || isEmbedded ? HashRouter : BrowserRouter;

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 30_000,
      retry: 1,
    },
  },
});

// Dashboard the embed asked to open first, if any.
const initialDashboard = window.__DAC_INITIAL_DASHBOARD__;

function DashboardContent() {
  useLiveReload();

  // Static → baked dashboard; embed → requested one; else the list.
  let home: ReactElement;
  if (staticPayload) {
    home = <Navigate to={`/d/${encodeURIComponent(staticPayload.dashboard.name)}`} replace />;
  } else if (isEmbedded && initialDashboard) {
    home = <Navigate to={`/d/${encodeURIComponent(initialDashboard)}`} replace />;
  } else {
    home = <DashboardList />;
  }

  return (
    <Routes>
      <Route path="/" element={home} />
      <Route path="/d/:name" element={<DashboardView />} />
    </Routes>
  );
}

function AppWithTemplate() {
  const { data: config, isLoading } = useQuery({
    queryKey: ["config"],
    queryFn: fetchConfig,
    staleTime: Infinity,
  });

  if (isLoading || !config) {
    return null;
  }

  const template = resolveTemplate(config.template, config.tokens);

  return (
    <TemplateProvider template={template}>
      <DashboardContent />
    </TemplateProvider>
  );
}

function AppRouter() {
  return (
    <Router>
      <Routes>
        <Route path="/admin" element={<Admin />} />
        <Route path="/*" element={<AppWithTemplate />} />
      </Routes>
    </Router>
  );
}

export default function App() {
  return (
    <QueryClientProvider client={queryClient}>
      <AppRouter />
    </QueryClientProvider>
  );
}
