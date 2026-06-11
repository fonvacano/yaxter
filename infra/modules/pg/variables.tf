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

variable "subnet_ids" {
  type        = list(string)
  description = "Private subnet IDs (one per AZ) where PG hosts are placed."
}

variable "security_group_ids" {
  type        = list(string)
  description = "Security group IDs to attach to PG clusters."
}

variable "az_names" {
  type        = list(string)
  description = "AZ names for host placement (subset of all AZs based on az_count)."
}

variable "pg_version" {
  type        = string
  description = "PostgreSQL major version."
}

variable "resource_preset" {
  type        = string
  description = "Resource preset ID for PG hosts (e.g. b2.medium, s3.large)."
}

variable "disk_size" {
  type        = number
  description = "Disk size in GB per PG host."
}

variable "disk_type" {
  type        = string
  description = "Disk type for PG hosts."
}

variable "host_count" {
  type        = number
  description = "Number of PG hosts per physical shard cluster (1 = demo; 3 = prod)."
}

variable "physical_shards" {
  type        = number
  description = "Number of physical PG shard clusters."
}

variable "database_name" {
  type        = string
  description = "Database name inside each cluster."
}

variable "user_name" {
  type        = string
  description = "Application database user name."
}

variable "user_password" {
  type        = string
  description = "Application database user password."
  sensitive   = true
}
