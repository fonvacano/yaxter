import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { TweetCard } from './TweetCard';
import type { Tweet } from '../../api/generated';

const tweet: Tweet = {
  id: '5', author: { id: '9', username: 'alice' }, text: 'hello world',
  likes_count: 3, retweets_count: 1, created_at: '2026-06-11T10:00:00Z',
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
} as any;

function renderCard(t: Tweet) {
  const qc = new QueryClient();
  return render(
    <QueryClientProvider client={qc}>
      <MemoryRouter>
        <TweetCard tweet={t} />
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

describe('TweetCard', () => {
  it('renders author, text, and counters', () => {
    renderCard(tweet);
    expect(screen.getByText('hello world')).toBeInTheDocument();
    expect(screen.getByText('alice')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /like/i })).toHaveTextContent('3');
  });
});
