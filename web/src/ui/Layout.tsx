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
    <Link to="/notifications" className="bell" aria-label="notifications">
      🔔{count > 0 && <span className="badge" aria-label="unread count">{count}</span>}
    </Link>
  );
}

export default function Layout() {
  const session = useSession();
  return (
    <div className="app">
      <header className="topbar">
        <Link to="/" className="brand">yaxter</Link>
        <nav className="topnav">
          <NotificationsBell />
          {session === 'authenticated' ? (
            <button type="button" className="btn-ghost" onClick={() => tokenStore.clear()}>Log out</button>
          ) : (
            <Link to="/login" className="btn-ghost">Log in</Link>
          )}
        </nav>
      </header>
      <main>
        <Outlet />
      </main>
    </div>
  );
}
