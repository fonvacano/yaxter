# Yaxter Infrastructure — Terraform

One module set, two sizes. `demo.tfvars` and `prod.tfvars` differ only in instance
sizes, replica counts, shard counts, partition counts, and feature flags. The
component graph, module set, IAM model, and network topology are identical.

## Directory layout

```
infra/
  bootstrap/           # One-time setup: state bucket + YDB lock table
  modules/
    network/           # VPC, subnets (az_count), NAT gateway, SGs
    iam-lockbox/       # Per-workload SAs, IAM roles, Lockbox secrets
    pg/                # Managed PostgreSQL (physical_shards clusters)
    redis/             # Managed Redis (sharded var)
    kafka/             # Managed Kafka + fixed topic set
    storage-cdn/       # media bucket, web bucket (SPA), optional CDN
    k8s/               # Managed K8s master + node groups
    alb/               # ALB with path routing /v1/* → api, else → web bucket
  main.tf              # Wires all modules
  variables.tf         # All cross-cutting + per-module sizing variables
  outputs.tf           # Cluster endpoint, ALB DNS, bucket names, Lockbox IDs
  providers.tf         # yandex-cloud/yandex ~> 0.122, TF >= 1.9
  backend.tf           # YC Object Storage + YDB state locking
  versions.tf          # required_version constraint
  demo.tfvars          # Demo sizing (1 AZ, preemptible nodes, 1 PG shard, ...)
  prod.tfvars          # Prod sizing (3 AZ, regional master, 4 PG shards, ...)
```

## Prerequisites

- Terraform >= 1.9
- `yc` CLI authenticated with a service account or user account that has
  `editor` on the target folder
- For remote state (after bootstrap): `YC_STORAGE_ACCESS_KEY` and
  `YC_STORAGE_SECRET_KEY` environment variables

## First-time bootstrap [needs YC]

The state bucket and YDB lock table must be created before using the remote
backend. Run this once with the local backend:

```bash
cd infra/bootstrap
terraform init
terraform apply \
  -var="cloud_id=<cloud-id>" \
  -var="folder_id=<folder-id>" \
  -var="bucket_name=yaxter-tf-state"
```

Record the outputs:
- `state_bucket_name` → use as `bucket` in `backend.tf`
- `ydb_endpoint` → use as `dynamodb_endpoint` in `backend.tf`
- `tf_state_access_key` / `tf_state_secret_key` → set as env vars

Update `infra/backend.tf` with the real values, then:

```bash
cd infra
terraform init \
  -backend-config="access_key=$YC_STORAGE_ACCESS_KEY" \
  -backend-config="secret_key=$YC_STORAGE_SECRET_KEY"
```

## Applying demo or prod [needs YC]

```bash
# Demo
terraform plan  -var-file=demo.tfvars
terraform apply -var-file=demo.tfvars

# Prod
terraform plan  -var-file=prod.tfvars
terraform apply -var-file=prod.tfvars
```

Fill in real values for `cloud_id`, `folder_id`, bucket names, domain name,
and `tls_certificate_id` in the tfvars files before applying.

## Validation (no YC credentials required)

```bash
# Root module
cd infra
terraform fmt -check -recursive
terraform init -backend=false
terraform validate

# Bootstrap module
cd infra/bootstrap
terraform fmt -check
terraform init -backend=false
terraform validate
```

`terraform validate` requires the provider schema (downloaded by `init`).
It does NOT require cloud credentials when run with `-backend=false`.

## Variable reference

| Variable | Demo | Prod | Description |
|---|---|---|---|
| `az_count` | 1 | 3 | AZs to instantiate (subnets, node groups, ALB) |
| `k8s_master_type` | zonal | regional | Zonal = single-AZ; regional = HA across 3 AZs |
| `k8s_node_preemptible` | true | false | Spot nodes for cost; false for reliability |
| `k8s_node_min/max` | 2/4 | 2/10 per group | Per node group; prod has 3 groups |
| `physical_shards` | 1 | 4 | PG cluster count; 256 logical shards distributed |
| `pg_host_count` | 1 | 3 | Hosts per PG cluster (1 = no replicas; 3 = HA) |
| `redis_sharded` | false | true | Single vs 6-shard Redis cluster |
| `redis_replica_count` | 1 | 3 | Replicas per Redis shard |
| `kafka_brokers` | 1 | 3 | Kafka broker count |
| `kafka_replication_factor` | 1 | 3 | Topic RF; must be ≤ broker count |
| `kafka_min_insync_replicas` | 1 | 2 | Durability guarantee |
| `kafka_partitions_tweets` | 3 | 64 | tweets.v1 partitions |
| `kafka_partitions_engagements` | 3 | 64 | engagements.v1 partitions |
| `kafka_partitions_follows` | 3 | 32 | follows.v1 partitions |
| `kafka_partitions_media` | 1 | 8 | media.v1 partitions |
| `cdn_enabled` | false | true | Enable CDN in front of media bucket |
| `alb_az_count` | 1 | 3 | ALB listener AZ count |

## Promotion path

See ARCHITECTURE.md §4 for the ordered promotion steps. The `terraform plan`
diff between `demo.tfvars` and `prod.tfvars` will show only sizing/count
changes — no module additions, no resource type changes.

## Secrets

OAuth client secrets are initialised with `replace-me` placeholder values in
Lockbox. After registering OAuth applications, update the Lockbox secret
versions directly (do not put real secrets in tfvars or git):

```bash
yc lockbox secret add-version --id <secret-id> \
  --payload '[{"key":"YANDEX_OAUTH_CLIENT_SECRET","textValue":"real-secret"}]'
```

The External Secrets Operator in the k8s cluster reads Lockbox secrets and
injects them as Kubernetes Secrets consumed by pods. No secrets are stored in
git or Terraform state (passwords are generated by the `random` provider and
stored only in Lockbox).
