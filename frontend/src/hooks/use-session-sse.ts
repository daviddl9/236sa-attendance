import { useEffect, useRef, useState } from "react";
import { useQueryClient } from "@tanstack/react-query";

// SSE Event types matching backend
interface SSEEvent {
  type:
    | "connected"
    | "attendance_marked"
    | "attendance_removed"
    | "session_closed"
    | "status_changed"
    | "heartbeat";
  payload: unknown;
}

interface UseSessionSSEOptions {
  sessionId: string;
  enabled?: boolean;
}

interface UseSessionSSEResult {
  isConnected: boolean;
}

export function useSessionSSE(
  options: UseSessionSSEOptions
): UseSessionSSEResult {
  const { sessionId, enabled = true } = options;
  const queryClient = useQueryClient();
  const eventSourceRef = useRef<EventSource | null>(null);
  const reconnectTimeoutRef = useRef<ReturnType<typeof setTimeout> | null>(
    null
  );
  const reconnectAttempts = useRef(0);
  const [isConnected, setIsConnected] = useState(false);

  useEffect(() => {
    if (!enabled || !sessionId) return;

    let isCancelled = false;

    const scheduleReconnect = (delay: number) => {
      if (isCancelled) return;
      reconnectTimeoutRef.current = setTimeout(connect, delay);
    };

    const connect = () => {
      if (isCancelled) return;

      // Clean up existing connection
      if (eventSourceRef.current) {
        eventSourceRef.current.close();
      }

      const apiUrl = import.meta.env.VITE_API_URL || "http://localhost:8080";
      const url = `${apiUrl}/api/sessions/${sessionId}/stream`;

      const eventSource = new EventSource(url, { withCredentials: true });
      eventSourceRef.current = eventSource;

      eventSource.onopen = () => {
        reconnectAttempts.current = 0;
        setIsConnected(true);
      };

      eventSource.onmessage = (event) => {
        try {
          const data: SSEEvent = JSON.parse(event.data);

          switch (data.type) {
            case "connected":
              // Connection established
              break;
            case "attendance_marked":
            case "attendance_removed":
            case "status_changed":
              // Invalidate queries to refresh data silently
              queryClient.invalidateQueries({
                queryKey: ["session-analytics", sessionId],
              });
              break;
            case "session_closed":
              // Invalidate session query to refresh status
              queryClient.invalidateQueries({
                queryKey: ["session", sessionId],
              });
              queryClient.invalidateQueries({
                queryKey: ["session-analytics", sessionId],
              });
              break;
            case "heartbeat":
              // Connection alive, no action needed
              break;
          }
        } catch {
          // Ignore parse errors
        }
      };

      eventSource.onerror = () => {
        setIsConnected(false);
        eventSource.close();

        // Exponential backoff reconnect (max 30 seconds)
        const delay = Math.min(
          1000 * Math.pow(2, reconnectAttempts.current),
          30000
        );
        reconnectAttempts.current++;

        scheduleReconnect(delay);
      };
    };

    connect();

    return () => {
      isCancelled = true;
      if (eventSourceRef.current) {
        eventSourceRef.current.close();
        eventSourceRef.current = null;
      }
      if (reconnectTimeoutRef.current) {
        clearTimeout(reconnectTimeoutRef.current);
        reconnectTimeoutRef.current = null;
      }
      setIsConnected(false);
    };
  }, [sessionId, enabled, queryClient]);

  return {
    isConnected,
  };
}
