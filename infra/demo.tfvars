# demo.tfvars — sizing-only variables for the demo environment.
# Apply with: terraform apply -var-file=demo.tfvars
# Architecture is IDENTICAL to prod; only instance sizes / counts differ.

# ─── Cloud / folder ───────────────────────────────────────────────────────────
# [needs YC] Set these to your actual Yandex Cloud IDs.
cloud_id     = "change-me"
folder_id    = "change-me"
default_zone = "ru-central1-a"

# ─── Network ─────────────────────────────────────────────────────────────────
# 3 AZ CIDRs are always declared; az_count=1 instantiates only the first.
az_count             = 1
az_names             = ["ru-central1-a", "ru-central1-b", "ru-central1-d"]
private_subnet_cidrs = ["10.0.1.0/24", "10.0.2.0/24", "10.0.3.0/24"]
public_subnet_cidrs  = ["10.0.11.0/24", "10.0.12.0/24", "10.0.13.0/24"]
vpc_name             = "yaxter-vpc"

# ─── Kubernetes ───────────────────────────────────────────────────────────────
k8s_version          = "1.30"
k8s_master_type      = "zonal" # single-AZ master (cost: ~₽6k/mo)
k8s_node_platform    = "standard-v3"
k8s_node_cores       = 2 # s2.small: 2 vCPU
k8s_node_memory      = 8 # s2.small: 8 GB
k8s_node_disk_size   = 64
k8s_node_disk_type   = "network-ssd"
k8s_node_preemptible = true # preemptible nodes reduce cost ~70%
k8s_node_min         = 2
k8s_node_max         = 4
k8s_node_initial     = 2

# ─── PostgreSQL ───────────────────────────────────────────────────────────────
pg_version         = "16"
pg_resource_preset = "b2.medium" # burstable, 2 vCPU / 4 GB
pg_disk_size       = 30          # GB SSD
pg_disk_type       = "network-ssd"
pg_host_count      = 1 # single host (no HA replicas)
physical_shards    = 1 # all 256 logical shards on one cluster
pg_database_name   = "yaxter"
pg_user_name       = "yaxter"

# ─── Redis ────────────────────────────────────────────────────────────────────
redis_version         = "7.2"
redis_resource_preset = "b2.medium" # burstable, 2 vCPU / 4 GB
redis_sharded         = false       # single-shard, no persistence
redis_replica_count   = 1

# ─── Kafka ────────────────────────────────────────────────────────────────────
kafka_version                = "3.6"
kafka_brokers                = 1
kafka_broker_resource_preset = "s2.micro" # 2 vCPU / 8 GB
kafka_broker_disk_size       = 32         # GB
kafka_broker_disk_type       = "network-ssd"
kafka_replication_factor     = 1
kafka_min_insync_replicas    = 1

# Topic partition counts (demo — exercisable but not at scale).
kafka_partitions_tweets      = 3
kafka_partitions_engagements = 3
kafka_partitions_follows     = 3
kafka_partitions_media       = 1

# ─── Object Storage / CDN ────────────────────────────────────────────────────
# [needs YC] bucket names must be globally unique.
media_bucket_name = "yaxter-demo-media"
web_bucket_name   = "yaxter-demo-web"
cdn_enabled       = false # CDN disabled; ALB serves media directly

# ─── ALB ─────────────────────────────────────────────────────────────────────
alb_az_count = 1
# [needs YC] set to your actual domain and certificate ID.
domain_name        = "demo.yaxter.example.com"
tls_certificate_id = "change-me"

# ─── Application ─────────────────────────────────────────────────────────────
app_name = "yaxter-demo"
