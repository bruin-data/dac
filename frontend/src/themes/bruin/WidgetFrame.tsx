import { useContext, useState } from "react";
import Markdown from "react-markdown";
import type { WidgetFrameProps } from "../../types/template";
import { useTemplate } from "../TemplateProvider";
import { RowHeightContext } from "../RowContext";
import { InfoTooltip } from "../../components/InfoTooltip";
import { QueryInfo } from "../../components/widgets/QueryInfo";
import { WidgetExportButton } from "../../components/widgets/WidgetExportButton";
import { CrossFilterContext, CrossFilterWidgetContext } from "../../hooks/useCrossFilter";
import { dashboardImageSrc } from "../../lib/dashboardImage";

const containerClass: Record<string, string> = {
  metric: "py-3 px-4 h-full dac-metric-border flex flex-col",
  chart: "py-3 px-4 h-full border border-[var(--dac-border)] rounded",
  table: "py-3 h-full border border-[var(--dac-border)] rounded overflow-hidden",
  text: "py-3 h-full",
  divider: "py-2 h-full flex items-center",
  image: "py-3 px-4 h-full border border-[var(--dac-border)] rounded",
};

export function BruinWidgetFrame({ widget, data, isLoading }: WidgetFrameProps) {
  const { MetricWidget, ChartWidget, TableWidget, TextWidget } = useTemplate();
  const rowHeight = useContext(RowHeightContext);
  const crossFilter = useContext(CrossFilterContext);
  const widgetId = useContext(CrossFilterWidgetContext);
  const resetNames = Object.entries(crossFilter?.sources ?? {})
    .filter(([, source]) => source.widgetId === widgetId).map(([name]) => name);
  const chartSkeletonHeight = rowHeight !== undefined ? Math.max(80, rowHeight - 60) : 240;

  // Divider: just a horizontal line, no title or data.
  if (widget.type === "divider") {
    return (
      <div className={containerClass.divider}>
        <hr className="w-full border-t border-[var(--dac-border)]" />
      </div>
    );
  }

  // Image: data-driven like a table — one image per result row. src/title/caption/alt
  // name the columns to read; fit is a literal applied to every image.
  if (widget.type === "image") {
    const cols = data?.columns ?? [];
    const idx = (name?: string) => (name ? cols.findIndex((c) => c.name === name) : -1);
    const si = idx(widget.src);
    const ti = idx(widget.title);
    const ci = idx(widget.caption);
    const ai = idx(widget.alt);
    const images =
      si < 0
        ? []
        : (data?.rows ?? [])
            .map((r) => ({
              src: r[si],
              safeSrc: dashboardImageSrc(r[si]),
              title: ti < 0 ? "" : r[ti],
              caption: ci < 0 ? "" : r[ci],
              alt: ai < 0 ? undefined : r[ai],
            }))
            .filter((im) => im.src != null && im.src !== "");
    const single = images.length === 1;
    return (
      <div className={`${containerClass.image} flex flex-col`}>
        {(widget.name || widget.description) && (
          <div className="flex shrink-0 items-center text-[11px] font-medium uppercase tracking-wider text-[var(--dac-text-muted)] mb-1.5">
            <span>{widget.name}</span>
            {widget.description && <InfoTooltip description={widget.description} name={widget.name} />}
          </div>
        )}
        {data?.error && (
          <div className="text-xs text-[var(--dac-error)] font-mono mt-1">{data.error}</div>
        )}
        {!data && isLoading && <LoadingSkeleton type={widget.type} chartHeight={chartSkeletonHeight} />}
        {!data?.error && !isLoading && images.length === 0 && (
          <div className="text-xs text-[var(--dac-text-muted)]">No data</div>
        )}
        {images.length > 0 && (
          <div className="flex-1 min-h-0 overflow-x-auto">
            <div className="flex gap-3 h-full">
              {images.map((img, i) => (
                <div key={`${i}:${String(img.src)}`} className={`flex flex-col h-full ${single ? "flex-1 min-w-0" : "shrink-0 w-56"}`}>
                  {img.title != null && img.title !== "" && (
                    <div className="text-[15px] font-semibold text-[var(--dac-text-primary)] mb-2 truncate">{String(img.title)}</div>
                  )}
                  <div className="flex-1 min-h-0 flex items-center justify-center overflow-hidden">
                    <DashboardImage
                      src={img.safeSrc}
                      alt={String(img.alt ?? widget.name ?? "")}
                      className={`rounded max-h-[320px] ${widget.fit === "cover" ? "w-full h-full object-cover" : "max-w-full object-contain"}`}
                    />
                  </div>
                  {img.caption != null && img.caption !== "" && (
                    <div className="dac-prose text-[13px] text-[var(--dac-text-secondary)] mt-2">
                      <Markdown
                        components={{
                          img: ({ src, alt, title }) => {
                            const safeSrc = dashboardImageSrc(src);
                            return safeSrc ? (
                              <img src={safeSrc} alt={alt ?? ""} title={title} loading="lazy" referrerPolicy="no-referrer" />
                            ) : null;
                          },
                        }}
                      >
                        {String(img.caption)}
                      </Markdown>
                    </div>
                  )}
                </div>
              ))}
            </div>
          </div>
        )}
      </div>
    );
  }

  const isTable = widget.type === "table" || widget.type === "pivot_table";
  const isVegaChart = widget.type === "chart" && widget.chart === "vega-lite";
  const isExportable = widget.type === "chart" || isTable;
  const canExport = isExportable && !isLoading;

  return (
    <div data-dac-widget-frame className={`group ${containerClass[widget.type] ?? (isTable ? containerClass.table : containerClass.text)} ${isVegaChart || (isTable && !!widget.grandTotal) ? "min-h-0 min-w-0 flex flex-col" : ""}`}>
      {widget.type !== "text" && (
        <div data-dac-reset className={`flex shrink-0 items-center text-[11px] font-medium uppercase tracking-wider text-[var(--dac-text-muted)] mb-1.5 ${isTable ? "px-4" : ""}`}>
          <span>{widget.name}</span>
          {widget.description && <InfoTooltip description={widget.description} name={widget.name} />}
          {widgetId && crossFilter?.reset && resetNames.length > 0 && (
            <button type="button" data-dac-control data-dac-export-control
              title="Clear this widget's selection" aria-label="Clear this widget's selection"
              className="ml-1 inline-flex h-5 w-5 shrink-0 items-center justify-center rounded-sm text-[var(--dac-text-muted)] hover:text-[var(--dac-text-primary)] hover:bg-[var(--dac-surface-hover)] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[var(--dac-accent)]"
              onClick={(event) => {
                event.stopPropagation();
                crossFilter.reset?.(resetNames);
              }}>
              <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.75" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
                <path d="M3 4h18l-7 8v6l-4 2v-8L3 4Z" />
                <circle cx="19" cy="18" r="5" fill="var(--dac-surface)" stroke="var(--dac-surface)" strokeWidth="3" />
                <circle cx="19" cy="18" r="4.3" fill="var(--dac-surface)" strokeWidth="1.35" />
                <path d="m17.5 16.5 3 3m0-3-3 3" strokeWidth="1.25" />
              </svg>
            </button>
          )}
          {(canExport || data?.query) && (
            <span className="ml-auto inline-flex items-center gap-1">
              {canExport && <WidgetExportButton widget={widget} data={data} />}
              {data?.query && <QueryInfo query={data.query} />}
            </span>
          )}
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
          {isTable && <TableWidget widget={widget} data={data} />}
        </>
      )}
      {widget.type === "text" && <TextWidget widget={widget} />}
      {!data && !isLoading && !["text", "divider", "image"].includes(widget.type) && (
        <div className={`text-xs text-[var(--dac-text-muted)] ${isTable ? "px-4" : ""}`}>No data</div>
      )}
    </div>
  );
}

function DashboardImage({ src, alt, className }: { src: string; alt: string; className: string }) {
  const [failed, setFailed] = useState(false);

  if (!src || failed) {
    return (
      <span className="text-xs text-[var(--dac-text-muted)]">
        {src ? "Image failed to load" : "Unsupported image URL"}
      </span>
    );
  }

  return (
    <img
      src={src}
      alt={alt}
      loading="lazy"
      referrerPolicy="no-referrer"
      className={className}
      onError={() => setFailed(true)}
    />
  );
}

function LoadingSkeleton({ type, chartHeight = 240 }: { type: string; chartHeight?: number }) {
  if (type === "metric") {
    return <div className="skeleton h-8 w-24 mt-1" />;
  }
  if (type === "chart") {
    return <div className="skeleton w-full mt-2 rounded" style={{ height: `${chartHeight}px` }} />;
  }
  if (type === "table" || type === "pivot_table") {
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
