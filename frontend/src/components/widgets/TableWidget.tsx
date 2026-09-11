import { useMemo, useState, type CSSProperties } from "react";
import { format as d3Format } from "d3-format";
import type { FormatLayer, Widget, WidgetData } from "../../types/dashboard";
import { useTokens } from "../../themes/TemplateProvider";
import { cellStyle, isGradient, resolveScale, toNumber, type ResolvedScale } from "./conditionalFormat";
import { pivotData } from "./pivot";

interface Props {
  widget: Widget;
  data?: WidgetData;
}

type SortDirection = "asc" | "desc";

interface SortState {
  column: string;
  direction: SortDirection;
}

interface TableColumn {
  name: string;
  label: string;
  number?: string; // value display (currency | number | d3-format)
  align?: "left" | "center" | "right"; // text-alignment override (header + body)
  border?: "left" | "right" | "both"; // non-colour vertical group divider on this edge
  format?: FormatLayer[]; // effective layers (own, or the mirrored column's if `like`)
  idx: number; // own data index (drives the displayed value)
  colorIdx: number; // data index whose value drives coloring (own, or `like` source)
}

// Effective column alignment (header + body): an explicit `align` override wins,
// else numbers fall right and text left. Returns the text- and justify- classes.
function alignClasses(align: TableColumn["align"], numeric: boolean) {
  const a = align === "left" || align === "center" || align === "right" ? align : numeric ? "right" : "left";
  return {
    text: a === "center" ? "text-center" : a === "right" ? "text-right" : "text-left",
    justify: a === "center" ? "justify-center" : a === "right" ? "justify-end" : "justify-start",
  };
}

