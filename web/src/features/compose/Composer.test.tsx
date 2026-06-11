import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import * as gen from '../../api/generated';
import { Composer } from './Composer';

function setup() {
  const qc = new QueryClient({ defaultOptions: { mutations: { retry: false } } });
  return render(
    <QueryClientProvider client={qc}>
      <Composer />
    </QueryClientProvider>,
  );
}

describe('Composer', () => {
  it('posts a text-only tweet', async () => {
    const spy = vi.spyOn(gen, 'createTweet').mockResolvedValue(
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      { id: '1', author: { id: '9', username: 'me' }, text: 'hi', likes_count: 0, retweets_count: 0, created_at: 'x' } as any,
    );
    setup();
    fireEvent.change(screen.getByLabelText(/what's happening/i), { target: { value: 'hi' } });
    fireEvent.click(screen.getByRole('button', { name: /post/i }));
    await waitFor(() => expect(spy).toHaveBeenCalledWith({ text: 'hi', media_ids: undefined, retweet_of_id: undefined }));
  });

  it('disables post when text is empty and no media', () => {
    setup();
    expect(screen.getByRole('button', { name: /post/i })).toBeDisabled();
  });
});
