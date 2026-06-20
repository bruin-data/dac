import { useEffect } from "react";
import { useQueryClient } from "@tanstack/react-query";

export function useLiveReload() {
  const queryClient = useQueryClient();

  useEffect(() => {
    // No server to connect to in static mode.
    if (window.__DAC_STATIC__) return;

    const es = new EventSource("/api/v1/events");

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
