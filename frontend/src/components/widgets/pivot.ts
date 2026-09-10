// Pivot-table reshaping of a flat `{ columns, rows }` result set. Ported from the
// Bruin Cloud renderer (keep the two in lockstep). See PivotConfig for the shape.

export interface PivotField {
  field: string;
  order?: "asc" | "desc";
  showTotals?: boolean;
}
export interface PivotValue {
  field: string;
  summarize?: string;
  label?: string;
  format?: import("../../types/dashboard").FormatLayer[]; // CF layers, scaled over this value's cells
}
export interface PivotConfig {
  rows?: PivotField[];
  columns?: PivotField[];
  values?: PivotValue[];
}
export interface PivotResult {
  columns: string[];
  rows: unknown[][];
  columnKinds: string[];
  rowKinds: string[];
  columnValues: (number | null)[];
}

const LABEL_SEP = " / ";

// ── Aggregations ──
// Numeric aggregations ignore non-numeric values; COUNTA/COUNTUNIQUE use raw.
const nonEmpty = (vals: unknown[]): unknown[] => vals.filter((v) => v !== null && v !== undefined && v !== "");
// Filter empties first since Number('') coerces to 0.
const nums = (vals: unknown[]): number[] => nonEmpty(vals).map(Number).filter((n) => Number.isFinite(n));
const sum = (a: number[]): number => a.reduce((s, v) => s + v, 0);

function variance(a: number[], population: boolean): number | null {
  if (a.length < (population ? 1 : 2)) return null;
  const mean = sum(a) / a.length;
  const ss = sum(a.map((v) => (v - mean) ** 2));
  return ss / (a.length - (population ? 0 : 1));
}

type Agg = (vals: unknown[]) => number | null;
export const AGGREGATIONS: Record<string, Agg> = {
  sum: (vals) => { const a = nums(vals); return a.length ? sum(a) : null; },
  counta: (vals) => nonEmpty(vals).length,
  count: (vals) => nums(vals).length,
  countunique: (vals) => new Set(nonEmpty(vals).map(String)).size,
  average: (vals) => { const a = nums(vals); return a.length ? sum(a) / a.length : null; },
  max: (vals) => { const a = nums(vals); return a.length ? Math.max(...a) : null; },
  min: (vals) => { const a = nums(vals); return a.length ? Math.min(...a) : null; },
  median: (vals) => {
    const a = nums(vals).sort((x, y) => x - y);
    if (!a.length) return null;
    const m = Math.floor(a.length / 2);
    return a.length % 2 ? a[m] : (a[m - 1] + a[m]) / 2;
  },
  product: (vals) => { const a = nums(vals); return a.length ? a.reduce((p, v) => p * v, 1) : null; },
  stdev: (vals) => { const v = variance(nums(vals), false); return v === null ? null : Math.sqrt(v); },
  stdevp: (vals) => { const v = variance(nums(vals), true); return v === null ? null : Math.sqrt(v); },
  var: (vals) => variance(nums(vals), false),
  varp: (vals) => variance(nums(vals), true),
};

function aggregate(fn: string | undefined, vals: unknown[]): number | null {
  return (AGGREGATIONS[fn ?? "sum"] ?? AGGREGATIONS.sum)(vals);
}

type FieldDef<T> = T & { idx: number; oi: number };
function fieldIndexes<T extends { field: string }>(fields: T[], colIndex: Record<string, number>): FieldDef<T>[] {
  return fields
    .map((f, oi) => ({ ...f, idx: colIndex[f.field], oi }))
    .filter((f) => f.idx !== undefined);
}

// Sort distinct group keys by their raw values, honouring each level's asc/desc.
function orderKeys(keys: string[], defs: FieldDef<PivotField>[], keyValues: Record<string, unknown[]>): string[] {
  const cmp = (a: string, b: string): number => {
    const av = keyValues[a], bv = keyValues[b];
    for (let i = 0; i < av.length; i++) {
      const an = Number(av[i]), bn = Number(bv[i]);
      let d = Number.isFinite(an) && Number.isFinite(bn) ? an - bn : String(av[i]).localeCompare(String(bv[i]));
      if (defs[i]?.order === "desc") d = -d;
      if (d) return d;
    }
    return 0;
  };
  return [...keys].sort(cmp);
}

