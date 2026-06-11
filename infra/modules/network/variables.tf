variable "folder_id" {
  type        = string
  description = "Yandex Cloud folder ID."
}

variable "app_name" {
  type        = string
  description = "Application name prefix for resource naming."
}

variable "vpc_name" {
  type        = string
  description = "Name of the VPC network."
}

variable "az_count" {
  type        = number
  description = "Number of AZs to instantiate subnets in (1–3)."
}

variable "az_names" {
  type        = list(string)
  description = "Full list of AZ names (always 3 declared; az_count controls how many are created)."
}

variable "private_subnet_cidrs" {
  type        = list(string)
  description = "CIDR blocks for private subnets, one per AZ."
}

variable "public_subnet_cidrs" {
  type        = list(string)
  description = "CIDR blocks for public subnets (ALB / NAT), one per AZ."
}
