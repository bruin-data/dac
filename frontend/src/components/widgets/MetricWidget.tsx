import { useLayoutEffect, useRef } from "react";
import type { Widget, WidgetData } from "../../types/dashboard";
import { buildFormatter } from "../../lib/format";

interface Props {
  widget: Widget;
  data?: WidgetData;
}

/** Pick the right font size so the value fits its container. */
function useAutoFit(text: string) {
  const containerRef = useRef<HTMLDivElement>(null);

  useLayoutEffect(() => {
    const el = containerRef.current;
    if (!el) return;

    const fit = () => {
      const maxPx = 32; // 2rem
      const minPx = 16; // 1rem — floor
      let size = maxPx;
      el.style.fontSize = `${size}px`;

      while (el.scrollWidth > el.clientWidth && size > minPx) {
        size -= 1;
        el.style.fontSize = `${size}px`;
      }
    };

    fit();

    if (typeof ResizeObserver === "undefined") return;
    const observer = new ResizeObserver(fit);
    observer.observe(el);
    return () => observer.disconnect();
  }, [text]);

  return containerRef;
}

export function MetricWidget({ widget, data }: Props) {
  const hasData = !!data?.rows?.length && !!data.columns?.length;

  const enc = widget.value;
  const colIdx = hasData && enc?.field ? data.columns.findIndex((c) => c.name === enc.field) : -1;
  const rawValue = hasData ? (colIdx >= 0 ? data.rows[0][colIdx] : data.rows[0][0]) : null;
  const formatted = hasData ? buildFormatter(enc)(rawValue) : "";

  // Hook must be called unconditionally (Rules of Hooks).
  const containerRef = useAutoFit(formatted);

  if (!hasData) {
    return (
      <div className="h-12">
        <div className="skeleton h-8 w-28" />
      </div>
    );
  }

  return (
    <div className="tabular-nums overflow-hidden">
      <div
        ref={containerRef}
        className="truncate text-[2rem] font-semibold leading-[1.1] tracking-tight text-[var(--dac-text-primary)]"
      >
        {formatted}
      </div>
    </div>
  );
}
