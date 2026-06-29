import { useContext } from "react";
import type { WidgetFrameProps } from "../../types/template";
import { useTemplate } from "../TemplateProvider";
import { RowHeightContext } from "../RowContext";
import { QueryInfo } from "../../components/widgets/QueryInfo";
import { WidgetExportButton } from "../../components/widgets/WidgetExportButton";

const containerClass: Record<string, string> = {
  metric: "py-3 px-4 h-full dac-metric-border flex flex-col",
  chart: "py-3 px-4 h-full border border-[var(--dac-border)] rounded",
  table: "py-3 h-full border border-[var(--dac-border)] rounded overflow-hidden",
  text: "py-3 h-full",
  divider: "py-2 h-full flex items-center",
  image: "py-3 h-full",
};

export function BruinWidgetFrame({ widget, data, isLoading }: WidgetFrameProps) {
  const { MetricWidget, ChartWidget, TableWidget, TextWidget } = useTemplate();
  const rowHeight = useContext(RowHeightContext);
  const chartSkeletonHeight = rowHeight !== undefined ? Math.max(80, rowHeight - 60) : 240;

  // Divider: just a horizontal line, no title or data.
  if (widget.type === "divider") {
    return (
      <div className={containerClass.divider}>
        <hr className="w-full border-t border-[var(--dac-border)]" />
      </div>
    );
  }

  // Image: render an img tag with optional title.
  if (widget.type === "image") {
    return (
      <div className={containerClass.image}>
        {widget.name && (
          <div className="text-[11px] font-medium uppercase tracking-wider text-[var(--dac-text-muted)] mb-1.5">
            {widget.name}
          </div>
        )}
        <img
          src={widget.src}
          alt={widget.alt ?? widget.name ?? ""}
          className="max-w-full rounded"
        />
      </div>
    );
  }

  const isTable = widget.type === "table";
  const isExportable = widget.type === "chart" || widget.type === "table";
  const canExport = isExportable && !isLoading;

  return (
    <div data-dac-widget-frame className={`group ${containerClass[widget.type] ?? containerClass.text}`}>
      {widget.type !== "text" && (
        <div className={`flex items-center text-[11px] font-medium uppercase tracking-wider text-[var(--dac-text-muted)] ${widget.description ? "mb-0.5" : "mb-1.5"} ${isTable ? "px-4" : ""}`}>
          <span>{widget.name}</span>
          {(canExport || data?.query) && (
            <span className="ml-auto inline-flex items-center gap-1">
              {canExport && <WidgetExportButton widget={widget} data={data} />}
              {data?.query && <QueryInfo query={data.query} />}
            </span>
          )}
        </div>
      )}
      {widget.type !== "text" && widget.description && (
        <div className={`text-[11px] leading-snug text-[var(--dac-text-muted)] opacity-70 mb-1.5 ${isTable ? "px-4" : ""}`}>
          {widget.description}
        </div>
      )}

      {data?.error && (
        <div className={`text-xs text-[var(--dac-error)] font-mono mt-1 ${isTable ? "px-4" : ""}`}>{data.error}</div>
      )}

      {!data && isLoading && <LoadingSkeleton type={widget.type} chartHeight={chartSkeletonHeight} />}

      {data && !data.error && (
        <>
          {widget.type === "metric" && <div className="mt-auto"><MetricWidget widget={widget} data={data} /></div>}
          {widget.type === "chart" && <ChartWidget widget={widget} data={data} />}
          {widget.type === "table" && <TableWidget widget={widget} data={data} />}
        </>
      )}
      {widget.type === "text" && <TextWidget widget={widget} />}
      {!data && !isLoading && !["text", "divider", "image"].includes(widget.type) && (
        <div className={`text-xs text-[var(--dac-text-muted)] ${isTable ? "px-4" : ""}`}>No data</div>
      )}
    </div>
  );
}

function LoadingSkeleton({ type, chartHeight = 240 }: { type: string; chartHeight?: number }) {
  if (type === "metric") {
    return <div className="skeleton h-8 w-24 mt-1" />;
  }
  if (type === "chart") {
    return <div className="skeleton w-full mt-2 rounded" style={{ height: `${chartHeight}px` }} />;
  }
  if (type === "table") {
    return (
      <div className="mt-2 space-y-1.5 px-4">
        <div className="skeleton h-6 w-full" />
        <div className="skeleton h-5 w-full" />
        <div className="skeleton h-5 w-full" />
        <div className="skeleton h-5 w-3/4" />
      </div>
    );
  }
  return <div className="skeleton h-8 w-full" />;
}
