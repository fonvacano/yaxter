output "cluster_id" {
  value       = yandex_mdb_redis_cluster.this.id
  description = "Redis cluster ID."
}

output "endpoint" {
  value       = yandex_mdb_redis_cluster.this.host[0].fqdn
  description = "FQDN of the first Redis host (use cluster FQDN for sharded mode)."
  sensitive   = true
}

output "hosts" {
  value       = yandex_mdb_redis_cluster.this.host[*].fqdn
  description = "FQDNs of all Redis hosts."
  sensitive   = true
}
