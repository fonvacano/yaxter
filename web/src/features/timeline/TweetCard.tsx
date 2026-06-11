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
  return (
    <article aria-label="tweet">
      <header>
        <Link to={"/u/" + tweet.author.username}>{tweet.author.username}</Link>
      </header>
      <p>{tweet.text}</p>
      {imgSrc && <img src={imgSrc} alt="" loading="lazy" />}
      <footer>
        <button type="button" aria-label="like" aria-pressed={liked} onClick={toggleLike}>
          ♥ {likes}
        </button>
        <span aria-label="retweets">↺ {tweet.retweets_count}</span>
      </footer>
    </article>
  );
}
