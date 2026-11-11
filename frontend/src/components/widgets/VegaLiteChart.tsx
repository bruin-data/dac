import { useContext, useEffect, useMemo, useRef, useState } from "react";
import type { Item, Loader, View } from "vega";
import type { VisualizationSpec } from "vega-embed";
import type { Widget, WidgetData } from "../../types/dashboard";
import { dashboardImageSrc } from "../../lib/dashboardImage";
import { DAC_EXPORT_PENDING_ATTRIBUTE } from "../../lib/renderExport";
import { RowHeightContext } from "../../themes/RowContext";
import { useTokens } from "../../themes/TemplateProvider";
import { useCrossFilters } from "../../hooks/useCrossFilter";
import { axisFields, buildSemanticFormatter, formatForField } from "../../lib/format";

const DAC_DATASET_NAME = "dac";
const VEGA_TOOLTIP_ID = "vg-tooltip-element";
const DEFAULT_CHART_HEIGHT = 240;
const FRAME_OVERHEAD = 60;

interface Props {
  widget: Widget;
  data?: WidgetData;
}

type JSONObject = Record<string, unknown>;

// Vega-Lite aggregates whose result is not in the metric's unit: counts, and
// (like pivot `var`, `varp` and `product`) variances and products.
const UNITLESS_AGGREGATES = ["count", "distinct", "valid", "missing", "variance", "variancep", "product"];

// Apply model defaults to field encodings throughout layered/faceted specs.
function inheritFormats(value: unknown, y: Widget["y"]): unknown {
  const metrics = axisFields(y).map((field) => field.toLowerCase());
  const config = isObject(value) && isObject(value.config) ? value.config : {};
  // A spec-wide number format is explicit and wins over model defaults.
  if (config.numberFormat != null || config.numberFormatType != null) return value;
  const inherit = (def: unknown, channel: string): unknown => {
    if (Array.isArray(def)) return def.map((child) => inherit(child, channel));
    // Dimensions, counts, variances and normalized shares keep Vega's defaults.
    if (!isObject(def) || typeof def.field !== "string" || !metrics.includes(def.field.toLowerCase())
      || UNITLESS_AGGREGATES.includes(def.aggregate as string) || def.stack === "normalize") return def;
    const format = formatForField(y?.format, def.field);
    if (!format) return def;
    const explicit = def.format != null || def.formatType != null;
    const defaults = explicit
      ? { format: def.format, formatType: def.formatType }
      : typeof format === "string" ? { format, formatType: undefined }
      : { format: JSON.stringify(format), formatType: "dacSemantic" };
    const guide = channel === "x" || channel === "y" ? "axis" : "legend";
    const own = isObject(def[guide]) ? def[guide] : undefined;
    const configs = guideConfigs(config, channel, own);
    const hasFormat = (source: JSONObject) => source.format != null || source.formatType != null;
    // Leave the guide alone when the spec already formats it, or when it is hidden:
    // an encoding guide object re-enables a guide hidden by `null`, `false` or config `disable`.
    const disabled = configs.reduce<unknown>((value, source) => source.disable ?? value, undefined) === true;
    if (def[guide] === null || def[guide] === false || (!own && disabled)
      || (own && hasFormat(own)) || configs.some(hasFormat)) return { ...defaults, ...def };
    // Guides show object formats compactly, like DAC chart ticks.
    const guideFormat = explicit || typeof format === "string" ? defaults : { ...defaults, format: JSON.stringify({ ...format, compact: true }) };
    return { ...defaults, ...def, [guide]: { ...guideFormat, ...own } };
  };
  const visit = (node: unknown): unknown => {
    if (Array.isArray(node)) return node.map(visit);
    if (!isObject(node)) return node;
    return Object.fromEntries(Object.entries(node).map(([key, child]) => [key,
      key === "encoding" && isObject(child)
        ? Object.fromEntries(Object.entries(child).map(([channel, def]) => [channel, inherit(def, channel)]))
        : visit(child),
    ]));
  };
  return visit(value);
}

