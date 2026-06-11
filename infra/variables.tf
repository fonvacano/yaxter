# ─── Cloud / folder ──────────────────────────────────────────────────────────

variable "cloud_id" {
  type        = string
  description = "Yandex Cloud cloud ID."
}

variable "folder_id" {
  type        = string
  description = "Yandex Cloud folder ID where all resources are created."
}

variable "default_zone" {
  type        = string
  description = "Default availability zone (e.g. ru-central1-a). Must be the first zone in az_names."
}

# ─── Network / topology ──────────────────────────────────────────────────────

variable "az_count" {
  type        = number
  description = "Number of AZs to instantiate (1 = demo; 3 = prod). Must be ≤ length(az_names)."

  validation {
    condition     = var.az_count >= 1 && var.az_count <= 3
    error_message = "az_count must be 1, 2, or 3."
  }
}

variable "az_names" {
  type        = list(string)
  description = "Ordered list of availability zone names. All three are declared even for demo."
  default     = ["ru-central1-a", "ru-central1-b", "ru-central1-d"]
}

variable "vpc_name" {
  type        = string
  description = "Name of the VPC network."
  default     = "yaxter-vpc"
}

# Private subnets for data services and k8s node groups.
variable "private_subnet_cidrs" {
  type        = list(string)
  description = "CIDR blocks for private subnets, one per AZ (index matches az_names)."
  default     = ["10.0.1.0/24", "10.0.2.0/24", "10.0.3.0/24"]
}

# Public subnets for ALB and NAT gateway.
variable "public_subnet_cidrs" {
  type        = list(string)
  description = "CIDR blocks for public subnets (ALB / NAT), one per AZ."
  default     = ["10.0.11.0/24", "10.0.12.0/24", "10.0.13.0/24"]
}

# ─── Kubernetes ───────────────────────────────────────────────────────────────

variable "k8s_version" {
  type        = string
  description = "Kubernetes version for the managed cluster."
  default     = "1.30"
}

variable "k8s_master_type" {
  type        = string
  description = "Master type: 'zonal' or 'regional'."

  validation {
    condition     = contains(["zonal", "regional"], var.k8s_master_type)
    error_message = "k8s_master_type must be 'zonal' or 'regional'."
  }
}

variable "k8s_node_platform" {
  type        = string
  description = "Platform (CPU architecture) for node VMs."
  default     = "standard-v3"
}

variable "k8s_node_cores" {
  type        = number
  description = "vCPU count per node VM."
}

variable "k8s_node_memory" {
  type        = number
  description = "RAM in GB per node VM."
}

variable "k8s_node_disk_size" {
  type        = number
  description = "Boot disk size in GB per node."
  default     = 64
}

variable "k8s_node_disk_type" {
  type        = string
  description = "Boot disk type for nodes."
  default     = "network-ssd"
}

variable "k8s_node_preemptible" {
  type        = bool
  description = "Whether node VMs are preemptible (spot). True for demo cost savings."
}

variable "k8s_node_min" {
  type        = number
  description = "Minimum number of nodes per node group (autoscaler lower bound)."
}

variable "k8s_node_max" {
  type        = number
  description = "Maximum number of nodes per node group (autoscaler upper bound)."
}

variable "k8s_node_initial" {
  type        = number
  description = "Initial number of nodes per node group."
}

# ─── PostgreSQL ───────────────────────────────────────────────────────────────

variable "pg_version" {
  type        = string
  description = "PostgreSQL major version."
  default     = "16"
}

variable "pg_resource_preset" {
  type        = string
  description = "Resource preset ID for PG hosts (e.g. b2.medium, s3.large)."
}

variable "pg_disk_size" {
  type        = number
  description = "Disk size in GB for each PG host."
}

variable "pg_disk_type" {
  type        = string
  description = "Disk type for PG hosts."
  default     = "network-ssd"
}

