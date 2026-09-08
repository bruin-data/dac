import { useMemo, useState, type CSSProperties } from "react";
import { format as d3Format } from "d3-format";
import type { FormatLayer, Widget, WidgetData } from "../../types/dashboard";
import { useTokens } from "../../themes/TemplateProvider";
import { cellColor, cellStyle, isGradient, resolveScale, toNumber, type ResolvedScale } from "./conditionalFormat";
import { pivotData } from "./pivot";

interface Props {
  widget: Widget;
  data?: WidgetData;
}

// Cohort heatmap gradient (light red → amber → green) over the value cells.
const HEATMAP_COLORS = ["#FECACA", "#FEF08A", "#BBF7D0"];

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

  const rows = effData?.rows ?? [];

  // All data columns by name → index, so cross-column rules can reference any
  // column (even ones not shown).
  const dataIndex = useMemo(() => {
    const m = new Map<string, number>();
    effData?.columns.forEach((c, i) => m.set(c.name, i));
    return m;
  }, [effData?.columns]);

  const sortedRows = useMemo(() => {
    if (!sort || rows.length === 0) return rows;

    const col = columns.find((c) => c.name === sort.column);
    if (!col || col.idx < 0) return rows;

    return sortRows(rows, col.idx, sort.direction, col.number != null);
  }, [columns, rows, sort]);

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

  // Heatmap scale over the pivot's leaf value cells (totals/labels excluded).
  const heatmapScale = useMemo(() => {
    if (!pivot || !widget.pivot?.heatmap) return null;
    const vals: number[] = [];
    pivot.rows.forEach((row, r) => {
      if (pivot.rowKinds[r] && pivot.rowKinds[r] !== "leaf") return;
      row.forEach((v, c) => { if (pivot.columnKinds[c] === "leaf") { const n = toNumber(v); if (n !== null) vals.push(n); } });
    });
    return vals.length ? resolveScale({ backgroundColor: HEATMAP_COLORS } as FormatLayer, vals, tokens) : null;
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

  // Cohort-style heatmap: one red→amber→green gradient over the leaf value cells.
  const heatmapStyle = (rIdx: number, dataIdx: number, value: unknown): CSSProperties | undefined => {
    if (!heatmapScale) return undefined;
    const rk = pivot?.rowKinds[rIdx];
    if (pivot?.columnKinds[dataIdx] !== "leaf" || (rk && rk !== "leaf")) return undefined;
    const n = toNumber(value);
    const hit = n === null ? undefined : cellColor(heatmapScale, n);
    return hit ? { backgroundColor: hit.background } : undefined;
  };

  return (
    <div className="overflow-x-auto">
      <table className="w-full text-[13px] min-w-[400px] border-separate [border-spacing:1px_1px]">
        <thead>
          <tr className="bg-[var(--dac-surface)]">
            {columns.map((col) => {
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
                  className={`py-0 px-0 whitespace-nowrap ${alignCls.text}`}
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
              {columns.map((col) => {
                const numeric = col.number != null;
                const alignCls = alignClasses(col.align, numeric);
                const raw = col.idx >= 0 ? row[col.idx] : null; // displayed value (own column)
                const style: CSSProperties = numeric ? { fontFamily: '"Geist Mono", monospace' } : {};
                const hm = heatmapStyle(i, col.idx, raw); // under any explicit CF below
                if (hm) Object.assign(style, hm);
                if (col.format) {
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
                    } ${totalRow || colIsTotal(col.idx) ? "font-bold" : ""}`}
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
