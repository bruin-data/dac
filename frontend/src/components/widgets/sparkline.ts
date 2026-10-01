export interface SparklinePoint {
  x: string | number;
  y: number;
}

const MAX_POINTS = 100;
const MAX_X_CHARS = 256;
const MAX_JSON_CHARS = 256 * 1024;

// Many warehouses return aggregated JSON as text, so a JSON array string is
// accepted as a fallback. Anything else (or oversized input) yields no points.
function decodeJsonArray(value: string): unknown {
  if (value.length > MAX_JSON_CHARS) return undefined;
  const text = value.trim();
  if (!text.startsWith("[")) return undefined;
  try {
    return JSON.parse(text);
  } catch {
    return undefined;
  }
}

export function parseSparklineSeries(input: unknown, xField?: string, yField?: string): SparklinePoint[] {
  const value = typeof input === "string" ? decodeJsonArray(input) : input;
  if (!xField || !yField || !Array.isArray(value) || !value.length) return [];

  const count = Math.min(value.length, MAX_POINTS);
  const points: SparklinePoint[] = [];
  for (let i = 0; i < count; i++) {
    const index = value.length === count ? i : Math.round((i * (value.length - 1)) / (count - 1));
    const point = value[index];
    if (point === null || typeof point !== "object") continue;
    let rawX: unknown;
    let rawY: unknown;
    if (Array.isArray(point)) {
      if (point.length !== 2) continue;
      [rawX, rawY] = point;
    } else {
      const record = point as Record<string, unknown>;
      rawX = Object.getOwnPropertyDescriptor(record, xField)?.value;
      rawY = Object.getOwnPropertyDescriptor(record, yField)?.value;
    }
    const y = typeof rawY === "number" ? rawY : typeof rawY === "string" && rawY.trim() ? Number(rawY) : NaN;
    if (!Number.isFinite(y) || (typeof rawX !== "string" && (typeof rawX !== "number" || !Number.isFinite(rawX)))) continue;
    const x = typeof rawX === "string" && rawX.length > MAX_X_CHARS ? `${rawX.slice(0, MAX_X_CHARS)}…` : rawX;
    points.push({ x, y });
  }
  return points;
}
