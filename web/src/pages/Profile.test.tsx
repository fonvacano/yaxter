import { describe, it, expect, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter, Routes, Route } from 'react-router-dom';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import * as gen from '../api/generated';
import Profile from './Profile';

function setup(path = '/u/alice') {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={qc}>
      <MemoryRouter initialEntries={[path]}>
        <Routes>
          <Route path="/u/:username" element={<Profile />} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

describe('Profile', () => {
  it('renders the user header and their tweets', async () => {
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    vi.spyOn(gen, 'getUser').mockResolvedValue({ id: '9', username: 'alice', followers_count: 2, following_count: 1 } as any);
    vi.spyOn(gen, 'getUserTweets').mockResolvedValue({
      items: [{ id: '3', author: { id: '9', username: 'alice' }, text: 'my tweet', likes_count: 0, retweets_count: 0, created_at: 'x' }],
      next_cursor: null,
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
    } as any);
    setup();
    await waitFor(() => expect(screen.getByRole('heading', { name: 'alice' })).toBeInTheDocument());
    expect(screen.getByText('my tweet')).toBeInTheDocument();
  });
});
