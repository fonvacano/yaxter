import { tokenStore } from '../auth/token';

// Refresh uses the HttpOnly cookie (same-origin /v1 proxy), so no body needed.
async function tryRefresh(): Promise<boolean> {
  const res = await fetch('/v1/auth/refresh', {
    method: 'POST',
    credentials: 'include',
  });
  if (!res.ok) return false;
  const body = (await res.json()) as { access_token?: string };
  if (!body.access_token) return false;
  tokenStore.set(body.access_token);
  return true;
}

export async function customFetch<T>(url: string, init?: RequestInit): Promise<T> {
  // Mutating endpoints require an Idempotency-Key (UUID). Generate one per
  // logical call and reuse it across the 401-refresh retry below, so the retry
  // is deduped rather than treated as a second write.
  const method = (init?.method ?? 'GET').toUpperCase();
  const baseHeaders = new Headers(init?.headers);
  if (method !== 'GET' && method !== 'HEAD' && !baseHeaders.has('Idempotency-Key')) {
    baseHeaders.set('Idempotency-Key', crypto.randomUUID());
  }

  const doFetch = () => {
    const headers = new Headers(baseHeaders);
    const token = tokenStore.get();
    if (token) headers.set('Authorization', `Bearer ${token}`);
    return fetch(url, { ...init, headers, credentials: 'include' });
  };

  let res = await doFetch();
  if (res.status === 401 && tokenStore.get() && (await tryRefresh())) {
    res = await doFetch();
  }
  if (res.status === 204) return undefined as T;
  const body = await res.json().catch(() => undefined);
  if (!res.ok) {
    throw Object.assign(new Error(body?.message ?? `HTTP ${res.status}`), {
      status: res.status,
      code: body?.error,
    });
  }
  return body as T;
}
