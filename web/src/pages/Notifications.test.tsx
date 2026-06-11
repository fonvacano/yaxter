import { describe, it, expect, vi } from 'vitest';
import { render, screen, waitFor, fireEvent } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import * as gen from '../api/generated';
import Notifications from './Notifications';

function setup() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={qc}>
      <MemoryRouter><Notifications /></MemoryRouter>
    </QueryClientProvider>,
  );
}

describe('Notifications', () => {
  it('lists notifications and marks read', async () => {
    vi.spyOn(gen, 'listNotifications').mockResolvedValue({
      items: [
        { id: '10', kind: 'follow', actor: { id: '2', username: 'bob' }, created_at: 'x', read: false },
        { id: '9', kind: 'like', actor: { id: '3', username: 'cy' }, subject_id: '99', created_at: 'x', read: false },
      ],
      next_cursor: null,
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
    } as any);
    const mark = vi.spyOn(gen, 'markNotificationsRead').mockResolvedValue(undefined as never);

    setup();
    await waitFor(() => expect(screen.getByText(/bob/)).toBeInTheDocument());
    fireEvent.click(screen.getByRole('button', { name: /mark all read/i }));
    await waitFor(() => expect(mark).toHaveBeenCalledWith({ up_to_id: '10' }));
  });
});
