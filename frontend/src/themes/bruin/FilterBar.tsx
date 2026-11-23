import { useEffect, useState } from "react";
import { buildAxisFormatter } from "../../lib/format";
import type { CSSProperties } from "react";
import { createPortal } from "react-dom";
import { DayPicker } from "react-day-picker";
import type { DateRange } from "react-day-picker";
import Select, { components, type ValueContainerProps } from "react-select";
import type { FilterBarProps } from "../../types/template";
import type { Filter } from "../../types/dashboard";
import { popoverPortalTarget, usePopover } from "../../hooks/usePopover";

export function BruinFilterBar({ filters, values, onChange }: FilterBarProps) {
  if (!filters.length) return null;

  const useGrid = filters.some((filter) => filter.col !== undefined);

  return (
    <div className={`${useGrid ? "grid grid-cols-1 sm:grid-cols-12" : "flex flex-wrap"} items-end gap-3 mb-5 pb-4 border-b border-[var(--dac-border)]`}>
      {filters.map((filter) => (
        <div
          key={filter.name}
          className={useGrid ? "dac-filter-column min-w-0" : undefined}
          style={useGrid ? { "--dac-filter-col": filter.col ?? 3 } as CSSProperties : undefined}
        >
          <FilterControl
            filter={filter}
            value={values[filter.name]}
            onChange={(v) => onChange(filter.name, v)}
          />
        </div>
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

// Presets resolve in the viewer's local time zone, like Bruin Cloud dashboards.
function fmt(d: Date): string {
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")}`;
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
  "today", "yesterday", "last_7_days", "last_30_days", "last_90_days",
  "this_month", "this_quarter", "this_year", "all_time",
];

/** "Last N days" presets can end yesterday instead of today. */
function supportsIncludeToday(preset: DatePreset): boolean {
  return /^last_\d+_days$/.test(preset.key);
}

function resolveWithToday(preset: DatePreset, includeToday: boolean): { start: string; end: string } {
  const resolved = preset.resolve();
  if (includeToday || !supportsIncludeToday(preset)) return resolved;
  const yesterday = new Date();
  yesterday.setDate(yesterday.getDate() - 1);
  return { ...resolved, end: fmt(yesterday) };
}

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
function detectPreset(value: { start: string; end: string }, presets: DatePreset[], includeToday = true): DatePreset | null {
  for (const p of presets) {
    const resolved = resolveWithToday(p, includeToday);
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
    case "breakdown":
      if (filter.multiple) {
        return <MultiSelectFilter filter={filter} value={value} onChange={onChange} label={label} />;
      }
      return <SingleSelectFilter filter={filter} value={value} onChange={onChange} label={label} />;

    case "date-range":
      return <DateRangeFilter filter={filter} value={value} onChange={onChange} label={label} />;

    case "date":
      return (
        <div className="flex flex-col gap-1">
          <label className="text-[10px] font-medium uppercase tracking-wider text-[var(--dac-text-muted)]">
            {label}
          </label>
          <input
            type="date"
            className={`${inputClass} min-w-[128px]`}
            value={String(value ?? filter.default ?? "")}
            onChange={(e) => onChange(e.target.value)}
          />
        </div>
      );

    case "number":
      return (
        <div className="flex flex-col gap-1">
          <label className="text-[10px] font-medium uppercase tracking-wider text-[var(--dac-text-muted)]">
            {label}
          </label>
          <input
            type="number"
            className={`${inputClass} w-[112px] tabular-nums`}
            value={String(value ?? filter.default ?? "")}
            onChange={(e) => {
              const raw = e.target.value;
              const next = e.target.valueAsNumber;
              onChange(raw === "" || Number.isNaN(next) ? raw : next);
            }}
            placeholder={label}
          />
        </div>
      );

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

type DateParts = { start: string; end: string };

const DATE_FIELDS = [
  { key: "start", label: "From" },
  { key: "end", label: "To" },
] as const;

/** Accepts YYYY-MM-DD or DD.MM.YYYY and returns a real calendar date, or null. */
function parseTypedDate(text: string): string | null {
  const value = /^\d{2}\.\d{2}\.\d{4}$/.test(text) ? text.split(".").reverse().join("-") : text;
  if (!/^\d{4}-\d{2}-\d{2}$/.test(value)) return null;
  const [y, m, d] = value.split("-").map(Number);
  const date = new Date(y, m - 1, d);
  return date.getFullYear() === y && date.getMonth() === m - 1 && date.getDate() === d ? value : null;
}

function monthOf(value: string): Date {
  const date = value ? parseLocalDate(value) : new Date();
  return new Date(date.getFullYear(), date.getMonth(), 1);
}

// Mirrors Bruin Cloud's DateRangeFilter as dashboards use it (date-only,
// auto-apply): searchable presets with "Include today", From/To inputs, and one
// calendar. Every complete range applies immediately and the picker stays open.
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
  const range: DateParts = {
    start: (value as Partial<DateParts> | undefined)?.start ?? "",
    end: (value as Partial<DateParts> | undefined)?.end ?? "",
  };

  const { open, setOpen, popoverRef, triggerRef, position } = usePopover();
  const [draft, setDraft] = useState<DateParts>(range);
  const [activeField, setActiveField] = useState<"start" | "end">("start");
  const [month, setMonth] = useState<Date>(() => monthOf(range.start));
  const [search, setSearch] = useState("");
  const [includeToday, setIncludeToday] = useState(true);

  // A "Last N days" value may end today or yesterday; find its preset either way.
  const excludesToday = !detectPreset(range, presets) && !!detectPreset(range, presets, false);
  const activePreset = detectPreset(range, presets, !excludesToday);
  const draftPreset = detectPreset(draft, presets, includeToday);
  const hasTodayOption = presets.some(supportsIncludeToday);
  const canToggleToday = draftPreset != null && supportsIncludeToday(draftPreset);
  const filteredPresets = presets.filter((p) => p.label.toLowerCase().includes(search.toLowerCase()));

  // Follow outside changes (Reset, URL, cross-filtering) unless a range is half edited.
  useEffect(() => {
    if (draft.start && draft.end) setDraft(range);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [range.start, range.end]);

  // dac queries need both ends of a range, so only complete ranges apply.
  const commit = (next: DateParts) => {
    setDraft(next);
    if (next.start && next.end) onChange(next);
  };

  const togglePicker = () => {
    if (!open) {
      setDraft(range);
      setIncludeToday(!excludesToday);
      setActiveField(range.start && !range.end ? "end" : "start");
      setMonth(monthOf(range.start));
      setSearch("");
    }
    setOpen(!open);
  };

  const applyPreset = (preset: DatePreset) => {
    const resolved = resolveWithToday(preset, includeToday);
    setActiveField("start");
    setMonth(monthOf(resolved.start));
    commit(resolved);
  };

  const toggleIncludeToday = (checked: boolean) => {
    setIncludeToday(checked);
    if (draftPreset && canToggleToday) commit({ ...draft, end: resolveWithToday(draftPreset, checked).end });
  };

  const setPart = (part: "start" | "end", date: string) => {
    const next = { ...draft, [part]: date };
    if (next.start && next.end && next.start > next.end) {
      if (part === "end") return false;
      next.end = "";
    }
    setActiveField(part);
    commit(next);
    return true;
  };

  const commitTyped = (part: "start" | "end", input: HTMLInputElement) => {
    const text = input.value.trim();
    const date = text ? parseTypedDate(text) : "";
    // A rejected date (unparseable, or an end before the start) reverts the input.
    const accepted = date !== null && (date === draft[part] || setPart(part, date));
    if (accepted && date) setMonth(monthOf(date));
    input.value = accepted ? date : draft[part];
  };

  const handleDayClick = (day: Date) => {
    setPart(activeField, fmt(day));
    setActiveField(activeField === "start" ? "end" : "start");
  };

  let triggerLabel: string;
  if (activePreset) {
    triggerLabel = activePreset.label;
  } else if (range.start && range.end) {
    triggerLabel = range.start === range.end
      ? fmtDisplay(range.start)
      : `${fmtDisplay(range.start)} – ${fmtDisplay(range.end)}`;
  } else {
    triggerLabel = "Select dates";
  }

  const calendarStart = draft.start || draft.end;
  const selected: DateRange | undefined = calendarStart
    ? { from: parseLocalDate(calendarStart), to: draft.end ? parseLocalDate(draft.end) : undefined }
    : undefined;
  const width = Math.min(480, window.innerWidth - 32);
  const left = Math.max(16, Math.min(position.left, window.innerWidth - width - 16));

  return (
    <div className="flex flex-col gap-1">
      <label className="text-[10px] font-medium uppercase tracking-wider text-[var(--dac-text-muted)]">
        {label}
      </label>
      <button
        ref={triggerRef}
        type="button"
        onClick={togglePicker}
        aria-expanded={open}
        className={`${inputClass} cursor-pointer flex items-center gap-1.5 min-w-0 overflow-hidden`}
      >
        <CalendarIcon className="h-3.5 w-3.5 shrink-0 text-[var(--dac-text-muted)]" />
        <span className={`truncate min-w-0 flex-1 text-left ${range.start || range.end ? "" : "text-[var(--dac-text-muted)]"}`}>
          {triggerLabel}
        </span>
        <svg width="14" height="14" viewBox="0 0 16 16" fill="none" aria-hidden="true" className="shrink-0 text-[var(--dac-text-muted)]">
          <path d="M5 6l3-3 3 3M5 10l3 3 3-3" stroke="currentColor" strokeWidth="1.3" strokeLinecap="round" strokeLinejoin="round"/>
        </svg>
      </button>

      {open && createPortal(
        <div
          ref={popoverRef}
          role="dialog"
          aria-label="Date range"
          className="dac-popover dac-date-range"
          // Fit below the button and scroll inside, so short screens still reach the calendar.
          style={{ position: "fixed", top: position.top, left, width, maxHeight: window.innerHeight - position.top - 16, overflowY: "auto", zIndex: 9999 }}
        >
          <div className="flex items-center justify-between border-b border-[var(--dac-border)] px-3 py-2">
            <h2 className="text-[13px] font-semibold text-[var(--dac-text-primary)]">Date range</h2>
            <button
              type="button"
              aria-label="Close date picker"
              onClick={() => setOpen(false)}
              className="rounded-sm p-1 text-[var(--dac-text-muted)] hover:bg-[var(--dac-surface-hover)] hover:text-[var(--dac-text-primary)]"
            >
              <XIcon className="h-3.5 w-3.5" />
            </button>
          </div>

          <div className="flex flex-col min-[560px]:flex-row">
            <aside
              aria-label="Quick date ranges"
              className="border-b border-[var(--dac-border)] p-2 min-[560px]:w-36 min-[560px]:shrink-0 min-[560px]:border-b-0 min-[560px]:border-r"
            >
              <div className="relative mb-2">
                <input
                  type="text"
                  value={search}
                  onChange={(e) => setSearch(e.target.value)}
                  placeholder="Search ranges"
                  aria-label="Search date presets"
                  className={`${inputClass} w-full pl-7 text-[11px]`}
                />
                <svg width="14" height="14" viewBox="0 0 16 16" fill="none" aria-hidden="true" className="absolute left-2 top-1/2 -translate-y-1/2 text-[var(--dac-text-muted)]">
                  <circle cx="7" cy="7" r="4.5" stroke="currentColor" strokeWidth="1.3"/>
                  <path d="M10.5 10.5L14 14" stroke="currentColor" strokeWidth="1.3" strokeLinecap="round"/>
                </svg>
              </div>
              <div className="grid max-h-32 grid-cols-2 gap-0.5 overflow-y-auto overscroll-contain min-[560px]:max-h-[300px] min-[560px]:grid-cols-1">
                {filteredPresets.map((p) => {
                  const active = draftPreset?.key === p.key;
                  return (
                    <button
                      key={p.key}
                      type="button"
                      onClick={() => applyPreset(p)}
                      aria-pressed={active}
                      className={`relative flex h-7 items-center rounded-sm pl-6 pr-2 text-left text-xs transition-colors duration-75 ${
                        active
                          ? "bg-[var(--dac-accent-subtle)] font-medium text-[var(--dac-accent)]"
                          : "text-[var(--dac-text-primary)] hover:bg-[var(--dac-surface-hover)]"
                      }`}
                    >
                      {active && (
                        <svg width="14" height="14" viewBox="0 0 16 16" fill="none" aria-hidden="true" className="absolute left-1.5">
                          <path d="M3.5 8.5l3 3 6-7" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round" strokeLinejoin="round"/>
                        </svg>
                      )}
                      <span className="truncate">{p.label}</span>
                    </button>
                  );
                })}
              </div>
              {!filteredPresets.length && (
                <p className="py-4 text-center text-xs text-[var(--dac-text-muted)]">No matching ranges</p>
              )}
              {hasTodayOption && (
                <label
                  className={`mt-2 flex items-center gap-2 border-t border-[var(--dac-border)] pt-2 text-xs ${
                    canToggleToday ? "cursor-pointer text-[var(--dac-text-primary)]" : "text-[var(--dac-text-muted)]"
                  }`}
                >
                  <input
                    type="checkbox"
                    checked={includeToday}
                    disabled={!canToggleToday}
                    onChange={(e) => toggleIncludeToday(e.target.checked)}
                    className="h-3.5 w-3.5 accent-[var(--dac-accent)] disabled:opacity-50"
                  />
                  Include today
                </label>
              )}
            </aside>

            <div className="min-w-0 flex-1 p-3">
              <div className="mb-2 grid grid-cols-2 gap-2">
                {DATE_FIELDS.map((field) => (
                  <div key={field.key} className="min-w-0">
                    <span className="mb-1 block text-[11px] font-medium text-[var(--dac-text-secondary)]">
                      {field.label}
                    </span>
                    <div className="relative">
                      <input
                        key={draft[field.key]}
                        defaultValue={draft[field.key]}
                        aria-label={`${field.label} date`}
                        placeholder="YYYY-MM-DD"
                        type="text"
                        inputMode="numeric"
                        autoComplete="off"
                        onFocus={() => setActiveField(field.key)}
                        onBlur={(e) => commitTyped(field.key, e.currentTarget)}
                        onKeyDown={(e) => {
                          if (e.key === "Enter") {
                            e.preventDefault();
                            commitTyped(field.key, e.currentTarget);
                          }
                        }}
                        className={`${inputClass} w-full pr-11 text-[11px] font-normal ${
                          activeField === field.key ? "!border-[var(--dac-accent)]" : ""
                        }`}
                      />
                      <div className="absolute inset-y-0 right-1 flex items-center">
                        {draft[field.key] && (
                          <button
                            type="button"
                            aria-label={`Clear ${field.label.toLowerCase()} date`}
                            onClick={() => setPart(field.key, "")}
                            className="rounded-sm p-0.5 text-[var(--dac-text-muted)] hover:text-[var(--dac-text-primary)]"
                          >
                            <XIcon className="h-3 w-3" />
                          </button>
                        )}
                        <button
                          type="button"
                          aria-label={`Select ${field.label.toLowerCase()} date`}
                          onClick={() => setActiveField(field.key)}
                          className="rounded-sm p-0.5 text-[var(--dac-text-muted)] hover:text-[var(--dac-text-primary)]"
                        >
                          <CalendarIcon className="h-3.5 w-3.5" />
                        </button>
                      </div>
                    </div>
                  </div>
                ))}
              </div>
              <p className="mb-1 text-[11px] text-[var(--dac-text-muted)]" aria-live="polite">
                Select {activeField} date
              </p>
              <DayPicker
                mode="range"
                selected={selected}
                onSelect={(_, day) => handleDayClick(day)}
                month={month}
                onMonthChange={setMonth}
                disabled={activeField === "end" && draft.start ? { before: parseLocalDate(draft.start) } : undefined}
                showOutsideDays
              />
            </div>
          </div>

          <div className="border-t border-[var(--dac-border)] px-3 py-1.5 text-[11px] leading-4 text-[var(--dac-text-muted)]">
            All times are in your local time zone
          </div>
        </div>,
        popoverPortalTarget(),
      )}
    </div>
  );
}

function CalendarIcon({ className }: { className?: string }) {
  return (
    <svg viewBox="0 0 16 16" fill="none" aria-hidden="true" className={className}>
      <path d="M5 1v2M11 1v2M1.5 6h13M2.5 3h11a1 1 0 0 1 1 1v10a1 1 0 0 1-1 1h-11a1 1 0 0 1-1-1V4a1 1 0 0 1 1-1z" stroke="currentColor" strokeWidth="1.2" strokeLinecap="round" strokeLinejoin="round"/>
    </svg>
  );
}

function XIcon({ className }: { className?: string }) {
  return (
    <svg viewBox="0 0 16 16" fill="none" aria-hidden="true" className={className}>
      <path d="M4 4l8 8M12 4l-8 8" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round"/>
    </svg>
  );
}

interface Option {
  label: string;
  value: string;
  metric?: string;
}

/** Metrics are display-only. Selection, search and URLs keep the option value. */
function filterMetricText(filter: Filter, value: string): string | undefined {
  const options = filter.options;
  const metrics = options?.metric_values;
  if (!metrics || !Object.prototype.hasOwnProperty.call(metrics, value)) return undefined;
  const raw = metrics[value];
  if (raw == null) return "—";
  const metric = typeof raw === "string" && raw.trim() !== "" ? Number(raw) : raw;
  if (typeof metric !== "number") return String(raw);
  if (!Number.isFinite(metric)) return typeof raw === "string" ? raw : "—";
  const encoding = options?.metric;
  if (encoding && typeof encoding !== "string" && encoding.format) {
    return buildAxisFormatter(encoding, String)(metric);
  }
  const format = options?.metric_format;
  if (format) {
    const decimals = format.decimals == null ? undefined : Math.max(0, Math.min(20, format.decimals));
    try {
      return new Intl.NumberFormat("en-US", {
        style: format.type === "currency" ? "currency" : format.type === "percentage" ? "percent" : "decimal",
        ...(format.type === "currency" ? { currency: format.currency || "USD" } : {}),
        ...(decimals == null ? {} : { minimumFractionDigits: decimals, maximumFractionDigits: decimals }),
      }).format(metric);
    } catch { return String(metric); }
  }
  return String(raw);
}

function selectOptions(filter: Filter): Option[] {
  return (filter.options?.values ?? []).map((value) => ({ label: value, value, metric: filterMetricText(filter, value) }));
}

function optionLabel(option: Option, context: { context: "menu" | "value" }) {
  if (context.context !== "menu" || option.metric === undefined) return option.label;
  return <span className="flex w-full items-center justify-between gap-5">
    <span className="min-w-0 flex-1 truncate" title={option.label}>{option.label}</span>
    <span className="max-w-[45%] shrink-0 truncate tabular-nums text-[var(--dac-text-muted)]" title={option.metric}>{option.metric}</span>
  </span>;
}

/** Keep selected values that aren't in the loaded options visible in the menu. */
function withSelected(options: Option[], selected: Option[]): Option[] {
  const missing = selected.filter((s) => !options.some((o) => o.value === s.value));
  return missing.length ? [...missing, ...options] : options;
}

function SingleSelectFilter({
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
  const options = selectOptions(filter);
  const current = value ?? filter.default;
  const selected = current != null && current !== ""
    ? options.find((o) => o.value === String(current)) ?? { label: String(current), value: String(current) }
    : null;
  const menuOptions = withSelected(options, selected ? [selected] : []);
  const inputId = `select-${filter.name}`;

  return (
    <div className="flex flex-col gap-1">
      <label
        htmlFor={inputId}
        className="text-[10px] font-medium uppercase tracking-wider text-[var(--dac-text-muted)]"
      >
        {label}
      </label>
      <Select<Option, false>
        formatOptionLabel={optionLabel}
        inputId={inputId}
        classNamePrefix="dac-select"
        aria-label={label}
        isLoading={filter.options?.isLoading}
        aria-invalid={!!filter.options?.error}
        noOptionsMessage={() => filter.options?.error ? "Unable to load options" : "No options"}
        options={menuOptions}
        value={selected}
        onChange={(opt) => onChange(opt ? opt.value : "")}
        isClearable={!!(filter.options?.query || filter.options?.dimension)}
        placeholder="Select..."
        isSearchable
        menuPortalTarget={popoverPortalTarget() as HTMLElement}
        menuPosition="fixed"
        unstyled
        classNames={selectClassNames}
        styles={filterSelectStyles(menuOptions)}
      />
      {filter.options?.error && (
        <span role="alert" title={filter.options.error} className="text-[11px] text-[var(--dac-error)]">
          Unable to load options
        </span>
      )}
    </div>
  );
}

// A real null selection (e.g. default: [US, null]) selects SQL NULL rows. It gets
// its own option so it never turns into the string "null".
const NULL_OPTION: Option = { label: "(null)", value: "\u0000null" };

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
  const options = selectOptions(filter);
  const selected = Array.isArray(value)
    ? value.map((v) => v === null ? NULL_OPTION : options.find((o) => o.value === String(v)) ?? { label: String(v), value: String(v) })
    : [];
  const menuOptions = withSelected(options, selected);
  const inputId = `multi-${filter.name}`;

  return (
    <div className="flex flex-col gap-1">
      <label
        htmlFor={inputId}
        className="text-[10px] font-medium uppercase tracking-wider text-[var(--dac-text-muted)]"
      >
        {label}
      </label>
      <Select<Option, true>
        formatOptionLabel={optionLabel}
        inputId={inputId}
        classNamePrefix="dac-select"
        aria-label={label}
        isLoading={filter.options?.isLoading}
        aria-invalid={!!filter.options?.error}
        noOptionsMessage={() => filter.options?.error ? "Unable to load options" : "No options"}
        isMulti
        isSearchable
        options={menuOptions}
        value={selected}
        onChange={(opts) => onChange(opts.map((o) => o.value === NULL_OPTION.value ? null : o.value))}
        placeholder="All"
        menuPortalTarget={popoverPortalTarget() as HTMLElement}
        menuPosition="fixed"
        closeMenuOnSelect={false}
        hideSelectedOptions={false}
        controlShouldRenderValue
        components={{ ValueContainer: CompactValueContainer }}
        unstyled
        classNames={selectClassNames}
        styles={filterSelectStyles(menuOptions)}
      />
      {filter.options?.error && (
        <span role="alert" title={filter.options.error} className="text-[11px] text-[var(--dac-error)]">
          Unable to load options
        </span>
      )}
    </div>
  );
}

function CompactValueContainer({ children, ...props }: ValueContainerProps<Option, true>) {
  const count = props.getValue().length;
  const isTyping = Boolean(props.selectProps.inputValue);
  if (count === 0) {
    return <components.ValueContainer {...props}>{children}</components.ValueContainer>;
  }
  const childArray = children as React.ReactNode[];
  const input = childArray[childArray.length - 1];
  return (
    <components.ValueContainer {...props}>
      {/* Hide the "N selected" summary while the user is typing a search. */}
      {!isTyping && (
        <span className="text-[13px] text-[var(--dac-text-primary)] truncate">
          {count === 1 ? props.getValue()[0].label : `${count} selected`}
        </span>
      )}
      {input}
    </components.ValueContainer>
  );
}

const selectControlBase =
  "rounded-sm text-[13px] border bg-[var(--dac-background)] text-[var(--dac-text-primary)] transition-colors duration-100 cursor-pointer";

const selectClassNames = {
  control: ({ isFocused }: { isFocused: boolean }) =>
    `${selectControlBase} ${
      isFocused ? "border-[var(--dac-accent)]" : "border-[var(--dac-border)]"
    }`,
  valueContainer: () => "overflow-hidden",
  placeholder: () => "text-[var(--dac-text-muted)]",
  singleValue: () => "text-[var(--dac-text-primary)] truncate",
  input: () => "text-[var(--dac-text-primary)]",
  indicatorsContainer: () => "text-[var(--dac-text-muted)]",
  indicatorSeparator: () => "hidden",
  dropdownIndicator: () => "opacity-50 hover:opacity-80 transition-opacity",
  clearIndicator: () => "opacity-50 hover:opacity-80 transition-opacity cursor-pointer",
  menu: () =>
    "dac-popover mt-1 overflow-hidden text-[13px]",
  menuList: () => "py-1 max-h-[260px] overflow-y-auto",
  option: ({ isFocused, isSelected }: { isFocused: boolean; isSelected: boolean }) =>
    `px-3 py-1.5 cursor-pointer transition-colors duration-75 ${
      isSelected
        ? "text-[var(--dac-accent)] bg-[var(--dac-accent-subtle)]"
        : isFocused
          ? "bg-[var(--dac-surface-hover)] text-[var(--dac-text-primary)]"
          : "text-[var(--dac-text-primary)]"
    }`,
  noOptionsMessage: () => "px-3 py-2 text-[var(--dac-text-muted)]",
};

const selectStyles = {
  control: (base: Record<string, unknown>) => ({
    ...base,
    minHeight: 28,
    height: 28,
    minWidth: 140,
    display: "flex",
    alignItems: "center",
    flexWrap: "nowrap" as const,
    padding: "0 4px 0 8px",
  }),
  valueContainer: (base: Record<string, unknown>) => ({
    ...base,
    padding: 0,
    height: "100%",
    display: "flex",
    alignItems: "center",
    flexWrap: "nowrap" as const,
  }),
  input: (base: Record<string, unknown>) => ({
    ...base,
    margin: 0,
    padding: 0,
  }),
  indicatorsContainer: (base: Record<string, unknown>) => ({
    ...base,
    height: "100%",
    padding: 0,
  }),
  dropdownIndicator: (base: Record<string, unknown>) => ({
    ...base,
    padding: "0 2px",
  }),
  clearIndicator: (base: Record<string, unknown>) => ({
    ...base,
    padding: "0 2px",
  }),
  menuPortal: (base: Record<string, unknown>) => ({ ...base, zIndex: 9999 }),
};

function filterSelectStyles(options: Option[]) {
  if (!options.some((option) => option.metric !== undefined)) return selectStyles;
  return {
    ...selectStyles,
    menuPortal: (base: Record<string, unknown>) => {
      const width = Math.min(Math.max(Number(base.width), 220), window.innerWidth - 16);
      const left = Math.max(8, Math.min(Number(base.left), window.innerWidth - width - 8));
      return { ...base, zIndex: 9999, width, left };
    },
  };
}
