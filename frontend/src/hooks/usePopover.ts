import { useCallback, useEffect, useLayoutEffect, useRef, useState } from "react";

export interface PopoverPosition {
  top: number;
  left: number;
  width: number;
}

export function usePopover(initialOpen = false) {
  const [open, setOpen] = useState(initialOpen);
  const popoverRef = useRef<HTMLDivElement>(null);
  const triggerRef = useRef<HTMLButtonElement>(null);
  const [position, setPosition] = useState<PopoverPosition>({ top: 0, left: 0, width: 0 });

  const updatePosition = useCallback(() => {
    if (!triggerRef.current) return;
    const rect = triggerRef.current.getBoundingClientRect();
    setPosition({ top: rect.bottom + 4, left: rect.left, width: rect.width });
  }, []);

  useLayoutEffect(() => {
    if (open) updatePosition();
  }, [open, updatePosition]);

  useEffect(() => {
    if (!open) return;
    function handleMouseDown(e: MouseEvent) {
      if (
        popoverRef.current &&
        !popoverRef.current.contains(e.target as Node) &&
        triggerRef.current &&
        !triggerRef.current.contains(e.target as Node)
      ) {
        setOpen(false);
      }
    }
    function handleKey(e: KeyboardEvent) {
      if (e.key === "Escape") setOpen(false);
    }
    document.addEventListener("mousedown", handleMouseDown);
    document.addEventListener("keydown", handleKey);
    window.addEventListener("scroll", updatePosition, true);
    return () => {
      document.removeEventListener("mousedown", handleMouseDown);
      document.removeEventListener("keydown", handleKey);
      window.removeEventListener("scroll", updatePosition, true);
    };
  }, [open, updatePosition]);

  return { open, setOpen, popoverRef, triggerRef, position };
}

export function popoverPortalTarget(): Element {
  return document.querySelector(".dac-root") ?? document.body;
}
