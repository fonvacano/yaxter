import { useSession } from '../auth/session';
import { Link } from 'react-router-dom';
import { TimelineList } from '../features/timeline/TimelineList';
import { Composer } from '../features/compose/Composer';

export default function Home() {
  const session = useSession();
  if (session === 'loading') return <p role="status">Loading…</p>;
  if (session === 'anonymous')
    return (
      <p>
        <Link to="/login">Log in</Link> or <Link to="/register">register</Link> to see your timeline.
      </p>
    );
  return (
    <>
      <Composer />
      <TimelineList />
    </>
  );
}