// Vega-Lite config sections that apply to a quantitative axis, or to a legend,
// in Vega-Lite's precedence order (later sections win).
function guideConfigs(config: JSONObject, channel: string, axis?: JSONObject): JSONObject[] {
  if (channel !== "x" && channel !== "y") return [config.legend].filter(isObject);
  const axisChannel = `axis${channel.toUpperCase()}`;
  const orients = [axis, config[axisChannel], config.axis].map((source) => isObject(source) ? source.orient : undefined);
  const orient = orients.find((value): value is string => typeof value === "string") ?? (channel === "x" ? "bottom" : "left");
  const keys = ["axis", `axis${orient.charAt(0).toUpperCase()}${orient.slice(1)}`, axisChannel, "axisQuantitative", `${axisChannel}Quantitative`];
  return keys.map((key) => config[key]).filter(isObject);
}

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

// Image marks load URLs from query data, so they follow the same rule as image
// widgets and table image columns. Links and other contexts keep Vega's default.
function dashboardLoader(createLoader: () => Loader): Loader {
  const base = createLoader();
  const sanitize = base.sanitize.bind(base);
  base.sanitize = async (uri, options) => {
    if (options?.context !== "image") return sanitize(uri, options);
    const href = dashboardImageSrc(uri);
    if (!href) throw new Error(`Vega-Lite image URL is not allowed: ${String(uri)}`);
    return { href };
  };
  return base;
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

// Vega ellipsizes long axis and legend labels; show the full text on hover.
function titleTruncatedLabels(container: HTMLElement) {
  for (const label of container.querySelectorAll<SVGTextElement>(".role-axis-label text, .role-legend-label text")) {
    label.querySelector("title")?.remove();
    const full = [(label as SVGTextElement & { __data__?: { text?: unknown } }).__data__?.text].flat().join(" ").trim();
    // Ignore whitespace: multi-line labels render their lines without separators.
    const truncated = full !== "" && label.textContent?.replace(/\s/g, "") !== full.replace(/\s/g, "");
    label.style.pointerEvents = truncated ? "auto" : "";
    if (!truncated) continue;
    const title = document.createElementNS("http://www.w3.org/2000/svg", "title");
    title.textContent = full;
    label.appendChild(title);
  }
}

function themedSpec(
  source: JSONObject,
  records: JSONObject[],
  tokens: Record<string, string>,
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

  // Charts always fit their widget, like the built-in chart types: explicit and
  // step sizes are replaced so the plot squeezes into the available space.
  // Composed views (concat, facet, repeat, or row/column/facet channels) cannot
  // fit, so CSS scales them down.
  const encoding = isObject(source.encoding) ? source.encoding : {};
  const isFaceted = "row" in encoding || "column" in encoding || "facet" in encoding;
  const isSingleOrLayeredView = ("mark" in source || "layer" in source) && !isFaceted;
  if (isSingleOrLayeredView) {
    spec.width = "container";
    spec.height = "container";
    spec.autosize = { type: "fit", contains: "padding", resize: true };
  }

  return spec;
}

export function VegaLiteChart({ widget, data }: Props) {
  const containerRef = useRef<HTMLDivElement>(null);
  const bindings = useCrossFilters(widget, data);
  const interactionRef = useRef({ bindings, originalDate: (() => undefined) as (field: string, timestamp: number) => string | undefined });
  const highlightRef = useRef<(() => void) | null>(null);
  const [renderError, setRenderError] = useState<string | null>(null);
  const tokens = useTokens();
  const rowHeight = useContext(RowHeightContext);
  const chartHeight = rowHeight !== undefined
    ? Math.max(80, rowHeight - FRAME_OVERHEAD)
    : DEFAULT_CHART_HEIGHT;

  const records = useMemo(() => data ? toRecords(data) : [], [data]);
  // Vega parses temporal fields into epoch timestamps. Index each field's
  // original query values once, so highlighting stays linear in the marks.
  const originalDate = useMemo(() => {
    const indexes = new Map<string, Map<number, string>>();
    return (field: string, timestamp: number) => {
      let index = indexes.get(field);
      if (!index) {
        index = new Map();
        for (const row of records) {
          const raw = row[field];
          // Vega's toDate uses Date.parse for any string, e.g. "2024-01" or "2024-01-01 10:00:00".
          if (typeof raw !== "string") continue;
          const parsed = Date.parse(raw);
          if (Number.isFinite(parsed) && !index.has(parsed)) index.set(parsed, raw);
        }
        indexes.set(field, index);
      }
      return index.get(timestamp);
    };
  }, [records]);
  // Keep listeners current without rebuilding a Vega view on each filter click.
  useEffect(() => {
    interactionRef.current = { bindings, originalDate };
    highlightRef.current?.();
  }, [bindings, originalDate]);

  const resolvedSpec = useMemo(() => {
    try {
      const spec = themedSpec(inheritFormats(widget.spec ?? {}, widget.y) as JSONObject, records, tokens);
      spec.config = { ...spec.config as JSONObject, customFormatTypes: true };
      return spec;
    } catch (error) {
      return error instanceof Error ? error : new Error(String(error));
    }
  }, [widget.spec, widget.y, records, tokens]);

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
    let markObserver: MutationObserver | undefined;
    let animationFrame = 0;
    let lastWidth = -1;
    let lastHeight = -1;
    let resizeGeneration = 0;

    setRenderError(null);
    container.setAttribute(DAC_EXPORT_PENDING_ATTRIBUTE, "true");
    container.replaceChildren();
    syncTooltipTheme(tokens);

    void Promise.all([import("vega-embed"), import("vega"), import("vega-interpreter")])
      .then(([{ default: vegaEmbed }, { loader, expressionFunction }, { expressionInterpreter }]) => {
        if (disposed) return;
        expressionFunction("dacSemantic", (value: unknown, format: string) => {
          const { compact, ...spec } = JSON.parse(format);
          return buildSemanticFormatter(spec, compact)?.(value) ?? String(value);
        });
        // Let our observer own responsive sizing instead of racing Vega's
        // independent window-resize signals generated for "container" sizes.
        const size = getComputedStyle(container);
        const spec = { ...resolvedSpec };
        if (spec.width === "container") spec.width = Math.floor(parseFloat(size.width));
        if (spec.height === "container") spec.height = Math.floor(parseFloat(size.height));
        return vegaEmbed(
          container,
          spec as VisualizationSpec,
          {
            mode: "vega-lite",
            loader: dashboardLoader(loader),
            renderer: "svg",
            actions: false,
            defaultStyle: false,
            // Interpreter instead of `new Function`, so charts render without CSP 'unsafe-eval'.
            ast: true,
            expr: expressionInterpreter,
          },
        );
      })
      .then(async (result) => {
        if (!result) return;
        if (disposed) {
          result.view.finalize();
          return;
        }
        view = result.view;
        // A line/area path represents a whole series. Only marks with their own
        // scalar datum may emit clicks; guides and group backgrounds stay inert.
        const datumFor = (item?: Item | null): JSONObject | undefined => {
          if (item?.mark.role !== "mark" || ["group", "line", "area"].includes(item.mark.marktype) || !isObject(item.datum)) return;
          const datum = { ...item.datum };
          for (const binding of interactionRef.current.bindings) {
            const value = datum[binding.field];
            // Recover the original query value so dates and select options keep their type.
            if (typeof value === "number" || value instanceof Date) {
              const original = interactionRef.current.originalDate(binding.field, Number(value));
              if (original !== undefined) datum[binding.field] = original;
            }
          }
          return datum;
        };
        const select = (item?: Item | null) => {
          const datum = datumFor(item);
          if (!datum) return;
          const seen = new Set<string>();
          const selections = interactionRef.current.bindings.filter((binding) => {
            if (seen.has(binding.filter.name) || !binding.canSelect(datum[binding.field])) return false;
            seen.add(binding.filter.name);
            return true;
          });
          const selected = selections.every((binding) => binding.isSelected(datum[binding.field]));
          for (const binding of selections) {
            if (selected || !binding.isSelected(datum[binding.field])) binding.select(datum[binding.field]);
          }
        };
        const highlight = () => {
          for (const element of container.querySelectorAll<SVGElement>("svg *")) {
            const item = (element as SVGElement & { __data__?: Item & { opacity?: number } }).__data__;
            const role = item?.mark?.role;
            if (!role) continue;
            if (role.startsWith("axis") || role.startsWith("legend")) {
              element.setAttribute("data-dac-control", "");
              continue;
            }
            if (role !== "mark") continue;
            element.setAttribute("data-dac-mark", "");
            // Text spans can share their parent's scene item; keep one focus target.
            if ((element.parentElement as (Element & { __data__?: Item }) | null)?.__data__ === item) continue;
            const datum = datumFor(item);
            const matches = datum ? interactionRef.current.bindings.filter((binding) => binding.accepts(datum[binding.field])) : [];
            const enabled = matches.some((binding) => binding.ready);
            const selected = matches.filter((binding) => binding.isSource && binding.active)
              .every((binding) => binding.isSelected(datum?.[binding.field]));
            // Inline styles beat Vega's opacity attribute, so only set one while dimming;
            // otherwise hover/param opacity encodings stop updating.
            if (selected) element.style.removeProperty("opacity");
            else element.style.opacity = String((item?.opacity ?? 1) * 0.35);
            element.style.cursor = enabled ? "pointer" : "";
            // Marks stay focusable while a refetch is pending, so keyboard focus survives a selection.
            if (matches.length) {
              element.setAttribute("role", "button");
              element.setAttribute("tabindex", "0");
              element.setAttribute("aria-label", matches.map((binding) => `${binding.field}: ${String(datum?.[binding.field])}`).join(", "));
              if (enabled) element.removeAttribute("aria-disabled");
              else element.setAttribute("aria-disabled", "true");
            } else {
              element.removeAttribute("tabindex");
              element.removeAttribute("aria-disabled");
            }
          }
        };
        highlightRef.current = highlight;
        result.view.addEventListener("click", (event, item) => {
          if (!("button" in event) || event.button === 0) select(item);
        });
        result.view.addEventListener("keydown", (event, item) => {
          if ("key" in event && (event.key === "Enter" || event.key === " ") && datumFor(item)) {
            event.preventDefault();
            select(item);
          }
        });
        await result.view.resize().runAsync();
        if (disposed) return;
        titleTruncatedLabels(container);
        // Vega parameters may create new marks or bound input controls after rendering.
        const markElements = () => {
          highlight();
          for (const control of container.querySelectorAll("input, select, textarea, button, a, [contenteditable=true]")) {
            control.setAttribute("data-dac-control", "");
            control.closest("label")?.setAttribute("data-dac-control", "");
          }
        };
        markElements();
        markObserver = new MutationObserver(markElements);
        markObserver.observe(container, { childList: true, subtree: true });
        container.removeAttribute(DAC_EXPORT_PENDING_ATTRIBUTE);

        observer = new ResizeObserver(([entry]) => {
          // Stay within fractional bounds so rounding cannot create scrollbars
          // that change the available space and trigger another resize.
          const width = Math.floor(entry.contentRect.width);
          const height = Math.floor(entry.contentRect.height);
          if (width === lastWidth && height === lastHeight) return;
          lastWidth = width;
          lastHeight = height;
          const generation = ++resizeGeneration;
          container.setAttribute(DAC_EXPORT_PENDING_ATTRIBUTE, "true");
          cancelAnimationFrame(animationFrame);
          animationFrame = requestAnimationFrame(() => {
            const activeView = view;
            if (!activeView || disposed || generation !== resizeGeneration) return;
            // Vega's container signals listen for window resizes. Frame changes
            // (wrapped descriptions, tabs, embedded hosts) need an explicit size.
            if (resolvedSpec.width === "container") activeView.width(width);
            if (resolvedSpec.height === "container") activeView.height(height);
            void activeView.resize().runAsync()
              .then(() => {
                if (!disposed && generation === resizeGeneration) {
                  titleTruncatedLabels(container);
                  highlight();
                }
              })
              .catch((error: unknown) => {
                if (!disposed && generation === resizeGeneration) {
                  setRenderError(error instanceof Error ? error.message : String(error));
                }
              })
              .finally(() => {
                if (!disposed && generation === resizeGeneration) {
                  container.removeAttribute(DAC_EXPORT_PENDING_ATTRIBUTE);
                }
              });
          });
        });
        observer.observe(container.parentElement ?? container);
      })
      .catch((error: unknown) => {
        if (!disposed) {
          container.removeAttribute(DAC_EXPORT_PENDING_ATTRIBUTE);
          setRenderError(error instanceof Error ? error.message : String(error));
        }
      });

    return () => {
      disposed = true;
      cancelAnimationFrame(animationFrame);
      observer?.disconnect();
      markObserver?.disconnect();
      highlightRef.current = null;
      view?.finalize();
      container.removeAttribute(DAC_EXPORT_PENDING_ATTRIBUTE);
      container.replaceChildren();
    };
  }, [records, resolvedSpec, tokens]);

  if (!data?.rows?.length) {
    return <div className="text-[var(--dac-text-muted)] text-xs py-6 text-center">No data</div>;
  }

  return (
    <div className="relative min-h-0 min-w-0 flex-auto overflow-hidden" style={{ height: chartHeight }}>
      <div
        ref={containerRef}
        data-dac-plot
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
