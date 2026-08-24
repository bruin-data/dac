import { useContext, useEffect, useMemo, useRef, useState } from "react";
import type { ReactNode } from "react";
import {
  LineChart, Line, BarChart, Bar, AreaChart, Area, PieChart, Pie, Cell,
  ScatterChart, Scatter, ZAxis,
  ComposedChart, Sankey,
  Treemap,
  RadarChart, Radar, PolarGrid, PolarAngleAxis, PolarRadiusAxis,
  XAxis, YAxis, CartesianGrid, Tooltip, ResponsiveContainer, Legend,
  ReferenceLine, ReferenceArea, ErrorBar,
} from "recharts";
import type { TreemapNode } from "recharts";
import type { ColorScale, Widget, WidgetData } from "../../types/dashboard";
import { axisField, axisFields, buildAxisFormatter, valueField } from "../../lib/format";
import { useTokens } from "../../themes/TemplateProvider";
import { cellColor, resolveScale } from "./conditionalFormat";
import { RowHeightContext } from "../../themes/RowContext";
import { VegaLiteChart } from "./VegaLiteChart";

const DEFAULT_CHART_HEIGHT = 240;
// Approx pixels consumed by the widget frame's title/padding above the chart.
const FRAME_OVERHEAD = 60;
// Keep auto-sized value axes from consuming the plot on narrow or dual-axis charts.
const MAX_Y_AXIS_TICK_WIDTH = 96;

// CI / annotation helpers (mirror the cloud ApexCharts renderer) ───────────
const CI_BAND_OPACITY = 0.18;
const REF_LINE_COLOR = "#6B7280";
const REF_BAND_COLOR = "#F59E0B";
const FOREST_SIG = "#3B82F6";
const FOREST_NOSIG = "#9CA3AF";

// Resolve a yMin/yMax bound (a column name, or a per-series {seriesField: column}
// map) to the bound column for a given series.
function boundColumn(bound: string | Record<string, string> | undefined, field: string): string | undefined {
  if (typeof bound === "string") return bound || undefined;
  if (bound && typeof bound === "object") return bound[field];
  return undefined;
}

// Reference guide lines (refLines) and shaded bands (refBands) as Recharts
// <ReferenceLine>/<ReferenceArea> elements; dropped into any cartesian chart.
// yAxisId binds them to an explicit axis on dual-axis charts (undefined = default).
function renderRefs(widget: Widget, yAxisId?: "left" | "right"): ReactNode[] {
  const els: ReactNode[] = [];
  (widget.refLines ?? []).forEach((l, i) => {
    const pos = l.axis === "y" ? { y: l.value } : { x: l.value };
    els.push(
      <ReferenceLine
        key={`rl-${i}`}
        yAxisId={yAxisId}
        {...pos}
        stroke={l.color ?? REF_LINE_COLOR}
        strokeDasharray="4 4"
        label={l.label ? { value: l.label, position: "insideTopRight", fontSize: 10, fill: REF_LINE_COLOR } : undefined}
      />,
    );
  });
  (widget.refBands ?? []).forEach((b, i) => {
    const pos = b.axis === "y" ? { y1: b.from, y2: b.to } : { x1: b.from, x2: b.to };
    els.push(
      <ReferenceArea
        key={`rb-${i}`}
        yAxisId={yAxisId}
        {...pos}
        fill={b.color ?? REF_BAND_COLOR}
        fillOpacity={0.12}
        stroke={b.color ?? REF_BAND_COLOR}
        strokeOpacity={0.4}
        strokeDasharray="4 4"
        label={b.label ? { value: b.label, position: "insideTop", fontSize: 10, fill: REF_LINE_COLOR } : undefined}
      />,
    );
  });
  return els;
}

interface Props {
  widget: Widget;
  data?: WidgetData;
}

function toChartData(data: WidgetData): Record<string, unknown>[] {
  return data.rows.map((row) => {
    const obj: Record<string, unknown> = {};
    data.columns.forEach((col, i) => {
      obj[col.name] = row[i];
    });
    return obj;
  });
}

function formatAxisTick(value: unknown): string {
  const s = String(value);
  const isoMatch = s.match(/^(\d{4})-(\d{2})-(\d{2})T/);
  if (isoMatch) {
    const [, , m, d] = isoMatch;
    const months = ["Jan","Feb","Mar","Apr","May","Jun","Jul","Aug","Sep","Oct","Nov","Dec"];
    const monthIdx = parseInt(m, 10) - 1;
    const day = parseInt(d, 10);
    if (day === 1) return months[monthIdx];
    return `${months[monthIdx]} ${day}`;
  }
  const monthMatch = s.match(/^(\d{4})-(\d{2})$/);
  if (monthMatch) {
    const months = ["Jan","Feb","Mar","Apr","May","Jun","Jul","Aug","Sep","Oct","Nov","Dec"];
    return months[parseInt(monthMatch[2], 10) - 1] ?? s;
  }
  const dateMatch = s.match(/^(\d{4})-(\d{2})-(\d{2})$/);
  if (dateMatch) {
    const months = ["Jan","Feb","Mar","Apr","May","Jun","Jul","Aug","Sep","Oct","Nov","Dec"];
    return `${months[parseInt(dateMatch[2], 10) - 1]} ${parseInt(dateMatch[3], 10)}`;
  }
  return s;
}

function formatTooltipValue(value: unknown): string {
  const num = Number(value);
  if (!isNaN(num)) return num.toLocaleString();
  return String(value);
}

function formatYTick(value: unknown): string {
  const num = Number(value);
  if (isNaN(num)) return String(value);
  if (Math.abs(num) >= 1_000_000) return `${(num / 1_000_000).toFixed(1)}M`;
  if (Math.abs(num) >= 1_000) return `${(num / 1_000).toFixed(0)}k`;
  return num.toLocaleString();
}

const CHART_COLORS = [
  "chart-1", "chart-2", "chart-3", "chart-4",
  "chart-5", "chart-6", "chart-7", "chart-8",
];

const AXIS_STYLE = { fontSize: 11, fontFamily: '"Geist", system-ui' };

interface TooltipPayloadEntry {
  color?: string;
  fill?: string;
  name?: string;
  dataKey?: string;
  value?: unknown;
}

function CustomTooltip({ active, payload, label, labelFormatter = formatAxisTick, valueFormatter = formatTooltipValue, valueFormatterFor }: {
  active?: boolean;
  payload?: TooltipPayloadEntry[];
  label?: unknown;
  labelFormatter?: (val: unknown) => string;
  valueFormatter?: (val: unknown) => string;
  // Per-series formatter keyed by dataKey; used for dual-axis charts so each
  // row formats against its own axis. Falls back to valueFormatter.
  valueFormatterFor?: (dataKey: unknown) => (val: unknown) => string;
}) {
  if (!active || !payload?.length) return null;
  return (
    <div className="dac-tooltip">
      <div className="dac-tooltip-label">{labelFormatter(label)}</div>
      {payload.map((p, i) => (
        <div key={i} className="dac-tooltip-row">
          <span className="dac-tooltip-dot" style={{ background: p.color ?? p.fill }} />
          <span className="dac-tooltip-name">{p.name ?? p.dataKey}</span>
          <span className="dac-tooltip-value">{(valueFormatterFor?.(p.dataKey) ?? valueFormatter)(p.value)}</span>
        </div>
      ))}
    </div>
  );
}

// --- Color (long-format) pivot ---

/**
 * Pivot long-format rows (one row per x/category pair) into wide rows
 * (one row per x, one column per category) so Recharts can render one
 * series per category.
 */
function pivotByColor(
  rawData: Record<string, unknown>[],
  xKey: string,
  yKey: string,
  colorKey: string,
): { rows: Record<string, unknown>[]; series: string[] } {
  const series: string[] = [];
  const seen = new Set<string>();
  const byX = new Map<string, Record<string, unknown>>();
  for (const d of rawData) {
    const x = String(d[xKey]);
    const cat = String(d[colorKey]);
    if (!seen.has(cat)) {
      seen.add(cat);
      series.push(cat);
    }
    let row = byX.get(x);
    if (!row) {
      row = { [xKey]: d[xKey] };
      byX.set(x, row);
    }
    row[cat] = d[yKey];
  }
  return { rows: Array.from(byX.values()), series };
}

/** Convert each row's series values to percentages of the row total (0–100). */
function normalizeRows(
  rows: Record<string, unknown>[],
  series: string[],
): Record<string, unknown>[] {
  return rows.map((row) => {
    const total = series.reduce((sum, s) => sum + (Number(row[s]) || 0), 0);
    if (total === 0) return row;
    const out = { ...row };
    for (const s of series) {
      out[s] = ((Number(row[s]) || 0) / total) * 100;
    }
    return out;
  });
}

function formatPercentTick(value: unknown): string {
  const num = Number(value);
  if (isNaN(num)) return String(value);
  return `${Math.round(num)}%`;
}

// --- Histogram data transformation ---

function buildHistogramData(
  rawData: Record<string, unknown>[],
  column: string,
  binCount: number,
): { bin: string; count: number }[] {
  const values = rawData
    .map((d) => Number(d[column]))
    .filter((v) => !isNaN(v));
  if (values.length === 0) return [];

  const min = Math.min(...values);
  const max = Math.max(...values);
  if (min === max) return [{ bin: String(min), count: values.length }];

  const binWidth = (max - min) / binCount;
  const bins: { bin: string; count: number }[] = [];
  for (let i = 0; i < binCount; i++) {
    const lo = min + i * binWidth;
    const hi = lo + binWidth;
    bins.push({
      bin: `${lo.toFixed(1)}–${hi.toFixed(1)}`,
      count: 0,
    });
  }
  for (const v of values) {
    let idx = Math.floor((v - min) / binWidth);
    if (idx >= binCount) idx = binCount - 1;
    bins[idx].count++;
  }
  return bins;
}

