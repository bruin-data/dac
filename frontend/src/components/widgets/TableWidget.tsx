import { useEffect, useLayoutEffect, useMemo, useRef, useState, type CSSProperties, type KeyboardEvent as ReactKeyboardEvent, type PointerEvent as ReactPointerEvent } from "react";
import { format as d3Format } from "d3-format";
import type { AxisEncoding, FormatLayer, Widget, WidgetData } from "../../types/dashboard";
import { axisField, buildAxisFormatter } from "../../lib/format";
import { useTokens } from "../../themes/TemplateProvider";
import { cellStyle, isGradient, resolveScale, toNumber, type ResolvedScale } from "./conditionalFormat";
import { pivotData } from "./pivot";
import { SparklineCell } from "./SparklineCell";
import { dashboardImageSrc } from "../../lib/dashboardImage";
import { parseSparklineSeries, type SparklinePoint } from "./sparkline";

interface Props {
  widget: Widget;
  data?: WidgetData;
}

type SortDirection = "asc" | "desc";

const MIN_COLUMN_WIDTH = 80;
const DEFAULT_MAX_COLUMN_WIDTH = 450;

interface SortState {
  column: string;
  direction: SortDirection;
}

interface TableColumn {
  name: string;
  label: string;
  type?: "text" | "image" | "sparkline"; // structured sparkline data or ordinary cell rendering
  number?: string; // value display (currency | number | d3-format)
  x?: AxisEncoding; // sparkline point x key/type/format
  y?: AxisEncoding; // sparkline point y key/type/format/domain
  align?: "left" | "center" | "right"; // text-alignment override (header + body)
  border?: "left" | "right" | "both"; // non-colour vertical group border on this edge
  frozen?: boolean; // freeze to the left; frozen columns render first
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
  const [columnWidths, setColumnWidths] = useState<Record<string, number>>({});
  const [autoColumnWidths, setAutoColumnWidths] = useState<Record<string, number>>({});
  const [columnMaxWidths, setColumnMaxWidths] = useState<Record<string, number>>({});
  const [copiedCellKey, setCopiedCellKey] = useState<string | null>(null);
  const resizeRef = useRef<{ name: string; startX: number; startWidth: number; maxWidth: number } | null>(null);
  const copiedCellTimerRef = useRef<number | null>(null);
  const tokens = useTokens();
  const [pinOverrides, setPinOverrides] = useState(() => new Map<string, boolean>());

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
              type: m?.type,
              number: m?.number,
              x: m?.x,
              y: m?.y,
              align: m?.align,
              border: m?.border,
              like: m?.like,
              hidden: m?.hidden ?? false,
              frozen: !pivot && (pinOverrides.get(col.name) ?? m?.frozen ?? false),
              format: m?.format,
              idx,
            };
          })
        : widget.columns.map((col) => ({
            name: col.name,
            label: col.label || col.name,
            type: col.type,
            number: col.number,
            x: col.x,
            y: col.y,
            align: col.align,
            border: col.border,
            like: col.like,
            hidden: col.hidden ?? false,
            frozen: !pivot && (pinOverrides.get(col.name) ?? col.frozen ?? false),
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

    const resolved = raw
      .map((c) => {
        const src = c.like ? likeSource(c) : undefined;
        if (src) {
          return { ...c, format: src.format, colorIdx: src.idx };
        }
        return { ...c, colorIdx: c.idx };
      })
      .filter((c) => !c.hidden);

    // Frozen columns render first, in listed order; not supported on pivots.
    if (pivot) return resolved;
    const frozen = resolved.filter((c) => c.frozen);
    return frozen.length ? [...frozen, ...resolved.filter((c) => !c.frozen)] : resolved;
  }, [widget.columns, effData?.columns, pivot, pinOverrides]);

  const togglePin = (name: string) => {
    const frozenByDefault = !pivot && !!widget.columns?.find((col) => col.name === name)?.frozen;
    setPinOverrides((current) => {
      const pinned = !(current.get(name) ?? frozenByDefault);
      const next = new Map(current);
      if (pinned === frozenByDefault) next.delete(name);
      else next.set(name, pinned);
      return next;
    });
  };

  // Per-column `border: left|right|both` group-border classes, de-duping an
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

  const rows = useMemo(() => effData?.rows ?? [], [effData?.rows]);
  // Parse each sparkline cell once; rendering reuses the points by row.
  const sparklineSeries = useMemo(() => {
    const series = new Map<string, { points: Map<unknown[], SparklinePoint[]>; domain?: readonly [number, number] }>();
    for (const col of columns) {
      if (col.type !== "sparkline") continue;
      const xField = axisField(col.x);
      const yField = axisField(col.y);
      const points = new Map<unknown[], SparklinePoint[]>();
      let min = Infinity;
      let max = -Infinity;
      for (const row of rows) {
        const rowPoints = parseSparklineSeries(row[col.idx], xField, yField);
        points.set(row, rowPoints);
        for (const point of rowPoints) {
          min = Math.min(min, point.y);
          max = Math.max(max, point.y);
        }
      }
      series.set(col.name, { points, domain: Number.isFinite(min) && Number.isFinite(max) ? [min, max] : undefined });
    }
    return series;
  }, [columns, rows]);

  // Frozen columns render first and stick to the left; 0 on pivots.
  const frozenCount = useMemo(() => (pivot ? 0 : columns.filter((c) => c.frozen).length), [columns, pivot]);
  const headerRowRef = useRef<HTMLTableRowElement>(null);
  const [frozenOffsets, setFrozenOffsets] = useState<number[]>([]);
  useLayoutEffect(() => {
    if (!frozenCount) {
      setFrozenOffsets([]);
      return;
    }
    let cancelled = false;
    const recompute = () => {
      const ths = headerRowRef.current?.children;
      if (cancelled || !ths) return;
      const offsets: number[] = [];
      let acc = 0;
      for (let i = 0; i < frozenCount; i++) {
        offsets[i] = acc;
        acc += (ths[i] as HTMLElement)?.getBoundingClientRect().width ?? 0; // no gap, so no seam bleeds
      }
      setFrozenOffsets(offsets);
    };
    recompute();
    // Observe each cell, not the row: a w-full table can reflow (e.g. font load)
    // without the row's box changing.
    const ths = headerRowRef.current?.children;
    let ro: ResizeObserver | undefined;
    if (typeof ResizeObserver !== "undefined" && ths) {
      ro = new ResizeObserver(recompute);
      for (let i = 0; i < ths.length; i++) ro.observe(ths[i]);
    }
    document.fonts?.ready.then(recompute).catch(() => {});
    return () => {
      cancelled = true;
      ro?.disconnect();
    };
  }, [frozenCount, columns, rows]);

  // Sticky position for a frozen cell; its opaque background is class-based (below).
  const frozenStyle = (ci: number, header: boolean): CSSProperties | undefined => {
    if (ci >= frozenCount) return undefined;
    return { position: "sticky", left: frozenOffsets[ci] ?? 0, zIndex: header ? 2 : 1 };
  };
  // Opaque background for a frozen cell that still tracks row hover; a
  // conditional-format colour (inline) still wins.
  const frozenBgClass = (ci: number) =>
    ci < frozenCount ? "bg-[var(--dac-background)] group-hover:bg-[var(--dac-surface)]" : "";

  const columnWidthStyle = (col: TableColumn): CSSProperties => {
    const width = columnWidths[col.name] ?? autoColumnWidths[col.name];
    if (width == null) return { maxWidth: DEFAULT_MAX_COLUMN_WIDTH, boxSizing: "border-box" };
    return { width, minWidth: width, maxWidth: width, boxSizing: "border-box" };
  };

  const tableWidthStyle = useMemo<CSSProperties | undefined>(() => {
    const widths = columns.map((col) => columnWidths[col.name] ?? autoColumnWidths[col.name]);
    if (!widths.length || widths.some((width) => width == null)) return undefined;
    const total = widths.reduce((sum, width) => sum + (width ?? 0), 0) + columns.length + 1;
    return { width: total, minWidth: total, tableLayout: "fixed" };
  }, [autoColumnWidths, columns, columnWidths]);

  useLayoutEffect(() => {
    let measureFrame = 0;
    const clearFrame = window.requestAnimationFrame(() => {
      setAutoColumnWidths({});
      setColumnMaxWidths({});
      measureFrame = window.requestAnimationFrame(() => {
        const ths = headerRowRef.current?.children;
        if (!ths) return;
        const widths: Record<string, number> = {};
        columns.forEach((col, index) => {
          widths[col.name] = Math.min(
            DEFAULT_MAX_COLUMN_WIDTH,
            Math.max(MIN_COLUMN_WIDTH, Math.round(ths[index]?.getBoundingClientRect().width || MIN_COLUMN_WIDTH)),
          );
        });
        setAutoColumnWidths(widths);
      });
    });
    return () => {
      window.cancelAnimationFrame(clearFrame);
      window.cancelAnimationFrame(measureFrame);
    };
  }, [columns, effData?.rows]);

  const measureColumnMaxWidth = (col: TableColumn, index: number) => {
    if (columnMaxWidths[col.name] != null) return columnMaxWidths[col.name];
    let maxWidth = DEFAULT_MAX_COLUMN_WIDTH;
    const table = headerRowRef.current?.closest("table");
    const sample = table?.tBodies[0]?.rows[0]?.cells[index]?.querySelector<HTMLElement>("[data-column-value]");
    try {
      const context = document.createElement("canvas").getContext("2d");
      if (context && sample) {
        context.font = window.getComputedStyle(sample).font;
        for (const row of sortedRows) {
          const raw = col.idx >= 0 ? row[col.idx] : null;
          const value = formatCell(raw, col.number, numberFormatters.get(col.name));
          maxWidth = Math.max(maxWidth, Math.ceil(context.measureText(String(value ?? "")).width) + 32);
        }
      }
    } catch { /* keep the default maximum */ }
    return maxWidth;
  };

  const startColumnResize = (col: TableColumn, event: ReactPointerEvent<HTMLSpanElement>) => {
    const th = event.currentTarget.closest("th");
    if (!th) return;
    event.preventDefault();
    event.stopPropagation();
    const index = columns.findIndex((column) => column.name === col.name);
    const maxWidth = measureColumnMaxWidth(col, index);
    setColumnMaxWidths((current) => ({ ...current, [col.name]: maxWidth }));
    const startWidth = Math.min(maxWidth, Math.max(MIN_COLUMN_WIDTH, Math.round(th.getBoundingClientRect().width)));
    setColumnWidths((current) => ({ ...current, [col.name]: startWidth }));
    resizeRef.current = { name: col.name, startX: event.clientX, startWidth, maxWidth };
    event.currentTarget.setPointerCapture(event.pointerId);
  };

  const moveColumnResize = (event: ReactPointerEvent<HTMLSpanElement>) => {
    const resize = resizeRef.current;
    if (!resize) return;
    const width = Math.min(resize.maxWidth, Math.max(MIN_COLUMN_WIDTH, Math.round(resize.startWidth + event.clientX - resize.startX)));
    setColumnWidths((current) => ({ ...current, [resize.name]: width }));
  };

  const finishColumnResize = (event: ReactPointerEvent<HTMLSpanElement>) => {
    if (!resizeRef.current) return;
    resizeRef.current = null;
    if (event.currentTarget.hasPointerCapture(event.pointerId)) event.currentTarget.releasePointerCapture(event.pointerId);
  };

  const resetColumnWidth = (name: string, index: number) => {
    setColumnWidths((current) => {
      const next = { ...current };
      delete next[name];
      return next;
    });
    setAutoColumnWidths((current) => {
      const next = { ...current };
      delete next[name];
      return next;
    });
    window.requestAnimationFrame(() => {
      const th = headerRowRef.current?.children[index];
      if (!th) return;
      const width = Math.min(
        DEFAULT_MAX_COLUMN_WIDTH,
        Math.max(MIN_COLUMN_WIDTH, Math.round(th.getBoundingClientRect().width || MIN_COLUMN_WIDTH)),
      );
      setAutoColumnWidths((current) => ({ ...current, [name]: width }));
    });
  };

  const resizeColumnWithKeyboard = (col: TableColumn, index: number, event: ReactKeyboardEvent<HTMLSpanElement>) => {
    if (!["ArrowLeft", "ArrowRight", "Home", "End"].includes(event.key)) return;
    event.preventDefault();
    event.stopPropagation();
    const maxWidth = measureColumnMaxWidth(col, index);
    setColumnMaxWidths((current) => ({ ...current, [col.name]: maxWidth }));
    const current = columnWidths[col.name] ?? autoColumnWidths[col.name] ?? DEFAULT_MAX_COLUMN_WIDTH;
    const width = event.key === "Home"
      ? MIN_COLUMN_WIDTH
      : event.key === "End"
        ? maxWidth
        : Math.min(maxWidth, Math.max(MIN_COLUMN_WIDTH, current + (event.key === "ArrowRight" ? 10 : -10)));
    setColumnWidths((widths) => ({ ...widths, [col.name]: width }));
  };

  const copyCell = (value: unknown, key: string) => {
    if (value == null) return;
    navigator.clipboard?.writeText(String(value)).then(() => {
      setCopiedCellKey(key);
      if (copiedCellTimerRef.current != null) window.clearTimeout(copiedCellTimerRef.current);
      copiedCellTimerRef.current = window.setTimeout(() => setCopiedCellKey(null), 1200);
    }).catch(() => {});
  };

  const updateCellTooltip = (cell: HTMLElement, value: unknown) => {
    if (cell.scrollWidth > cell.clientWidth && value != null) cell.title = String(value);
    else cell.removeAttribute("title");
  };

  useEffect(() => () => {
    if (copiedCellTimerRef.current != null) window.clearTimeout(copiedCellTimerRef.current);
  }, []);

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
    if (!col || col.idx < 0 || col.type === "sparkline") return rows;

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
    <div className="w-full min-w-0 max-w-full overflow-x-auto">
      <table
        className="inline-table min-w-max text-[13px] border-separate [border-spacing:1px_1px]"
        style={tableWidthStyle}
      >
        <thead>
          <tr ref={headerRowRef} className="bg-[var(--dac-surface)]">
            {columns.map((col, ci) => {
              const numeric = col.number != null;
              const sortable = !pivot && col.type !== "sparkline";
              const active = sortable && sort?.column === col.name;
              const alignCls = alignClasses(col.align, numeric);
              const pinAction = col.frozen ? "Unfreeze" : "Freeze";
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
                  className={`group/header relative py-0 px-0 whitespace-nowrap overflow-hidden ${alignCls.text} ${ci < frozenCount ? "bg-[var(--dac-surface)]" : ""} ${borderClasses[ci]}`}
                  style={{ ...columnWidthStyle(col), ...frozenStyle(ci, true) }}
                >
                  <button
                    type="button"
                    onClick={sortable ? () => handleHeaderClick(col.name) : undefined}
                    className={`group w-full min-w-0 flex items-center gap-1 py-2 pl-4 ${pivot ? "pr-4" : "pr-10"} text-[10px] font-semibold uppercase tracking-wider text-[var(--dac-text-muted)] hover:text-[var(--dac-text-primary)] transition-colors duration-75 border-0 bg-transparent ${alignCls.justify} ${active ? "text-[var(--dac-text-primary)]" : ""} ${sortable ? "cursor-pointer" : "cursor-default"} ${colIsTotal(col.idx) ? "!font-bold" : ""}`}
                    style={columnWidthStyle(col)}
                  >
                    <span
                      className="truncate"
                      onMouseEnter={(event) => updateCellTooltip(event.currentTarget, col.label)}
                    >
                      {col.label}
                    </span>
                    <SortIndicator direction={active ? sort!.direction : null} />
                  </button>
                  {!pivot && (
                    <>
                      <button
                        type="button"
                        onClick={() => togglePin(col.name)}
                        title={`${pinAction} ${col.label} column`}
                        aria-label={`${pinAction} ${col.label} column`}
                        aria-pressed={col.frozen}
                        className={`absolute right-3 top-1/2 z-20 -translate-y-1/2 transition-opacity focus:outline-none focus:opacity-100 ${
                          col.frozen
                            ? "text-[var(--dac-accent)] opacity-100"
                            : "text-[var(--dac-text-muted)] opacity-100 [@media(hover:hover)]:opacity-0 group-hover/header:opacity-100 focus-visible:opacity-100 hover:text-[var(--dac-text-primary)]"
                        }`}
                      >
                        <PinIcon />
                      </button>
                      <span
                        role="separator"
                        tabIndex={0}
                        aria-orientation="vertical"
                        aria-label={`Resize ${col.label} column`}
                        aria-valuemin={MIN_COLUMN_WIDTH}
                        aria-valuemax={columnMaxWidths[col.name] ?? DEFAULT_MAX_COLUMN_WIDTH}
                        aria-valuenow={columnWidths[col.name] ?? autoColumnWidths[col.name] ?? DEFAULT_MAX_COLUMN_WIDTH}
                        className="absolute inset-y-0 right-0 z-10 w-3 cursor-col-resize touch-none opacity-0 hover:opacity-100 focus:opacity-100 after:absolute after:inset-y-1 after:right-0 after:w-0.5 after:rounded-full after:bg-[var(--dac-accent)]"
                        onPointerDown={(event) => startColumnResize(col, event)}
                        onPointerMove={moveColumnResize}
                        onPointerUp={finishColumnResize}
                        onPointerCancel={finishColumnResize}
                        onDoubleClick={(event) => {
                          event.stopPropagation();
                          resetColumnWidth(col.name, ci);
                        }}
                        onKeyDown={(event) => resizeColumnWithKeyboard(col, ci, event)}
                        onClick={(event) => event.stopPropagation()}
                      />
                    </>
                  )}
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
              className={`group transition-colors duration-75 ${totalRow ? "bg-[var(--dac-surface)]" : subtotalRow ? "bg-[var(--dac-surface)]/60" : "hover:bg-[var(--dac-surface)]"}`}
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
                // Sticky position merges under conditional-format style so the cell colour wins.
                const frozen = frozenStyle(ci, false);
                const tdStyle = { ...columnWidthStyle(col), ...frozen, ...style };
                const displayValue = formatCell(raw, col.number, numberFormatters.get(col.name));
                return (
                  <td
                    key={col.name}
                    className={`relative whitespace-nowrap align-middle rounded-none ${alignCls.text} ${
                      numeric ? "tabular-nums text-[12px]" : ""
                    } ${totalRow || colIsTotal(col.idx) ? "font-bold" : ""} ${frozenBgClass(ci)} ${borderClasses[ci]}`}
                    style={tdStyle}
                  >
                    <div
                      data-column-value={col.type === undefined || col.type === "text" ? "" : undefined}
                      className="relative box-border w-full min-w-0 py-1.5 px-4 whitespace-nowrap overflow-hidden text-ellipsis"
                      style={columnWidthStyle(col)}
                      onMouseEnter={(event) => (col.type === undefined || col.type === "text") && updateCellTooltip(event.currentTarget, displayValue)}
                      onDoubleClick={
                        col.type === undefined || col.type === "text"
                          ? (event) => {
                              event.preventDefault();
                              copyCell(displayValue, `${i}:${col.idx}`);
                            }
                          : undefined
                      }
                    >
                      {col.type === "image" && !pivot && raw ? (
                        <TableImage key={String(raw)} value={raw} fallback={displayValue} />
                      ) : col.type === "sparkline" && !pivot ? (
                        <SparklineCell
                          points={sparklineSeries.get(col.name)?.points.get(row) ?? []}
                          yDomain={sparklineSeries.get(col.name)?.domain}
                          beginAtZero={col.y?.beginAtZero}
                          formatX={buildAxisFormatter(col.x, String)}
                          formatY={buildAxisFormatter(
                            col.y,
                            (value) => formatCell(value, col.number, numberFormatters.get(col.name)),
                          )}
                        />
                      ) : (
                        displayValue
                      )}
                      {copiedCellKey === `${i}:${col.idx}` && (
                        <span
                          aria-label="Copied"
                          data-dac-export-control
                          className="pointer-events-none absolute right-2 top-1/2 flex h-4 w-4 -translate-y-1/2 items-center justify-center rounded-full bg-blue-50 text-[11px] font-semibold text-blue-600"
                        >
                          ✓
                        </span>
                      )}
                    </div>
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

function TableImage({ value, fallback }: { value: unknown; fallback: string }) {
  const [failed, setFailed] = useState(false);
  const src = dashboardImageSrc(value);

  if (!src || failed) return fallback;

  return (
    <img
      src={src}
      alt=""
      loading="lazy"
      referrerPolicy="no-referrer"
      className="h-10 w-auto max-w-[120px] rounded object-cover"
      onError={() => setFailed(true)}
    />
  );
}

function PinIcon() {
  return (
    <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden>
      <path d="M12 17v5" />
      <path d="M5 17h14" />
      <path d="M6 3h12l-2 8 3 3H5l3-3-2-8Z" />
    </svg>
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