variable "pg_host_count" {
  type        = number
  description = "Number of PG hosts per physical shard cluster (1 = demo; 3 = prod HA)."
}

variable "physical_shards" {
  type        = number
  description = "Number of physical PG shard clusters (1 = demo; 4 = prod). Each cluster holds 256/N logical shards."

  validation {
    condition     = var.physical_shards >= 1
    error_message = "physical_shards must be at least 1."
  }
}

variable "pg_database_name" {
  type        = string
  description = "Database name inside each PG cluster."
  default     = "yaxter"
}

variable "pg_user_name" {
  type        = string
  description = "Application database user name."
  default     = "yaxter"
}

# ─── Redis ────────────────────────────────────────────────────────────────────

variable "redis_version" {
  type        = string
  description = "Redis major version."
  default     = "7.2"
}

variable "redis_resource_preset" {
  type        = string
  description = "Resource preset ID for Redis hosts (e.g. b2.medium, s3.medium)."
}

variable "redis_sharded" {
  type        = bool
  description = "Enable Redis cluster sharding (false = demo; true = prod)."
}

variable "redis_replica_count" {
  type        = number
  description = "Number of replica hosts per Redis shard."
}

# ─── Kafka ────────────────────────────────────────────────────────────────────

variable "kafka_version" {
  type        = string
  description = "Kafka version."
  default     = "3.6"
}

variable "kafka_brokers" {
  type        = number
  description = "Number of Kafka broker nodes (1 = demo; 3+ = prod)."
}

variable "kafka_broker_resource_preset" {
  type        = string
  description = "Resource preset ID for Kafka brokers."
}

variable "kafka_broker_disk_size" {
  type        = number
  description = "Disk size in GB per Kafka broker."
}

variable "kafka_broker_disk_type" {
  type        = string
  description = "Disk type for Kafka broker nodes."
  default     = "network-ssd"
}

variable "kafka_replication_factor" {
  type        = number
  description = "Default replication factor for Kafka topics (1 = demo; 3 = prod)."
}

variable "kafka_min_insync_replicas" {
  type        = number
  description = "min.insync.replicas for Kafka topics (1 = demo; 2 = prod)."
}

# Per-topic partition counts.
variable "kafka_partitions_tweets" {
  type        = number
  description = "Partition count for topic tweets.v1."
}

variable "kafka_partitions_engagements" {
  type        = number
  description = "Partition count for topic engagements.v1."
}

variable "kafka_partitions_follows" {
  type        = number
  description = "Partition count for topic follows.v1."
}

variable "kafka_partitions_media" {
  type        = number
  description = "Partition count for topic media.v1."
}

# ─── Object Storage / CDN ────────────────────────────────────────────────────

variable "media_bucket_name" {
  type        = string
  description = "Name of the Object Storage bucket for media originals and variants."
}

variable "web_bucket_name" {
  type        = string
  description = "Name of the Object Storage bucket serving the web SPA (static website)."
}

variable "cdn_enabled" {
  type        = bool
  description = "Enable Yandex CDN in front of the media bucket (false = demo; true = prod)."
}

# ─── ALB ─────────────────────────────────────────────────────────────────────

variable "alb_az_count" {
  type        = number
  description = "Number of AZs for ALB listeners (1 = demo; 3 = prod)."

  validation {
    condition     = var.alb_az_count >= 1 && var.alb_az_count <= 3
    error_message = "alb_az_count must be 1, 2, or 3."
  }
}

variable "domain_name" {
  type        = string
  description = "Primary domain name used for TLS certificates and ALB routing (e.g. yaxter.example.com)."
}

variable "tls_certificate_id" {
  type        = string
  description = "Yandex Certificate Manager certificate ID for ALB HTTPS listener."
}

# ─── Application config ───────────────────────────────────────────────────────

variable "app_name" {
  type        = string
  description = "Application name prefix used for resource naming."
  default     = "yaxter"
}