// --- Waterfall data transformation ---

function buildWaterfallData(
  rawData: Record<string, unknown>[],
  xKey: string,
  yKey: string,
): { name: string; base: number; value: number; total: number; fill: string }[] {
  let cumulative = 0;
  return rawData.map((d) => {
    const val = Number(d[yKey]) || 0;
    const base = cumulative;
    cumulative += val;
    return {
      name: String(d[xKey]),
      base: val >= 0 ? base : cumulative,
      value: Math.abs(val),
      total: cumulative,
      fill: val >= 0 ? "positive" : "negative",
    };
  });
}

// --- Boxplot data transformation ---

function quantile(sorted: number[], q: number): number {
  const pos = (sorted.length - 1) * q;
  const lo = Math.floor(pos);
  const hi = Math.ceil(pos);
  if (lo === hi) return sorted[lo];
  return sorted[lo] + (sorted[hi] - sorted[lo]) * (pos - lo);
}

function buildBoxplotData(
  rawData: Record<string, unknown>[],
  categoryKey: string,
  valueKey: string,
): Record<string, unknown>[] {
  const groups = new Map<string, number[]>();
  for (const d of rawData) {
    const cat = String(d[categoryKey]);
    const val = Number(d[valueKey]);
    if (isNaN(val)) continue;
    if (!groups.has(cat)) groups.set(cat, []);
    groups.get(cat)!.push(val);
  }

  return Array.from(groups.entries()).map(([cat, values]) => {
    values.sort((a, b) => a - b);
    const min = values[0];
    const q1 = quantile(values, 0.25);
    const median = quantile(values, 0.5);
    const q3 = quantile(values, 0.75);
    const max = values[values.length - 1];
    return {
      category: cat,
      min,
      q1,
      median,
      q3,
      max,
      _q1ToMedian: median - q1,
      _medianToQ3: q3 - median,
    };
  });
}

// --- Sankey data transformation ---

function buildSankeyData(
  rawData: Record<string, unknown>[],
  sourceKey: string,
  targetKey: string,
  valueKey: string,
): { nodes: { name: string }[]; links: { source: number; target: number; value: number }[] } {
  const nodeSet = new Set<string>();
  for (const d of rawData) {
    nodeSet.add(String(d[sourceKey]));
    nodeSet.add(String(d[targetKey]));
  }
  const nodeList = Array.from(nodeSet);
  const nodeIndex = new Map(nodeList.map((n, i) => [n, i]));

  return {
    nodes: nodeList.map((name) => ({ name })),
    links: rawData.map((d) => ({
      source: nodeIndex.get(String(d[sourceKey]))!,
      target: nodeIndex.get(String(d[targetKey]))!,
      value: Number(d[valueKey]) || 0,
    })),
  };
}

// --- Calendar heatmap rendering ---

function SvgTooltip({ x, y, lines }: { x: number; y: number; lines: string[] }) {
  const lineH = 16;
  const padX = 8;
  const padY = 6;
  const w = Math.max(...lines.map((l) => l.length * 6.5)) + padX * 2;
  const h = lines.length * lineH + padY * 2;
  return (
    <g style={{ pointerEvents: "none" }}>
      <rect x={x + 8} y={y - h / 2} width={w} height={h} rx={4}
        fill="var(--dac-surface, #fff)" stroke="var(--dac-border, #e5e7eb)" strokeWidth={1} />
      {lines.map((line, i) => (
        <text key={i} x={x + 8 + padX} y={y - h / 2 + padY + (i + 1) * lineH - 4}
          fill="var(--dac-text-primary, #111)" fontSize={11} fontFamily='"Geist", system-ui'>
          {line}
        </text>
      ))}
    </g>
  );
}

function CalendarHeatmap({
  data,
  dateKey,
  valueKey,
  colors,
  axisColor,
}: {
  data: Record<string, unknown>[];
  dateKey: string;
  valueKey: string;
  colors: string[];
  axisColor: string;
}) {
  const [hover, setHover] = useState<{ x: number; y: number; date: string; value: number } | null>(null);

  const { weeks, maxVal, months } = useMemo(() => {
    const valueMap = new Map<string, number>();
    let maxV = 0;
    for (const d of data) {
      const date = String(d[dateKey]).slice(0, 10);
      const val = Number(d[valueKey]) || 0;
      valueMap.set(date, val);
      if (val > maxV) maxV = val;
    }

    const dates = Array.from(valueMap.keys()).sort();
    if (dates.length === 0) return { weeks: [], maxVal: 0, months: [] };

    const start = new Date(dates[0]);
    const end = new Date(dates[dates.length - 1]);
    start.setDate(start.getDate() - start.getDay());

    const wks: { date: string; value: number; day: number; week: number }[] = [];
    const mos: { label: string; week: number }[] = [];
    let weekIdx = 0;
    let lastMonth = -1;
    const cursor = new Date(start);

    while (cursor <= end) {
      const ds = cursor.toISOString().slice(0, 10);
      const month = cursor.getMonth();
      if (month !== lastMonth) {
        const monthNames = ["Jan","Feb","Mar","Apr","May","Jun","Jul","Aug","Sep","Oct","Nov","Dec"];
        mos.push({ label: monthNames[month], week: weekIdx });
        lastMonth = month;
      }
      wks.push({
        date: ds,
        value: valueMap.get(ds) ?? 0,
        day: cursor.getDay(),
        week: weekIdx,
      });
      cursor.setDate(cursor.getDate() + 1);
      if (cursor.getDay() === 0) weekIdx++;
    }

    return { weeks: wks, maxVal: maxV, months: mos };
  }, [data, dateKey, valueKey]);

  if (weeks.length === 0) return null;

  const cellSize = 11;
  const gap = 2;
  const totalWeeks = (weeks[weeks.length - 1]?.week ?? 0) + 1;
  const width = totalWeeks * (cellSize + gap) + 30;
  const height = 7 * (cellSize + gap) + 20;

  const baseColor = colors[0] ?? "#4338CA";

  return (
    <svg width="100%" viewBox={`0 0 ${width} ${height}`} style={{ maxHeight: 180 }}
      onMouseLeave={() => setHover(null)}>
      {months.map((m, i) => (
        <text key={i} x={30 + m.week * (cellSize + gap)} y={10} fill={axisColor} fontSize={9} fontFamily='"Geist", system-ui'>
          {m.label}
        </text>
      ))}
      {weeks.map((cell, i) => {
        const intensity = maxVal > 0 ? cell.value / maxVal : 0;
        const opacity = cell.value === 0 ? 0.06 : 0.15 + intensity * 0.85;
        const cx = 30 + cell.week * (cellSize + gap);
        const cy = 16 + cell.day * (cellSize + gap);
        return (
          <rect
            key={i}
            x={cx}
            y={cy}
            width={cellSize}
            height={cellSize}
            rx={2}
            fill={baseColor}
            opacity={opacity}
            onMouseEnter={() => setHover({ x: cx + cellSize, y: cy + cellSize / 2, date: cell.date, value: cell.value })}
            onMouseLeave={() => setHover(null)}
          />
        );
      })}
      {hover && (
        <SvgTooltip x={hover.x} y={hover.y} lines={[hover.date, formatTooltipValue(hover.value)]} />
      )}
    </svg>
  );
}

// --- Heatmap rendering ---

const HEATMAP_SHADES = [
  { color: "#DBEAFE", name: "Low" },
  { color: "#93C5FD", name: "Medium" },
  { color: "#3B82F6", name: "High" },
  { color: "#1E3A8A", name: "Very High" },
];

// Ink for a value printed on a shaded cell. getTreemapTextColor's 0.32 threshold
// would put white on #3B82F6 (3.7:1) — at 11px the darker ink wins there (5.3:1).
function heatmapCellInk(fill: string): string {
  return (getHexLuminance(fill) ?? 1) > 0.18 ? "#111827" : "#F9FAFB";
}

