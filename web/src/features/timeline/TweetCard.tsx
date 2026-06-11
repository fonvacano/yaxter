import { Link } from 'react-router-dom';
import { useState } from 'react';
import type { Tweet } from '../../api/generated';
import { useOptimisticLike } from '../../api/hooks';

export function TweetCard({ tweet }: { tweet: Tweet }) {
  const like = useOptimisticLike();
  const [liked, setLiked] = useState(tweet.liked_by_me ?? false);
  const [likes, setLikes] = useState(tweet.likes_count);

  function toggleLike() {
    const next = !liked;
    setLiked(next);
    setLikes((n) => n + (next ? 1 : -1));
    like.mutate(
      { id: String(tweet.id), like: next },
      {
        onError: () => {
          setLiked(!next);
          setLikes((n) => n + (next ? -1 : 1));
        },
      },
    );
  }

  const first = tweet.media?.[0];
  const imgSrc = first?.urls?.feed ?? first?.urls?.orig ?? null;
  const avatarUrl = tweet.author.avatar_url;
  const initial = tweet.author.username.charAt(0);
  return (
    <article className="tweet" aria-label="tweet">
      <Link to={"/u/" + tweet.author.username} className="avatar" aria-hidden="true">
        {avatarUrl ? <img src={avatarUrl} alt="" /> : initial}
      </Link>
      <div className="tweet-main">
        <div className="tweet-head">
          <Link to={"/u/" + tweet.author.username} className="tweet-author">{tweet.author.username}</Link>
          <span className="tweet-handle">@{tweet.author.username}</span>
        </div>
        <p className="tweet-text">{tweet.text}</p>
        {imgSrc && <img className="tweet-media" src={imgSrc} alt="" loading="lazy" />}
        <div className="tweet-actions">
          <button type="button" className="action like" aria-label="like" aria-pressed={liked} onClick={toggleLike}>
            ♥ {likes}
          </button>
          <span className="action" aria-label="retweets">↺ {tweet.retweets_count}</span>
        </div>
      </div>
    </article>
  );
}
