import { useContext, useMemo, useState } from "react";
import {
  LineChart, Line, BarChart, Bar, AreaChart, Area, PieChart, Pie, Cell,
  ScatterChart, Scatter, ZAxis,
  ComposedChart, FunnelChart, Funnel, LabelList, Sankey,
  Treemap,
  RadarChart, Radar, PolarGrid, PolarAngleAxis, PolarRadiusAxis,
  XAxis, YAxis, CartesianGrid, Tooltip, ResponsiveContainer, Legend,
} from "recharts";
import type { TreemapNode } from "recharts";
import type { Widget, WidgetData } from "../../types/dashboard";
import { useTokens } from "../../themes/TemplateProvider";
import { RowHeightContext } from "../../themes/RowContext";

const DEFAULT_CHART_HEIGHT = 240;
// Approx pixels consumed by the widget frame's title/padding above the chart.
const FRAME_OVERHEAD = 60;

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

function CustomTooltip({ active, payload, label }: {
  active?: boolean;
  payload?: TooltipPayloadEntry[];
  label?: unknown;
}) {
  if (!active || !payload?.length) return null;
  return (
    <div className="dac-tooltip">
      <div className="dac-tooltip-label">{formatAxisTick(label)}</div>
      {payload.map((p, i) => (
        <div key={i} className="dac-tooltip-row">
          <span className="dac-tooltip-dot" style={{ background: p.color ?? p.fill }} />
          <span className="dac-tooltip-name">{p.name ?? p.dataKey}</span>
          <span className="dac-tooltip-value">{formatTooltipValue(p.value)}</span>
        </div>
      ))}
    </div>
  );
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

function HeatmapChart({
  data,
  xKey,
  yKey,
  valueKey,
  colors,
  axisColor,
}: {
  data: Record<string, unknown>[];
  xKey: string;
  yKey: string;
  valueKey: string;
  colors: string[];
  axisColor: string;
}) {
  const [hover, setHover] = useState<{ x: number; y: number; xLabel: string; yLabel: string; value: number } | null>(null);

  const { cells, xLabels, yLabels, maxVal } = useMemo(() => {
    const xs = [...new Set(data.map((d) => String(d[xKey])))];
    const ys = [...new Set(data.map((d) => String(d[yKey])))];
    let maxV = 0;
    const cellMap = new Map<string, number>();
    for (const d of data) {
      const val = Number(d[valueKey]) || 0;
      cellMap.set(`${d[xKey]}_${d[yKey]}`, val);
      if (val > maxV) maxV = val;
    }
    return { cells: cellMap, xLabels: xs, yLabels: ys, maxVal: maxV };
  }, [data, xKey, yKey, valueKey]);

  const cellW = Math.max(20, Math.min(50, 600 / xLabels.length));
  const cellH = Math.max(16, Math.min(30, 200 / yLabels.length));
  const leftPad = 60;
  const topPad = 24;
  const width = leftPad + xLabels.length * cellW;
  const height = topPad + yLabels.length * cellH;
  const baseColor = colors[0] ?? "#4338CA";

  return (
    <svg width="100%" viewBox={`0 0 ${width} ${height}`} style={{ maxHeight: 240 }}
      onMouseLeave={() => setHover(null)}>
      {xLabels.map((x, i) => (
        <text key={`x-${i}`} x={leftPad + i * cellW + cellW / 2} y={14} fill={axisColor} fontSize={9} fontFamily='"Geist", system-ui' textAnchor="middle">
          {formatAxisTick(x)}
        </text>
      ))}
      {yLabels.map((y, j) => (
        <text key={`y-${j}`} x={leftPad - 4} y={topPad + j * cellH + cellH / 2 + 3} fill={axisColor} fontSize={9} fontFamily='"Geist", system-ui' textAnchor="end">
          {String(y).slice(0, 8)}
        </text>
      ))}
      {xLabels.map((x, i) =>
        yLabels.map((y, j) => {
          const val = cells.get(`${x}_${y}`) ?? 0;
          const intensity = maxVal > 0 ? val / maxVal : 0;
          const opacity = val === 0 ? 0.04 : 0.12 + intensity * 0.88;
          const rx = leftPad + i * cellW + 1;
          const ry = topPad + j * cellH + 1;
          return (
            <rect
              key={`${i}-${j}`}
              x={rx}
              y={ry}
              width={cellW - 2}
              height={cellH - 2}
              rx={2}
              fill={baseColor}
              opacity={opacity}
              onMouseEnter={() => setHover({ x: rx + cellW, y: ry + cellH / 2, xLabel: x, yLabel: y, value: val })}
              onMouseLeave={() => setHover(null)}
            />
          );
        }),
      )}
      {hover && (
        <SvgTooltip x={hover.x} y={hover.y}
          lines={[`${formatAxisTick(hover.xLabel)}, ${hover.yLabel}`, formatTooltipValue(hover.value)]} />
      )}
    </svg>
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

// --- Main chart component ---

export function ChartWidget({ widget, data }: Props) {
  const tokens = useTokens();
  const rowHeight = useContext(RowHeightContext);
  const chartHeight = rowHeight !== undefined
    ? Math.max(80, rowHeight - FRAME_OVERHEAD)
    : DEFAULT_CHART_HEIGHT;

  if (!data?.rows?.length) {
    return <div className="text-[var(--dac-text-muted)] text-xs py-6 text-center">No data</div>;
  }

  const chartData = toChartData(data);
  const colors = CHART_COLORS.map((key) => tokens[key] || "#888");
  const gridColor = tokens["border"];
  const axisColor = tokens["text-muted"];

  const commonAxisProps = {
    tick: { ...AXIS_STYLE, fill: axisColor },
    axisLine: false,
    tickLine: false,
  };

  const cartesianMargin = { top: 4, right: 8, bottom: 4, left: -4 };
  const gridProps = { vertical: false, stroke: gridColor, strokeOpacity: 0.5, strokeDasharray: "3 3" };

  switch (widget.chart) {
    case "line":
      return (
        <ResponsiveContainer width="100%" height={chartHeight}>
          <LineChart data={chartData} margin={cartesianMargin}>
            <CartesianGrid {...gridProps} />
            <XAxis dataKey={widget.x} {...commonAxisProps} dy={6} tickFormatter={formatAxisTick} />
            <YAxis {...commonAxisProps} dx={-4} tickFormatter={formatYTick} />
            <Tooltip content={<CustomTooltip />} />
            {widget.y?.map((field, i) => (
              <Line
                key={field}
                type="monotone"
                dataKey={field}
                stroke={colors[i % colors.length]}
                strokeWidth={1.5}
                dot={false}
                activeDot={{ r: 3, strokeWidth: 0 }}
                isAnimationActive={false}
              />
            ))}
          </LineChart>
        </ResponsiveContainer>
      );

    case "bar": {
      const yFields = widget.y ?? [];
      const isStacked = widget.stacked && yFields.length > 1;
      return (
        <ResponsiveContainer width="100%" height={chartHeight}>
          <BarChart data={chartData} margin={cartesianMargin}>
            <CartesianGrid {...gridProps} />
            <XAxis dataKey={widget.x} {...commonAxisProps} dy={6} tickFormatter={formatAxisTick} />
            <YAxis {...commonAxisProps} dx={-4} tickFormatter={formatYTick} />
            <Tooltip content={<CustomTooltip />} cursor={{ fill: gridColor, fillOpacity: 0.2 }} />
            {isStacked && <Legend wrapperStyle={AXIS_STYLE} iconSize={7} />}
            {yFields.map((field, i) => (
              <Bar
                key={field}
                dataKey={field}
                fill={colors[i % colors.length]}
                stackId={isStacked ? "stack" : undefined}
                radius={isStacked && i < yFields.length - 1 ? undefined : [2, 2, 0, 0]}
                isAnimationActive={false}
              />
            ))}
          </BarChart>
        </ResponsiveContainer>
      );
    }

    case "area": {
      const yFields = widget.y ?? [];
      const isStacked = widget.stacked && yFields.length > 1;
      return (
        <ResponsiveContainer width="100%" height={chartHeight}>
          <AreaChart data={chartData} margin={cartesianMargin}>
            <CartesianGrid {...gridProps} />
            <XAxis dataKey={widget.x} {...commonAxisProps} dy={6} tickFormatter={formatAxisTick} />
            <YAxis {...commonAxisProps} dx={-4} tickFormatter={formatYTick} />
            <Tooltip content={<CustomTooltip />} />
            {yFields.map((field, i) => (
              <Area
                key={field}
                type="monotone"
                dataKey={field}
                stroke={colors[i % colors.length]}
                fill={colors[i % colors.length]}
                fillOpacity={0.06}
                strokeWidth={1.5}
                stackId={isStacked ? "stack" : undefined}
                isAnimationActive={false}
              />
            ))}
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
              dataKey={widget.value || "value"}
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
      const xIsNumeric = chartData.length > 0 && typeof chartData[0][widget.x!] === "number";
      const yIsNumeric = chartData.length > 0 && typeof chartData[0][widget.y?.[0] ?? ""] === "number";
      return (
        <ResponsiveContainer width="100%" height={chartHeight}>
          <ScatterChart margin={cartesianMargin}>
            <CartesianGrid {...gridProps} />
            <XAxis dataKey={widget.x} name={widget.x} {...commonAxisProps} dy={6}
              tickFormatter={formatAxisTick}
              type={xIsNumeric ? "number" : "category"} allowDuplicatedCategory={false} />
            <YAxis dataKey={widget.y?.[0]} name={widget.y?.[0]} {...commonAxisProps} dx={-4}
              tickFormatter={formatYTick} type={yIsNumeric ? "number" : "category"} />
            <Tooltip content={<CustomTooltip />} />
            <Scatter data={chartData} fill={colors[0]} r={3} isAnimationActive={false} />
          </ScatterChart>
        </ResponsiveContainer>
      );
    }

    case "bubble": {
      const xIsNumeric = chartData.length > 0 && typeof chartData[0][widget.x!] === "number";
      const yIsNumeric = chartData.length > 0 && typeof chartData[0][widget.y?.[0] ?? ""] === "number";
      return (
        <ResponsiveContainer width="100%" height={chartHeight}>
          <ScatterChart margin={cartesianMargin}>
            <CartesianGrid {...gridProps} />
            <XAxis dataKey={widget.x} name={widget.x} {...commonAxisProps} dy={6}
              type={xIsNumeric ? "number" : "category"} allowDuplicatedCategory={false}
              tickFormatter={formatAxisTick} />
            <YAxis dataKey={widget.y?.[0]} name={widget.y?.[0]} {...commonAxisProps} dx={-4}
              tickFormatter={formatYTick} type={yIsNumeric ? "number" : "category"} />
            <ZAxis dataKey={widget.size} range={[40, 500]} name={widget.size} />
            <Tooltip content={<CustomTooltip />} />
            <Scatter data={chartData} fill={colors[0]} fillOpacity={0.6} isAnimationActive={false} />
          </ScatterChart>
        </ResponsiveContainer>
      );
    }

    case "combo": {
      const yFields = widget.y ?? [];
      const lineSet = new Set(widget.lines ?? []);
      return (
        <ResponsiveContainer width="100%" height={chartHeight}>
          <ComposedChart data={chartData} margin={cartesianMargin}>
            <CartesianGrid {...gridProps} />
            <XAxis dataKey={widget.x} {...commonAxisProps} dy={6} tickFormatter={formatAxisTick} />
            <YAxis {...commonAxisProps} dx={-4} tickFormatter={formatYTick} />
            <Tooltip content={<CustomTooltip />} />
            <Legend wrapperStyle={AXIS_STYLE} iconSize={7} />
            {yFields.map((field, i) =>
              lineSet.has(field) ? (
                <Line
                  key={field}
                  type="monotone"
                  dataKey={field}
                  stroke={colors[i % colors.length]}
                  strokeWidth={1.5}
                  dot={false}
                  isAnimationActive={false}
                />
              ) : (
                <Bar
                  key={field}
                  dataKey={field}
                  fill={colors[i % colors.length]}
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
      const histData = buildHistogramData(chartData, widget.x!, widget.bins || 10);
      return (
        <ResponsiveContainer width="100%" height={chartHeight}>
          <BarChart data={histData} margin={cartesianMargin}>
            <CartesianGrid {...gridProps} />
            <XAxis dataKey="bin" {...commonAxisProps} dy={6} angle={-30} textAnchor="end" height={50} />
            <YAxis {...commonAxisProps} dx={-4} tickFormatter={formatYTick} />
            <Tooltip content={<CustomTooltip />} />
            <Bar dataKey="count" fill={colors[0]} radius={[2, 2, 0, 0]} isAnimationActive={false} />
          </BarChart>
        </ResponsiveContainer>
      );
    }

    case "boxplot": {
      const yField = widget.y?.[0] ?? "value";
      const boxData = buildBoxplotData(chartData, widget.x!, yField);
      return (
        <ResponsiveContainer width="100%" height={chartHeight}>
          <ComposedChart data={boxData} margin={cartesianMargin}>
            <CartesianGrid {...gridProps} />
            <XAxis dataKey="category" {...commonAxisProps} dy={6} />
            <YAxis {...commonAxisProps} dx={-4} tickFormatter={formatYTick} />
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
        <ResponsiveContainer width="100%" height={chartHeight}>
          <FunnelChart>
            <Tooltip content={<CustomTooltip />} />
            <Funnel
              data={chartData.map((d, i) => ({
                ...d,
                fill: colors[i % colors.length],
              }))}
              dataKey={widget.value || "value"}
              nameKey={widget.label || "label"}
              isAnimationActive={false}
            >
              <LabelList
                position="center"
                fill="#fff"
                style={{ fontSize: 11, fontFamily: '"Geist", system-ui', fontWeight: 500 }}
                formatter={(v: unknown) => formatTooltipValue(v)}
              />
            </Funnel>
          </FunnelChart>
        </ResponsiveContainer>
      );

    case "sankey": {
      const sankeyData = buildSankeyData(
        chartData,
        widget.source || "source",
        widget.target || "target",
        widget.value || "value",
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
          xKey={widget.x!}
          yKey={widget.y?.[0] ?? "y"}
          valueKey={widget.value || "value"}
          colors={colors}
          axisColor={axisColor}
        />
      );

    case "calendar":
      return (
        <CalendarHeatmap
          data={chartData}
          dateKey={widget.x!}
          valueKey={widget.value || "value"}
          colors={colors}
          axisColor={axisColor}
        />
      );

    case "sparkline":
      return (
        <ResponsiveContainer width="100%" height={60}>
          <LineChart data={chartData} margin={{ top: 4, right: 4, bottom: 4, left: 4 }}>
            {widget.y?.map((field, i) => (
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
      const wfData = buildWaterfallData(chartData, widget.x!, widget.y?.[0] ?? "value");
      return (
        <ResponsiveContainer width="100%" height={chartHeight}>
          <BarChart data={wfData} margin={cartesianMargin}>
            <CartesianGrid {...gridProps} />
            <XAxis dataKey="name" {...commonAxisProps} dy={6} tickFormatter={formatAxisTick} />
            <YAxis {...commonAxisProps} dx={-4} tickFormatter={formatYTick} />
            <Tooltip content={<CustomTooltip />} />
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
      const yField = widget.y?.[0] ?? "value";
      return (
        <ResponsiveContainer width="100%" height={chartHeight}>
          <LineChart data={chartData} margin={cartesianMargin}>
            <CartesianGrid {...gridProps} />
            <XAxis dataKey={widget.x} {...commonAxisProps} dy={6} tickFormatter={formatAxisTick} />
            <YAxis {...commonAxisProps} dx={-4} tickFormatter={formatYTick} />
            <Tooltip content={<CustomTooltip />} />
            <Line type="monotone" dataKey={yField} stroke={colors[0]} strokeWidth={1.5} dot={{ r: 2, strokeWidth: 0, fill: colors[0] }} isAnimationActive={false} />
            {widget.yMin && (
              <Line type="monotone" dataKey={widget.yMin} stroke={colors[3] ?? "#DC2626"} strokeWidth={1} strokeDasharray="4 4" dot={false} isAnimationActive={false} />
            )}
            {widget.yMax && (
              <Line type="monotone" dataKey={widget.yMax} stroke={colors[3] ?? "#DC2626"} strokeWidth={1} strokeDasharray="4 4" dot={false} isAnimationActive={false} />
            )}
            {widget.y && widget.y.length > 1 && (
              <Line type="monotone" dataKey={widget.y[1]} stroke={colors[1]} strokeWidth={1} strokeDasharray="6 3" dot={false} isAnimationActive={false} />
            )}
          </LineChart>
        </ResponsiveContainer>
      );
    }

    case "dumbbell":
      return (
        <DumbbellChart
          data={chartData}
          xKey={widget.x!}
          yFields={widget.y ?? []}
          colors={colors}
          axisColor={axisColor}
          gridColor={gridColor}
        />
      );

    case "gauge": {
      const valueKey = widget.value || "value";
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
      const valueKey = widget.value || "value";
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
      const yFields = widget.y ?? [];
      return (
        <ResponsiveContainer width="100%" height={240}>
          <RadarChart data={chartData} margin={{ top: 8, right: 24, bottom: 8, left: 24 }}>
            <PolarGrid stroke={gridColor} strokeOpacity={0.5} />
            <PolarAngleAxis dataKey={widget.x} tick={{ ...AXIS_STYLE, fill: axisColor }} />
            <PolarRadiusAxis tick={{ ...AXIS_STYLE, fill: axisColor }} axisLine={false} tickFormatter={formatYTick} />
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
          xKey={widget.x!}
          openKey={widget.open || "open"}
          highKey={widget.high || "high"}
          lowKey={widget.low || "low"}
          closeKey={widget.close || "close"}
          colors={colors}
          axisColor={axisColor}
          gridColor={gridColor}
        />
      );

    default:
      return <div className="text-[var(--dac-text-muted)] text-xs">Unknown chart: {widget.chart}</div>;
  }
}
