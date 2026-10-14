import type { ApiErrorResponse } from './types';

export const API_BASE = '';

export class ApiError extends Error {
  status: number;
  data?: unknown;

  constructor(status: number, message: string, data?: unknown) {
    super(message);
    this.name = 'ApiError';
    this.status = status;
    this.data = data;
  }
}

export class RateLimitError extends ApiError {
  retryAfterSeconds: number;

  constructor(retryAfterSeconds: number, message = 'Rate limit exceeded. Please wait.') {
    super(429, message);
    this.name = 'RateLimitError';
    this.retryAfterSeconds = retryAfterSeconds;
  }
}

// In-flight refresh token mutex
let refreshPromise: Promise<boolean> | null = null;

// Auth failure listeners (subscribed by AuthContext)
type AuthExpiredListener = () => void;
const authExpiredListeners = new Set<AuthExpiredListener>();

export function subscribeAuthExpired(listener: AuthExpiredListener): () => void {
  authExpiredListeners.add(listener);
  return () => {
    authExpiredListeners.delete(listener);
  };
}

export function notifyAuthExpired(): void {
  for (const listener of authExpiredListeners) {
    try {
      listener();
    } catch {
      // Ignore errors inside listeners
    }
  }
}

// Reset state for testing
export function _resetRefreshState(): void {
  refreshPromise = null;
  authExpiredListeners.clear();
}

/**
 * Attempts a silent token refresh via /api/auth/refresh behind a concurrency mutex.
 * If multiple requests 401 simultaneously, they await the identical in-flight promise,
 * preventing duplicate refresh calls that trigger security alerts & session revocation.
 */
export async function executeSilentRefresh(): Promise<boolean> {
  if (refreshPromise) {
    return refreshPromise;
  }

  refreshPromise = (async () => {
    try {
      const response = await fetch('/api/auth/refresh', {
        method: 'POST',
        credentials: 'include',
        headers: {
          'Content-Type': 'application/json',
        },
      });

      if (!response.ok) {
        notifyAuthExpired();
        return false;
      }

      return true;
    } catch {
      notifyAuthExpired();
      return false;
    } finally {
      // Clear mutex after completion
      refreshPromise = null;
    }
  })();

  return refreshPromise;
}

interface FetchApiOptions extends RequestInit {
  skipAuthRefresh?: boolean;
  _retryCount?: number;
}

export async function fetchApi<T>(endpoint: string, options: FetchApiOptions = {}): Promise<T> {
  const { skipAuthRefresh = false, _retryCount = 0, ...init } = options;
  const url = endpoint.startsWith('http') ? endpoint : `${API_BASE}${endpoint}`;

  const config: RequestInit = {
    ...init,
    credentials: 'include',
    headers: {
      'Content-Type': 'application/json',
      ...init.headers,
    },
  };

  const response = await fetch(url, config);

  // Handle 401 Unauthorized with single-use mutex refresh
  if (response.status === 401 && !skipAuthRefresh && _retryCount < 1) {
    // Avoid refreshing if the failing endpoint is auth itself
    const isAuthRoute =
      endpoint.includes('/api/auth/refresh') ||
      endpoint.includes('/api/login') ||
      endpoint.includes('/api/auth/logout');

    if (!isAuthRoute) {
      const refreshSuccess = await executeSilentRefresh();
      if (refreshSuccess) {
        // Replay original request with refreshed session
        return fetchApi<T>(endpoint, {
          ...options,
          _retryCount: _retryCount + 1,
        });
      }
    }
  }

  // Handle 429 Too Many Requests
  if (response.status === 429) {
    const retryAfterHeader = response.headers?.get('Retry-After');
    let retryAfterSeconds = 60; // default 60s
    if (retryAfterHeader) {
      const parsed = parseInt(retryAfterHeader, 10);
      if (!isNaN(parsed) && parsed > 0) {
        retryAfterSeconds = parsed;
      }
    }

    let message = `Rate limit exceeded. Please try again in ${retryAfterSeconds}s.`;
    try {
      const body = (await response.json()) as ApiErrorResponse;
      if (body.error) {
        message = body.error;
      }
    } catch {
      // Body not JSON
    }

    throw new RateLimitError(retryAfterSeconds, message);
  }

  // Handle other non-2xx responses
  if (!response.ok) {
    let errorMessage = `Request failed with status ${response.status}`;
    let errorData: unknown;
    try {
      errorData = await response.json();
      if (typeof errorData === 'object' && errorData !== null) {
        const errObj = errorData as Record<string, unknown>;
        if (typeof errObj.error === 'string') {
          errorMessage = errObj.error;
        } else if (typeof errObj.message === 'string') {
          errorMessage = errObj.message;
        }
      }
    } catch {
      // Body not JSON
    }
    throw new ApiError(response.status, errorMessage, errorData);
  }

  // Handle 204 No Content
  if (response.status === 204) {
    return {} as T;
  }

  return response.json() as Promise<T>;
}
