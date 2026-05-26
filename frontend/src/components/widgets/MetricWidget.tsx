import { useRef, useEffect, useState } from "react";
import type { Widget, WidgetData } from "../../types/dashboard";
import { formatValue } from "../../lib/formatValue";
import { valueField, valueFormat } from "../../lib/widgetValue";

interface Props {
  widget: Widget;
  data?: WidgetData;
}

/** Pick the right font size so the value fits its container. */
function useAutoFit(text: string) {
  const containerRef = useRef<HTMLDivElement>(null);
  const [fontSize, setFontSize] = useState<string>("clamp(1.35rem, 2.5vw, 2rem)");

  useEffect(() => {
    const el = containerRef.current;
    if (!el) return;

    // Reset to max size, then shrink if needed.
    const maxPx = 32; // 2rem
    const minPx = 16; // 1rem — floor
    let size = maxPx;
    el.style.fontSize = `${size}px`;

    // Shrink until text fits or we hit the floor.
    while (el.scrollWidth > el.clientWidth && size > minPx) {
      size -= 1;
      el.style.fontSize = `${size}px`;
    }

    setFontSize(`${size}px`);
  }, [text]);

  return { containerRef, fontSize };
}

export function MetricWidget({ widget, data }: Props) {
  const hasData = !!data?.rows?.length && !!data.columns?.length;

  const columnName = valueField(widget);
  const formatSpec = valueFormat(widget);
  const colIdx = hasData && columnName ? data.columns.findIndex((c) => c.name === columnName) : -1;
  const rawValue = hasData ? (colIdx >= 0 ? data.rows[0][colIdx] : data.rows[0][0]) : null;
  const formatted = hasData ? formatValue(rawValue, formatSpec) : "";

  const { containerRef, fontSize } = useAutoFit(formatted);

  if (!hasData) {
    return (
      <div className="h-12">
        <div className="skeleton h-8 w-28" />
      </div>
    );
  }

  return (
    <div className="tabular-nums overflow-hidden">
      <div ref={containerRef} className="flex items-baseline gap-1 whitespace-nowrap" style={{ fontSize, lineHeight: 1.1 }}>
        <span className="font-semibold tracking-tight text-[var(--dac-text-primary)]">
          {formatted}
        </span>
      </div>
    </div>
  );
}
