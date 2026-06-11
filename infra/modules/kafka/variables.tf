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
  description = "Private subnet IDs (one per AZ) for Kafka broker placement."
}

variable "security_group_ids" {
  type        = list(string)
  description = "Security group IDs to attach to the Kafka cluster."
}

variable "az_names" {
  type        = list(string)
  description = "AZ names for broker placement (subset based on az_count)."
}

variable "kafka_version" {
  type        = string
  description = "Kafka version."
}

variable "broker_count" {
  type        = number
  description = "Total number of Kafka broker nodes (1 = demo; 3 = prod)."
}

variable "broker_resource_preset" {
  type        = string
  description = "Resource preset ID for Kafka brokers (e.g. s2.micro, s3.medium)."
}

variable "broker_disk_size" {
  type        = number
  description = "Disk size in GB per Kafka broker."
}

variable "broker_disk_type" {
  type        = string
  description = "Disk type for Kafka broker nodes."
}

variable "replication_factor" {
  type        = number
  description = "Default topic replication factor (1 = demo; 3 = prod)."
}

variable "min_insync_replicas" {
  type        = number
  description = "min.insync.replicas for all topics (1 = demo; 2 = prod)."
}

variable "partitions_tweets" {
  type        = number
  description = "Partition count for topic tweets.v1."
}

variable "partitions_engagements" {
  type        = number
  description = "Partition count for topic engagements.v1."
}

variable "partitions_follows" {
  type        = number
  description = "Partition count for topic follows.v1."
}

variable "partitions_media" {
  type        = number
  description = "Partition count for topic media.v1."
}
