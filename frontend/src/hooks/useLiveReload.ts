import { useEffect } from "react";
import { useQueryClient } from "@tanstack/react-query";

// Embed injects the dac-serve URL; else relative.
const API_BASE = (window.__DAC_API_BASE__ || "") + "/api/v1";

export function useLiveReload() {
  const queryClient = useQueryClient();

  useEffect(() => {
    // No server to connect to in static mode.
    if (window.__DAC_STATIC__) return;

    const es = new EventSource(`${API_BASE}/events`);

    es.onmessage = (event) => {
      try {
        const data = JSON.parse(event.data);
        if (data.type === "full_reload") {
          queryClient.invalidateQueries();
        }
      } catch {
        // ignore parse errors
      }
    };

    es.onerror = () => {
      // EventSource will automatically reconnect
    };

    return () => es.close();
  }, [queryClient]);
}
