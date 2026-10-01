import { useMemo, useState, type CSSProperties, type PointerEvent as ReactPointerEvent } from "react";
import { createPortal } from "react-dom";
import { Curve } from "recharts";
import { useTokens } from "../../themes/TemplateProvider";
import type { SparklinePoint } from "./sparkline";

interface Props {
  points: SparklinePoint[];
  yDomain?: readonly [number, number];
  beginAtZero?: boolean;
  formatX?: (value: string | number) => string;
  formatY?: (value: number) => string;
}

interface HoverPoint {
  index: number;
  clientX: number;
  clientY: number;
}

const WIDTH = 132;
const HEIGHT = 30;
const PADDING = 2;
// Half the tooltip's max width (max-w-64 plus padding), used to keep it on screen.
const TOOLTIP_HALF_WIDTH = 136;
const TOOLTIP_HEIGHT = 48;

export function SparklineCell({
  points,
  yDomain,
  beginAtZero = false,
  formatX = String,
  formatY = String,
}: Props) {
  const [hover, setHover] = useState<HoverPoint | null>(null);
  const tokens = useTokens();
  // The tooltip portals to document.body, outside TemplateProvider's token
  // scope, so copy the active theme variables onto it.
  const tooltipTheme = useMemo(
    () => Object.fromEntries(Object.entries(tokens).map(([key, value]) => [`--dac-${key}`, value])) as CSSProperties,
    [tokens],
  );

  const geometry = useMemo(() => {
    if (!points.length) return [];
    let min = yDomain?.[0] ?? points[0].y;
    let max = yDomain?.[1] ?? points[0].y;
    if (!yDomain) {
      for (let i = 1; i < points.length; i++) {
        min = Math.min(min, points[i].y);
        max = Math.max(max, points[i].y);
      }
    }
    if (beginAtZero) {
      min = Math.min(0, min);
      max = Math.max(0, max);
    }
    const span = max - min;
    const innerWidth = WIDTH - PADDING * 2;
    const innerHeight = HEIGHT - PADDING * 2;
    return points.map((point, index) => ({
      x: PADDING + (points.length === 1 ? innerWidth / 2 : (index / (points.length - 1)) * innerWidth),
      y: span === 0 ? HEIGHT / 2 : PADDING + ((max - point.y) / span) * innerHeight,
    }));
  }, [beginAtZero, points, yDomain]);
  if (!points.length) {
    return <span aria-label="No sparkline data" className="text-[var(--dac-text-muted)]">—</span>;
  }

  const active = hover ? geometry[hover.index] : null;
  const activePoint = hover ? points[hover.index] : null;
  const latest = points[points.length - 1];

  const updateHover = (event: ReactPointerEvent<SVGSVGElement>) => {
    const rect = event.currentTarget.getBoundingClientRect();
    if (rect.width <= 0) return;
    const ratio = Math.min(1, Math.max(0, (event.clientX - rect.left) / rect.width));
    const index = Math.round(ratio * (points.length - 1));
    setHover({ index, clientX: event.clientX, clientY: event.clientY });
  };

  return (
    <div
      className="relative h-8 min-w-0 w-full"
      role="img"
      aria-label={`Sparkline with ${points.length} points. Latest: ${formatX(latest.x)}, ${formatY(latest.y)}`}
    >
      <svg
        viewBox={`0 0 ${WIDTH} ${HEIGHT}`}
        preserveAspectRatio="none"
        className="block h-full w-full touch-pan-x touch-pan-y"
        onPointerMove={updateHover}
        onPointerLeave={() => setHover(null)}
        onPointerCancel={() => setHover(null)}
      >
        {geometry.length > 1 && (
          <Curve
            type="monotoneX"
            points={geometry}
            fill="none"
            stroke="var(--dac-chart-1)"
            strokeWidth="1.75"
            strokeLinecap="round"
            strokeLinejoin="round"
            vectorEffect="non-scaling-stroke"
          />
        )}
        {active && (
          <line
            x1={active.x}
            x2={active.x}
            y1={PADDING}
            y2={HEIGHT - PADDING}
            stroke="var(--dac-text-muted)"
            strokeWidth="1"
            strokeDasharray="2 2"
            vectorEffect="non-scaling-stroke"
          />
        )}
      </svg>
      {geometry.length === 1 && !active && <SparklineDot x={geometry[0].x} y={geometry[0].y} size={5} />}
      {active && <SparklineDot x={active.x} y={active.y} size={6} ring />}
      {hover && activePoint && typeof document !== "undefined" && createPortal(
        <div
          role="tooltip"
          className={`pointer-events-none fixed z-[100] -translate-x-1/2 rounded border border-[var(--dac-border)] bg-[var(--dac-background)] px-2 py-1 text-left text-[11px] leading-4 shadow-lg ${
            hover.clientY < TOOLTIP_HEIGHT ? "" : "-translate-y-full"
          }`}
          style={{
            ...tooltipTheme,
            left: Math.min(
              Math.max(hover.clientX, TOOLTIP_HALF_WIDTH),
              Math.max(TOOLTIP_HALF_WIDTH, window.innerWidth - TOOLTIP_HALF_WIDTH),
            ),
            top: hover.clientY < TOOLTIP_HEIGHT ? hover.clientY + 16 : hover.clientY - 8,
          }}
        >
          <div className="max-w-64 truncate text-[var(--dac-text-secondary)]">{formatX(activePoint.x)}</div>
          <div className="font-mono tabular-nums text-[var(--dac-text-primary)]">{formatY(activePoint.y)}</div>
        </div>,
        document.body,
      )}
    </div>
  );
}

function SparklineDot({ x, y, size, ring = false }: { x: number; y: number; size: number; ring?: boolean }) {
  return (
    <span
      aria-hidden="true"
      className={`pointer-events-none absolute -translate-x-1/2 -translate-y-1/2 rounded-full bg-[var(--dac-chart-1)] ${
        ring ? "ring-[1.5px] ring-[var(--dac-background)]" : ""
      }`}
      style={{ left: `${(x / WIDTH) * 100}%`, top: `${(y / HEIGHT) * 100}%`, width: size, height: size }}
    />
  );
}
