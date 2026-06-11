import { describe, it, expect, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import * as gen from '../../api/generated';
import { TimelineList } from './TimelineList';

function setup() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={qc}>
      <MemoryRouter>
        <TimelineList />
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

describe('TimelineList', () => {
  it('renders fetched tweets', async () => {
    vi.spyOn(gen, 'getHomeTimeline').mockResolvedValue({
      items: [
        { id: '2', author: { id: '9', username: 'a' }, text: 'first', likes_count: 0, retweets_count: 0, created_at: '2026-06-11T10:00:00Z' },
      ],
      next_cursor: null,
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
    } as any);
    setup();
    await waitFor(() => expect(screen.getByText('first')).toBeInTheDocument());
  });

  it('shows empty state', async () => {
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    vi.spyOn(gen, 'getHomeTimeline').mockResolvedValue({ items: [], next_cursor: null } as any);
    setup();
    await waitFor(() => expect(screen.getByText(/nothing here yet/i)).toBeInTheDocument());
  });
});
