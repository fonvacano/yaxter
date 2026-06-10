import { createContext, useContext, useEffect, useState, type ReactNode } from 'react';
import { tokenStore } from './token';

type SessionState = 'loading' | 'anonymous' | 'authenticated';
const SessionContext = createContext<SessionState>('loading');

export function SessionProvider({ children }: { children: ReactNode }) {
  const [state, setState] = useState<SessionState>('loading');

  useEffect(() => {
    const sync = () => setState(tokenStore.get() ? 'authenticated' : 'anonymous');
    const unsub = tokenStore.subscribe(sync);
    // One refresh attempt on boot: the HttpOnly cookie may hold a session.
    fetch('/v1/auth/refresh', { method: 'POST', credentials: 'include' })
      .then(async (res) => {
        if (res.ok) {
          const body = (await res.json()) as { access_token?: string };
          if (body.access_token) tokenStore.set(body.access_token);
          else sync();
        } else sync();
      })
      .catch(sync);
    return () => {
      unsub();
    };
  }, []);

  return <SessionContext.Provider value={state}>{children}</SessionContext.Provider>;
}

export const useSession = () => useContext(SessionContext);
