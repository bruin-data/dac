const EXPORT_CONTROL_SELECTOR = "[data-dac-export-control]";

function exportFilter(node: HTMLElement): boolean {
  if (!(node instanceof Element)) return true;
  return !node.closest(EXPORT_CONTROL_SELECTOR);
}

function elementBackground(element: HTMLElement): string {
  const elementBg = getComputedStyle(element).backgroundColor;
  if (elementBg && elementBg !== "rgba(0, 0, 0, 0)" && elementBg !== "transparent") {
    return elementBg;
  }

  const tokenBg = getComputedStyle(document.documentElement)
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

async function elementToPNGDataURL(element: HTMLElement): Promise<string> {
  await waitForFonts();

  const { toPng } = await import("html-to-image");
  const { width, height } = elementSize(element);
  return toPng(element, {
    width,
    height,
    backgroundColor: elementBackground(element),
    cacheBust: true,
    imagePlaceholder: "data:image/gif;base64,R0lGODlhAQABAIAAAAAAAP///ywAAAAAAQABAAACAUwAOw==",
    pixelRatio: Math.min(2, Math.max(1, window.devicePixelRatio || 1)),
    filter: exportFilter,
  });
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
