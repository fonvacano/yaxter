output "cluster_ids" {
  value       = yandex_mdb_postgresql_cluster.shard[*].id
  description = "List of PG cluster IDs, one per physical shard."
}

output "cluster_hosts" {
  value = {
    for idx, cluster in yandex_mdb_postgresql_cluster.shard :
    tostring(idx) => cluster.host[*].fqdn
  }
  description = "Map of shard index (string) → list of host FQDNs."
  sensitive   = true
}

output "database_name" {
  value       = var.database_name
  description = "Database name used across all shard clusters."
}

output "user_name" {
  value       = var.user_name
  description = "Database user name used across all shard clusters."
}
