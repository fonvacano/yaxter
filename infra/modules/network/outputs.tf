output "vpc_id" {
  value       = yandex_vpc_network.this.id
  description = "VPC network ID."
}

output "private_subnet_ids" {
  value       = yandex_vpc_subnet.private[*].id
  description = "IDs of private subnets (one per instantiated AZ)."
}

output "public_subnet_ids" {
  value       = yandex_vpc_subnet.public[*].id
  description = "IDs of public subnets (one per instantiated AZ)."
}

output "k8s_node_sg_id" {
  value       = yandex_vpc_security_group.k8s_node.id
  description = "Security group ID for Kubernetes node VMs."
}

output "alb_sg_id" {
  value       = yandex_vpc_security_group.alb.id
  description = "Security group ID for the Application Load Balancer."
}

output "data_sg_id" {
  value       = yandex_vpc_security_group.data.id
  description = "Security group ID for data services (PG, Redis, Kafka)."
}
