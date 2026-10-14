import { useQueries } from "@tanstack/react-query";
import { fetchFilterOptions, getStaticPayload } from "../api/client";
import type { Dashboard, Filter } from "../types/dashboard";

/** Resolve options independently so a slow or failed filter query never blocks widgets. */
export function useFilterOptions(
  dashboard: Dashboard | undefined,
  values?: Record<string, unknown>,
): Filter[] {
  const filters = dashboard?.filters ?? [];
  const dynamic = filters.filter((f) =>
    f.type === "select" && f.options?.query && !f.options.values?.length,
  );
  const results = useQueries({
    queries: dynamic.map((filter) => {
      const dependencies = optionQueryDependencies(filter.options!.query!, filters, values);
      return {
        queryKey: ["filterOptions", dashboard?.name, filter.name, filter.options?.query,
          filter.options?.connection, dashboard?.connection, filter.options?.values, dependencies],
        queryFn: () => fetchFilterOptions(dashboard!.name, filter.name, dependencies),
        enabled: !getStaticPayload(),
      };
    }),
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

/** Track filters.name and filters["name"] options-query dependencies. */
function optionQueryDependencies(
  query: string,
  filters: Filter[],
  values: Record<string, unknown> = {},
): Record<string, unknown> {
  const dependencies = new Set(
    [...query.matchAll(/\bfilters\s*(?:\.\s*([A-Za-z_]\w*)|\[\s*(["'])(.*?)\2\s*\])/g)]
      .map((match) => match[1] ?? match[3]),
  );
  return Object.fromEntries(filters.filter((filter) => dependencies.has(filter.name)).map((filter) => [
    filter.name, values[filter.name] !== undefined ? values[filter.name] : filter.default,
  ]));
}
