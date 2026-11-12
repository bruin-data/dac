import { useEffect, useId, useRef } from "react";
import { createPortal } from "react-dom";
import { popoverPortalTarget, usePopover } from "../hooks/usePopover";

export function InfoTooltip({ description, name }: { description: string; name?: string }) {
  const { open, setOpen, triggerRef, popoverRef, position } = usePopover(false, "top");
  const id = useId();
  const hideTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const center = position.left + position.width / 2;

  function show() {
    if (hideTimer.current) clearTimeout(hideTimer.current);
    setOpen(true);
  }

  function scheduleHide() {
    if (hideTimer.current) clearTimeout(hideTimer.current);
    hideTimer.current = setTimeout(() => {
      // Keyboard focus or a pointer over the button/tooltip keeps it open. A mouse click focuses the
      // button in some browsers, and blur fires when clicking inside the tooltip, so check live state.
      if (triggerRef.current?.matches(":focus-visible, :hover") || popoverRef.current?.matches(":hover")) return;
      setOpen(false);
    }, 150);
  }

  useEffect(() => () => {
    if (hideTimer.current) clearTimeout(hideTimer.current);
  }, []);

  return (
    <>
      <button
        ref={triggerRef}
        type="button"
        data-dac-control data-dac-export-control
        aria-label={name ? `About ${name}` : "Widget description"}
        aria-describedby={id}
        onMouseEnter={show}
        onMouseLeave={scheduleHide}
        // Open only for keyboard focus; window refocus after a mouse click would otherwise reopen it.
        onFocus={(event) => { if (event.currentTarget.matches(":focus-visible")) show(); }}
        onBlur={scheduleHide}
        onClick={(event) => { event.stopPropagation(); show(); }}
        className="ml-1 inline-flex h-5 w-5 shrink-0 items-center justify-center rounded-sm text-[var(--dac-text-muted)] hover:text-[var(--dac-text-primary)] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[var(--dac-accent)]"
      >
        <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.75" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
          <circle cx="12" cy="12" r="10" />
          <path d="M12 16v-4M12 8h.01" />
        </svg>
      </button>
      {/* Always in the DOM so assistive tech gets the description without opening the visual tooltip. */}
      <span id={id} hidden>{description}</span>
      {open && createPortal(
        <div
          ref={popoverRef}
          aria-hidden="true"
          data-dac-control data-dac-export-control
          onMouseEnter={show}
          onMouseLeave={scheduleHide}
          className="dac-popover fixed z-[9999] text-xs font-normal normal-case leading-relaxed tracking-normal whitespace-pre-wrap overflow-y-auto"
          style={{
            color: "var(--dac-text-primary)",
            padding: "6px 10px",
            top: position.top,
            left: `clamp(8px, ${center - position.popoverWidth / 2}px, max(8px, calc(100vw - ${position.popoverWidth + 8}px)))`,
            width: "max-content",
            maxWidth: "min(576px, calc(100vw - 16px))",
            // Half the viewport always fits above or below the trigger, so long text scrolls instead of clipping.
            maxHeight: "min(360px, calc(50dvh - 24px))",
            overflowWrap: "anywhere",
          }}
        >
          {description}
        </div>,
        popoverPortalTarget(),
      )}
    </>
  );
}
