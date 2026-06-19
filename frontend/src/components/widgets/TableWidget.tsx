import { useMemo, useState } from "react";
import type { Widget, WidgetData } from "../../types/dashboard";

interface Props {
  widget: Widget;
  data?: WidgetData;
}

type SortDirection = "asc" | "desc";

interface SortState {
  column: string;
  direction: SortDirection;
}

interface TableColumn {
  name: string;
  label: string;
  format?: string;
  idx: number;
}

export function TableWidget({ widget, data }: Props) {
  const [sort, setSort] = useState<SortState | null>(null);

  const columns: TableColumn[] = useMemo(() => {
    if (!data?.columns) return [];

    return widget.columns?.length
      ? widget.columns.map((col) => ({
          name: col.name,
          label: col.label || col.name,
          format: col.format,
          idx: data.columns.findIndex((c) => c.name === col.name),
        }))
      : data.columns.map((col, idx) => ({
          name: col.name,
          label: col.name,
          format: undefined as string | undefined,
          idx,
        }));
  }, [widget.columns, data?.columns]);

  const rows = data?.rows ?? [];

  const sortedRows = useMemo(() => {
    if (!sort || rows.length === 0) return rows;

    const col = columns.find((c) => c.name === sort.column);
    if (!col || col.idx < 0) return rows;

    return sortRows(rows, col.idx, sort.direction, isNumericFormat(col.format));
  }, [columns, rows, sort]);

  if (!rows.length) {
    return <div className="text-[var(--dac-text-muted)] text-xs py-4 text-center">No data</div>;
  }

  const handleHeaderClick = (columnName: string) => {
    setSort((current) => nextSortState(current, columnName));
  };

  return (
    <div className="overflow-x-auto">
      <table className="w-full text-[13px] min-w-[400px]">
        <thead>
          <tr className="bg-[var(--dac-surface)]">
            {columns.map((col) => {
              const numeric = isNumericFormat(col.format);
              const active = sort?.column === col.name;
              return (
                <th
                  key={col.name}
                  aria-sort={
                    active
                      ? sort!.direction === "asc"
                        ? "ascending"
                        : "descending"
                      : "none"
                  }
                  className={`py-0 px-0 border-b border-[var(--dac-border)] whitespace-nowrap ${
                    numeric ? "text-right" : "text-left"
                  }`}
                >
                  <button
                    type="button"
                    onClick={() => handleHeaderClick(col.name)}
                    className={`group w-full flex items-center gap-1 py-2 px-4 text-[10px] font-semibold uppercase tracking-wider text-[var(--dac-text-muted)] hover:text-[var(--dac-text-primary)] transition-colors duration-75 cursor-pointer border-0 bg-transparent ${
                      numeric ? "justify-end" : "justify-start"
                    } ${active ? "text-[var(--dac-text-primary)]" : ""}`}
                  >
                    <span>{col.label}</span>
                    <SortIndicator direction={active ? sort!.direction : null} />
                  </button>
                </th>
              );
            })}
          </tr>
        </thead>
        <tbody>
          {sortedRows.map((row, i) => (
            <tr
              key={i}
              className="border-b border-[var(--dac-border)] border-opacity-40 last:border-0 hover:bg-[var(--dac-surface)] transition-colors duration-75"
            >
              {columns.map((col) => (
                <td
                  key={col.name}
                  className={`py-1.5 px-4 whitespace-nowrap ${
                    isNumericFormat(col.format) ? "text-right tabular-nums text-[12px]" : ""
                  }`}
                  style={isNumericFormat(col.format) ? { fontFamily: '"Geist Mono", monospace' } : undefined}
                >
                  {formatCell(col.idx >= 0 ? row[col.idx] : null, col.format)}
                </td>
              ))}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

function SortIndicator({ direction }: { direction: SortDirection | null }) {
  return (
    <span
      className={`inline-flex flex-col leading-none shrink-0 ${
        direction ? "opacity-100" : "opacity-0 group-hover:opacity-40"
      }`}
      aria-hidden
    >
      <span className={direction === "asc" ? "text-[var(--dac-accent)]" : "text-[var(--dac-text-muted)] opacity-40"}>
        ▲
      </span>
      <span className={`-mt-1 ${direction === "desc" ? "text-[var(--dac-accent)]" : "text-[var(--dac-text-muted)] opacity-40"}`}>
        ▼
      </span>
    </span>
  );
}

function isNumericFormat(format?: string): boolean {
  return format === "currency" || format === "number";
}

function nextSortState(current: SortState | null, column: string): SortState | null {
  if (!current || current.column !== column) {
    return { column, direction: "asc" };
  }
  if (current.direction === "asc") {
    return { column, direction: "desc" };
  }
  return null;
}

function sortRows(
  rows: unknown[][],
  colIdx: number,
  direction: SortDirection,
  numeric: boolean,
): unknown[][] {
  const indexed = rows.map((row, i) => ({ row, i }));
  indexed.sort((a, b) => {
    const cmp = compareValues(a.row[colIdx], b.row[colIdx], numeric);
    if (cmp !== 0) {
      return direction === "asc" ? cmp : -cmp;
    }
    return a.i - b.i;
  });
  return indexed.map(({ row }) => row);
}

function compareValues(a: unknown, b: unknown, numeric: boolean): number {
  if (a == null && b == null) return 0;
  if (a == null) return 1;
  if (b == null) return -1;

  if (numeric) {
    const na = Number(a);
    const nb = Number(b);
    if (!isNaN(na) && !isNaN(nb)) {
      return na - nb;
    }
  }

  return String(a).localeCompare(String(b), undefined, { numeric: true, sensitivity: "base" });
}

function formatCell(value: unknown, format?: string): string {
  if (value === null || value === undefined) return "—";
  if (format === "currency") {
    const num = Number(value);
    return isNaN(num) ? String(value) : `$${num.toLocaleString(undefined, { minimumFractionDigits: 2 })}`;
  }
  if (format === "number") {
    const num = Number(value);
    return isNaN(num) ? String(value) : num.toLocaleString();
  }
  const s = String(value);
  const isoMatch = s.match(/^(\d{4})-(\d{2})-(\d{2})T/);
  if (isoMatch) {
    return `${isoMatch[1]}-${isoMatch[2]}-${isoMatch[3]}`;
  }
  return s;
}
