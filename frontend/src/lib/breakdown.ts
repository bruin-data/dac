import type { Dashboard, Widget } from "../types/dashboard";

const REF = /^\{\{\s*filters\.(\w+)\s*\}\}$/;

/**
 * Points fields written as `{{ filters.<breakdown> }}` at the selected option's dimensions:
 * chart `x` takes the first and `color` the second (unless color is set); a table column
 * becomes one column per selected dimension.
 */
export function resolveBreakdown(widget: Widget, dashboard: Pick<Dashboard, "filters" | "queries">, values: Record<string, unknown>): Widget {
  const resolvedTabs = widget.tabs?.map((tab) => resolveBreakdown(tab, dashboard, values));
  const tabs = resolvedTabs?.some((tab, i) => tab !== widget.tabs![i]) ? resolvedTabs : undefined;
  const breakdown = (field: unknown) => {
    const name = typeof field === "string" ? REF.exec(field.trim())?.[1] : undefined;
    return dashboard.filters?.find((f) => f.name === name && f.type === "breakdown");
  };
  const dims = (field: unknown) => {
    const filter = breakdown(field);
    return filter && (filter.options?.breakdown?.find((o) => o.label === values[filter.name])?.dimensions.map((d) => d.split(":")[0]) ?? []);
  };
  const x = dims(widget.x?.field);
  const color = dims(widget.color?.field);
  // Columns written for a referenced breakdown's dimensions keep their own settings and show only
  // while selected or fixed in the query; the `{{ filters.x }}` column becomes the selected dimensions.
  const refs = widget.columns?.map((column) => dims(column.name)).filter((names) => names) ?? [];
  const optionDims = new Set(widget.columns?.flatMap((column) => breakdown(column.name)?.options?.breakdown
    ?.flatMap((o) => o.dimensions.map((d) => d.split(":")[0])) ?? []));
  const fixed = [...widget.dimensions ?? [], ...(widget.query && dashboard.queries?.[widget.query]?.dimensions) || []];
  const added = new Set<string>(); // dimensions already placed by a `{{ }}` column
  const columns = refs.length
    ? widget.columns!.flatMap((column) => {
      const names = dims(column.name);
      if (!names) return refs.some((selected) => selected!.includes(column.name)) || !optionDims.has(column.name)
        || fixed.some((d) => d.name === column.name) ? [column] : [];
      return names.filter((name) => !widget.columns!.some((c) => c.name === name) && !added.has(name) && added.add(name))
        .map((name) => names.length === 1 ? { ...column, name } : { ...column, name, label: undefined });
    })
    : undefined;
  // Only bar/line/area split a single y series by color; a right axis (y2) can't be split.
  const autoColor = !widget.color && !widget.y2 && ["bar", "line", "area"].includes(widget.chart ?? "")
    && [widget.y?.field ?? []].flat().length === 1 ? x?.[1] : undefined;
  if (!tabs && !x && !color && !columns) return widget;
  return {
    ...widget,
    ...(tabs && { tabs }),
    ...(columns && { columns }),
    ...(x?.[0] && { x: { ...widget.x!, field: x[0] } }),
    ...((color?.[0] || autoColor) && { color: { field: color?.[0] ?? autoColor! } }),
  };
}
