import { useEffect, useState } from "react";
import { createPortal } from "react-dom";
import { DayPicker } from "react-day-picker";
import type { DateRange } from "react-day-picker";
import type { FilterBarProps } from "../../types/template";
import type { Filter } from "../../types/dashboard";
import { popoverPortalTarget, usePopover } from "../../hooks/usePopover";

export function BruinFilterBar({ filters, values, onChange }: FilterBarProps) {
  if (!filters.length) return null;

  return (
    <div className="flex items-end flex-wrap gap-3 mb-5 pb-4 border-b border-[var(--dac-border)]">
      {filters.map((filter) => (
        <FilterControl
          key={filter.name}
          filter={filter}
          value={values[filter.name]}
          onChange={(v) => onChange(filter.name, v)}
        />
      ))}
    </div>
  );
}

const inputClass =
  "h-7 px-2 rounded-sm text-[13px] border border-[var(--dac-border)] bg-[var(--dac-background)] text-[var(--dac-text-primary)] focus:outline-none focus:border-[var(--dac-accent)] transition-colors duration-100";

// --- Date range presets ---

interface DatePreset {
  key: string;
  label: string;
  resolve: () => { start: string; end: string };
}

function fmt(d: Date): string {
  return d.toISOString().slice(0, 10);
}

function fmtDisplay(d: string): string {
  const date = parseLocalDate(d);
  return date.toLocaleDateString("en-US", { month: "short", day: "numeric", year: "numeric" });
}

const ALL_PRESETS: DatePreset[] = [
  {
    key: "today",
    label: "Today",
    resolve: () => {
      const d = fmt(new Date());
      return { start: d, end: d };
    },
  },
  {
    key: "yesterday",
    label: "Yesterday",
    resolve: () => {
      const d = new Date();
      d.setDate(d.getDate() - 1);
      const s = fmt(d);
      return { start: s, end: s };
    },
  },
  {
    key: "last_7_days",
    label: "Last 7 days",
    resolve: () => {
      const end = new Date();
      const start = new Date();
      start.setDate(start.getDate() - 6);
      return { start: fmt(start), end: fmt(end) };
    },
  },
  {
    key: "last_30_days",
    label: "Last 30 days",
    resolve: () => {
      const end = new Date();
      const start = new Date();
      start.setDate(start.getDate() - 29);
      return { start: fmt(start), end: fmt(end) };
    },
  },
  {
    key: "last_90_days",
    label: "Last 90 days",
    resolve: () => {
      const end = new Date();
      const start = new Date();
      start.setDate(start.getDate() - 89);
      return { start: fmt(start), end: fmt(end) };
    },
  },
  {
    key: "this_month",
    label: "This month",
    resolve: () => {
      const now = new Date();
      const start = new Date(now.getFullYear(), now.getMonth(), 1);
      const end = new Date(now.getFullYear(), now.getMonth() + 1, 0);
      return { start: fmt(start), end: fmt(end) };
    },
  },
  {
    key: "last_month",
    label: "Last month",
    resolve: () => {
      const now = new Date();
      const start = new Date(now.getFullYear(), now.getMonth() - 1, 1);
      const end = new Date(now.getFullYear(), now.getMonth(), 0);
      return { start: fmt(start), end: fmt(end) };
    },
  },
  {
    key: "this_quarter",
    label: "This quarter",
    resolve: () => {
      const now = new Date();
      const q = Math.floor(now.getMonth() / 3);
      const start = new Date(now.getFullYear(), q * 3, 1);
      const end = new Date(now.getFullYear(), q * 3 + 3, 0);
      return { start: fmt(start), end: fmt(end) };
    },
  },
  {
    key: "this_year",
    label: "This year",
    resolve: () => {
      const y = new Date().getFullYear();
      return { start: `${y}-01-01`, end: `${y}-12-31` };
    },
  },
  {
    key: "year_to_date",
    label: "Year to date",
    resolve: () => {
      const now = new Date();
      return { start: `${now.getFullYear()}-01-01`, end: fmt(now) };
    },
  },
  {
    key: "all_time",
    label: "All time",
    resolve: () => ({ start: "1970-01-01", end: "2099-12-31" }),
  },
];

const DEFAULT_PRESET_KEYS = [
  "last_7_days", "last_30_days", "last_90_days",
  "this_month", "this_quarter", "this_year", "all_time",
];

