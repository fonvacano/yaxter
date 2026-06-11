# yaxter Helm chart

One chart, two sizes. Deploy the same chart with `values-demo.yaml` or
`values-prod.yaml` — the only differences are replica counts, resource limits,
and feature flags.

## Prerequisites

- Helm 3.x
- External Secrets Operator installed in the cluster (for Lockbox → k8s Secret sync)
- Argo Rollouts installed if `rollouts.enabled: true` (production)
- `ClusterSecretStore` named `lockbox-secret-store` pointing at Yandex Lockbox

## Quick deploy

```sh
# Demo
helm upgrade --install yaxter ./deploy/chart \
  -f deploy/chart/values-demo.yaml \
  --namespace yaxter-demo --create-namespace \
  --set externalSecrets.lockboxSecretIds.postgresDsn=<lockbox-id> \
  --set externalSecrets.lockboxSecretIds.jwtSeed=<lockbox-id> \
  --set image.tag=$(git rev-parse --short HEAD)

# Production
helm upgrade --install yaxter ./deploy/chart \
  -f deploy/chart/values-prod.yaml \
  --namespace yaxter-prod --create-namespace \
  --set externalSecrets.lockboxSecretIds.postgresDsn=<lockbox-id> \
  --set externalSecrets.lockboxSecretIds.jwtSeed=<lockbox-id> \
  --set externalSecrets.lockboxSecretIds.oauthSecrets=<lockbox-id> \
  --set image.tag=$(git rev-parse --short HEAD)
```

## Local render gate (run before every merge)

```sh
# Lint
helm lint deploy/chart

# Template both value sets and count Deployments
helm template yaxter deploy/chart -f deploy/chart/values-demo.yaml | grep 'kind: Deployment'
# Expected: 3 lines (api + all-roles worker + pgbouncer)

helm template yaxter deploy/chart -f deploy/chart/values-prod.yaml | grep 'kind: Deployment'
# Expected: 7 lines (api + relay/fanout/counters/notifications/media workers + pgbouncer)
```

## Secrets

Secrets are managed by External Secrets Operator syncing from Yandex Lockbox.
Fill in `externalSecrets.lockboxSecretIds.*` (Terraform outputs) to enable sync.
Each Lockbox secret must expose the following properties:

| Lockbox secret | Required properties |
|---|---|
| `postgresDsn` | `dsn` (full DSN pointing at PgBouncer), `password` |
| `jwtSeed` | `seed_b64` (base64-encoded EdDSA seed), `kid` |
| `oauthSecrets` | `client_secret` |

## Migrations

The `migrations` pre-install/pre-upgrade Job copies migration files from the app
image (`/migrations`) into an emptyDir volume, then runs `migrate/migrate:v4.18.1`
against the PgBouncer DSN.

The app image must have the `migrations/` directory at `/migrations` (the
Dockerfile COPY copies the full source tree including migrations).

## PgBouncer

PgBouncer runs as a Deployment inside the cluster in transaction-pooling mode.
The api and worker pods connect to the PgBouncer Service (`yaxter-pgbouncer:5432`),
never to the managed PostgreSQL directly.

## Canary (production)

When `rollouts.enabled: true` the chart creates an Argo `Rollout` instead of
a plain Deployment for the api. Adjust `rollouts.canaryWeight` and
`rollouts.stableWeight` in `values-prod.yaml` to control traffic split.

## Monitoring

`monitoring.enabled` is `false` in both demo and prod (T3.3 deferred).
When enabled, a `ServiceMonitor` (api) and `PodMonitor` (workers) are created.
