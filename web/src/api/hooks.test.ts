import { describe, it, expect, vi, beforeEach } from 'vitest';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { renderHook, waitFor } from '@testing-library/react';
import type { ReactNode } from 'react';
import { createElement } from 'react';
import * as gen from './generated';
import { useHomeTimeline } from './hooks';

function wrapper() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return ({ children }: { children: ReactNode }) =>
    createElement(QueryClientProvider, { client: qc }, children);
}

describe('useHomeTimeline', () => {
  beforeEach(() => vi.restoreAllMocks());

  it('flattens paginated items and exposes the next cursor', async () => {
    vi.spyOn(gen, 'getHomeTimeline').mockResolvedValue({
      items: [{ id: '2', author: { id: '9', username: 'a' }, text: 'hi', likes_count: 0, retweets_count: 0, created_at: '2026-01-01T00:00:00Z' }],
      next_cursor: null,
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
    } as any);

    const { result } = renderHook(() => useHomeTimeline(), { wrapper: wrapper() });
    await waitFor(() => expect(result.current.tweets.length).toBe(1));
    expect(result.current.tweets[0].text).toBe('hi');
  });
});