function getPresets(filter: Filter): DatePreset[] {
  const keys = filter.options?.presets;
  if (keys && keys.length > 0) {
    return keys
      .map((k) => ALL_PRESETS.find((p) => p.key === k))
      .filter((p): p is DatePreset => p != null);
  }
  return ALL_PRESETS.filter((p) => DEFAULT_PRESET_KEYS.includes(p.key));
}

/** Resolve a preset key string to {start, end}. Returns null if not a known preset. */
export function resolvePreset(key: string): { start: string; end: string } | null {
  const p = ALL_PRESETS.find((pr) => pr.key === key);
  return p ? p.resolve() : null;
}

/** Detect which preset matches the current value, if any. */
function detectPreset(value: { start: string; end: string }, presets: DatePreset[]): DatePreset | null {
  for (const p of presets) {
    const resolved = p.resolve();
    if (resolved.start === value.start && resolved.end === value.end) {
      return p;
    }
  }
  return null;
}

// --- Filter controls ---

function FilterControl({
  filter,
  value,
  onChange,
}: {
  filter: Filter;
  value: unknown;
  onChange: (value: unknown) => void;
}) {
  const label = filter.name.replace(/_/g, " ");

  switch (filter.type) {
    case "select":
      if (filter.multiple) {
        return <MultiSelectFilter filter={filter} value={value} onChange={onChange} label={label} />;
      }
      return (
        <div className="flex flex-col gap-1">
          <label className="text-[10px] font-medium uppercase tracking-wider text-[var(--dac-text-muted)]">
            {label}
          </label>
          <select
            className={inputClass}
            value={String(value ?? filter.default ?? "")}
            onChange={(e) => onChange(e.target.value)}
          >
            {filter.options?.values?.map((v) => (
              <option key={v} value={v}>{v}</option>
            ))}
          </select>
        </div>
      );

    case "date-range":
      return <DateRangeFilter filter={filter} value={value} onChange={onChange} label={label} />;

    case "text":
      return (
        <div className="flex flex-col gap-1">
          <label className="text-[10px] font-medium uppercase tracking-wider text-[var(--dac-text-muted)]">
            {label}
          </label>
          <input
            type="text"
            className={inputClass}
            value={String(value ?? filter.default ?? "")}
            onChange={(e) => onChange(e.target.value)}
            placeholder={`Filter by ${label}`}
          />
        </div>
      );

    default:
      return null;
  }
}

function parseLocalDate(s: string): Date {
  const [y, m, d] = s.split("-").map(Number);
  return new Date(y, m - 1, d);
}

