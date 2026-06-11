import { useHomeTimeline } from '../../api/hooks';
import { Async } from '../../ui/Async';
import { TweetCard } from './TweetCard';

export function TimelineList() {
  const { tweets, isLoading, error, hasNextPage, fetchNextPage, isFetchingNextPage } =
    useHomeTimeline();

  return (
    <Async isLoading={isLoading} error={error} empty={!isLoading && tweets.length === 0}>
      <div aria-label="home timeline">
        {tweets.map((t) => (
          <TweetCard key={String(t.id)} tweet={t} />
        ))}
        {hasNextPage && (
          <button type="button" onClick={() => fetchNextPage()} disabled={isFetchingNextPage}>
            {isFetchingNextPage ? 'Loading…' : 'Load more'}
          </button>
        )}
      </div>
    </Async>
  );
}
