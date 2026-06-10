let accessToken: string | null = null;
const listeners = new Set<() => void>();

export const tokenStore = {
  get: () => accessToken,
  set(token: string | null) {
    accessToken = token;
    listeners.forEach((l) => l());
  },
  clear() {
    tokenStore.set(null);
  },
  subscribe(l: () => void) {
    listeners.add(l);
    return () => listeners.delete(l);
  },
};
