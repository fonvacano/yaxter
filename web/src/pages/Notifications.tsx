import { useNotifications, useMarkRead } from '../api/hooks';
import { Async } from '../ui/Async';
import { Link } from 'react-router-dom';
import type { Notification } from '../api/generated';

function formatNotif(n: Notification): string {
  switch (n.kind) {
    case 'follow':
      return n.actor.username + ' followed you';
    case 'like':
      return n.actor.username + ' liked your tweet';
    case 'retweet':
      return n.actor.username + ' retweeted your tweet';
    default:
      return n.actor.username + ' interacted';
  }
}

export default function Notifications() {
  const { items, isLoading, error, hasNextPage, fetchNextPage, isFetchingNextPage } =
    useNotifications();
  const mark = useMarkRead();

  return (
    <section>
      <header>
        <h1>Notifications</h1>
        {items.length > 0 && (
          <button
            type="button"
            onClick={() => mark.mutate({ up_to_id: String(items[0].id) })}
            disabled={mark.isPending}
          >
            Mark all read
          </button>
        )}
      </header>
      <Async isLoading={isLoading} error={error} empty={!isLoading && items.length === 0} emptyMessage="No notifications">
        <ul aria-label="notifications list">
          {items.map((n) => (
            <li key={String(n.id)} aria-current={n.read ? undefined : 'true'}>
              {n.subject_id ? <Link to="/">{formatNotif(n)}</Link> : formatNotif(n)}
            </li>
          ))}
        </ul>
        {hasNextPage && (
          <button type="button" onClick={() => fetchNextPage()} disabled={isFetchingNextPage}>
            Load more
          </button>
        )}
      </Async>
    </section>
  );
}
