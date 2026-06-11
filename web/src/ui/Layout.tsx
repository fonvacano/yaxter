import { Link, Outlet } from 'react-router-dom';
import { useSession } from '../auth/session';
import { useUnreadCount } from '../api/hooks';
import { tokenStore } from '../auth/token';

function NotificationsBell() {
  const session = useSession();
  const { data } = useUnreadCount();
  if (session !== 'authenticated') return null;
  const count = data?.count ?? 0;
  return (
    <Link to="/notifications" aria-label="notifications">
      🔔{count > 0 && <span aria-label="unread count">{count}</span>}
    </Link>
  );
}

export default function Layout() {
  const session = useSession();
  return (
    <div>
      <header>
        <Link to="/">yaxter</Link>
        <nav>
          <NotificationsBell />
          {session === 'authenticated' ? (
            <button type="button" onClick={() => tokenStore.clear()}>Log out</button>
          ) : (
            <Link to="/login">Log in</Link>
          )}
        </nav>
      </header>
      <main>
        <Outlet />
      </main>
    </div>
  );
}
