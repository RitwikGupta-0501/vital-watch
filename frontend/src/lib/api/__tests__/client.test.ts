import { describe, it, expect, vi, beforeEach } from 'vitest';
import {
  fetchApi,
  RateLimitError,
  _resetRefreshState,
} from '../client';

describe('API Client', () => {
  beforeEach(() => {
    vi.restoreAllMocks();
    _resetRefreshState();
  });

  it('successfully fetches and parses JSON on 200 OK', async () => {
    const mockData = { id: '123', name: 'Dr. Smith' };
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(JSON.stringify(mockData), {
        status: 200,
        headers: { 'Content-Type': 'application/json' },
      })
    );
    vi.stubGlobal('fetch', fetchMock);

    const result = await fetchApi<typeof mockData>('/api/test');
    expect(result).toEqual(mockData);
    expect(fetchMock).toHaveBeenCalledTimes(1);
  });

  it('handles 204 No Content smoothly', async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(null, { status: 204 })
    );
    vi.stubGlobal('fetch', fetchMock);

    const result = await fetchApi('/api/empty');
    expect(result).toEqual({});
  });

  it('throws RateLimitError on 429 with parsed Retry-After header', async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(JSON.stringify({ error: 'Too many requests' }), {
        status: 429,
        headers: {
          'Content-Type': 'application/json',
          'Retry-After': '45',
        },
      })
    );
    vi.stubGlobal('fetch', fetchMock);

    await expect(fetchApi('/api/limited')).rejects.toThrow(RateLimitError);

    try {
      await fetchApi('/api/limited');
    } catch (err) {
      expect(err).toBeInstanceOf(RateLimitError);
      expect((err as RateLimitError).retryAfterSeconds).toBe(45);
      expect((err as RateLimitError).status).toBe(429);
    }
  });

  it('triggers silent refresh on 401 and replays original request', async () => {
    let callCount = 0;
    const fetchMock = vi.fn().mockImplementation((url: string) => {
      if (url.includes('/api/auth/refresh')) {
        return Promise.resolve(
          new Response(JSON.stringify({ access_token: 'new-token' }), {
            status: 200,
            headers: { 'Content-Type': 'application/json' },
          })
        );
      }
      if (url.includes('/api/protected')) {
        callCount++;
        if (callCount === 1) {
          // First call 401
          return Promise.resolve(
            new Response(JSON.stringify({ error: 'Token expired' }), {
              status: 401,
              headers: { 'Content-Type': 'application/json' },
            })
          );
        }
        // Second call 200
        return Promise.resolve(
          new Response(JSON.stringify({ message: 'Success' }), {
            status: 200,
            headers: { 'Content-Type': 'application/json' },
          })
        );
      }
      return Promise.reject(new Error(`Unexpected url: ${url}`));
    });
    vi.stubGlobal('fetch', fetchMock);

    const res = await fetchApi<{ message: string }>('/api/protected');
    expect(res).toEqual({ message: 'Success' });
    expect(callCount).toBe(2);
  });

  it('survives concurrent 401 storm with EXACTLY ONE call to /api/auth/refresh', async () => {
    let refreshCalls = 0;
    const requestCalls: Record<string, number> = {};

    const fetchMock = vi.fn().mockImplementation(async (url: string) => {
      if (url.includes('/api/auth/refresh')) {
        refreshCalls++;
        // Simulate small network delay for refresh
        await new Promise((resolve) => setTimeout(resolve, 30));
        return new Response(JSON.stringify({ access_token: 'new-jwt' }), {
          status: 200,
          headers: { 'Content-Type': 'application/json' },
        });
      }

      // Track individual endpoint calls
      requestCalls[url] = (requestCalls[url] || 0) + 1;
      if (requestCalls[url] === 1) {
        // First attempt 401
        return new Response(JSON.stringify({ error: 'Unauthorized' }), {
          status: 401,
          headers: { 'Content-Type': 'application/json' },
        });
      }
      // Replayed attempt 200
      return new Response(JSON.stringify({ data: `Success for ${url}` }), {
        status: 200,
        headers: { 'Content-Type': 'application/json' },
      });
    });
    vi.stubGlobal('fetch', fetchMock);

    // Fire 10 parallel requests that simultaneously hit 401
    const endpoints = Array.from({ length: 10 }, (_, i) => `/api/resource/${i}`);
    const results = await Promise.all(
      endpoints.map((ep) => fetchApi<{ data: string }>(ep))
    );

    // Verify all 10 completed successfully
    expect(results).toHaveLength(10);
    results.forEach((res, i) => {
      expect(res.data).toBe(`Success for /api/resource/${i}`);
    });

    // CRITICAL: Refresh must have been called EXACTLY ONCE
    expect(refreshCalls).toBe(1);
  });
});
