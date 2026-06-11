import { useParams } from 'react-router-dom';
import { useUser, useProfileTimeline, useFollow } from '../api/hooks';
import { Async } from '../ui/Async';
import { TweetCard } from '../features/timeline/TweetCard';
import { useState } from 'react';

export default function Profile() {
  const { username = '' } = useParams();
  const user = useUser(username);
  const tl = useProfileTimeline(username);
  const follow = useFollow();
  const [following, setFollowing] = useState<boolean>(false);

  function toggle() {
    const next = !following;
    follow.mutate(username, {
      onSuccess: () => setFollowing(next),
    });
  }

  return (
    <Async isLoading={user.isLoading} error={user.error}>
      {user.data && (
        <>
          <header>
            <h1>{user.data.username}</h1>
            <p>
              <span>{user.data.followers_count} followers</span> ·{' '}
              <span>{user.data.following_count} following</span>
            </p>
            <button type="button" onClick={toggle} disabled={follow.isPending} aria-pressed={following}>
              {following ? 'Unfollow' : 'Follow'}
            </button>
          </header>
          <Async
            isLoading={tl.isLoading}
            error={tl.error}
            empty={!tl.isLoading && tl.tweets.length === 0}
            emptyMessage="No tweets yet"
          >
            <div aria-label="profile timeline">
              {tl.tweets.map((t) => (
                <TweetCard key={String(t.id)} tweet={t} />
              ))}
              {tl.hasNextPage && (
                <button type="button" onClick={() => tl.fetchNextPage()} disabled={tl.isFetchingNextPage}>
                  Load more
                </button>
              )}
            </div>
          </Async>
        </>
      )}
    </Async>
  );
}
