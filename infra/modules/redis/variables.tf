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
  description = "Private subnet IDs (one per AZ) for Redis host placement."
}

variable "security_group_ids" {
  type        = list(string)
  description = "Security group IDs to attach to the Redis cluster."
}

variable "az_names" {
  type        = list(string)
  description = "AZ names for host placement (subset based on az_count)."
}

variable "redis_version" {
  type        = string
  description = "Redis major version."
}

variable "resource_preset" {
  type        = string
  description = "Resource preset ID for Redis hosts (e.g. b2.medium, s3.medium)."
}

variable "sharded" {
  type        = bool
  description = "Enable cluster sharding (false = single-shard demo; true = 6-shard prod)."
}

variable "replica_count" {
  type        = number
  description = "Number of replica hosts per shard (1 = demo; 3 = prod)."
}