function valueLabel(v: PivotValue): string {
  return v.label || `${(v.summarize || "sum").toUpperCase()} of ${v.field}`;
}

interface Segment { path: unknown[]; keys: string[]; kind: "leaf" | "subtotal" | "grandtotal"; level: number; }

// Walk a nested axis into ordered segments: a 'leaf' per full path, plus a
// 'subtotal' after each group of a showTotals level.
function axisSegments(orderedKeys: string[], keyVals: Record<string, unknown[]>, defs: FieldDef<PivotField>[]): Segment[] {
  if (!defs.length) return [{ path: [], keys: orderedKeys, kind: "leaf", level: 0 }];
  const build = (level: number, keys: string[]): Segment[] => {
    const out: Segment[] = [];
    const groups: { v: unknown; keys: string[] }[] = [];
    const seen = new Map<string, number>();
    for (const k of keys) {
      // JSON-encode so distinct values (null vs "", 1 vs "1") stay separate groups.
      const groupKey = JSON.stringify(keyVals[k][level] ?? null);
      if (!seen.has(groupKey)) { seen.set(groupKey, groups.length); groups.push({ v: keyVals[k][level], keys: [] }); }
      groups[seen.get(groupKey)!].keys.push(k);
    }
    for (const g of groups) {
      if (level === defs.length - 1) {
        out.push({ path: [g.v], keys: g.keys, kind: "leaf", level });
      } else {
        const child = build(level + 1, g.keys);
        for (const seg of child) seg.path = [g.v, ...seg.path];
        out.push(...child);
        if (defs[level].showTotals) out.push({ path: [g.v], keys: g.keys, kind: "subtotal", level });
      }
    }
    return out;
  };
  return build(0, orderedKeys);
}