export function TableWidget({ widget, data }: Props) {
  const [sort, setSort] = useState<SortState | null>(null);
  const tokens = useTokens();

  // A pivot reshapes the result client-side; the rest of the table renders the
  // reshaped `effData` exactly as it would a plain result set.
  const pivot = useMemo(() => {
    if (!widget.pivot || !data?.columns) return null;
    return pivotData(data.columns.map((c) => c.name), data.rows ?? [], widget.pivot);
  }, [widget.pivot, data]);
  const effData: WidgetData | undefined = useMemo(
    () => (pivot ? { columns: pivot.columns.map((name) => ({ name })), rows: pivot.rows } : data),
    [pivot, data],
  );

  const columns: TableColumn[] = useMemo(() => {
    if (!effData?.columns) return [];

    const meta = new Map((widget.columns ?? []).map((c) => [c.name, c]));
    const raw =
      pivot || !widget.columns?.length
        ? effData.columns.map((col, idx) => {
            const m = meta.get(col.name);
            return {
              name: col.name,
              label: m?.label || col.name,
              number: m?.number,
              align: m?.align,
              border: m?.border,
              like: m?.like,
              hidden: m?.hidden ?? false,
              format: m?.format,
              idx,
            };
          })
        : widget.columns.map((col) => ({
            name: col.name,
            label: col.label || col.name,
            number: col.number,
            align: col.align,
            border: col.border,
            like: col.like,
            hidden: col.hidden ?? false,
            format: col.format,
            idx: effData.columns.findIndex((c) => c.name === col.name),
          }));

    // `like`: adopt the source column's coloring driven by its per-row value,
    // keeping own `number`. Followed transitively (cycle-guarded) to the terminal source.
    const byName = new Map(raw.map((c) => [c.name, c]));
    const likeSource = (col: (typeof raw)[number]) => {
      let src = col.like ? byName.get(col.like) : undefined;
      const seen = new Set([col.name]);
      while (src && src.like && !seen.has(src.name)) {
        seen.add(src.name);
        src = byName.get(src.like);
      }
      // Exited on a cycle (src still points at a `like` column) → no valid source.
      return src?.like ? undefined : src;
    };

    return raw
      .map((c) => {
        const src = c.like ? likeSource(c) : undefined;
        if (src) {
          return { ...c, format: src.format, colorIdx: src.idx };
        }
        return { ...c, colorIdx: c.idx };
      })
      .filter((c) => !c.hidden);
  }, [widget.columns, effData?.columns]);

  // Per-column `border: left|right|both` group-divider classes, de-duping an
  // adjacent right+left pair into one line (border-separate would draw two).
  // Plain tables only — pivots reject `border` (see validator).
  const borderClasses = useMemo(() => {
    if (pivot) return columns.map(() => "");
    const want = columns.map((c) => ({
      left: c.border === "left" || c.border === "both",
      right: c.border === "right" || c.border === "both",
    }));
    for (let i = 0; i < want.length - 1; i++) {
      if (want[i].right && want[i + 1].left) want[i].right = false;
    }
    return want.map((w) =>
      [w.left ? "border-l-2 border-[var(--dac-border)]" : "", w.right ? "border-r-2 border-[var(--dac-border)]" : ""]
        .filter(Boolean)
        .join(" "),
    );
  }, [columns, pivot]);

  const rows = effData?.rows ?? [];

  // All data columns by name → index, so cross-column rules can reference any
  // column (even ones not shown).
  const dataIndex = useMemo(() => {
    const m = new Map<string, number>();
    effData?.columns.forEach((c, i) => m.set(c.name, i));
    return m;
  }, [effData?.columns]);

  const sortedRows = useMemo(() => {
    // A pivot keeps its own order (row order carries subtotal/total structure and
    // per-row gradient scales); never apply a leftover sort from a prior table.
    if (pivot || !sort || rows.length === 0) return rows;

    const col = columns.find((c) => c.name === sort.column);
    if (!col || col.idx < 0) return rows;

    return sortRows(rows, col.idx, sort.direction, col.number != null);
  }, [pivot, columns, rows, sort]);

  // Resolve gradient scales per column against the full (unsorted) value range,
  // one entry per layer (null for non-gradient layers) so cell colors stay stable
  // regardless of the active sort.
  const scales = useMemo(() => {
    const map = new Map<string, (ResolvedScale | null)[]>();
    for (const col of columns) {
      if (!col.format || col.colorIdx < 0) continue;
      const values: number[] = [];
      for (const row of rows) {
        const n = toNumber(row[col.colorIdx]);
        if (n !== null) values.push(n);
      }
      map.set(
        col.name,
        col.format.map((layer) => (isGradient(layer) ? resolveScale(layer, values, tokens) : null)),
      );
    }
    return map;
  }, [columns, rows, tokens]);

  // Compile each column's d3-format spec once (currency/number are handled
  // separately). Invalid specs are skipped so cells fall back to raw text.
  const numberFormatters = useMemo(() => {
    const m = new Map<string, (n: number) => string>();
    for (const col of columns) {
      const fmt = col.number;
      if (!fmt || fmt === "currency" || fmt === "number") continue;
      try {
        m.set(col.name, d3Format(fmt));
      } catch {
        // Invalid d3-format spec: leave unset; formatCell renders raw text.
      }
    }
    return m;
  }, [columns]);

  // Per pivot value: its `format` layers plus, per gradient layer, the resolved
  // scale(s). A layer's `scaleBy` picks the domain: 'all' (default, one scale over
  // the whole grid), 'row' (per row), or 'column' (per leaf column).
  const pivotValueFormats = useMemo(() => {
    if (!pivot) return null;
    const values = widget.pivot?.values ?? [];
    type LayerScales =
      | { by: "all"; scale: ResolvedScale | null }
      | { by: "row" | "column"; m: Record<number, ResolvedScale | null> }
      | null;
    const out = new Map<number, { format: FormatLayer[]; layerScales: LayerScales[] }>();
    const mapScales = (layer: FormatLayer, groups: Record<number, number[]>) => {
      const m: Record<number, ResolvedScale | null> = {};
      for (const k in groups) m[k] = resolveScale(layer, groups[k], tokens);
      return m;
    };
    values.forEach((v, vi) => {
      const fmt = Array.isArray(v.format) ? v.format : null;
      if (!fmt || !fmt.length) return;
      const all: number[] = [];
      const byRow: Record<number, number[]> = {};
      const byCol: Record<number, number[]> = {};
      pivot.rows.forEach((row, r) => {
        if (pivot.rowKinds[r] && pivot.rowKinds[r] !== "leaf") return;
        row.forEach((cell, c) => {
          if (pivot.columnKinds[c] !== "leaf" || pivot.columnValues[c] !== vi) return;
          const n = toNumber(cell);
          if (n === null) return;
          all.push(n);
          (byRow[r] ??= []).push(n);
          (byCol[c] ??= []).push(n);
        });
      });
      const layerScales: LayerScales[] = fmt.map((layer) => {
        if (!isGradient(layer)) return null;
        if (layer.scaleBy === "row") return { by: "row", m: mapScales(layer, byRow) };
        if (layer.scaleBy === "column") return { by: "column", m: mapScales(layer, byCol) };
        return { by: "all", scale: resolveScale(layer, all, tokens) };
      });
      out.set(vi, { format: fmt, layerScales });
    });
    return out;
  }, [pivot, widget.pivot, tokens]);

  if (!rows.length) {
    return <div className="text-[var(--dac-text-muted)] text-xs py-4 text-center">No data</div>;
  }

  const handleHeaderClick = (columnName: string) => {
    if (pivot) return; // pivots use their own order; sort would break rowKinds alignment
    setSort((current) => nextSortState(current, columnName));
  };

  // Structural total emphasis for pivots (never label-based, so data named
  // "… Total" isn't mistaken for a total).
  const colIsTotal = (idx: number) => pivot?.columnKinds[idx] === "grandtotal";
  const rowKind = (i: number) => pivot?.rowKinds[i];

  // Apply a pivot value's `format` layers (first match wins) to its leaf cells.
  const pivotCellStyle = (rIdx: number, dataIdx: number, value: unknown): CSSProperties | undefined => {
    if (!pivotValueFormats || !pivot) return undefined;
    if (pivot.columnKinds[dataIdx] !== "leaf") return undefined;
    const rk = pivot.rowKinds[rIdx];
    if (rk && rk !== "leaf") return undefined;
    const vi = pivot.columnValues[dataIdx];
    const entry = vi == null ? undefined : pivotValueFormats.get(vi);
    if (!entry) return undefined;
    const scales = entry.layerScales.map((ls) => {
      if (!ls) return null;
      if (ls.by === "all") return ls.scale;
      return ls.m[ls.by === "row" ? rIdx : dataIdx] ?? null;
    });
    const style = cellStyle(entry.format, scales, value, tokens, () => undefined);
    return Object.keys(style).length ? style : undefined;
  };

  return (
    <div className="overflow-x-auto">
      <table className="w-full text-[13px] min-w-[400px] border-separate [border-spacing:1px_1px]">
        <thead>
          <tr className="bg-[var(--dac-surface)]">
            {columns.map((col, ci) => {
              const numeric = col.number != null;
              const active = sort?.column === col.name;
              const alignCls = alignClasses(col.align, numeric);
              return (
                <th
                  key={col.name}
                  aria-sort={
                    active
                      ? sort!.direction === "asc"
                        ? "ascending"
                        : "descending"
                      : "none"
                  }
                  className={`py-0 px-0 whitespace-nowrap ${alignCls.text} ${borderClasses[ci]}`}
                >
                  <button
                    type="button"
                    onClick={() => handleHeaderClick(col.name)}
                    className={`group w-full flex items-center gap-1 py-2 px-4 text-[10px] font-semibold uppercase tracking-wider text-[var(--dac-text-muted)] hover:text-[var(--dac-text-primary)] transition-colors duration-75 border-0 bg-transparent ${alignCls.justify} ${active ? "text-[var(--dac-text-primary)]" : ""} ${pivot ? "cursor-default" : "cursor-pointer"} ${colIsTotal(col.idx) ? "!font-bold" : ""}`}
                  >
                    <span>{col.label}</span>
                    <SortIndicator direction={active ? sort!.direction : null} />
                  </button>
                </th>
              );
            })}
          </tr>
        </thead>
        <tbody>
          {sortedRows.map((row, i) => {
            const kind = rowKind(i);
            const totalRow = kind === "grandtotal";
            const subtotalRow = kind === "subtotal";
            return (
            <tr
              key={i}
              className={`transition-colors duration-75 ${totalRow ? "bg-[var(--dac-surface)]" : subtotalRow ? "bg-[var(--dac-surface)]/60" : "hover:bg-[var(--dac-surface)]"}`}
            >
              {columns.map((col, ci) => {
                const numeric = col.number != null;
                const alignCls = alignClasses(col.align, numeric);
                const raw = col.idx >= 0 ? row[col.idx] : null; // displayed value (own column)
                const style: CSSProperties = numeric ? { fontFamily: '"Geist Mono", monospace' } : {};
                if (pivot) {
                  // Pivots colour by each value's own gradient; no per-column CF.
                  const hm = pivotCellStyle(i, col.idx, raw);
                  if (hm) Object.assign(style, hm);
                } else if (col.format) {
                  const lookup = (name: string) => {
                    const idx = dataIndex.get(name);
                    return idx === undefined ? undefined : row[idx];
                  };
                  // Coloring reads the color-source column (own, or `like` source).
                  const colorRaw = col.colorIdx >= 0 ? row[col.colorIdx] : null;
                  Object.assign(style, cellStyle(col.format, scales.get(col.name) ?? [], colorRaw, tokens, lookup));
                }
                return (
                  <td
                    key={col.name}
                    className={`py-1.5 px-4 whitespace-nowrap align-middle rounded-none ${alignCls.text} ${
                      numeric ? "tabular-nums text-[12px]" : ""
                    } ${totalRow || colIsTotal(col.idx) ? "font-bold" : ""} ${borderClasses[ci]}`}
                    style={Object.keys(style).length ? style : undefined}
                  >
                    {formatCell(raw, col.number, numberFormatters.get(col.name))}
                  </td>
                );
              })}
            </tr>
            );
          })}
        </tbody>
      </table>
    </div>
  );
}

