import type { Filter, Widget } from "../types/dashboard";

const REF = /^\{\{\s*filters\.(\w+)\s*\}\}$/;

/**
 * Points fields written as `{{ filters.<breakdown> }}` at the selected option's dimensions:
 * chart `x` takes the first and `color` the second (unless color is set); a table column
 * becomes one column per selected dimension.
 */
export function resolveBreakdown(widget: Widget, filters: Filter[] | undefined, values: Record<string, unknown>): Widget {
  const resolvedTabs = widget.tabs?.map((tab) => resolveBreakdown(tab, filters, values));
  const tabs = resolvedTabs?.some((tab, i) => tab !== widget.tabs![i]) ? resolvedTabs : undefined;
  const dims = (field: unknown) => {
    const name = typeof field === "string" ? REF.exec(field.trim())?.[1] : undefined;
    const filter = filters?.find((f) => f.name === name && f.type === "breakdown");
    return filter && (filter.options?.breakdown?.find((o) => o.label === values[filter.name])?.dimensions.map((d) => d.split(":")[0]) ?? []);
  };
  const x = dims(widget.x?.field);
  const color = dims(widget.color?.field);
  // Columns written for a breakdown's dimensions keep their own settings and show only while
  // selected; the `{{ filters.x }}` column becomes the selected dimensions at its position.
  const refs = widget.columns?.map((column) => dims(column.name)).filter((names) => names) ?? [];
  const optionDims = new Set(filters?.filter((f) => f.type === "breakdown" && refs.length)
    .flatMap((f) => f.options?.breakdown?.flatMap((o) => o.dimensions.map((d) => d.split(":")[0])) ?? []));
  const columns = refs.length
    ? widget.columns!.flatMap((column) => {
      const names = dims(column.name);
      // Keep a column while it is selected, fixed in `dimensions`, or not a breakdown dimension.
      if (!names) return refs.some((selected) => selected!.includes(column.name)) || !optionDims.has(column.name)
        || widget.dimensions?.some((d) => d.name === column.name) ? [column] : [];
      return names.filter((name) => !widget.columns!.some((c) => c.name === name))
        .map((name) => names.length === 1 ? { ...column, name } : { ...column, name, label: undefined });
    })
    : undefined;
  // Only bar/line/area split a single y series by color.
  const autoColor = !widget.color && ["bar", "line", "area"].includes(widget.chart ?? "")
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