function DateRangeFilter({
  filter,
  value,
  onChange,
  label,
}: {
  filter: Filter;
  value: unknown;
  onChange: (value: unknown) => void;
  label: string;
}) {
  const presets = getPresets(filter);
  const dateValue = value as { start: string; end: string } | undefined;
  const activePreset = dateValue ? detectPreset(dateValue, presets) : null;

  const { open, setOpen, popoverRef, triggerRef, position } = usePopover();
  const [showCalendar, setShowCalendar] = useState(false);

  useEffect(() => {
    if (!open) setShowCalendar(false);
  }, [open]);

  const handlePresetClick = (preset: DatePreset) => {
    onChange(preset.resolve());
    setOpen(false);
    setShowCalendar(false);
  };

  const handleRangeSelect = (range: DateRange | undefined) => {
    if (range?.from) {
      onChange({
        start: fmt(range.from),
        end: range.to ? fmt(range.to) : fmt(range.from),
      });
      if (range.to) {
        setOpen(false);
        setShowCalendar(false);
      }
    }
  };

  // Display label for the trigger button
  let triggerLabel: string;
  if (activePreset) {
    triggerLabel = activePreset.label;
  } else if (dateValue) {
    triggerLabel = `${fmtDisplay(dateValue.start)} – ${fmtDisplay(dateValue.end)}`;
  } else {
    triggerLabel = "Select dates";
  }

  const selected: DateRange | undefined = dateValue
    ? { from: parseLocalDate(dateValue.start), to: parseLocalDate(dateValue.end) }
    : undefined;

  const defaultMonth = selected?.to
    ? new Date(selected.to.getFullYear(), selected.to.getMonth() - 1, 1)
    : new Date();

  return (
    <div className="flex flex-col gap-1">
      <label className="text-[10px] font-medium uppercase tracking-wider text-[var(--dac-text-muted)]">
        {label}
      </label>
      <button
        ref={triggerRef}
        type="button"
        onClick={() => { setOpen((v) => !v); if (open) setShowCalendar(false); }}
        className={`${inputClass} cursor-pointer flex items-center gap-1.5`}
      >
        <svg width="14" height="14" viewBox="0 0 16 16" fill="none" className="shrink-0 opacity-50">
          <path d="M5 1v2M11 1v2M1.5 6h13M2.5 3h11a1 1 0 0 1 1 1v10a1 1 0 0 1-1 1h-11a1 1 0 0 1-1-1V4a1 1 0 0 1 1-1z" stroke="currentColor" strokeWidth="1.2" strokeLinecap="round" strokeLinejoin="round"/>
        </svg>
        <span className="text-[13px] whitespace-nowrap">{triggerLabel}</span>
        <svg width="10" height="10" viewBox="0 0 10 10" fill="none" className="shrink-0 opacity-40 ml-auto">
          <path d="M2.5 4L5 6.5L7.5 4" stroke="currentColor" strokeWidth="1.2" strokeLinecap="round" strokeLinejoin="round"/>
        </svg>
      </button>

      {open && createPortal(
        <div
          ref={popoverRef}
          className="dac-popover dac-popover--padded"
          style={{
            position: "fixed",
            top: position.top,
            left: position.left,
            zIndex: 9999,
          }}
        >
          {!showCalendar ? (
            <div className="flex flex-col min-w-[180px]">
              {presets.map((p) => (
                <button
                  key={p.key}
                  type="button"
                  onClick={() => handlePresetClick(p)}
                  className={`text-left px-3 py-1.5 text-[13px] rounded-sm transition-colors duration-75 ${
                    activePreset?.key === p.key
                      ? "bg-[var(--dac-accent-subtle)] text-[var(--dac-accent)] font-medium"
                      : "text-[var(--dac-text-primary)] hover:bg-[var(--dac-surface-hover)]"
                  }`}
                >
                  {p.label}
                </button>
              ))}
              <div className="border-t border-[var(--dac-border)] mt-1 pt-1">
                <button
                  type="button"
                  onClick={() => setShowCalendar(true)}
                  className={`w-full text-left px-3 py-1.5 text-[13px] rounded-sm transition-colors duration-75 ${
                    !activePreset && dateValue
                      ? "bg-[var(--dac-accent-subtle)] text-[var(--dac-accent)] font-medium"
                      : "text-[var(--dac-text-secondary)] hover:bg-[var(--dac-surface-hover)]"
                  }`}
                >
                  Custom range...
                </button>
              </div>
            </div>
          ) : (
            <div>
              <div className="flex items-center gap-2 px-1 pb-2 mb-2 border-b border-[var(--dac-border)]">
                <button
                  type="button"
                  onClick={() => setShowCalendar(false)}
                  className="text-[var(--dac-text-secondary)] hover:text-[var(--dac-text-primary)] transition-colors p-0.5"
                >
                  <svg width="14" height="14" viewBox="0 0 14 14" fill="none">
                    <path d="M8.5 3L4.5 7L8.5 11" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round"/>
                  </svg>
                </button>
                <span className="text-[12px] font-medium text-[var(--dac-text-secondary)]">
                  {dateValue
                    ? `${fmtDisplay(dateValue.start)} – ${fmtDisplay(dateValue.end)}`
                    : "Select a range"}
                </span>
              </div>
              <DayPicker
                mode="range"
                selected={selected}
                onSelect={handleRangeSelect}
                defaultMonth={defaultMonth}
                numberOfMonths={2}
                showOutsideDays
              />
            </div>
          )}
        </div>,
        popoverPortalTarget(),
      )}
    </div>
  );
}

