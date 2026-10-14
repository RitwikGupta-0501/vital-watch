import { useEffect, useRef, useState, useCallback } from 'react';
import { useQueryClient } from '@tanstack/react-query';

export type SSEConnectionStatus = 'connecting' | 'connected' | 'disconnected' | 'reconnecting';

export interface SSEMessageEvent<T = unknown> {
  type: string;
  data: T;
  timestamp: string;
}

interface UseNotificationStreamOptions {
  enabled?: boolean;
  onEvent?: (event: SSEMessageEvent) => void;
}

export function useNotificationStream({
  enabled = true,
  onEvent,
}: UseNotificationStreamOptions = {}) {
  const [status, setStatus] = useState<SSEConnectionStatus>(enabled ? 'connecting' : 'disconnected');
  const [lastEvent, setLastEvent] = useState<SSEMessageEvent | null>(null);

  const eventSourceRef = useRef<EventSource | null>(null);
  const reconnectTimeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const heartbeatTimeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const retryCountRef = useRef(0);
  const queryClient = useQueryClient();

  const connectRef = useRef<() => void>(() => {});
  const scheduleReconnectRef = useRef<() => void>(() => {});

  const resetHeartbeat = useCallback(() => {
    if (heartbeatTimeoutRef.current) {
      clearTimeout(heartbeatTimeoutRef.current);
    }
    // 30 seconds timeout (heartbeat arrives every 15s)
    heartbeatTimeoutRef.current = setTimeout(() => {
      if (eventSourceRef.current) {
        eventSourceRef.current.close();
        setStatus('reconnecting');
        scheduleReconnectRef.current();
      }
    }, 30000);
  }, []);

  const scheduleReconnect = useCallback(() => {
    if (reconnectTimeoutRef.current) {
      clearTimeout(reconnectTimeoutRef.current);
    }
    const delay = Math.min(1000 * Math.pow(2, retryCountRef.current), 30000);
    retryCountRef.current += 1;
    setStatus('reconnecting');

    reconnectTimeoutRef.current = setTimeout(() => {
      connectRef.current();
    }, delay);
  }, []);

  const connect = useCallback(() => {
    if (!enabled) return;

    if (eventSourceRef.current) {
      eventSourceRef.current.close();
    }

    setStatus(retryCountRef.current === 0 ? 'connecting' : 'reconnecting');

    try {
      // Browser EventSource automatically includes credentials when withCredentials is true
      const es = new EventSource('/api/notifications/stream', {
        withCredentials: true,
      });
      eventSourceRef.current = es;

      es.onopen = () => {
        setStatus('connected');
        retryCountRef.current = 0;
        resetHeartbeat();

        // The broker drops on slow consumers, so refetch active queries upon connect/reconnect
        queryClient.invalidateQueries();
      };

      const handleMessage = (type: string, rawData: string) => {
        resetHeartbeat();
        let parsedData: unknown = rawData;
        try {
          parsedData = JSON.parse(rawData);
        } catch {
          // Keep as string
        }

        const event: SSEMessageEvent = {
          type,
          data: parsedData,
          timestamp: new Date().toISOString(),
        };

        setLastEvent(event);
        onEvent?.(event);

        // Auto-invalidate relevant TanStack Query keys based on notification types
        if (type.startsWith('prescription.')) {
          queryClient.invalidateQueries({ queryKey: ['prescriptions'] });
        } else if (type.startsWith('appointment.')) {
          queryClient.invalidateQueries({ queryKey: ['appointments'] });
        } else if (type === 'vital.alert') {
          queryClient.invalidateQueries({ queryKey: ['vitals'] });
        } else if (type === 'medication.logged') {
          queryClient.invalidateQueries({ queryKey: ['medication-schedule'] });
        }
      };

      es.onmessage = (e) => {
        handleMessage('message', e.data);
      };

      // Named custom events from backend
      const namedEvents = [
        'prescription.ocr_completed',
        'prescription.approved',
        'prescription.rejected',
        'appointment.booked',
        'appointment.completed',
        'appointment.cancelled',
        'vital.alert',
        'medication.logged',
      ];

      namedEvents.forEach((eventName) => {
        es.addEventListener(eventName, (e: MessageEvent) => {
          handleMessage(eventName, e.data);
        });
      });

      es.onerror = () => {
        es.close();
        setStatus('reconnecting');
        scheduleReconnectRef.current();
      };
    } catch {
      setStatus('reconnecting');
      scheduleReconnectRef.current();
    }
  }, [enabled, onEvent, queryClient, resetHeartbeat]);

  useEffect(() => {
    connectRef.current = connect;
    scheduleReconnectRef.current = scheduleReconnect;
  }, [connect, scheduleReconnect]);

  useEffect(() => {
    if (enabled) {
      connectRef.current();
    } else {
      if (eventSourceRef.current) {
        eventSourceRef.current.close();
      }
    }

    return () => {
      if (eventSourceRef.current) {
        eventSourceRef.current.close();
      }
      if (reconnectTimeoutRef.current) {
        clearTimeout(reconnectTimeoutRef.current);
      }
      if (heartbeatTimeoutRef.current) {
        clearTimeout(heartbeatTimeoutRef.current);
      }
    };
  }, [enabled]);

  return {
    status,
    lastEvent,
    reconnect: connect,
  };
}
