output "alb_id" {
  value       = yandex_alb_load_balancer.this.id
  description = "Application Load Balancer ID."
}

output "alb_address" {
  value       = yandex_alb_load_balancer.this.listener[0].endpoint[0].address[0].external_ipv4_address[0].address
  description = "External IPv4 address of the ALB HTTPS listener."
}

output "alb_dns_name" {
  value       = "${var.app_name}-alb.${var.folder_id}.yandexcloud.net"
  description = "Constructed DNS name for the ALB (update with actual DNS zone when configured)."
}

output "http_router_id" {
  value       = yandex_alb_http_router.this.id
  description = "HTTP router ID — used by the k8s Ingress controller annotation."
}

output "api_backend_group_id" {
  value       = yandex_alb_backend_group.api.id
  description = "Backend group ID for the api pods (referenced in Helm values for Ingress)."
}

output "web_backend_group_id" {
  value       = yandex_alb_backend_group.web.id
  description = "Backend group ID for the web SPA bucket."
}