function HeatmapChart({
  data,
  xKey,
  yKey,
  valueKey,
  xTitle,
  axisColor,
  showValues,
  valueFmt,
  colorScale,
  tokens,
}: {
  data: Record<string, unknown>[];
  xKey: string;
  yKey: string;
  valueKey: string;
  xTitle?: string;
  axisColor: string;
  showValues?: boolean;
  valueFmt: (val: unknown) => string;
  colorScale?: ColorScale;
  tokens: Record<string, string>;
}) {
  const [hover, setHover] = useState<{ x: number; y: number; xLabel: string; yLabel: string; value: number } | null>(null);

  const { cells, xLabels, yLabels, minVal, maxVal } = useMemo(() => {
    const xs = [...new Set(data.map((d) => String(d[xKey])))];
    const ys = [...new Set(data.map((d) => String(d[yKey])))];
    let maxV = -Infinity;
    let minV = Infinity;
    const cellMap = new Map<string, number>();
    for (const d of data) {
      const val = Number(d[valueKey]) || 0;
      cellMap.set(`${d[xKey]}_${d[yKey]}`, val);
      if (val > maxV) maxV = val;
      if (val < minV) minV = val;
    }
    if (!Number.isFinite(minV)) minV = 0;
    if (!Number.isFinite(maxV)) maxV = 1;
    return { cells: cellMap, xLabels: xs, yLabels: ys, minVal: minV, maxVal: maxV };
  }, [data, xKey, yKey, valueKey]);

  // Discrete 4-bucket scale (equal-width min→max); a continuous opacity ramp made near-equal values indistinguishable.
  const span = maxVal > minVal ? maxVal - minVal : 1;
  const bucketOf = (v: number) => Math.min(3, Math.max(0, Math.floor(((v - minVal) / span) * 4)));

  // A `colorScale` replaces the buckets with the table gradient's continuous ramp,
  // resolved by the same code — the difference is the domain: every cell in the
  // grid, not one column. Scaled once per data/scale change.
  const scale = useMemo(() => {
    if (!colorScale?.backgroundColor?.length) return null;
    return resolveScale(
      { backgroundColor: colorScale.backgroundColor, range: colorScale.range, unit: colorScale.unit },
      [...cells.values()],
      tokens,
    );
  }, [colorScale, cells, tokens]);

  // Fill + readable ink for one cell, from whichever scale is in play.
  const paint = (v: number): { fill: string; ink: string } => {
    if (scale) {
      const c = cellColor(scale, v);
      if (c) return { fill: c.background, ink: c.text };
    }
    const shade = HEATMAP_SHADES[bucketOf(v)].color;
    return { fill: shade, ink: heatmapCellInk(shade) };
  };

  // Measure width to draw 1:1; a small viewBox scaled to 100% magnified fonts/cells.
  const wrapRef = useRef<HTMLDivElement>(null);
  const [boxW, setBoxW] = useState(720);
  useEffect(() => {
    const el = wrapRef.current;
    if (!el || typeof ResizeObserver === "undefined") return;
    const ro = new ResizeObserver((entries) => {
      const w = entries[0]?.contentRect.width;
      if (w && w > 0) setBoxW(w);
    });
    ro.observe(el);
    return () => ro.disconnect();
  }, []);

  // y-labels left, x-ticks + centred x-title below the grid (matches the cloud heatmap).
  const leftPad = 92;
  const rightPad = 16;
  const topPad = 10;
  const cellH = Math.max(28, Math.min(96, 220 / yLabels.length));
  const gridBottom = topPad + yLabels.length * cellH;
  const tickY = gridBottom + 18;
  const titleY = gridBottom + 40;
  const height = gridBottom + (xTitle ? 50 : 26);
  const width = Math.max(240, boxW);
  const gridW = Math.max(48, width - leftPad - rightPad);
  const cellW = gridW / xLabels.length;
  const rectW = Math.max(1, cellW - 4);
  const rectH = Math.max(1, cellH - 4);

  // Cell values, formatted and width-checked once per data/size change — hovering
  // re-renders the whole grid, so this keeps the formatter off that path. A label
  // that would not fit its cell is dropped: the shade carries the magnitude and
  // the tooltip has the exact number.
  const cellLabels = useMemo(() => {
    if (!showValues) return null;
    const fitted = new Map<string, string>();
    for (const [key, val] of cells) {
      const label = valueFmt(val);
      if (truncateTreemapLabel(label, rectW, 11) === label) fitted.set(key, label);
    }
    return fitted;
  }, [cells, showValues, valueFmt, rectW]);

  return (
    <div ref={wrapRef} style={{ width: "100%" }}>
      {scale ? (
        <div style={{ display: "flex", justifyContent: "center", alignItems: "center", gap: 8, marginBottom: 8 }}>
          <span style={{ fontSize: 12, color: axisColor, fontFamily: '"Geist", system-ui' }}>{valueFmt(scale.stops[0].value)}</span>
          <span
            style={{
              width: 160,
              height: 10,
              borderRadius: 2,
              background: `linear-gradient(to right, ${scale.stops.map((st) => `rgb(${st.rgb.join(",")})`).join(", ")})`,
            }}
          />
          <span style={{ fontSize: 12, color: axisColor, fontFamily: '"Geist", system-ui' }}>
            {valueFmt(scale.stops[scale.stops.length - 1].value)}
          </span>
        </div>
      ) : (
      <div style={{ display: "flex", justifyContent: "center", gap: 16, marginBottom: 8, flexWrap: "wrap" }}>
        {HEATMAP_SHADES.map((s) => (
          <div key={s.name} style={{ display: "flex", alignItems: "center", gap: 5 }}>
            <span style={{ width: 12, height: 12, borderRadius: 2, background: s.color, display: "inline-block" }} />
            <span style={{ fontSize: 12, color: axisColor, fontFamily: '"Geist", system-ui' }}>{s.name}</span>
          </div>
        ))}
      </div>
      )}
      <svg width="100%" height={height} viewBox={`0 0 ${width} ${height}`}
        onMouseLeave={() => setHover(null)}>
        {yLabels.map((y, j) => (
          <text key={`y-${j}`} x={leftPad - 8} y={topPad + j * cellH + cellH / 2 + 4} fill={axisColor} fontSize={12} fontFamily='"Geist", system-ui' textAnchor="end">
            {String(y)}
          </text>
        ))}
        {xLabels.map((x, i) =>
          yLabels.map((y, j) => {
            const raw = cells.get(`${x}_${y}`);
            const present = raw !== undefined;
            const val = raw ?? 0;
            const fill = present ? paint(val).fill : "#E5E7EB";
            const rx = leftPad + i * cellW + 2;
            const ry = topPad + j * cellH + 2;
            return (
              <rect
                key={`${i}-${j}`}
                x={rx}
                y={ry}
                width={rectW}
                height={rectH}
                rx={3}
                fill={fill}
                opacity={present ? 1 : 0.4}
                onMouseEnter={() => setHover({ x: rx + cellW, y: ry + cellH / 2, xLabel: x, yLabel: y, value: val })}
                onMouseLeave={() => setHover(null)}
              />
            );
          }),
        )}
        {cellLabels && cellLabels.size > 0 && (
          <g {...AXIS_STYLE} textAnchor="middle" pointerEvents="none">
            {xLabels.map((x, i) =>
              yLabels.map((y, j) => {
                const label = cellLabels.get(`${x}_${y}`);
                if (!label) return null;
                const val = cells.get(`${x}_${y}`) ?? 0;
                return (
                  <text
                    key={`${i}-${j}`}
                    x={leftPad + i * cellW + 2 + rectW / 2}
                    y={topPad + j * cellH + 2 + rectH / 2 + 4}
                    fill={paint(val).ink}
                  >
                    {label}
                  </text>
                );
              }),
            )}
          </g>
        )}
        {xLabels.map((x, i) => (
          <text key={`xt-${i}`} x={leftPad + i * cellW + cellW / 2} y={tickY} fill={axisColor} fontSize={12} fontFamily='"Geist", system-ui' textAnchor="middle">
            {formatAxisTick(x)}
          </text>
        ))}
        {xTitle && (
          <text x={leftPad + gridW / 2} y={titleY} fill={axisColor} fontSize={13} fontWeight={600} fontFamily='"Geist", system-ui' textAnchor="middle">
            {xTitle}
          </text>
        )}
        {hover && (
          <SvgTooltip x={hover.x} y={hover.y}
            lines={[`${formatAxisTick(hover.xLabel)}, ${hover.yLabel}`, valueFmt(hover.value)]} />
        )}
      </svg>
    </div>
  );
}

// --- Dumbbell rendering ---

function DumbbellChart({
  data,
  xKey,
  yFields,
  colors,
  axisColor,
  gridColor,
}: {
  data: Record<string, unknown>[];
  xKey: string;
  yFields: string[];
  colors: string[];
  axisColor: string;
  gridColor: string;
}) {
  const startField = yFields[0];
  const endField = yFields[1] ?? yFields[0];

  const { items, maxVal } = useMemo(() => {
    const rows = data.map((d) => ({
      label: String(d[xKey]),
      start: Number(d[startField]) || 0,
      end: Number(d[endField]) || 0,
    }));
    const maxV = rows.reduce((m, r) => Math.max(m, r.start, r.end), 0);
    return { items: rows, maxVal: maxV };
  }, [data, xKey, startField, endField]);

  const barH = 20;
  const gap = 6;
  const leftPad = 80;
  const rightPad = 16;
  const topPad = 4;
  const chartWidth = 500;
  const height = topPad + items.length * (barH + gap);
  const barWidth = chartWidth - leftPad - rightPad;

  return (
    <svg width="100%" viewBox={`0 0 ${chartWidth} ${height}`} style={{ maxHeight: 240 }}>
      {items.map((item, i) => {
        const y = topPad + i * (barH + gap) + barH / 2;
        const x1 = maxVal > 0 ? leftPad + (item.start / maxVal) * barWidth : leftPad;
        const x2 = maxVal > 0 ? leftPad + (item.end / maxVal) * barWidth : leftPad;
        const increasing = item.end >= item.start;
        return (
          <g key={i}>
            <line x1={leftPad} x2={leftPad + barWidth} y1={y} y2={y} stroke={gridColor} strokeOpacity={0.3} />
            <text x={leftPad - 6} y={y + 3} fill={axisColor} fontSize={10} fontFamily='"Geist", system-ui' textAnchor="end">
              {item.label.slice(0, 10)}
            </text>
            <line x1={x1} x2={x2} y1={y} y2={y} stroke={increasing ? colors[0] : colors[3] ?? colors[0]} strokeWidth={2} />
            <circle cx={x1} cy={y} r={4} fill={colors[1] ?? colors[0]} />
            <circle cx={x2} cy={y} r={4} fill={increasing ? colors[0] : colors[3] ?? colors[0]} />
          </g>
        );
      })}
    </svg>
  );
}

// --- Treemap rendering ---