function SortIndicator({ direction }: { direction: SortDirection | null }) {
  return (
    <span
      className={`inline-flex flex-col leading-none shrink-0 ${
        direction ? "opacity-100" : "opacity-0 group-hover:opacity-40"
      }`}
      aria-hidden
    >
      <span className={direction === "asc" ? "text-[var(--dac-accent)]" : "text-[var(--dac-text-muted)] opacity-40"}>
        ▲
      </span>
      <span className={`-mt-1 ${direction === "desc" ? "text-[var(--dac-accent)]" : "text-[var(--dac-text-muted)] opacity-40"}`}>
        ▼
      </span>
    </span>
  );
}

function nextSortState(current: SortState | null, column: string): SortState | null {
  if (!current || current.column !== column) {
    return { column, direction: "asc" };
  }
  if (current.direction === "asc") {
    return { column, direction: "desc" };
  }
  return null;
}

function sortRows(
  rows: unknown[][],
  colIdx: number,
  direction: SortDirection,
  numeric: boolean,
): unknown[][] {
  const indexed = rows.map((row, i) => ({ row, i }));
  indexed.sort((a, b) => {
    const cmp = compareValues(a.row[colIdx], b.row[colIdx], numeric);
    if (cmp !== 0) {
      return direction === "asc" ? cmp : -cmp;
    }
    return a.i - b.i;
  });
  return indexed.map(({ row }) => row);
}

