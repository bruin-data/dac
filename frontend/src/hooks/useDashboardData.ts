import { useState, useEffect, useRef, useCallback } from "react";
import { streamDashboardData } from "../api/client";
import type { WidgetData } from "../types/dashboard";

// Embed injects the dac-serve URL; else relative.
const API_BASE = (window.__DAC_API_BASE__ || "") + "/api/v1";

interface StreamState {
  /** Accumulated widget results, updated as each arrives. */
  data: Record<string, WidgetData> | undefined;
  /** True while the stream is open and results are still arriving. */
  isLoading: boolean;
  /** Set if the stream fails entirely. */
  error: Error | undefined;
}

/**
 * Listens for SSE `full_reload` events and bumps a counter so hooks can
 * re-fetch. Shared across all instances via a module-level listener.
 */
const reloadListeners = new Set<() => void>();

function onReloadTick(fn: () => void) {
  reloadListeners.add(fn);
  return () => { reloadListeners.delete(fn); };
}

// Single SSE listener (module scope, starts once).
if (typeof window !== "undefined" && !window.__DAC_STATIC__) {
  const es = new EventSource(`${API_BASE}/events`);
  es.onmessage = (event) => {
    try {
      const data = JSON.parse(event.data);
      if (data.type === "full_reload") {
        for (const fn of reloadListeners) fn();
      }
    } catch { /* ignore */ }
  };
}

export function useDashboardData(
  name: string,
  filters?: Record<string, unknown>,
  enabled = true,
): StreamState {
  const [data, setData] = useState<Record<string, WidgetData> | undefined>(undefined);
  const [isLoading, setIsLoading] = useState(!!name && enabled);
  const [error, setError] = useState<Error | undefined>(undefined);
  const abortRef = useRef<(() => void) | null>(null);

  // Stable serialized key so we can detect filter changes.
  const filterKey = filters ? JSON.stringify(filters) : "";

  const startStream = useCallback(() => {
    // Abort any in-flight stream.
    abortRef.current?.();

    setData(undefined);
    setIsLoading(true);
    setError(undefined);

    const abort = streamDashboardData(
      name,
      filters,
      (id, widgetData) => {
        setData((prev) => ({ ...prev, [id]: widgetData }));
      },
      () => {
        setIsLoading(false);
      },
      (err) => {
        setError(err);
        setIsLoading(false);
      },
    );

    abortRef.current = abort;
  }, [name, filterKey]); // eslint-disable-line react-hooks/exhaustive-deps

  useEffect(() => {
    if (!name || !enabled) {
      setIsLoading(false);
      return;
    }

    startStream();

    return () => {
      abortRef.current?.();
      abortRef.current = null;
    };
  }, [name, enabled, startStream]);

  // Re-stream when dashboard files change (SSE full_reload).
  useEffect(() => {
    if (!name || !enabled) return;
    return onReloadTick(() => {
      startStream();
    });
  }, [name, enabled, startStream]);

  return { data, isLoading, error };
}
