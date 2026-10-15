import { useId } from "react";
import { createPortal } from "react-dom";
import { popoverPortalTarget, usePopover } from "../../hooks/usePopover";

interface Props {
  name: string;
  columnLabel: string;
  collapsed: boolean;
  onToggle: () => void;
}

export function ColumnGroupToggle({ name, columnLabel, collapsed, onToggle }: Props) {
  const tooltipId = useId();
  const { open, setOpen, triggerRef, popoverRef, position } = usePopover();

  return (
    <>
      <button
        ref={triggerRef}
        type="button"
        aria-expanded={!collapsed}
        aria-label={`${collapsed ? "Expand" : "Collapse"} ${name} column group for ${columnLabel}`}
        aria-describedby={open ? tooltipId : undefined}
        onMouseEnter={() => setOpen(true)}
        onMouseLeave={() => setOpen(false)}
        onFocus={() => setOpen(true)}
        onBlur={() => setOpen(false)}
        onClick={onToggle}
        className="absolute left-4 top-1/2 z-10 flex h-3.5 w-3.5 -translate-y-1/2 items-center justify-center rounded-sm border border-[var(--dac-border)] text-[var(--dac-text-muted)] hover:border-[var(--dac-text-muted)] hover:text-[var(--dac-text-primary)] focus-visible:outline-2 focus-visible:outline-[var(--dac-accent)] focus-visible:outline-offset-2"
      >
        <svg width="10" height="10" viewBox="0 0 12 12" fill="none" stroke="currentColor" strokeWidth="1.5" aria-hidden="true">
          <path d={collapsed ? "M2 6h8M6 2v8" : "M2 6h8"} />
        </svg>
      </button>
      {open && createPortal(
        <div
          ref={popoverRef}
          id={tooltipId}
          role="tooltip"
          className="pointer-events-none fixed z-[9999] max-w-[min(240px,calc(100vw-16px))] break-words rounded border border-[var(--dac-border)] bg-[var(--dac-surface)] px-2 py-1 text-[11px] font-normal text-[var(--dac-text-primary)] shadow-sm"
          style={{ top: position.top, left: Math.max(8, Math.min(position.left, window.innerWidth - 248)) }}
        >
          {name}
        </div>,
        popoverPortalTarget(),
      )}
    </>
  );
}
