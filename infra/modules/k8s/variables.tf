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
  description = "Public subnet IDs for zonal master placement."
}

variable "private_subnet_ids" {
  type        = list(string)
  description = "Private subnet IDs for node group placement."
}

variable "node_sg_id" {
  type        = string
  description = "Security group ID to attach to node VMs."
}

variable "az_names" {
  type        = list(string)
  description = "AZ names for node group distribution (subset based on az_count)."
}

variable "k8s_version" {
  type        = string
  description = "Kubernetes version."
}

variable "master_type" {
  type        = string
  description = "'zonal' or 'regional'."
}

variable "node_platform" {
  type        = string
  description = "Platform (CPU family) for node VMs."
}

variable "node_cores" {
  type        = number
  description = "vCPU count per node VM."
}

variable "node_memory" {
  type        = number
  description = "RAM in GB per node VM."
}

variable "node_disk_size" {
  type        = number
  description = "Boot disk size in GB per node."
}

variable "node_disk_type" {
  type        = string
  description = "Boot disk type for nodes."
}

variable "node_preemptible" {
  type        = bool
  description = "Whether node VMs are preemptible (spot)."
}

variable "node_min" {
  type        = number
  description = "Minimum nodes per node group (autoscaler lower bound)."
}

variable "node_max" {
  type        = number
  description = "Maximum nodes per node group (autoscaler upper bound)."
}

variable "node_initial" {
  type        = number
  description = "Initial number of nodes per node group."
}
