import { useContext, useEffect, useMemo, useRef, useState } from "react";
import type { View } from "vega";
import type { VisualizationSpec } from "vega-embed";
import type { Widget, WidgetData } from "../../types/dashboard";
import { DAC_EXPORT_PENDING_ATTRIBUTE } from "../../lib/renderExport";
import { RowHeightContext } from "../../themes/RowContext";
import { useTokens } from "../../themes/TemplateProvider";

const DAC_DATASET_NAME = "dac";
const VEGA_TOOLTIP_ID = "vg-tooltip-element";
const DEFAULT_CHART_HEIGHT = 240;
const FRAME_OVERHEAD = 60;

interface Props {
  widget: Widget;
  data?: WidgetData;
}

type JSONObject = Record<string, unknown>;

function isObject(value: unknown): value is JSONObject {
  return value !== null && typeof value === "object" && !Array.isArray(value);
}

function toRecords(data: WidgetData): JSONObject[] {
  return data.rows.map((row) => {
    const record: JSONObject = {};
    data.columns.forEach((column, index) => {
      record[column.name] = row[index];
    });
    return record;
  });
}

function hasExternalDataURL(value: unknown): boolean {
  if (Array.isArray(value)) return value.some(hasExternalDataURL);
  if (!isObject(value)) return false;

  if (isObject(value.data) && "url" in value.data) return true;
  return Object.values(value).some(hasExternalDataURL);
}

function mergeConfig(defaults: JSONObject, override: unknown): JSONObject {
  if (!isObject(override)) return defaults;

  const merged: JSONObject = { ...defaults, ...override };
  for (const key of ["axis", "legend", "title", "view", "range"]) {
    if (isObject(defaults[key]) && isObject(override[key])) {
      merged[key] = { ...defaults[key], ...override[key] };
    }
  }
  return merged;
}

// Vega Tooltip appends its element to document.body, outside TemplateProvider's
// token scope. Create the shared element up front and copy the active DAC theme
// variables onto it so its colors do not become invalid/transparent.
function syncTooltipTheme(tokens: Record<string, string>): void {
  let tooltip = document.getElementById(VEGA_TOOLTIP_ID);
  if (!tooltip) {
    tooltip = document.createElement("div");
    tooltip.id = VEGA_TOOLTIP_ID;
    tooltip.classList.add("vg-tooltip");
    tooltip.setAttribute("role", "tooltip");
    document.body.appendChild(tooltip);
  }

  for (const [key, value] of Object.entries(tokens)) {
    tooltip.style.setProperty(`--dac-${key}`, value);
  }
}

function themedSpec(
  source: JSONObject,
  records: JSONObject[],
  tokens: Record<string, string>,
  chartHeight: number,
): JSONObject {
  if (hasExternalDataURL(source)) {
    throw new Error("Vega-Lite data.url is not allowed; load data through the widget query");
  }

  const category = Array.from({ length: 8 }, (_, index) => tokens[`chart-${index + 1}`] || "#888888");
  const defaultConfig: JSONObject = {
    axis: {
      domain: false,
      gridColor: tokens.border,
      gridOpacity: 0.55,
      labelColor: tokens["text-muted"],
      labelFont: "Geist, system-ui, sans-serif",
      labelFontSize: 11,
      tickColor: tokens.border,
      titleColor: tokens["text-muted"],
      titleFont: "Geist, system-ui, sans-serif",
      titleFontSize: 11,
      titleFontWeight: 500,
    },
    legend: {
      labelColor: tokens["text-secondary"],
      labelFont: "Geist, system-ui, sans-serif",
      labelFontSize: 11,
      titleColor: tokens["text-primary"],
      titleFont: "Geist, system-ui, sans-serif",
      titleFontSize: 11,
    },
    title: {
      color: tokens["text-primary"],
      font: "Geist, system-ui, sans-serif",
      fontSize: 12,
      fontWeight: 500,
    },
    view: { stroke: null },
    range: { category },
  };

  const sourceData = isObject(source.data) ? source.data : {};
  const dataOptions = { ...sourceData };
  delete dataOptions.url;
  delete dataOptions.values;
  const sourceDatasets = isObject(source.datasets) ? source.datasets : {};
  const spec: JSONObject = {
    ...source,
    background: source.background ?? "transparent",
    data: { ...dataOptions, name: DAC_DATASET_NAME },
    datasets: { ...sourceDatasets, [DAC_DATASET_NAME]: records },
    config: mergeConfig(defaultConfig, source.config),
  };

  const isSingleOrLayeredView = "mark" in source || "layer" in source;
  if (isSingleOrLayeredView) {
    spec.width ??= "container";
    spec.height ??= chartHeight;
    spec.autosize ??= { type: "fit", contains: "padding", resize: true };
  }

  return spec;
}

