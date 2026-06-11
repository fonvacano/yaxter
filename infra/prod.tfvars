# prod.tfvars — sizing-only variables for the production environment.
# Apply with: terraform apply -var-file=prod.tfvars
# Architecture is IDENTICAL to demo; only instance sizes / counts differ.

# ─── Cloud / folder ───────────────────────────────────────────────────────────
# [needs YC] Set these to your actual Yandex Cloud IDs.
cloud_id     = "change-me"
folder_id    = "change-me"
default_zone = "ru-central1-a"

# ─── Network ─────────────────────────────────────────────────────────────────
az_count             = 3 # regional: all 3 AZs active
az_names             = ["ru-central1-a", "ru-central1-b", "ru-central1-d"]
private_subnet_cidrs = ["10.0.1.0/24", "10.0.2.0/24", "10.0.3.0/24"]
public_subnet_cidrs  = ["10.0.11.0/24", "10.0.12.0/24", "10.0.13.0/24"]
vpc_name             = "yaxter-vpc"

# ─── Kubernetes ───────────────────────────────────────────────────────────────
k8s_version          = "1.30"
k8s_master_type      = "regional" # HA master across 3 AZs
k8s_node_platform    = "standard-v3"
k8s_node_cores       = 8  # s2.large: 8 vCPU
k8s_node_memory      = 32 # s2.large: 32 GB
k8s_node_disk_size   = 128
k8s_node_disk_type   = "network-ssd"
k8s_node_preemptible = false # non-preemptible for production reliability
k8s_node_min         = 2     # per node group; 3 groups × 2 = 6 total min
k8s_node_max         = 10    # 3 groups × 10 = 30 total max
k8s_node_initial     = 2

# ─── PostgreSQL ───────────────────────────────────────────────────────────────
pg_version         = "16"
pg_resource_preset = "s3.large" # 8 vCPU / 32 GB
pg_disk_size       = 1024       # 1 TB per host
pg_disk_type       = "network-ssd"
pg_host_count      = 3 # 1 primary + 2 replicas across AZs
physical_shards    = 4 # 4 physical clusters; 64 logical shards each
pg_database_name   = "yaxter"
pg_user_name       = "yaxter"

# ─── Redis ────────────────────────────────────────────────────────────────────
redis_version         = "7.2"
redis_resource_preset = "s3.medium" # 4 vCPU / 16 GB
redis_sharded         = true        # 6 shards × 3 replicas = 18 hosts
redis_replica_count   = 3

# ─── Kafka ────────────────────────────────────────────────────────────────────
kafka_version                = "3.6"
kafka_brokers                = 3
kafka_broker_resource_preset = "s3.medium" # 4 vCPU / 16 GB
kafka_broker_disk_size       = 512         # GB per broker
kafka_broker_disk_type       = "network-ssd"
kafka_replication_factor     = 3
kafka_min_insync_replicas    = 2

# Topic partition counts (prod — sized for target throughput).
kafka_partitions_tweets      = 64
kafka_partitions_engagements = 64
kafka_partitions_follows     = 32
kafka_partitions_media       = 8

# ─── Object Storage / CDN ────────────────────────────────────────────────────
# [needs YC] bucket names must be globally unique.
media_bucket_name = "yaxter-prod-media"
web_bucket_name   = "yaxter-prod-web"
cdn_enabled       = true # CDN enabled for media reads in prod

# ─── ALB ─────────────────────────────────────────────────────────────────────
alb_az_count = 3 # listeners in all 3 AZs
# [needs YC] set to your actual domain and certificate ID.
domain_name        = "yaxter.example.com"
tls_certificate_id = "change-me"

# ─── Application ─────────────────────────────────────────────────────────────
app_name = "yaxter-prod"
