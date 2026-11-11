import { useQueries } from "@tanstack/react-query";
import { getStaticPayload } from "../api/client";
import { dataClientKey, useDataClient } from "../api/dataClient";
import { queryFilterReferences } from "../lib/crossFilter";
import type { Dashboard, Filter } from "../types/dashboard";

/** Resolve options independently so a slow or failed filter query never blocks widgets. */
export function useFilterOptions(
  dashboard: Dashboard | undefined,
  values?: Record<string, unknown>,
): Filter[] {
  const client = useDataClient();
  const filters = dashboard?.filters ?? [];
  const dynamic = filters.filter((f) =>
    f.type === "select" && (f.options?.query || f.options?.dimension) && !f.options?.values?.length,
  );
  const results = useQueries({
    queries: dynamic.map((filter) => {
      // Model options do not depend on other controls; the server applies filter defaults.
      const dependencies = filter.options?.dimension
        ? {}
        : optionQueryDependencies(filter.options!.query!, filters, values);
      return {
        queryKey: ["filterOptions", dashboard?.name, filter.name, filter.options?.query,
          filter.options?.connection, dashboard?.connection, filter.options?.values, dependencies,
          filter.options?.dimension, filter.options?.metric,
          dashboard?.model, dashboard?.models,
          dataClientKey(client)],
        queryFn: () => client.fetchFilterOptions(dashboard!.name, filter.name, dependencies),
        enabled: !getStaticPayload(),
      };
    }),
  });
  return filters.map((filter) => {
    const index = dynamic.indexOf(filter);
    if (index < 0 || getStaticPayload()) return filter;
    const result = results[index];
    const resolved = Array.isArray(result.data) ? { values: result.data } : result.data;
    return {
      ...filter,
      options: {
        ...filter.options,
        values: resolved?.values ?? filter.options?.values ?? [],
        metric_values: resolved?.metric_values,
        metric_format: resolved?.metric_format,
        isLoading: result.isPending,
        error: result.error?.message,
      },
    };
  });
}

/** Track filters.name and filters["name"] options-query dependencies. */
function optionQueryDependencies(
  query: string,
  filters: Filter[],
  values: Record<string, unknown> = {},
): Record<string, unknown> {
  const dependencies = queryFilterReferences({ sql: query });
  return Object.fromEntries(filters.filter((filter) => dependencies.has(filter.name)).map((filter) => [
    filter.name, values[filter.name] !== undefined ? values[filter.name] : filter.default,
  ]));
}