export function VegaLiteChart({ widget, data }: Props) {
  const containerRef = useRef<HTMLDivElement>(null);
  const [renderError, setRenderError] = useState<string | null>(null);
  const tokens = useTokens();
  const rowHeight = useContext(RowHeightContext);
  const chartHeight = rowHeight !== undefined
    ? Math.max(80, rowHeight - FRAME_OVERHEAD)
    : DEFAULT_CHART_HEIGHT;

  const records = useMemo(() => data ? toRecords(data) : [], [data]);
  const resolvedSpec = useMemo(() => {
    try {
      return themedSpec(widget.spec ?? {}, records, tokens, chartHeight);
    } catch (error) {
      return error instanceof Error ? error : new Error(String(error));
    }
  }, [widget.spec, records, tokens, chartHeight]);

  useEffect(() => {
    const container = containerRef.current;
    if (!container || records.length === 0) return;

    if (resolvedSpec instanceof Error) {
      container.removeAttribute(DAC_EXPORT_PENDING_ATTRIBUTE);
      setRenderError(resolvedSpec.message);
      return;
    }

    let disposed = false;
    let view: View | undefined;
    let observer: ResizeObserver | undefined;
    let animationFrame = 0;
    let lastWidth = -1;
    let lastHeight = -1;

    setRenderError(null);
    container.setAttribute(DAC_EXPORT_PENDING_ATTRIBUTE, "true");
    container.replaceChildren();
    syncTooltipTheme(tokens);

    void import("vega-embed")
      .then(({ default: vegaEmbed }) => vegaEmbed(
        container,
        resolvedSpec as VisualizationSpec,
        {
          mode: "vega-lite",
          renderer: "svg",
          actions: false,
          defaultStyle: false,
        },
      ))
      .then(async (result) => {
        if (disposed) {
          result.view.finalize();
          return;
        }
        view = result.view;
        observer = new ResizeObserver(([entry]) => {
          const width = Math.round(entry.contentRect.width);
          const height = Math.round(entry.contentRect.height);
          if (width === lastWidth && height === lastHeight) return;
          lastWidth = width;
          lastHeight = height;
          cancelAnimationFrame(animationFrame);
          animationFrame = requestAnimationFrame(() => {
            void view?.resize().runAsync();
          });
        });
        observer.observe(container);
        await result.view.resize().runAsync();
        if (!disposed) container.removeAttribute(DAC_EXPORT_PENDING_ATTRIBUTE);
      })
      .catch((error: unknown) => {
        container.removeAttribute(DAC_EXPORT_PENDING_ATTRIBUTE);
        if (!disposed) {
          setRenderError(error instanceof Error ? error.message : String(error));
        }
      });

    return () => {
      disposed = true;
      cancelAnimationFrame(animationFrame);
      observer?.disconnect();
      view?.finalize();
      container.removeAttribute(DAC_EXPORT_PENDING_ATTRIBUTE);
      container.replaceChildren();
    };
  }, [records, resolvedSpec, tokens]);

  if (!data?.rows?.length) {
    return <div className="text-[var(--dac-text-muted)] text-xs py-6 text-center">No data</div>;
  }

  return (
    <div className="relative" style={{ height: chartHeight }}>
      <div
        ref={containerRef}
        className="dac-vega-lite h-full w-full"
        data-dac-export-pending="true"
      />
      {renderError && (
        <div className="absolute inset-0 flex items-center justify-center bg-[var(--dac-background)]/90 px-4 text-center font-mono text-xs text-[var(--dac-error)]">
          {renderError}
        </div>
      )}
    </div>
  );
}
