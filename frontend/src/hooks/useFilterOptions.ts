import { useQueries } from "@tanstack/react-query";
import { fetchFilterOptions, getStaticPayload } from "../api/client";
import type { Dashboard, Filter } from "../types/dashboard";

/** Resolve options independently so a slow or failed filter query never blocks widgets. */
export function useFilterOptions(dashboard: Dashboard | undefined): Filter[] {
  const filters = dashboard?.filters ?? [];
  const dynamic = filters.filter((f) =>
    f.type === "select" && f.options?.query && !f.options.values?.length,
  );
  const results = useQueries({
    queries: dynamic.map((filter) => ({
      queryKey: ["filterOptions", dashboard?.name, filter.name, filter.options?.query,
        filter.options?.connection, dashboard?.connection, filter.options?.values],
      queryFn: () => fetchFilterOptions(dashboard!.name, filter.name),
      enabled: !getStaticPayload(),
    })),
  });
  return filters.map((filter) => {
    const index = dynamic.indexOf(filter);
    if (index < 0 || getStaticPayload()) return filter;
    const result = results[index];
    return {
      ...filter,
      options: {
        ...filter.options,
        values: result.data ?? filter.options?.values ?? [],
        isLoading: result.isPending,
        error: result.error?.message,
      },
    };
  });
}
