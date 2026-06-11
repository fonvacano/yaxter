# E2E Tests

## Mock-backed e2e

Run against the prism mock (start with `npm run mock` in a separate terminal, then `npm run e2e`).

Alternatively, `npm run e2e` will start both `npm run mock` and `npm run dev` automatically
via Playwright's `webServer` config — no manual pre-start needed in most environments.

```sh
# In one terminal (optional — Playwright can start it automatically):
npm run mock

# In another terminal:
npm run e2e
```

## Specs

| File | What it tests |
|------|---------------|
| `smoke.spec.ts` | Register flow, login form, 404 routing |
| `timeline.spec.ts` | Anonymous home → login prompt; authed home → compose form; notifications page |

## How auth works in tests

The app uses an in-memory `tokenStore` (not `localStorage`). Session state resolves via
`POST /v1/auth/refresh` on boot — if the server returns `{ access_token }`, the session
becomes `authenticated`. Tests intercept this endpoint via `page.route()` to simulate either
state without a full login flow.

## Full-stack e2e (CI gate)

The mock specs (`smoke.spec.ts`, `timeline.spec.ts`) run on every PR against the prism mock.
Full data-flow assertions require the compose stack
(api + workers + PG + Redis + Kafka + MinIO + mock-OAuth). They run in CI's integration job.

### Scenarios (full-stack, future)

1. Register → login → compose a tweet with image → appears in follower timeline.
2. Like a tweet → optimistic counter settles to server value after flush.
3. Pagination stable while new tweets posted (cursor by Snowflake id).
4. OAuth login via mock provider (T1.6).
