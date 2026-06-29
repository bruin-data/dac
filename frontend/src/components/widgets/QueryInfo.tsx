import { useState, useRef, useCallback, useEffect } from "react";
import { createPortal } from "react-dom";
import { useShikiHighlight } from "../../hooks/useShikiHighlight";

interface Props {
  query: string;
}

function CopyIcon() {
  return (
    <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
      <rect x="9" y="9" width="13" height="13" rx="2" ry="2" />
      <path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1" />
    </svg>
  );
}

function CheckIcon() {
  return (
    <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round">
      <polyline points="20 6 9 17 4 12" />
    </svg>
  );
}

export function QueryInfo({ query }: Props) {
  const [visible, setVisible] = useState(false);
  const [copied, setCopied] = useState(false);
  const showTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const hideTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const buttonRef = useRef<HTMLButtonElement>(null);
  const popoverRef = useRef<HTMLDivElement>(null);
  const [pos, setPos] = useState<{ top: number; left: number } | null>(null);

  const trimmed = query.trim();
  const html = useShikiHighlight(visible ? trimmed : null, "sql");

  const cancelHide = useCallback(() => {
    if (hideTimerRef.current) {
      clearTimeout(hideTimerRef.current);
      hideTimerRef.current = null;
    }
  }, []);

  const cancelShow = useCallback(() => {
    if (showTimerRef.current) {
      clearTimeout(showTimerRef.current);
      showTimerRef.current = null;
    }
  }, []);

  const scheduleHide = useCallback(() => {
    cancelHide();
    hideTimerRef.current = setTimeout(() => {
      setVisible(false);
      setPos(null);
      setCopied(false);
    }, 150);
  }, [cancelHide]);

  const handleButtonEnter = useCallback(() => {
    cancelHide();
    cancelShow();
    showTimerRef.current = setTimeout(() => setVisible(true), 800);
  }, [cancelHide, cancelShow]);

  const handleButtonLeave = useCallback(() => {
    cancelShow();
    scheduleHide();
  }, [cancelShow, scheduleHide]);

  const handlePopoverEnter = useCallback(() => {
    cancelHide();
    cancelShow();
  }, [cancelHide, cancelShow]);

  const handlePopoverLeave = useCallback(() => {
    scheduleHide();
  }, [scheduleHide]);

  const handleCopy = useCallback(() => {
    navigator.clipboard.writeText(trimmed).then(() => {
      setCopied(true);
      setTimeout(() => setCopied(false), 1500);
    });
  }, [trimmed]);

  // Position the popover after it renders.
  useEffect(() => {
    if (!visible || !buttonRef.current || !popoverRef.current) return;

    const btnRect = buttonRef.current.getBoundingClientRect();
    const popRect = popoverRef.current.getBoundingClientRect();

    let top = btnRect.top - popRect.height - 6;
    let left = btnRect.left + btnRect.width / 2 - popRect.width / 2;

    if (left < 8) left = 8;
    if (left + popRect.width > window.innerWidth - 8) {
      left = window.innerWidth - 8 - popRect.width;
    }
    if (top < 8) {
      top = btnRect.bottom + 6;
    }

    setPos({ top, left });
  }, [visible]);

  useEffect(() => {
    return () => {
      if (showTimerRef.current) clearTimeout(showTimerRef.current);
      if (hideTimerRef.current) clearTimeout(hideTimerRef.current);
    };
  }, []);

  return (
    <>
      <button
        ref={buttonRef}
        type="button"
        data-dac-export-control
        onMouseEnter={handleButtonEnter}
        onMouseLeave={handleButtonLeave}
        className="inline-flex items-center justify-center w-4 h-4 rounded-full text-[9px] font-medium leading-none opacity-0 group-hover:opacity-30 hover:!opacity-60 transition-opacity duration-150 cursor-default select-none"
        style={{
          color: "var(--dac-text-muted)",
          border: "1px solid currentColor",
        }}
        tabIndex={-1}
      >
        i
      </button>

      {visible &&
        createPortal(
          <div
            ref={popoverRef}
            onMouseEnter={handlePopoverEnter}
            onMouseLeave={handlePopoverLeave}
            className="fixed z-[9999]"
            style={{
              top: pos?.top ?? -9999,
              left: pos?.left ?? -9999,
              opacity: pos ? 1 : 0,
              transition: "opacity 100ms ease",
            }}
          >
            <div
              className="rounded-md border shadow-lg overflow-hidden"
              style={{
                background: "var(--dac-surface, #fff)",
                borderColor: "var(--dac-border, #e5e7eb)",
                minWidth: 320,
                maxWidth: "min(640px, 90vw)",
                maxHeight: 360,
                width: "max-content",
              }}
            >
              <div
                className="flex items-center px-3 py-1.5 border-b"
                style={{
                  borderColor: "var(--dac-border, #e5e7eb)",
                  background: "var(--dac-background, #f9fafb)",
                }}
              >
                <span
                  className="text-[10px] font-medium uppercase tracking-wider"
                  style={{ color: "var(--dac-text-muted, #6b7280)" }}
                >
                  SQL Query
                </span>
                <button
                  type="button"
                  onClick={handleCopy}
                  className="ml-auto flex items-center gap-1 px-1.5 py-0.5 rounded text-[10px] transition-colors duration-100"
                  style={{
                    color: copied ? "var(--dac-success, #22c55e)" : "var(--dac-text-muted, #6b7280)",
                  }}
                  onMouseOver={(e) => {
                    if (!copied) (e.currentTarget.style.color = "var(--dac-text-primary, #111)");
                  }}
                  onMouseOut={(e) => {
                    if (!copied) (e.currentTarget.style.color = "var(--dac-text-muted, #6b7280)");
                  }}
                >
                  {copied ? <CheckIcon /> : <CopyIcon />}
                  {copied ? "Copied" : "Copy"}
                </button>
              </div>
              <div
                className="shiki-highlight px-3 py-2.5 overflow-auto"
                style={{ maxHeight: 310 }}
              >
                {html ? (
                  <div dangerouslySetInnerHTML={{ __html: html }} />
                ) : (
                  <pre
                    className="text-[11.5px] leading-[1.55] m-0"
                    style={{
                      color: "var(--dac-text-primary, #111)",
                      fontFamily: '"Geist Mono", ui-monospace, monospace',
                      whiteSpace: "pre",
                    }}
                  >
                    {trimmed}
                  </pre>
                )}
              </div>
            </div>
          </div>,
          document.body,
        )}
    </>
  );
}
