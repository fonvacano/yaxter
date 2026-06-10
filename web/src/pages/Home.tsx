import { useSession } from '../auth/session';
import { Link } from 'react-router-dom';

export default function Home() {
  const session = useSession();
  if (session === 'loading') return <p role="status">Loading…</p>;
  if (session === 'anonymous')
    return (
      <p>
        <Link to="/login">Log in</Link> or <Link to="/register">register</Link> to see your timeline.
      </p>
    );
  return <p>Timeline arrives with T2.4.</p>;
}
