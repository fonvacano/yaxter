output "cluster_id" {
  value       = yandex_kubernetes_cluster.this.id
  description = "Managed Kubernetes cluster ID."
}

output "cluster_endpoint" {
  value       = yandex_kubernetes_cluster.this.master[0].external_v4_endpoint
  description = "Kubernetes API server external endpoint."
}

output "cluster_ca_certificate" {
  value       = yandex_kubernetes_cluster.this.master[0].cluster_ca_certificate
  description = "Base64-encoded cluster CA certificate."
  sensitive   = true
}

output "node_group_ids" {
  value       = yandex_kubernetes_node_group.this[*].id
  description = "IDs of all node groups."
}

output "cluster_sa_id" {
  value       = yandex_iam_service_account.k8s_cluster.id
  description = "Service account ID for the cluster control plane."
}

output "nodes_sa_id" {
  value       = yandex_iam_service_account.k8s_nodes.id
  description = "Service account ID for node VMs."
}