function compareValues(a: unknown, b: unknown, numeric: boolean): number {
  if (a == null && b == null) return 0;
  if (a == null) return 1;
  if (b == null) return -1;

  if (numeric) {
    const na = Number(a);
    const nb = Number(b);
    if (!isNaN(na) && !isNaN(nb)) {
      return na - nb;
    }
  }

  return String(a).localeCompare(String(b), undefined, { numeric: true, sensitivity: "base" });
}

function formatCell(value: unknown, format?: string, d3fmt?: (n: number) => string): string {
  if (value === null || value === undefined) return "—";
  if (format === "currency") {
    const num = Number(value);
    return isNaN(num) ? String(value) : `$${num.toLocaleString(undefined, { minimumFractionDigits: 2 })}`;
  }
  if (format === "number") {
    const num = Number(value);
    return isNaN(num) ? String(value) : num.toLocaleString();
  }
  if (d3fmt) {
    // d3-format spec (e.g. "$,.2f", ".0%"); applies only to finite numbers.
    const num = Number(value);
    if (Number.isFinite(num)) return d3fmt(num);
  }
  const s = String(value);
  const isoMatch = s.match(/^(\d{4})-(\d{2})-(\d{2})T/);
  if (isoMatch) {
    return `${isoMatch[1]}-${isoMatch[2]}-${isoMatch[3]}`;
  }
  return s;
}
