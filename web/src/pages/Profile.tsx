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
          <header className="profile-head">
            <h1>{user.data.username}</h1>
            <p className="profile-stats">
              <b>{user.data.followers_count}</b> followers ·{' '}
              <b>{user.data.following_count}</b> following
            </p>
            <button type="button" className={following ? 'btn-ghost' : 'btn-primary'} onClick={toggle} disabled={follow.isPending} aria-pressed={following}>
              {following ? 'Unfollow' : 'Follow'}
            </button>
          </header>
          <Async
            isLoading={tl.isLoading}
            error={tl.error}
            empty={!tl.isLoading && tl.tweets.length === 0}
            emptyMessage="No tweets yet"
          >
            <div className="feed" aria-label="profile timeline">
              {tl.tweets.map((t) => (
                <TweetCard key={String(t.id)} tweet={t} />
              ))}
              {tl.hasNextPage && (
                <button type="button" className="btn-load" onClick={() => tl.fetchNextPage()} disabled={tl.isFetchingNextPage}>
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
