import { createPortal } from "react-dom";
import { popoverPortalTarget, usePopover } from "../hooks/usePopover";

export interface ExportMenuItem {
  key: string;
  label: string;
  onSelect: (trigger: HTMLButtonElement) => void | Promise<void>;
  disabled?: boolean;
}

interface Props {
  label: string;
  ariaLabel: string;
  title: string;
  items: ExportMenuItem[];
  disabled?: boolean;
  variant?: "header" | "widget";
}

function DownloadIcon() {
  return (
    <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
      <path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4" />
      <polyline points="7 10 12 15 17 10" />
      <line x1="12" y1="15" x2="12" y2="3" />
    </svg>
  );
}

const headerButtonClass =
  "inline-flex items-center gap-1.5 h-7 px-2 rounded-sm border text-[13px] transition-all duration-100 border-[var(--dac-border)] bg-[var(--dac-background)] text-[var(--dac-text-secondary)] hover:text-[var(--dac-text-primary)] hover:border-[var(--dac-text-muted)] hover:bg-[var(--dac-surface-hover)] disabled:opacity-50 disabled:cursor-not-allowed";

const widgetButtonClass =
  "inline-flex items-center justify-center w-4 h-4 rounded opacity-0 group-hover:opacity-40 hover:!opacity-80 focus:opacity-80 focus-visible:!opacity-80 focus-visible:outline focus-visible:outline-1 focus-visible:outline-offset-1 focus-visible:outline-[var(--dac-accent)] transition-opacity duration-150 cursor-pointer disabled:cursor-not-allowed";

export function ExportMenuButton({
  label,
  ariaLabel,
  title,
  items,
  disabled = false,
  variant = "header",
}: Props) {
  const { open, setOpen, popoverRef, triggerRef, position } = usePopover(false);
  const isWidget = variant === "widget";

  const handleSelect = async (item: ExportMenuItem) => {
    const trigger = triggerRef.current;
    if (!trigger || item.disabled) return;

    setOpen(false);
    try {
      await item.onSelect(trigger);
    } catch (err) {
      console.error("Export failed", err);
    }
  };

  const menuLeft = Math.max(8, Math.min(position.left, window.innerWidth - 112));

  return (
    <>
      <button
        ref={triggerRef}
        type="button"
        data-dac-export-control
        aria-label={ariaLabel}
        aria-haspopup="menu"
        aria-expanded={open}
        title={title}
        disabled={disabled}
        onClick={() => setOpen((v) => !v)}
        className={isWidget ? widgetButtonClass : headerButtonClass}
        style={{ color: isWidget ? "var(--dac-text-muted)" : undefined }}
      >
        <span aria-hidden="true"><DownloadIcon /></span>
        {!isWidget && <span>{label}</span>}
      </button>

      {open && createPortal(
        <div
          ref={popoverRef}
          data-dac-export-control
          role="menu"
          className="fixed z-[9999] min-w-24 rounded-sm border border-[var(--dac-border)] bg-[var(--dac-surface)] shadow-lg py-1"
          style={{ top: position.top, left: menuLeft }}
        >
          {items.map((item) => (
            <button
              key={item.key}
              type="button"
              role="menuitem"
              disabled={item.disabled}
              onClick={() => { void handleSelect(item); }}
              className="block w-full px-2.5 py-1.5 text-left text-[12px] text-[var(--dac-text-secondary)] hover:text-[var(--dac-text-primary)] hover:bg-[var(--dac-surface-hover)] disabled:opacity-50 disabled:cursor-not-allowed"
            >
              {item.label}
            </button>
          ))}
        </div>,
        popoverPortalTarget(),
      )}
    </>
  );
}
