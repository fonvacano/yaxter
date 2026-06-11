import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import * as gen from '../../api/generated';
import { TweetCard } from './TweetCard';
import type { Tweet } from '../../api/generated';

const tweet = {
  id: '5', author: { id: '9', username: 'alice' }, text: 'hi',
  likes_count: 3, retweets_count: 0, liked_by_me: false, created_at: 'x',
} as unknown as Tweet;

describe('TweetCard optimistic like', () => {
  it('increments immediately before the request resolves', async () => {
    let resolve!: () => void;
    vi.spyOn(gen, 'likeTweet').mockReturnValue(
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      new Promise<any>((r) => { resolve = () => r(undefined); }),
    );
    const qc = new QueryClient();
    render(
      <QueryClientProvider client={qc}>
        <MemoryRouter><TweetCard tweet={tweet} /></MemoryRouter>
      </QueryClientProvider>,
    );
    const btn = screen.getByRole('button', { name: /like/i });
    expect(btn).toHaveTextContent('3');
    fireEvent.click(btn);
    // optimistic: count jumps to 4 without awaiting the resolve
    expect(btn).toHaveTextContent('4');
    resolve();
  });
});
