const EXPORT_CONTROL_SELECTOR = "[data-dac-export-control]";
export const DAC_EXPORT_PENDING_ATTRIBUTE = "data-dac-export-pending";
const EXPORT_PENDING_SELECTOR = `[${DAC_EXPORT_PENDING_ATTRIBUTE}]`;
const EXPORT_READY_TIMEOUT_MS = 15_000;

function exportFilter(node: HTMLElement): boolean {
  if (!(node instanceof Element)) return true;
  return !node.closest(EXPORT_CONTROL_SELECTOR);
}

function elementBackground(element: HTMLElement): string {
  const elementBg = getComputedStyle(element).backgroundColor;
  if (elementBg && elementBg !== "rgba(0, 0, 0, 0)" && elementBg !== "transparent") {
    return elementBg;
  }

  const tokenBg = getComputedStyle(element)
    .getPropertyValue("--dac-background")
    .trim();
  return tokenBg || "#ffffff";
}

function elementSize(element: HTMLElement): { width: number; height: number } {
  const rect = element.getBoundingClientRect();
  const width = Math.ceil(Math.max(element.scrollWidth, rect.width));
  const height = Math.ceil(Math.max(element.scrollHeight, rect.height));
  if (width <= 0 || height <= 0) {
    throw new Error("Cannot export an empty element");
  }
  return { width, height };
}

async function waitForFonts(): Promise<void> {
  if ("fonts" in document) {
    await document.fonts.ready;
  }
}

function hasPendingExportContent(element: HTMLElement): boolean {
  return element.matches(EXPORT_PENDING_SELECTOR) || element.querySelector(EXPORT_PENDING_SELECTOR) !== null;
}

async function waitForExportContent(element: HTMLElement): Promise<void> {
  if (!hasPendingExportContent(element)) return;

  await new Promise<void>((resolve, reject) => {
    const observer = new MutationObserver(() => {
      if (!hasPendingExportContent(element)) finish();
    });
    const timeout = window.setTimeout(() => {
      finish(new Error("Timed out waiting for dashboard content to finish rendering"));
    }, EXPORT_READY_TIMEOUT_MS);
    const finish = (error?: Error) => {
      window.clearTimeout(timeout);
      observer.disconnect();
      if (error) reject(error);
      else resolve();
    };

    observer.observe(element, {
      attributeFilter: [DAC_EXPORT_PENDING_ATTRIBUTE],
      attributes: true,
      childList: true,
      subtree: true,
    });
    if (!hasPendingExportContent(element)) finish();
  });
}

// Rework only the export copy; live chart sizes and scroll positions stay intact.
// Animations are always frozen; overflowing charts are expanded only when asked.
function exportSnapshot(dataURL: string, expand: boolean): { element: HTMLElement; cleanup: () => void } {
  const xml = decodeURIComponent(dataURL.slice(dataURL.indexOf(",") + 1));
  const content = new DOMParser().parseFromString(xml, "image/svg+xml")
    .querySelector("foreignObject")?.firstElementChild;
  if (!content) throw new Error("Could not prepare chart export");
  const copy = document.importNode(content, true) as HTMLElement;
  copy.setAttribute("data-dac-export-copy", "");
  const style = document.createElement("style");
  style.textContent = `
    [data-dac-export-copy], [data-dac-export-copy] * { animation: none !important; transition: none !important; }
  `;
  if (expand) {
    for (const row of copy.querySelectorAll<HTMLElement>(".dac-row")) {
      row.style.flexDirection = row.style.gridTemplateColumns.split(" ").length === 1 ? "column" : "row";
    }
    style.textContent += `
      [data-dac-export-copy], [data-dac-export-copy] :has(.dac-vega-lite), [data-dac-export-copy] .dac-vega-lite {
        width: max-content !important; height: auto !important;
        max-width: none !important; max-height: none !important;
        overflow: visible !important; flex-shrink: 0 !important;
      }
      [data-dac-export-copy] .dac-row:has(.dac-vega-lite) { display: flex !important; }
    `;
  }
  copy.appendChild(style);
  const host = document.createElement("div");
  host.style.cssText = "position:fixed;left:-100000px;top:0;opacity:0;pointer-events:none";
  host.inert = true;
  host.appendChild(copy);
  document.body.appendChild(host);

  return { element: copy, cleanup: () => host.remove() };
}

async function elementToPNGDataURL(element: HTMLElement): Promise<string> {
  await Promise.all([waitForFonts(), waitForExportContent(element)]);

  const { toPng, toSvg } = await import("html-to-image");
  const { width, height } = elementSize(element);
  const options = {
    width,
    height,
    backgroundColor: elementBackground(element),
    cacheBust: true,
    imagePlaceholder: "data:image/gif;base64,R0lGODlhAQABAIAAAAAAAP///ywAAAAAAQABAAACAUwAOw==",
    pixelRatio: Math.min(2, Math.max(1, window.devicePixelRatio || 1)),
    filter: exportFilter,
  };
  const hasOverflow = Array.from(element.querySelectorAll<HTMLElement>(".dac-vega-lite"))
    .some((chart) => chart.scrollWidth > chart.clientWidth || chart.scrollHeight > chart.clientHeight);
  const hasAnimation = element.matches(".animate-in") || element.querySelector(".animate-in") !== null;
  if (!hasOverflow && !hasAnimation) return toPng(element, options);

  const snapshot = exportSnapshot(await toSvg(element, options), hasOverflow);
  try {
    return await toPng(snapshot.element, { ...options, ...elementSize(snapshot.element) });
  } finally {
    snapshot.cleanup();
  }
}

function downloadDataURL(filename: string, dataURL: string): void {
  const a = document.createElement("a");
  a.href = dataURL;
  a.download = filename;
  document.body.appendChild(a);
  a.click();
  document.body.removeChild(a);
}

function loadImage(src: string): Promise<HTMLImageElement> {
  return new Promise((resolve, reject) => {
    const img = new Image();
    img.onload = () => resolve(img);
    img.onerror = () => reject(new Error("Failed to load exported image"));
    img.src = src;
  });
}

export async function exportElementAsPNG(element: HTMLElement, filename: string): Promise<void> {
  const dataURL = await elementToPNGDataURL(element);
  downloadDataURL(filename, dataURL);
}

export async function exportElementAsPDF(element: HTMLElement, filename: string): Promise<void> {
  const [{ jsPDF }, dataURL] = await Promise.all([
    import("jspdf"),
    elementToPNGDataURL(element),
  ]);
  const image = await loadImage(dataURL);
  const orientation = image.width >= image.height ? "landscape" : "portrait";
  const pdf = new jsPDF({ orientation, unit: "pt", format: "a4", compress: true });

  const margin = 24;
  const pageWidth = pdf.internal.pageSize.getWidth();
  const pageHeight = pdf.internal.pageSize.getHeight();
  const printableWidth = pageWidth - margin * 2;
  const printableHeight = pageHeight - margin * 2;
  const renderedWidth = printableWidth;
  const renderedHeight = (image.height * renderedWidth) / image.width;

  let offsetY = 0;
  while (offsetY < renderedHeight) {
    if (offsetY > 0) {
      pdf.addPage();
    }
    pdf.addImage(dataURL, "PNG", margin, margin - offsetY, renderedWidth, renderedHeight, "dac-export", "FAST");
    offsetY += printableHeight;
  }

  await pdf.save(filename, { returnPromise: true });
}