function MultiSelectFilter({
  filter,
  value,
  onChange,
  label,
}: {
  filter: Filter;
  value: unknown;
  onChange: (value: unknown) => void;
  label: string;
}) {
  const options = filter.options?.values ?? [];
  const selected: string[] = Array.isArray(value) ? (value as string[]) : [];

  const { open, setOpen, popoverRef, triggerRef, position } = usePopover();
  const listboxId = `multi-${filter.name}`;

  const toggle = (v: string) => {
    if (selected.includes(v)) onChange(selected.filter((s) => s !== v));
    else onChange([...selected, v]);
  };

  return (
    <div className="flex flex-col gap-1">
      <label
        htmlFor={`${listboxId}-trigger`}
        className="text-[10px] font-medium uppercase tracking-wider text-[var(--dac-text-muted)]"
      >
        {label}
      </label>
      <button
        id={`${listboxId}-trigger`}
        ref={triggerRef}
        type="button"
        onClick={() => setOpen((v) => !v)}
        aria-haspopup="listbox"
        aria-expanded={open}
        aria-controls={listboxId}
        className={`${inputClass} cursor-pointer flex items-center gap-1.5 min-w-[140px]`}
      >
        <svg width="14" height="14" viewBox="0 0 16 16" fill="none" className="shrink-0 opacity-50">
          <path d="M3 4h10M3 8h10M3 12h6" stroke="currentColor" strokeWidth="1.4" strokeLinecap="round"/>
        </svg>
        <MultiSelectTriggerLabel selected={selected} />
        <svg width="10" height="10" viewBox="0 0 10 10" fill="none" className="shrink-0 opacity-40 ml-auto">
          <path d="M2.5 4L5 6.5L7.5 4" stroke="currentColor" strokeWidth="1.2" strokeLinecap="round" strokeLinejoin="round"/>
        </svg>
      </button>

      {open && createPortal(
        <div
          ref={popoverRef}
          id={listboxId}
          role="listbox"
          aria-multiselectable
          aria-label={label}
          className="dac-popover"
          style={{
            position: "fixed",
            top: position.top,
            left: position.left,
            minWidth: position.width,
            zIndex: 9999,
          }}
        >
          {selected.length > 0 && (
            <button
              type="button"
              onClick={() => onChange([])}
              className="w-full text-left px-3 py-1.5 text-[12px] text-[var(--dac-text-secondary)] hover:text-[var(--dac-text-primary)] hover:bg-[var(--dac-surface-hover)] border-b border-[var(--dac-border)] transition-colors"
            >
              Clear selection
            </button>
          )}
          <div className="flex flex-col max-h-[260px] overflow-y-auto py-1">
            {options.map((v) => {
              const checked = selected.includes(v);
              return (
                <div
                  key={v}
                  role="option"
                  aria-selected={checked}
                  onClick={() => toggle(v)}
                  onKeyDown={(e) => {
                    if (e.key === "Enter" || e.key === " ") {
                      e.preventDefault();
                      toggle(v);
                    }
                  }}
                  tabIndex={0}
                  className="flex items-center gap-2 px-3 py-1.5 text-[13px] cursor-pointer hover:bg-[var(--dac-surface-hover)] focus:bg-[var(--dac-surface-hover)] focus:outline-none transition-colors duration-75"
                >
                  <CheckboxIcon checked={checked} />
                  <span className="text-[var(--dac-text-primary)] truncate">{v}</span>
                </div>
              );
            })}
          </div>
        </div>,
        popoverPortalTarget(),
      )}
    </div>
  );
}

function MultiSelectTriggerLabel({ selected }: { selected: string[] }) {
  if (selected.length === 0) {
    return <span className="text-[13px] text-[var(--dac-text-muted)]">All</span>;
  }
  if (selected.length <= 2) {
    return (
      <span className="text-[13px] whitespace-nowrap truncate text-[var(--dac-text-primary)]">
        {selected.join(", ")}
      </span>
    );
  }
  return (
    <span className="text-[13px] whitespace-nowrap text-[var(--dac-text-primary)]">
      {selected[0]}, {selected[1]}{" "}
      <span className="text-[var(--dac-text-muted)]">+{selected.length - 2}</span>
    </span>
  );
}

function CheckboxIcon({ checked }: { checked: boolean }) {
  return (
    <span
      className={`inline-flex items-center justify-center w-3.5 h-3.5 rounded-sm border shrink-0 transition-colors ${
        checked
          ? "bg-[var(--dac-accent)] border-[var(--dac-accent)]"
          : "border-[var(--dac-border)] bg-[var(--dac-background)]"
      }`}
    >
      {checked && (
        <svg width="10" height="10" viewBox="0 0 10 10" fill="none">
          <path d="M2 5L4 7L8 3" stroke="white" strokeWidth="1.6" strokeLinecap="round" strokeLinejoin="round"/>
        </svg>
      )}
    </span>
  );
}
