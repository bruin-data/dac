import { useEffect, useMemo, useState } from "react";
import type { Widget, WidgetData } from "../../types/dashboard";

interface Props {
  widget: Widget;
  data?: WidgetData;
}

export function TableWidget({ data }: Props) {
  const [sort, setSort] = useState<{ col: number; dir: "asc" | "desc" } | null>(null);

  useEffect(() => {
    setSort(null);
  }, [data]);

  const rows = data?.rows ?? [];
  const columns = data?.columns ?? [];

  const numericCols = useMemo(
    () =>
      columns.map((col, idx) => {
        if (isIdColumn(col.name)) return false;
        let sawNumber = false;
        for (const row of rows) {
          const val = row[idx];
          if (val === null || val === undefined || val === "") continue;
          if (!Number.isFinite(Number(val))) return false;
          sawNumber = true;
        }
        return sawNumber;
      }),
    [columns, rows],
  );

  const sortedRows = useMemo(() => {
    if (!sort) return rows;
    const { col, dir } = sort;
    const sign = dir === "asc" ? 1 : -1;
    return [...rows].sort((a, b) => {
      const av = a?.[col] ?? "";
      const bv = b?.[col] ?? "";
      const an = Number(av);
      const bn = Number(bv);
      if (Number.isFinite(an) && Number.isFinite(bn)) return (an - bn) * sign;
      return String(av).localeCompare(String(bv)) * sign;
    });
  }, [rows, sort]);

  if (!rows.length) {
    return <div className="text-[var(--dac-text-muted)] text-xs py-4 text-center">No data</div>;
  }

  const toggleSort = (col: number) =>
    setSort((prev) =>
      prev?.col === col
        ? { col, dir: prev.dir === "asc" ? "desc" : "asc" }
        : { col, dir: "asc" },
    );

  return (
    <div className="overflow-x-auto">
      <table className="w-full text-[13px] min-w-[400px]">
        <thead>
          <tr className="bg-[var(--dac-surface)]">
            {columns.map((col, idx) => (
              <th
                key={col.name}
                onClick={() => toggleSort(idx)}
                className={`text-left py-2 px-4 text-[10px] font-semibold uppercase tracking-wider text-[var(--dac-text-muted)] border-b border-[var(--dac-border)] whitespace-nowrap cursor-pointer select-none hover:text-[var(--dac-text)] transition-colors ${
                  numericCols[idx] ? "text-right" : ""
                }`}
              >
                {col.name}
                {sort?.col === idx && (
                  <span className="ml-1">{sort.dir === "asc" ? "↑" : "↓"}</span>
                )}
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {sortedRows.map((row, i) => (
            <tr
              key={i}
              className="border-b border-[var(--dac-border)] border-opacity-40 last:border-0 hover:bg-[var(--dac-surface)] transition-colors duration-75"
            >
              {columns.map((col, idx) => (
                <td
                  key={col.name}
                  className={`py-1.5 px-4 whitespace-nowrap ${
                    numericCols[idx] ? "text-right tabular-nums text-[12px]" : ""
                  }`}
                  style={numericCols[idx] ? { fontFamily: '"Geist Mono", monospace' } : undefined}
                >
                  {formatCell(row[idx], col.name)}
                </td>
              ))}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

// id-like columns hold identifiers, not quantities — never format or right-align them.
function isIdColumn(name: string): boolean {
  const lower = name.toLowerCase();
  return lower === "id" || lower.endsWith("_id");
}

function formatCell(value: unknown, colName: string): string {
  if (value === null || value === undefined || value === "") return "—";
  const s = String(value);
  if (isIdColumn(colName)) return s;
  const num = Number(value);
  if (Number.isFinite(num)) {
    if (!Number.isInteger(num)) {
      return num.toLocaleString("en-US", { minimumFractionDigits: 2, maximumFractionDigits: 6 });
    }
    return num.toLocaleString("en-US");
  }
  const isoMatch = s.match(/^(\d{4})-(\d{2})-(\d{2})T/);
  if (isoMatch) {
    return `${isoMatch[1]}-${isoMatch[2]}-${isoMatch[3]}`;
  }
  return s;
}
