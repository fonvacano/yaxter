import type { ReactNode } from 'react';

export function Async({
  isLoading,
  error,
  empty,
  emptyMessage = 'Nothing here yet',
  children,
}: {
  isLoading: boolean;
  error: unknown;
  empty?: boolean;
  emptyMessage?: string;
  children: ReactNode;
}) {
  if (isLoading) return <p role="status">Loading…</p>;
  if (error) return <p role="alert">Something went wrong: {String((error as Error).message ?? error)}</p>;
  if (empty) return <p>{emptyMessage}</p>;
  return <>{children}</>;
}
