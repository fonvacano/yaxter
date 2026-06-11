variable "folder_id" {
  type        = string
  description = "Yandex Cloud folder ID."
}

variable "app_name" {
  type        = string
  description = "Application name prefix for resource naming."
}

variable "network_id" {
  type        = string
  description = "VPC network ID."
}

variable "public_subnet_ids" {
  type        = list(string)
  description = "Public subnet IDs (one per AZ) for ALB allocation."
}

variable "alb_sg_id" {
  type        = string
  description = "Security group ID for the ALB."
}

variable "az_names" {
  type        = list(string)
  description = "AZ names for ALB allocation (subset based on alb_az_count)."
}

variable "alb_az_count" {
  type        = number
  description = "Number of AZs to allocate the ALB across."
}

variable "domain_name" {
  type        = string
  description = "Primary domain for TLS certificate and routing."
}

variable "tls_certificate_id" {
  type        = string
  description = "Yandex Certificate Manager certificate ID for HTTPS listener."
}

variable "web_bucket_name" {
  type        = string
  description = "Name of the web SPA Object Storage bucket (used as backend origin)."
}

variable "web_bucket_website_endpoint" {
  type        = string
  description = "HTTP website endpoint of the web SPA bucket."
}
