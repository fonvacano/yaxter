variable "cloud_id" {
  type        = string
  description = "Yandex Cloud cloud ID."
}

variable "folder_id" {
  type        = string
  description = "Yandex Cloud folder ID where bootstrap resources are created."
}

variable "bucket_name" {
  type        = string
  description = "Name of the Object Storage bucket that will hold Terraform state."
}

variable "ydb_database_name" {
  type        = string
  description = "Name of the YDB serverless database used for Terraform state locking."
  default     = "yaxter-tf-lock"
}

variable "ydb_table_name" {
  type        = string
  description = "Document table name inside the YDB database used for Terraform state locking."
  default     = "tf-state-lock"
}
