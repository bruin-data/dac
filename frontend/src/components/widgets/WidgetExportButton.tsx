import { useState } from "react";
import type { Widget, WidgetData } from "../../types/dashboard";
import { widgetDataToCSV, downloadTextFile, slugify } from "../../lib/csv";
import { pivotData } from "./pivot";
import { exportElementAsPDF, exportElementAsPNG } from "../../lib/renderExport";
import { ExportMenuButton, type ExportMenuItem } from "../ExportMenuButton";

interface Props {
  widget: Widget;
  data?: WidgetData;
}

function widgetFrameFor(trigger: HTMLButtonElement): HTMLElement {
  const frame = trigger.closest("[data-dac-widget-frame]");
  if (!(frame instanceof HTMLElement)) {
    throw new Error("Widget frame not found");
  }
  return frame;
}

/** Per-widget export menu. Renders nothing when there is no data. */
export function WidgetExportButton({ widget, data }: Props) {
  const [exporting, setExporting] = useState(false);

  if (!data || data.error || !data.rows || data.rows.length === 0) return null;

  const exportCSV = () => {
    // Export what's shown: a pivot_table is reshaped client-side, so CSV the
    // pivoted grid rather than the flat query result.
    let out = data;
    if (widget.pivot && data.columns) {
      const p = pivotData(data.columns.map((c) => c.name), data.rows ?? [], widget.pivot);
      if (p) out = { columns: p.columns.map((name) => ({ name })), rows: p.rows };
    }
    const csv = widgetDataToCSV(out);
    downloadTextFile(`${slugify(widget.name)}.csv`, csv, "text/csv;charset=utf-8");
  };

  const exportVisual = async (trigger: HTMLButtonElement, format: "png" | "pdf") => {
    const frame = widgetFrameFor(trigger);
    const filename = `${slugify(widget.name)}.${format}`;
    setExporting(true);
    try {
      if (format === "png") {
        await exportElementAsPNG(frame, filename);
      } else {
        await exportElementAsPDF(frame, filename);
      }
    } finally {
      setExporting(false);
    }
  };

  const items: ExportMenuItem[] = [
    { key: "csv", label: "CSV", onSelect: exportCSV },
    { key: "png", label: "PNG", onSelect: (trigger) => exportVisual(trigger, "png") },
    { key: "pdf", label: "PDF", onSelect: (trigger) => exportVisual(trigger, "pdf") },
  ];

  return (
    <ExportMenuButton
      label="Export"
      ariaLabel={`Export ${widget.name}`}
      title="Export widget"
      items={items}
      disabled={exporting}
      variant="widget"
    />
  );
}
