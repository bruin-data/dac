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
        className={`${inputClass} cursor-pointer flex items-center gap-1.5 min-w-0 overflow-hidden`}
      >
        <svg width="14" height="14" viewBox="0 0 16 16" fill="none" className="shrink-0 opacity-50">
          <path d="M5 1v2M11 1v2M1.5 6h13M2.5 3h11a1 1 0 0 1 1 1v10a1 1 0 0 1-1 1h-11a1 1 0 0 1-1-1V4a1 1 0 0 1 1-1z" stroke="currentColor" strokeWidth="1.2" strokeLinecap="round" strokeLinejoin="round"/>
        </svg>
        <span className="text-[13px] truncate min-w-0">{triggerLabel}</span>
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
    ? value.map((v) => options.find((o) => o.value === String(v)) ?? { label: String(v), value: String(v) })
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
        onChange={(opts) => onChange(opts.map((o) => o.value))}
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