function getHexLuminance(hex: string): number | null {
  const match = hex.match(/^#?([a-f\d]{2})([a-f\d]{2})([a-f\d]{2})$/i);
  if (!match) return null;

  const [, r, g, b] = match;
  const channels = [r, g, b].map((part) => {
    const value = parseInt(part, 16) / 255;
    return value <= 0.03928 ? value / 12.92 : ((value + 0.055) / 1.055) ** 2.4;
  });

  return 0.2126 * channels[0] + 0.7152 * channels[1] + 0.0722 * channels[2];
}

function getTreemapTextColor(fill: string): string {
  const luminance = getHexLuminance(fill);
  if (luminance === null) return "var(--dac-text-primary)";
  return luminance > 0.32 ? "#0A0D14" : "#FFFFFF";
}

function truncateTreemapLabel(label: string, width: number, fontSize: number): string {
  const maxChars = Math.floor((width - 16) / (fontSize * 0.58));
  if (maxChars <= 1) return "";
  if (label.length <= maxChars) return label;
  return `${label.slice(0, Math.max(1, maxChars - 3))}...`;
}

function TreemapCell(node: TreemapNode) {
  const fill = typeof node.fill === "string" ? node.fill : "var(--dac-accent)";
  const textColor = getTreemapTextColor(fill);
  const canShowLabel = node.width >= 42 && node.height >= 24;
  const fontSize = node.width >= 150 && node.height >= 56 ? 12 : 11;
  const label = canShowLabel ? truncateTreemapLabel(node.name, node.width, fontSize) : "";

  return (
    <g>
      <rect
        x={node.x}
        y={node.y}
        width={node.width}
        height={node.height}
        fill={fill}
        stroke="var(--dac-surface)"
        strokeWidth={1}
      />
      {label && (
        <text
          x={node.x + 8}
          y={node.y + 16}
          fill={textColor}
          stroke="none"
          fontFamily='"Geist", system-ui'
          fontSize={fontSize}
          fontWeight={500}
          dominantBaseline="middle"
          opacity={0.92}
          style={{ pointerEvents: "none" }}
        >
          {label}
        </text>
      )}
    </g>
  );
}

// --- Gauge rendering ---

function GaugeChart({
  current,
  target,
  colors,
  axisColor,
  gridColor,
}: {
  current: number;
  target: number;
  colors: string[];
  axisColor: string;
  gridColor: string;
}) {
  const safeTarget = target > 0 ? target : 1;
  const ratio = Math.max(0, Math.min(1, current / safeTarget));

  const width = 240;
  const height = 150;
  const cx = width / 2;
  const cy = height - 16;
  const r = 90;
  const stroke = 14;

  // Semi-circle from 180° (left) to 0° (right)
  const polarToCart = (deg: number) => {
    const rad = (deg * Math.PI) / 180;
    return { x: cx + r * Math.cos(rad), y: cy - r * Math.sin(rad) };
  };

  const bgPath = (() => {
    const a = polarToCart(180);
    const b = polarToCart(0);
    return `M ${a.x} ${a.y} A ${r} ${r} 0 0 1 ${b.x} ${b.y}`;
  })();

  const fgPath = (() => {
    const endDeg = 180 - ratio * 180;
    const a = polarToCart(180);
    const b = polarToCart(endDeg);
    return `M ${a.x} ${a.y} A ${r} ${r} 0 0 1 ${b.x} ${b.y}`;
  })();

  const pct = Math.round(ratio * 100);

  return (
    <svg width="100%" viewBox={`0 0 ${width} ${height}`} style={{ maxHeight: 240 }}>
      <path d={bgPath} stroke={gridColor} strokeWidth={stroke} strokeLinecap="round" fill="none" opacity={0.4} />
      {ratio > 0 && (
        <path d={fgPath} stroke={colors[0]} strokeWidth={stroke} strokeLinecap="round" fill="none" />
      )}
      <text x={cx} y={cy - 18} textAnchor="middle"
        fill="var(--dac-text-primary)" fontSize={26} fontWeight={600} fontFamily='"Geist", system-ui'>
        {formatTooltipValue(current)}
      </text>
      <text x={cx} y={cy + 2} textAnchor="middle"
        fill={axisColor} fontSize={11} fontFamily='"Geist", system-ui'>
        {pct}% of {formatTooltipValue(safeTarget)}
      </text>
    </svg>
  );
}

// --- Candlestick rendering ---

function CandlestickChart({
  data,
  xKey,
  openKey,
  highKey,
  lowKey,
  closeKey,
  colors,
  axisColor,
  gridColor,
}: {
  data: Record<string, unknown>[];
  xKey: string;
  openKey: string;
  highKey: string;
  lowKey: string;
  closeKey: string;
  colors: string[];
  axisColor: string;
  gridColor: string;
}) {
  const [hover, setHover] = useState<{
    x: number; y: number; label: string;
    open: number; high: number; low: number; close: number;
  } | null>(null);

  const items = useMemo(() => data.map((d) => ({
    label: String(d[xKey]),
    open: Number(d[openKey]) || 0,
    high: Number(d[highKey]) || 0,
    low: Number(d[lowKey]) || 0,
    close: Number(d[closeKey]) || 0,
  })), [data, xKey, openKey, highKey, lowKey, closeKey]);

  if (items.length === 0) return null;

  const allVals = items.flatMap((i) => [i.high, i.low, i.open, i.close]);
  const dataMin = Math.min(...allVals);
  const dataMax = Math.max(...allVals);
  const pad = (dataMax - dataMin) * 0.05 || 1;
  const yMin = dataMin - pad;
  const yMax = dataMax + pad;

  const width = 600;
  const height = 240;
  const padL = 44;
  const padR = 8;
  const padT = 8;
  const padB = 28;
  const plotW = width - padL - padR;
  const plotH = height - padT - padB;

  const yToPx = (v: number) => padT + plotH - ((v - yMin) / (yMax - yMin)) * plotH;
  const step = plotW / items.length;
  const bodyW = Math.max(2, Math.min(16, step * 0.6));

  const upColor = colors[0];
  const downColor = colors[3] ?? "#DC2626";

  const tickCount = 4;
  const yTicks = Array.from({ length: tickCount + 1 }, (_, i) => yMin + (i / tickCount) * (yMax - yMin));

  const labelStride = Math.max(1, Math.ceil(items.length / 8));

  return (
    <svg width="100%" viewBox={`0 0 ${width} ${height}`} style={{ maxHeight: 240 }}
      onMouseLeave={() => setHover(null)}>
      {yTicks.map((v, i) => {
        const y = yToPx(v);
        return (
          <g key={i}>
            <line x1={padL} x2={width - padR} y1={y} y2={y}
              stroke={gridColor} strokeOpacity={0.5} strokeDasharray="3 3" />
            <text x={padL - 6} y={y + 3} textAnchor="end"
              fill={axisColor} fontSize={11} fontFamily='"Geist", system-ui'>
              {formatYTick(v)}
            </text>
          </g>
        );
      })}
      {items.map((item, i) => {
        const cx = padL + i * step + step / 2;
        const yH = yToPx(item.high);
        const yL = yToPx(item.low);
        const yO = yToPx(item.open);
        const yC = yToPx(item.close);
        const up = item.close >= item.open;
        const color = up ? upColor : downColor;
        const top = Math.min(yO, yC);
        const h = Math.max(1, Math.abs(yC - yO));
        return (
          <g key={i}
            onMouseEnter={() => setHover({ x: cx, y: (yH + yL) / 2, ...item })}
            onMouseLeave={() => setHover(null)}>
            <line x1={cx} x2={cx} y1={yH} y2={yL} stroke={color} strokeWidth={1} />
            <rect x={cx - bodyW / 2} y={top} width={bodyW} height={h}
              fill={color} stroke={color} />
            {i % labelStride === 0 && (
              <text x={cx} y={height - padB + 14} textAnchor="middle"
                fill={axisColor} fontSize={10} fontFamily='"Geist", system-ui'>
                {formatAxisTick(item.label)}
              </text>
            )}
          </g>
        );
      })}
      {hover && (
        <SvgTooltip x={hover.x + bodyW / 2} y={hover.y}
          lines={[
            formatAxisTick(hover.label),
            `O ${formatTooltipValue(hover.open)}  H ${formatTooltipValue(hover.high)}`,
            `L ${formatTooltipValue(hover.low)}  C ${formatTooltipValue(hover.close)}`,
          ]} />
      )}
    </svg>
  );
}

// --- Funnel rendering ---

/**
 * A conversion funnel: one centered, tapering bar per stage. Unlike a plain
 * funnel chart, this surfaces the two numbers that matter for an onboarding
 * funnel — each stage's share of the top of the funnel (overall conversion)
 * and the step-to-step conversion between consecutive stages (drop-off).
 *
 * Rows are taken in the order the query returns them (top of funnel first);
 * no sorting is applied so stages stay in their intended sequence.
 *
 * Orientation follows the widget's `horizontal` flag: the default stacks
 * stages top-to-bottom (bars taper by width); `horizontal: true` lays them
 * left-to-right (bars taper by height, like a column funnel).
 */
function FunnelChart({
  data,
  labelKey,
  valueKey,
  colors,
  tokens,
  height,
  horizontal = false,
  valueFmt,
}: {
  data: Record<string, unknown>[];
  labelKey: string;
  valueKey: string;
  colors: string[];
  tokens: Record<string, string>;
  height: number;
  horizontal?: boolean;
  valueFmt: (val: unknown) => string;
}) {
  const raw = data
    .map((d) => ({ label: String(d[labelKey] ?? ""), value: Number(d[valueKey]) || 0 }))
    .filter((s) => s.label !== "");

  if (raw.length === 0) {
    return <div className="text-[var(--dac-text-muted)] text-xs py-6 text-center">No data</div>;
  }

  const topValue = raw[0].value || 1;
  const maxValue = Math.max(...raw.map((s) => s.value)) || 1;
  const textPrimary = tokens["text-primary"] || "#0A0D14";
  const textMuted = tokens["text-muted"] || "#868C98";
  const barColor = colors[0] || "#4338CA";

  // Per-stage derived numbers, shared by both orientations. `extent` is the
  // bar's share of the largest stage (width when vertical, height when
  // horizontal); a floor keeps thin stages readable.
  const stages = raw.map((s, i) => ({
    ...s,
    extent: Math.max(8, (s.value / maxValue) * 100),
    pctOfTop: topValue > 0 ? (s.value / topValue) * 100 : 0,
    stepConv: i === 0 ? null : raw[i - 1].value > 0 ? (s.value / raw[i - 1].value) * 100 : 0,
    // Fade successive stages so depth reads without extra chrome.
    opacity: 1 - (i / Math.max(raw.length, 1)) * 0.55,
  }));

  if (horizontal) {
    return (
      <div className="flex items-stretch overflow-x-auto px-1" style={{ height }}>
        {stages.map((s, i) => (
          <div key={i} className="flex min-w-0 flex-1 items-stretch">
            {i > 0 && s.stepConv !== null && (
              <div
                className="flex shrink-0 flex-col items-center justify-center px-0.5 text-[10px] font-medium leading-tight tabular-nums"
                style={{ color: textMuted }}
                title={`${s.stepConv.toFixed(1)}% of the previous stage continued`}
              >
                <span aria-hidden>→</span>
                <span>{s.stepConv.toFixed(1)}%</span>
              </div>
            )}
            <div className="flex min-w-0 flex-1 flex-col items-center">
              <span
                className="w-full truncate text-center text-[11px] font-medium leading-tight"
                style={{ color: textPrimary }}
                title={s.label}
              >
                {s.label}
              </span>
              <div className="flex w-full flex-1 items-center justify-center py-1">
                <div
                  className="flex w-[72%] items-center justify-center rounded-[3px]"
                  style={{ height: `${s.extent}%`, background: barColor, opacity: s.opacity }}
                >
                  <span
                    className="px-1 text-[11px] font-semibold tabular-nums text-white"
                    style={{ textShadow: "0 1px 2px rgba(0,0,0,0.25)" }}
                  >
                    {valueFmt(s.value)}
                  </span>
                </div>
              </div>
              <span className="text-[10px] tabular-nums leading-tight" style={{ color: textMuted }}>
                {Math.round(s.pctOfTop)}%
              </span>
            </div>
          </div>
        ))}
      </div>
    );
  }

  return (
    <div className="flex flex-col overflow-y-auto px-1" style={{ height }}>
      {/* my-auto centers the stages when they fit; when they exceed the height
          the container scrolls instead of clipping later stages. */}
      <div className="my-auto flex flex-col gap-0.5">
        {stages.map((s, i) => (
        <div key={i} className="flex flex-col items-center">
          {i > 0 && s.stepConv !== null && (
            <div
              className="flex items-center gap-1 py-[3px] text-[10px] font-medium tabular-nums"
              style={{ color: textMuted }}
              title={`${s.stepConv.toFixed(1)}% of the previous stage continued`}
            >
              <span aria-hidden>↓</span>
              <span>{s.stepConv.toFixed(1)}%</span>
            </div>
          )}
          <div className="flex w-full items-baseline justify-between gap-2 leading-none">
            <span
              className="truncate text-[11px] font-medium"
              style={{ color: textPrimary }}
              title={s.label}
            >
              {s.label}
            </span>
            <span className="shrink-0 text-[10px] tabular-nums" style={{ color: textMuted }}>
              {Math.round(s.pctOfTop)}%
            </span>
          </div>
          <div className="mt-1 flex w-full justify-center">
            <div
              className="flex h-7 items-center justify-center rounded-[3px]"
              style={{ width: `${s.extent}%`, background: barColor, opacity: s.opacity }}
            >
              <span
                className="px-2 text-[11px] font-semibold tabular-nums text-white"
                style={{ textShadow: "0 1px 2px rgba(0,0,0,0.25)" }}
              >
                {valueFmt(s.value)}
              </span>
            </div>
          </div>
        </div>
        ))}
      </div>
    </div>
  );
}

// --- Main chart component ---

// Height consumed by the y-axis title line rendered above the plot.
const Y_TITLE_OFFSET = 18;

export function ChartWidget({ widget, data }: Props) {
  const tokens = useTokens();
  if (widget.chart === "vega-lite") {
    return <VegaLiteChart widget={widget} data={data} />;
  }
  const yTitle = widget.y?.title;
  if (!yTitle) {
    return <ChartBody widget={widget} data={data} />;
  }
  // Like cloud: the y-axis title sits under the widget title, not rotated
  // alongside the axis.
  return (
    <div>
      <div style={{ ...AXIS_STYLE, color: tokens["text-muted"], marginBottom: 4 }}>{yTitle}</div>
      <ChartBody widget={widget} data={data} titleOffset={Y_TITLE_OFFSET} />
    </div>
  );
}

function ChartBody({ widget, data, titleOffset = 0 }: Props & { titleOffset?: number }) {
  const tokens = useTokens();
  const rowHeight = useContext(RowHeightContext);
  const chartHeight = (rowHeight !== undefined
    ? Math.max(80, rowHeight - FRAME_OVERHEAD)
    : DEFAULT_CHART_HEIGHT) - titleOffset;

  if (!data?.rows?.length) {
    return <div className="text-[var(--dac-text-muted)] text-xs py-6 text-center">No data</div>;
  }

  const chartData = toChartData(data);
  const colors = CHART_COLORS.map((key) => tokens[key] || "#888");
  const gridColor = tokens["border"];
  const axisColor = tokens["text-muted"];

  // Right value axis: a y-column plots against it when listed in widget.y2.field.
  // hasDual gates every branch so single-axis charts render exactly as before.
  const y2Keys = axisFields(widget.y2);
  const hasDual = y2Keys.length > 0;
  const rightSet = new Set(y2Keys);
  const allYKeys = [...axisFields(widget.y), ...y2Keys];
  // undefined yAxisId = Recharts' default axis, so non-dual charts pass no id.
  const axisIdFor = (field: string): "left" | "right" | undefined =>
    hasDual ? (rightSet.has(field) ? "right" : "left") : undefined;

  // Per-series line styling: grouped under widget.series ({col:{color,curve,dash}}),
  // each falling back to the chart-wide curve/dash. Overrides key by y-column, so
  // pass `own=false` in color-pivot mode where series are values, not columns.
  const curveType = (v?: string): "linear" | "stepAfter" | "monotone" =>
    v === "straight" ? "linear" : v === "stepline" ? "stepAfter" : "monotone";
  const DASH_ARRAY: Record<string, string> = { dotted: "1 5", dashed: "6 4", "long-dash": "12 6" };
  const dashArrayFor = (v?: string): string | undefined => (v && v !== "solid" ? DASH_ARRAY[v] : undefined);
  const styleOf = (field: string) => widget.series?.[field] ?? {};
  // Right-axis series inherit their chart-wide curve/dash from y2, not y.
  const axisOf = (field: string) => (rightSet.has(field) ? widget.y2 : widget.y);
  const seriesCurve = (field: string, own: boolean) => curveType(own ? (styleOf(field).curve ?? axisOf(field)?.curve) : widget.y?.curve);
  const seriesDash = (field: string, own: boolean) => dashArrayFor(own ? (styleOf(field).dash ?? axisOf(field)?.dash) : widget.y?.dash);
  const seriesColor = (field: string, i: number, own: boolean) => (own && styleOf(field).color) || colors[i % colors.length];
  const yDomain: [number, string] | undefined = widget.y?.beginAtZero ? [0, "auto"] : undefined;
  // Point markers: shown on sparse line/area series by default, hidden when
  // y.markers is false (dense series stay dotless to avoid clutter).
  const showMarkers = widget.y?.markers !== false;
  const dotFor = (n: number) => (showMarkers && n <= 30 ? { r: 2, strokeWidth: 0 } : false);

  const xKey = axisField(widget.x);
  const yKeys = axisFields(widget.y);
  const xTick = buildAxisFormatter(widget.x, formatAxisTick);
  const yTick = buildAxisFormatter(widget.y, formatYTick);
  const yTooltipValue = buildAxisFormatter(widget.y, formatTooltipValue);
  // The title renders at the bottom edge of a taller x-axis band so it never
  // collides with a legend rendered below the plot.
  const xLabelProps = widget.x?.title
    ? {
        label: { value: widget.x.title, position: "insideBottom" as const, offset: 0, style: { ...AXIS_STYLE, fill: axisColor } },
        height: 44,
      }
    : {};

  const commonAxisProps = {
    tick: { ...AXIS_STYLE, fill: axisColor },
    axisLine: false,
    tickLine: false,
  };
  // Formatted values can be wider than Recharts' fixed 60px default. Measure
  // Y axes from their rendered ticks so leading characters stay visible, but
  // truncate exceptionally long labels before they consume the plot area.
  const autoWidthYAxisProps = {
    ...commonAxisProps,
    tick: { ...commonAxisProps.tick, width: MAX_Y_AXIS_TICK_WIDTH, maxLines: 1, breakAll: true },
    width: "auto" as const,
  };

  const y2Tick = buildAxisFormatter(widget.y2, formatYTick);
  const y2TooltipValue = buildAxisFormatter(widget.y2, formatTooltipValue);
  const y2Domain: [number, string] | undefined = widget.y2?.beginAtZero ? [0, "auto"] : undefined;
  const leftAxisId = hasDual ? "left" : undefined;
  // Each tooltip row formats against its owning axis, so a right-axis % value
  // never borrows the left axis' $ format.
  const valueFormatterFor = hasDual
    ? (dataKey: unknown) => (rightSet.has(String(dataKey)) ? y2TooltipValue : yTooltipValue)
    : undefined;
  const rightYAxis = hasDual ? (
    <YAxis
      yAxisId="right"
      orientation="right"
      {...autoWidthYAxisProps}
      dx={4}
      tickFormatter={y2Tick}
      domain={y2Domain}
      {...(widget.y2?.title
        ? { label: { value: widget.y2.title, angle: 90, position: "insideRight" as const, style: { ...AXIS_STYLE, fill: axisColor } } }
        : {})}
    />
  ) : null;

  const cartesianMargin = { top: 4, right: hasDual ? 12 : 8, bottom: 4, left: 4 };
  const gridProps = { vertical: false, stroke: gridColor, strokeOpacity: 0.5, strokeDasharray: "3 3" };
  const cartesianTooltip = <CustomTooltip labelFormatter={xTick} valueFormatter={yTooltipValue} valueFormatterFor={valueFormatterFor} />;

  switch (widget.chart) {
    case "line": {
      const colorKey = widget.color?.field;
      const pivoted = colorKey && xKey && yKeys[0] ? pivotByColor(chartData, xKey, yKeys[0], colorKey) : null;
      const rows = pivoted ? pivoted.rows : chartData;
      const series = pivoted ? pivoted.series : allYKeys;
      // CI band: shade each series' yMin/yMax interval (a range <Area>) behind its
      // line. Needs a ComposedChart to mix Area + Line. Scalar or per-series map.
      const bandBounds = series.map((f) => ({ f, lo: boundColumn(widget.yMin, f), hi: boundColumn(widget.yMax, f) }));
      const hasBand = !colorKey && bandBounds.some((b) => b.lo && b.hi);
      const lineRows = hasBand
        ? rows.map((r) => {
            const o: Record<string, unknown> = { ...r };
            for (const b of bandBounds) {
              if (!b.lo || !b.hi) continue;
              const lo = Number(r[b.lo]), hi = Number(r[b.hi]);
              if (Number.isFinite(lo) && Number.isFinite(hi)) o[`__ci_${b.f}`] = [lo, hi];
            }
            return o;
          })
        : rows;
      const LineOrComposed = hasBand ? ComposedChart : LineChart;
      return (
        <ResponsiveContainer width="100%" height={chartHeight}>
          <LineOrComposed data={lineRows} margin={cartesianMargin}>
            <CartesianGrid {...gridProps} />
            <XAxis dataKey={xKey} {...commonAxisProps} dy={6} tickFormatter={xTick} {...xLabelProps} />
            <YAxis yAxisId={leftAxisId} {...autoWidthYAxisProps} tickFormatter={yTick} domain={yDomain} />
            {rightYAxis}
            <Tooltip content={cartesianTooltip} />
            {series.length > 1 && <Legend wrapperStyle={AXIS_STYLE} iconSize={7} />}
            {hasBand && bandBounds.map((b, i) => (b.lo && b.hi ? (
              <Area key={`ci-${b.f}`} yAxisId={axisIdFor(b.f)} type="monotone" dataKey={`__ci_${b.f}`} fill={colors[i % colors.length]} fillOpacity={CI_BAND_OPACITY} stroke="none" legendType="none" tooltipType="none" isAnimationActive={false} />
            ) : null))}
            {series.map((field, i) => (
              <Line
                key={field}
                yAxisId={axisIdFor(field)}
                type={seriesCurve(field, !colorKey)}
                dataKey={field}
                stroke={seriesColor(field, i, !colorKey)}
                strokeDasharray={seriesDash(field, !colorKey)}
                strokeWidth={1.5}
                dot={dotFor(rows.length)}
                activeDot={{ r: 3, strokeWidth: 0 }}
                isAnimationActive={false}
              />
            ))}
            {renderRefs(widget, leftAxisId)}
          </LineOrComposed>
        </ResponsiveContainer>
      );
    }

    case "bar": {
      const colorKey = widget.color?.field;
      const pivoted = colorKey && xKey && yKeys[0] ? pivotByColor(chartData, xKey, yKeys[0], colorKey) : null;
      const series = pivoted ? pivoted.series : allYKeys;
      const isStacked = widget.stacked && !!colorKey;
      const rows = pivoted
        ? (widget.normalized ? normalizeRows(pivoted.rows, pivoted.series) : pivoted.rows)
        : chartData;
      const horizontal = widget.horizontal;
      const valueTick = widget.normalized ? formatPercentTick : yTick;
      const valueTooltip = widget.normalized ? formatPercentTick : yTooltipValue;
      const lastRadius: [number, number, number, number] = horizontal ? [0, 2, 2, 0] : [2, 2, 0, 0];
      // Error-bar caps: pair each series with its yMin/yMax columns (scalar for a
      // single series, or a per-series map for grouped bars) and precompute the
      // asymmetric offsets [value-lo, hi-value] that Recharts <ErrorBar> expects.
      const errBounds = series.map((f) => ({ f, lo: boundColumn(widget.yMin, f), hi: boundColumn(widget.yMax, f) }));
      const hasErr = !colorKey && errBounds.some((b) => b.lo && b.hi);
      const barRows = hasErr
        ? rows.map((r) => {
            const o: Record<string, unknown> = { ...r };
            for (const b of errBounds) {
              if (!b.lo || !b.hi) continue;
              const v = Number(r[b.f]), lo = Number(r[b.lo]), hi = Number(r[b.hi]);
              // Clamp to ≥0: if the estimate falls outside its interval, a raw
              // offset would go negative and Recharts draws an inverted cap.
              if (Number.isFinite(v) && Number.isFinite(lo) && Number.isFinite(hi)) o[`__err_${b.f}`] = [Math.max(0, v - lo), Math.max(0, hi - v)];
            }
            return o;
          })
        : rows;
      return (
        <ResponsiveContainer width="100%" height={chartHeight}>
          <BarChart data={barRows} layout={horizontal ? "vertical" : "horizontal"} margin={cartesianMargin}>
            <CartesianGrid {...gridProps} vertical={horizontal} horizontal={!horizontal} />
            {horizontal ? (
              <>
                <XAxis type="number" {...commonAxisProps} dy={6} tickFormatter={valueTick} />
                <YAxis type="category" dataKey={xKey} {...commonAxisProps} width={80} tickFormatter={xTick} />
              </>
            ) : (
              <>
                <XAxis dataKey={xKey} {...commonAxisProps} dy={6} tickFormatter={xTick} {...xLabelProps} />
                <YAxis yAxisId={leftAxisId} {...autoWidthYAxisProps} tickFormatter={valueTick} />
                {rightYAxis}
              </>
            )}
            <Tooltip content={<CustomTooltip labelFormatter={xTick} valueFormatter={valueTooltip} valueFormatterFor={valueFormatterFor} />} cursor={{ fill: gridColor, fillOpacity: 0.2 }} />
            {series.length > 1 && <Legend wrapperStyle={AXIS_STYLE} iconSize={7} />}
            {series.map((field, i) => (
              <Bar
                key={field}
                yAxisId={horizontal ? undefined : axisIdFor(field)}
                dataKey={field}
                fill={seriesColor(field, i, !colorKey)}
                stackId={isStacked ? "stack" : undefined}
                radius={isStacked && i < series.length - 1 ? undefined : lastRadius}
                isAnimationActive={false}
              >
                {hasErr && errBounds[i].lo && errBounds[i].hi && (
                  <ErrorBar dataKey={`__err_${field}`} width={4} strokeWidth={1} stroke="#374151" direction={horizontal ? "x" : "y"} />
                )}
              </Bar>
            ))}
            {renderRefs(widget, horizontal ? undefined : leftAxisId)}
          </BarChart>
        </ResponsiveContainer>
      );
    }

    case "area": {
      const colorKey = widget.color?.field;
      const pivoted = colorKey && xKey && yKeys[0] ? pivotByColor(chartData, xKey, yKeys[0], colorKey) : null;
      const rows = pivoted ? pivoted.rows : chartData;
      const series = pivoted ? pivoted.series : allYKeys;
      // CI band: a translucent range <Area> per series (yMin/yMax) behind the fill.
      const bandBounds = series.map((f) => ({ f, lo: boundColumn(widget.yMin, f), hi: boundColumn(widget.yMax, f) }));
      const hasBand = !colorKey && bandBounds.some((b) => b.lo && b.hi);
      const areaRows = hasBand
        ? rows.map((r) => {
            const o: Record<string, unknown> = { ...r };
            for (const b of bandBounds) {
              if (!b.lo || !b.hi) continue;
              const lo = Number(r[b.lo]), hi = Number(r[b.hi]);
              if (Number.isFinite(lo) && Number.isFinite(hi)) o[`__ci_${b.f}`] = [lo, hi];
            }
            return o;
          })
        : rows;
      return (
        <ResponsiveContainer width="100%" height={chartHeight}>
          <AreaChart data={areaRows} margin={cartesianMargin}>
            <CartesianGrid {...gridProps} />
            <XAxis dataKey={xKey} {...commonAxisProps} dy={6} tickFormatter={xTick} {...xLabelProps} />
            <YAxis yAxisId={leftAxisId} {...autoWidthYAxisProps} tickFormatter={yTick} domain={yDomain} />
            {rightYAxis}
            <Tooltip content={cartesianTooltip} />
            {series.length > 1 && <Legend wrapperStyle={AXIS_STYLE} iconSize={7} />}
            {hasBand && bandBounds.map((b, i) => (b.lo && b.hi ? (
              <Area key={`ci-${b.f}`} yAxisId={axisIdFor(b.f)} type="monotone" dataKey={`__ci_${b.f}`} fill={colors[i % colors.length]} fillOpacity={CI_BAND_OPACITY} stroke="none" legendType="none" tooltipType="none" isAnimationActive={false} />
            ) : null))}
            {series.map((field, i) => (
              <Area
                key={field}
                yAxisId={axisIdFor(field)}
                type={seriesCurve(field, !colorKey)}
                dataKey={field}
                stroke={seriesColor(field, i, !colorKey)}
                fill={seriesColor(field, i, !colorKey)}
                strokeDasharray={seriesDash(field, !colorKey)}
                dot={dotFor(rows.length)}
                fillOpacity={0.06}
                strokeWidth={1.5}
                isAnimationActive={false}
              />
            ))}
            {renderRefs(widget, leftAxisId)}
          </AreaChart>
        </ResponsiveContainer>
      );
    }

    case "pie": {
      return (
        <ResponsiveContainer width="100%" height={chartHeight}>
          <PieChart>
            <Pie
              data={chartData}
              dataKey={valueField(widget.value) || "value"}
              nameKey={widget.label || "label"}
              cx="50%"
              cy="45%"
              outerRadius={75}
              innerRadius={40}
              strokeWidth={0}
              isAnimationActive={false}
              label={({ name, percent }: { name?: string; percent?: number }) =>
                `${name ?? ""} ${((percent ?? 0) * 100).toFixed(0)}%`
              }
              labelLine={false}
              style={AXIS_STYLE}
            >
              {chartData.map((_, i) => (
                <Cell key={i} fill={colors[i % colors.length]} />
              ))}
            </Pie>
            <Tooltip content={<CustomTooltip />} />
            <Legend wrapperStyle={AXIS_STYLE} iconSize={7} />
          </PieChart>
        </ResponsiveContainer>
      );
    }

    case "scatter": {
      const xIsNumeric = widget.x?.type
        ? widget.x.type === "number"
        : chartData.length > 0 && typeof chartData[0][xKey!] === "number";
      const yIsNumeric = widget.y?.type
        ? widget.y.type === "number"
        : chartData.length > 0 && typeof chartData[0][yKeys[0] ?? ""] === "number";
      return (
        <ResponsiveContainer width="100%" height={chartHeight}>
          <ScatterChart margin={cartesianMargin}>
            <CartesianGrid {...gridProps} />
            <XAxis dataKey={xKey} name={widget.x?.title ?? xKey} {...commonAxisProps} dy={6}
              tickFormatter={xTick} {...xLabelProps}
              type={xIsNumeric ? "number" : "category"} allowDuplicatedCategory={false} />
            <YAxis dataKey={yKeys[0]} name={widget.y?.title ?? yKeys[0]}
              {...(yIsNumeric ? autoWidthYAxisProps : commonAxisProps)}
              tickFormatter={yTick} type={yIsNumeric ? "number" : "category"} />
            <Tooltip content={cartesianTooltip} />
            <Scatter data={chartData} fill={colors[0]} r={3} isAnimationActive={false} />
          </ScatterChart>
        </ResponsiveContainer>
      );
    }

    case "bubble": {
      const xIsNumeric = widget.x?.type
        ? widget.x.type === "number"
        : chartData.length > 0 && typeof chartData[0][xKey!] === "number";
      const yIsNumeric = widget.y?.type
        ? widget.y.type === "number"
        : chartData.length > 0 && typeof chartData[0][yKeys[0] ?? ""] === "number";
      return (
        <ResponsiveContainer width="100%" height={chartHeight}>
          <ScatterChart margin={cartesianMargin}>
            <CartesianGrid {...gridProps} />
            <XAxis dataKey={xKey} name={widget.x?.title ?? xKey} {...commonAxisProps} dy={6}
              type={xIsNumeric ? "number" : "category"} allowDuplicatedCategory={false}
              tickFormatter={xTick} {...xLabelProps} />
            <YAxis dataKey={yKeys[0]} name={widget.y?.title ?? yKeys[0]}
              {...(yIsNumeric ? autoWidthYAxisProps : commonAxisProps)}
              tickFormatter={yTick} type={yIsNumeric ? "number" : "category"} />
            <ZAxis dataKey={widget.size} range={[40, 500]} name={widget.size} />
            <Tooltip content={cartesianTooltip} />
            <Scatter data={chartData} fill={colors[0]} fillOpacity={0.6} isAnimationActive={false} />
          </ScatterChart>
        </ResponsiveContainer>
      );
    }

    case "combo": {
      const yFields = allYKeys;
      const lineSet = new Set(widget.lines ?? []);
      return (
        <ResponsiveContainer width="100%" height={chartHeight}>
          <ComposedChart data={chartData} margin={cartesianMargin}>
            <CartesianGrid {...gridProps} />
            <XAxis dataKey={xKey} {...commonAxisProps} dy={6} tickFormatter={xTick} {...xLabelProps} />
            <YAxis yAxisId={leftAxisId} {...autoWidthYAxisProps} tickFormatter={yTick} domain={yDomain} />
            {rightYAxis}
            <Tooltip content={cartesianTooltip} />
            <Legend wrapperStyle={AXIS_STYLE} iconSize={7} />
            {yFields.map((field, i) =>
              lineSet.has(field) ? (
                <Line
                  key={field}
                  yAxisId={axisIdFor(field)}
                  type={seriesCurve(field, true)}
                  dataKey={field}
                  stroke={seriesColor(field, i, true)}
                  strokeDasharray={seriesDash(field, true)}
                  strokeWidth={1.5}
                  dot={dotFor(chartData.length)}
                  isAnimationActive={false}
                />
              ) : (
                <Bar
                  key={field}
                  yAxisId={axisIdFor(field)}
                  dataKey={field}
                  fill={seriesColor(field, i, true)}
                  radius={[2, 2, 0, 0]}
                  isAnimationActive={false}
                />
              ),
            )}
          </ComposedChart>
        </ResponsiveContainer>
      );
    }

    case "histogram": {
      const histData = buildHistogramData(chartData, xKey!, widget.bins || 10);
      return (
        <ResponsiveContainer width="100%" height={chartHeight}>
          <BarChart data={histData} margin={cartesianMargin}>
            <CartesianGrid {...gridProps} />
            <XAxis dataKey="bin" {...commonAxisProps} dy={6} angle={-30} textAnchor="end" height={50} />
            <YAxis {...autoWidthYAxisProps} tickFormatter={formatYTick} />
            <Tooltip content={<CustomTooltip />} />
            <Bar dataKey="count" fill={colors[0]} radius={[2, 2, 0, 0]} isAnimationActive={false} />
          </BarChart>
        </ResponsiveContainer>
      );
    }

    case "boxplot": {
      const yField = yKeys[0] ?? "value";
      const boxData = buildBoxplotData(chartData, xKey!, yField);
      return (
        <ResponsiveContainer width="100%" height={chartHeight}>
          <ComposedChart data={boxData} margin={cartesianMargin}>
            <CartesianGrid {...gridProps} />
            <XAxis dataKey="category" {...commonAxisProps} dy={6} {...xLabelProps} />
            <YAxis {...autoWidthYAxisProps} tickFormatter={yTick} />
            <Tooltip content={<CustomTooltip />} />
            {/* Invisible base to offset */}
            <Bar dataKey="min" fill="transparent" stackId="box" isAnimationActive={false} />
            {/* Q1 to median */}
            <Bar dataKey="_q1ToMedian" fill={colors[0]} fillOpacity={0.3} stackId="box" radius={[0, 0, 0, 0]} name="Q1–Median" isAnimationActive={false} />
            {/* Median to Q3 */}
            <Bar dataKey="_medianToQ3" fill={colors[0]} fillOpacity={0.5} stackId="box" radius={[0, 0, 0, 0]} name="Median–Q3" isAnimationActive={false} />
          </ComposedChart>
        </ResponsiveContainer>
      );
    }

    case "funnel":
      return (
        <FunnelChart
          data={chartData}
          labelKey={widget.label || "label"}
          valueKey={valueField(widget.value) || "value"}
          colors={colors}
          tokens={tokens}
          height={chartHeight}
          horizontal={widget.horizontal}
          valueFmt={buildAxisFormatter(widget.value, formatTooltipValue)}
        />
      );

    case "sankey": {
      const sankeyData = buildSankeyData(
        chartData,
        widget.source || "source",
        widget.target || "target",
        valueField(widget.value) || "value",
      );
      return (
        <ResponsiveContainer width="100%" height={chartHeight}>
          <Sankey
            data={sankeyData}
            // eslint-disable-next-line @typescript-eslint/no-explicit-any
            node={{ fill: colors[0], opacity: 0.8 } as any}
            // eslint-disable-next-line @typescript-eslint/no-explicit-any
            link={{ stroke: colors[0], strokeOpacity: 0.15 } as any}
            nodePadding={16}
            margin={{ top: 8, right: 8, bottom: 8, left: 8 }}
          >
            <Tooltip content={<CustomTooltip />} />
          </Sankey>
        </ResponsiveContainer>
      );
    }

    case "heatmap":
      return (
        <HeatmapChart
          data={chartData}
          xKey={xKey!}
          yKey={yKeys[0] ?? "y"}
          valueKey={valueField(widget.value) || "value"}
          xTitle={widget.x?.title}
          axisColor={axisColor}
          showValues={widget.showValues}
          valueFmt={buildAxisFormatter(widget.value, formatTooltipValue)}
          colorScale={widget.colorScale}
          tokens={tokens}
        />
      );

    case "calendar":
      return (
        <CalendarHeatmap
          data={chartData}
          dateKey={xKey!}
          valueKey={valueField(widget.value) || "value"}
          colors={colors}
          axisColor={axisColor}
        />
      );

    case "sparkline":
      return (
        <ResponsiveContainer width="100%" height={60}>
          <LineChart data={chartData} margin={{ top: 4, right: 4, bottom: 4, left: 4 }}>
            {yKeys.map((field, i) => (
              <Line
                key={field}
                type="monotone"
                dataKey={field}
                stroke={colors[i % colors.length]}
                strokeWidth={1.5}
                dot={false}
                isAnimationActive={false}
              />
            ))}
            <Tooltip content={<CustomTooltip />} />
          </LineChart>
        </ResponsiveContainer>
      );

    case "waterfall": {
      const wfData = buildWaterfallData(chartData, xKey!, yKeys[0] ?? "value");
      return (
        <ResponsiveContainer width="100%" height={chartHeight}>
          <BarChart data={wfData} margin={cartesianMargin}>
            <CartesianGrid {...gridProps} />
            <XAxis dataKey="name" {...commonAxisProps} dy={6} tickFormatter={xTick} {...xLabelProps} />
            <YAxis {...autoWidthYAxisProps} tickFormatter={yTick} />
            <Tooltip content={cartesianTooltip} />
            <Bar dataKey="base" fill="transparent" stackId="wf" isAnimationActive={false} />
            <Bar dataKey="value" stackId="wf" radius={[2, 2, 0, 0]} isAnimationActive={false}>
              {wfData.map((entry, i) => (
                <Cell key={i} fill={entry.fill === "positive" ? colors[0] : colors[3] ?? "#DC2626"} />
              ))}
            </Bar>
          </BarChart>
        </ResponsiveContainer>
      );
    }

    case "xmr": {
      const yField = yKeys[0] ?? "value";
      return (
        <ResponsiveContainer width="100%" height={chartHeight}>
          <LineChart data={chartData} margin={cartesianMargin}>
            <CartesianGrid {...gridProps} />
            <XAxis dataKey={xKey} {...commonAxisProps} dy={6} tickFormatter={xTick} {...xLabelProps} />
            <YAxis {...autoWidthYAxisProps} tickFormatter={yTick} />
            <Tooltip content={cartesianTooltip} />
            <Line type="monotone" dataKey={yField} stroke={colors[0]} strokeWidth={1.5} dot={{ r: 2, strokeWidth: 0, fill: colors[0] }} isAnimationActive={false} />
            {typeof widget.yMin === "string" && (
              <Line type="monotone" dataKey={widget.yMin} stroke={colors[3] ?? "#DC2626"} strokeWidth={1} strokeDasharray="4 4" dot={false} isAnimationActive={false} />
            )}
            {typeof widget.yMax === "string" && (
              <Line type="monotone" dataKey={widget.yMax} stroke={colors[3] ?? "#DC2626"} strokeWidth={1} strokeDasharray="4 4" dot={false} isAnimationActive={false} />
            )}
            {yKeys.length > 1 && (
              <Line type="monotone" dataKey={yKeys[1]} stroke={colors[1]} strokeWidth={1} strokeDasharray="6 3" dot={false} isAnimationActive={false} />
            )}
          </LineChart>
        </ResponsiveContainer>
      );
    }

    case "dumbbell":
      return (
        <DumbbellChart
          data={chartData}
          xKey={xKey!}
          yFields={yKeys}
          colors={colors}
          axisColor={axisColor}
          gridColor={gridColor}
        />
      );

    case "gauge": {
      const valueKey = valueField(widget.value) || "value";
      const targetKey = widget.target;
      const first = chartData[0] ?? {};
      const current = Number(first[valueKey]) || 0;
      const target = targetKey ? Number(first[targetKey]) || 0 : 100;
      return (
        <GaugeChart
          current={current}
          target={target}
          colors={colors}
          axisColor={axisColor}
          gridColor={gridColor}
        />
      );
    }

    case "treemap": {
      const labelKey = widget.label || "label";
      const valueKey = valueField(widget.value) || "value";
      const tmData = chartData.map((d, i) => ({
        name: String(d[labelKey]),
        size: Number(d[valueKey]) || 0,
        fill: colors[i % colors.length],
      }));
      return (
        <ResponsiveContainer width="100%" height={240}>
          <Treemap
            data={tmData}
            dataKey="size"
            nameKey="name"
            stroke="var(--dac-surface)"
            content={TreemapCell}
            isAnimationActive={false}
          >
            <Tooltip content={<CustomTooltip />} />
          </Treemap>
        </ResponsiveContainer>
      );
    }

    case "radar": {
      const yFields = yKeys;
      return (
        <ResponsiveContainer width="100%" height={240}>
          <RadarChart data={chartData} margin={{ top: 8, right: 24, bottom: 8, left: 24 }}>
            <PolarGrid stroke={gridColor} strokeOpacity={0.5} />
            <PolarAngleAxis dataKey={xKey} tick={{ ...AXIS_STYLE, fill: axisColor }} />
            <PolarRadiusAxis tick={{ ...AXIS_STYLE, fill: axisColor }} axisLine={false} tickFormatter={yTick} />
            <Tooltip content={<CustomTooltip />} />
            {yFields.length > 1 && <Legend wrapperStyle={AXIS_STYLE} iconSize={7} />}
            {yFields.map((field, i) => (
              <Radar
                key={field}
                name={field}
                dataKey={field}
                stroke={colors[i % colors.length]}
                fill={colors[i % colors.length]}
                fillOpacity={0.15}
                strokeWidth={1.5}
                isAnimationActive={false}
              />
            ))}
          </RadarChart>
        </ResponsiveContainer>
      );
    }

    case "candlestick":
      return (
        <CandlestickChart
          data={chartData}
          xKey={xKey!}
          openKey={widget.open || "open"}
          highKey={widget.high || "high"}
          lowKey={widget.low || "low"}
          closeKey={widget.close || "close"}
          colors={colors}
          axisColor={axisColor}
          gridColor={gridColor}
        />
      );

    case "forest": {
      // Point estimate + CI interval per category as a native dot + whisker
      // (Recharts Scatter + ErrorBar). Horizontal (default, matches cloud) puts the
      // value on x; horizontal: false gives a vertical dot-and-whisker (B1 style).
      // Grey when the interval spans 0 (not significant); coloured by series when
      // grouped.
      const horizontal = widget.horizontal !== false;
      const catField = xKey ?? "";
      const multi = yKeys.length > 1;
      // Grouped forests would draw every series on the same category line, so the
      // dots/whiskers overlap. Recharts has no native dodge for Scatter, so the
      // category axis runs as a numeric "slot" axis (one integer per category, ticks
      // relabelled with the names) and each series gets a small offset within its
      // slot. Single-series forests keep the plain category axis.
      const cats: string[] = [];
      for (const r of chartData) {
        const c = String(r[catField] ?? "");
        if (!cats.includes(c)) cats.push(c);
      }
      const n = yKeys.length;
      const seriesOffset = (si: number) => (multi ? (si - (n - 1) / 2) * (0.3 / (n - 1)) : 0);
      const forestSeries = yKeys.map((f, si) => {
        const loCol = boundColumn(widget.yMin, f);
        const hiCol = boundColumn(widget.yMax, f);
        const off = seriesOffset(si);
        const pts = chartData.map((r) => {
          const est = Number(r[f]);
          const lo = loCol ? Number(r[loCol]) : NaN;
          const hi = hiCol ? Number(r[hiCol]) : NaN;
          const cat = String(r[catField] ?? "");
          const hasCI = Number.isFinite(lo) && Number.isFinite(hi);
          const [low, high] = hasCI ? (lo <= hi ? [lo, hi] : [hi, lo]) : [est, est];
          const spansZero = low <= 0 && high >= 0;
          const err = hasCI ? [Math.max(0, est - low), Math.max(0, high - est)] : undefined;
          const catPos: number | string = multi ? cats.indexOf(cat) + off : cat;
          return horizontal ? { x: est, y: catPos, err, spansZero } : { x: catPos, y: est, err, spansZero };
        });
        return { field: f, pts, color: colors[si % colors.length] };
      });
      const slotAxisProps = {
        type: "number" as const,
        domain: [-0.5, cats.length - 0.5] as [number, number],
        ticks: cats.map((_, i) => i),
        tickFormatter: (v: unknown) => xTick(cats[Math.round(Number(v))] ?? ""),
        allowDecimals: false,
      };
      return (
        <ResponsiveContainer width="100%" height={chartHeight}>
          <ScatterChart margin={cartesianMargin}>
            <CartesianGrid {...gridProps} />
            {horizontal ? (
              <>
                <XAxis type="number" dataKey="x" {...commonAxisProps} dy={6} tickFormatter={yTick} />
                {multi
                  ? <YAxis dataKey="y" {...commonAxisProps} width={80} {...slotAxisProps} />
                  : <YAxis type="category" dataKey="y" {...commonAxisProps} width={80} tickFormatter={xTick} />}
              </>
            ) : (
              <>
                {multi
                  ? <XAxis dataKey="x" {...commonAxisProps} dy={6} {...slotAxisProps} {...xLabelProps} />
                  : <XAxis type="category" dataKey="x" {...commonAxisProps} dy={6} tickFormatter={xTick} {...xLabelProps} />}
                <YAxis type="number" dataKey="y" {...autoWidthYAxisProps} tickFormatter={yTick} />
              </>
            )}
            <ZAxis range={[36, 36]} />
            <Tooltip content={cartesianTooltip} cursor={{ strokeDasharray: "3 3" }} />
            {multi && <Legend wrapperStyle={AXIS_STYLE} iconSize={7} />}
            {horizontal
              ? <ReferenceLine x={0} stroke={REF_LINE_COLOR} strokeDasharray="4 4" />
              : <ReferenceLine y={0} stroke={REF_LINE_COLOR} strokeDasharray="4 4" />}
            {forestSeries.map((s) => (
              <Scatter key={s.field} name={s.field} data={s.pts} fill={multi ? s.color : FOREST_SIG} isAnimationActive={false}>
                {!multi && s.pts.map((p, pi) => <Cell key={pi} fill={p.spansZero ? FOREST_NOSIG : FOREST_SIG} />)}
                <ErrorBar dataKey="err" width={4} strokeWidth={1.5} stroke={multi ? s.color : "#374151"} direction={horizontal ? "x" : "y"} />
              </Scatter>
            ))}
            {renderRefs(widget)}
          </ScatterChart>
        </ResponsiveContainer>
      );
    }

    default:
      return <div className="text-[var(--dac-text-muted)] text-xs">Unknown chart: {widget.chart}</div>;
  }
}
