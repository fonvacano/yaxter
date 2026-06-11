import { useState, type FormEvent } from 'react';
import { useNavigate } from 'react-router-dom';
import { login } from '../api/generated';
import { tokenStore } from '../auth/token';
import { ProviderButtons } from '../auth/ProviderButtons';

export default function Login() {
  const [loginValue, setLoginValue] = useState('');
  const [password, setPassword] = useState('');
  const [error, setError] = useState<string | null>(null);
  const [pending, setPending] = useState(false);
  const navigate = useNavigate();

  async function onSubmit(e: FormEvent) {
    e.preventDefault();
    setPending(true);
    setError(null);
    try {
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      const res = await login({ login: loginValue, password }) as any;
      const accessToken = res?.data?.tokens?.access_token ?? res?.tokens?.access_token;
      tokenStore.set(accessToken);
      navigate('/');
    } catch (err) {
      setError((err as Error).message);
    } finally {
      setPending(false);
    }
  }

  return (
    <section className="auth-card">
      <h1>Log in</h1>
      <form onSubmit={onSubmit} aria-label="login form">
        <label>
          Username or email
          <input value={loginValue} onChange={(e) => setLoginValue(e.target.value)} required autoComplete="username" />
        </label>
        <label>
          Password
          <input type="password" value={password} onChange={(e) => setPassword(e.target.value)} required autoComplete="current-password" />
        </label>
        {error && <p className="error" role="alert">{error}</p>}
        <button type="submit" className="btn-primary" disabled={pending}>
          {pending ? 'Logging in…' : 'Log in'}
        </button>
      </form>
      <ProviderButtons />
    </section>
  );
}
