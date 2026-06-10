import { useState, type FormEvent } from 'react';
import { useNavigate, Link } from 'react-router-dom';
import { register } from '../api/generated';
import { tokenStore } from '../auth/token';

export default function Register() {
  const [username, setUsername] = useState('');
  const [email, setEmail] = useState('');
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
      const res = await register(
        { username, email, password },
        { headers: { 'Idempotency-Key': crypto.randomUUID() } },
      ) as any;
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
    <section>
      <h1>Create account</h1>
      <form onSubmit={onSubmit} aria-label="register form">
        <label>
          Username
          <input value={username} onChange={(e) => setUsername(e.target.value)} required pattern="[A-Za-z0-9_]{3,30}" autoComplete="username" />
        </label>
        <label>
          Email
          <input type="email" value={email} onChange={(e) => setEmail(e.target.value)} required autoComplete="email" />
        </label>
        <label>
          Password
          <input type="password" value={password} onChange={(e) => setPassword(e.target.value)} required minLength={8} autoComplete="new-password" />
        </label>
        {error && <p role="alert">{error}</p>}
        <button type="submit" disabled={pending}>
          {pending ? 'Creating…' : 'Register'}
        </button>
      </form>
      <p>
        Already have an account? <Link to="/login">Log in</Link>
      </p>
    </section>
  );
}
