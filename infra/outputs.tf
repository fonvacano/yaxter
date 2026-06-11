# ─── Kubernetes ───────────────────────────────────────────────────────────────

output "k8s_cluster_id" {
  value       = module.k8s.cluster_id
  description = "Managed Kubernetes cluster ID."
}

output "k8s_cluster_endpoint" {
  value       = module.k8s.cluster_endpoint
  description = "Kubernetes API endpoint."
}

output "k8s_cluster_ca_certificate" {
  value       = module.k8s.cluster_ca_certificate
  description = "Base64-encoded CA certificate for the cluster."
  sensitive   = true
}

# ─── ALB ─────────────────────────────────────────────────────────────────────

output "alb_address" {
  value       = module.alb.alb_address
  description = "External IP address of the Application Load Balancer."
}

output "alb_dns_name" {
  value       = module.alb.alb_dns_name
  description = "DNS name of the Application Load Balancer."
}

# ─── Object Storage ───────────────────────────────────────────────────────────

output "media_bucket_name" {
  value       = module.storage_cdn.media_bucket_name
  description = "Name of the media Object Storage bucket."
}

output "web_bucket_name" {
  value       = module.storage_cdn.web_bucket_name
  description = "Name of the web SPA Object Storage bucket."
}

output "web_bucket_website_endpoint" {
  value       = module.storage_cdn.web_bucket_website_endpoint
  description = "Static website endpoint for the web bucket."
}

output "cdn_endpoint" {
  value       = module.storage_cdn.cdn_endpoint
  description = "CDN endpoint (empty string when cdn_enabled=false)."
}

# ─── Lockbox ─────────────────────────────────────────────────────────────────

output "lockbox_api_secret_id" {
  value       = module.iam_lockbox.api_secret_id
  description = "Lockbox secret ID for the api workload."
}

output "lockbox_worker_secret_id" {
  value       = module.iam_lockbox.worker_secret_id
  description = "Lockbox secret ID for the worker workload."
}

output "lockbox_oauth_secret_id" {
  value       = module.iam_lockbox.oauth_secret_id
  description = "Lockbox secret ID for OAuth provider credentials."
}

# ─── PostgreSQL ───────────────────────────────────────────────────────────────

output "pg_cluster_hosts" {
  value       = module.pg.cluster_hosts
  description = "Map of physical_shard_index → list of PG host FQDNs."
  sensitive   = true
}

# ─── Redis ────────────────────────────────────────────────────────────────────

output "redis_endpoint" {
  value       = module.redis.endpoint
  description = "Redis cluster endpoint address."
  sensitive   = true
}

# ─── Kafka ────────────────────────────────────────────────────────────────────

output "kafka_bootstrap_brokers" {
  value       = module.kafka.bootstrap_brokers
  description = "Kafka bootstrap broker addresses."
  sensitive   = true
}

# ─── IAM ─────────────────────────────────────────────────────────────────────

output "api_sa_id" {
  value       = module.iam_lockbox.api_sa_id
  description = "Service account ID for the api workload."
}

output "worker_sa_id" {
  value       = module.iam_lockbox.worker_sa_id
  description = "Service account ID for the worker workload."
}

output "ci_sa_id" {
  value       = module.iam_lockbox.ci_sa_id
  description = "Service account ID for CI/CD pipelines."
}
