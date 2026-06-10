import { useQuery } from '@tanstack/react-query';
import { listAuthProviders } from '../api/generated';

// Buttons are driven entirely by GET /v1/auth/providers — the demo shows
// Yandex only; Google appears when the backend enables it (config, not code).
export function ProviderButtons() {
  const { data, isLoading, error } = useQuery({
    queryKey: ['auth', 'providers'],
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    queryFn: () => listAuthProviders() as any,
  });

  if (isLoading || error) return null; // social login is optional sugar
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const providers: any[] = data?.data?.providers ?? data?.providers ?? [];
  if (providers.length === 0) return null;

  return (
    <div data-testid="oauth-providers">
      <p>Or continue with:</p>
      {providers.map((p: { name: string; display_name: string; start_url: string }) => (
        <a key={p.name} href={p.start_url} role="button" data-provider={p.name}>
          {p.display_name}
        </a>
      ))}
    </div>
  );
}