// Returns a pivoted `{ columns, rows, columnKinds, rowKinds }`, or null when the
// pivot has no usable Values field (caller renders the raw result).
export function pivotData(columns: string[], rows: unknown[][], pivot: PivotConfig | undefined): PivotResult | null {
  if (!pivot) return null;
  const colIndex: Record<string, number> = {};
  columns.forEach((name, i) => { colIndex[name] = i; });

  // A field belongs to at most one axis, once (mirrors the editor + dac validation).
  const axisSeen = new Set<string>();
  const onceAcrossAxes = (defs: FieldDef<PivotField>[]) => defs.filter((d) => (axisSeen.has(d.field) ? false : (axisSeen.add(d.field), true)));
  const rowDefs = onceAcrossAxes(fieldIndexes(pivot.rows ?? [], colIndex));
  const colDefs = onceAcrossAxes(fieldIndexes(pivot.columns ?? [], colIndex));
  const valDefs = fieldIndexes(pivot.values ?? [], colIndex);
  if (!valDefs.length) return null;
  const kept = rows;

  // Bucket raw value cells by (rowKey, colKey, valueField).
  const rowKeys: string[] = [], colKeys: string[] = [];
  const rowKeyVals: Record<string, unknown[]> = {}, colKeyVals: Record<string, unknown[]> = {};
  const rowSeen = new Set<string>(), colSeen = new Set<string>();
  const buckets: Record<string, Record<string, unknown[][]>> = Object.create(null);

  const keyOf = (row: unknown[], defs: FieldDef<PivotField>[], keys: string[], keyVals: Record<string, unknown[]>, seen: Set<string>): string => {
    const vals = defs.map((d) => row[d.idx]);
    // JSON-encode the tuple so distinct values can't collide (e.g. ["a b","c"]
    // vs ["a","b c"]) and null is kept distinct from the empty string.
    const key = defs.length ? JSON.stringify(vals) : "";
    if (!seen.has(key)) { seen.add(key); keys.push(key); keyVals[key] = vals; }
    return key;
  };

  for (const row of kept) {
    const rk = keyOf(row, rowDefs, rowKeys, rowKeyVals, rowSeen);
    const ck = keyOf(row, colDefs, colKeys, colKeyVals, colSeen);
    const byCol = buckets[rk] || (buckets[rk] = Object.create(null));
    const cells = byCol[ck] || (byCol[ck] = valDefs.map(() => [] as unknown[]));
    valDefs.forEach((v, i) => cells[i].push(row[v.idx]));
  }

  const orderedRowKeys = orderKeys(rowKeys, rowDefs, rowKeyVals);
  const orderedColKeys = orderKeys(colKeys, colDefs, colKeyVals);

  // Outer levels' showTotals drive subtotals; the innermost drives the Grand Total.
  const showRowTotals = rowDefs.length ? !!rowDefs[rowDefs.length - 1].showTotals : false;
  const showColTotals = colDefs.length ? !!colDefs[colDefs.length - 1].showTotals : false;

  // One cell = aggregate over the cross product of a row-key set and col-key set.
  const cellAgg = (rks: string[], cks: string[], i: number): number | null =>
    aggregate(valDefs[i].summarize, rks.flatMap((rk) => cks.flatMap((ck) => buckets[rk]?.[ck]?.[i] || [])));

  // Column units (one per rendered measure column).
  const colHeader = (seg: Segment, v: PivotValue): string => {
    if (!colDefs.length) return valueLabel(v);
    const base = seg.kind === "subtotal" ? `${seg.path[seg.path.length - 1]} Total` : seg.path.join(LABEL_SEP);
    return valDefs.length > 1 ? `${base} — ${valueLabel(v)}` : base;
  };
  const colUnits: { cks: string[]; i: number; header: string; kind: string }[] = [];
  for (const seg of axisSegments(orderedColKeys, colKeyVals, colDefs)) {
    valDefs.forEach((v, i) => colUnits.push({ cks: seg.keys, i, header: colHeader(seg, v), kind: seg.kind }));
  }
  if (showColTotals) {
    valDefs.forEach((v, i) => colUnits.push({ cks: orderedColKeys, i, header: valDefs.length > 1 ? `Grand Total — ${valueLabel(v)}` : "Grand Total", kind: "grandtotal" }));
  }

  // Uniquify headers so duplicate value labels / total-named data don't collide.
  const nameSeen = new Map<string, number>();
  const uniq = (name: string) => {
    const n = (nameSeen.get(name) || 0) + 1;
    nameSeen.set(name, n);
    return n > 1 ? `${name} (${n})` : name;
  };
  const outColumns = [...rowDefs.map((d) => d.field), ...colUnits.map((u) => u.header)].map(uniq);
  const columnKinds = [...rowDefs.map(() => "label"), ...colUnits.map((u) => u.kind || "leaf")];
  // Original values[] index per output column (null for row-label columns), so
  // per-value colouring maps to the right measure even when a value's field is
  // dropped from the result (valDefs is filtered; oi is the pre-filter index).
  const columnValues: (number | null)[] = [...rowDefs.map(() => null), ...colUnits.map((u) => valDefs[u.i].oi)];

  // Row segments → output rows.
  const rowSegs = axisSegments(orderedRowKeys, rowKeyVals, rowDefs);
  if (rowDefs.length && showRowTotals) rowSegs.push({ path: [], keys: orderedRowKeys, kind: "grandtotal", level: 0 });

  // Row-label cells: a leaf blanks a level that repeats the previous leaf;
  // subtotal/grand-total rows carry their own label.
  let prev: unknown[] | null = null;
  const outRows = rowSegs.map((seg) => {
    const labels: unknown[] = rowDefs.map(() => "");
    if (seg.kind === "grandtotal") {
      labels[0] = "Grand Total";
    } else if (seg.kind === "subtotal") {
      labels[seg.level] = `${seg.path[seg.path.length - 1]} Total`;
    } else {
      let c = 0;
      while (prev && c < seg.path.length && String(prev[c]) === String(seg.path[c])) c++;
      for (let l = 0; l < seg.path.length; l++) labels[l] = l >= c ? seg.path[l] : "";
      prev = seg.path;
    }
    return [...labels, ...colUnits.map((u) => cellAgg(seg.keys, u.cks, u.i))];
  });
  const rowKinds = rowSegs.map((seg) => seg.kind || "leaf");

  return { columns: outColumns, rows: outRows, columnKinds, rowKinds, columnValues };
}
