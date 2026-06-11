variable "folder_id" {
  type        = string
  description = "Yandex Cloud folder ID."
}

variable "app_name" {
  type        = string
  description = "Application name prefix for resource naming."
}

variable "media_bucket_name" {
  type        = string
  description = "Name of the Object Storage bucket for media originals and variants."
}

variable "web_bucket_name" {
  type        = string
  description = "Name of the Object Storage bucket serving the web SPA (static website)."
}

variable "cdn_enabled" {
  type        = bool
  description = "When true, creates a Yandex CDN resource in front of the media bucket."
}

variable "worker_sa_id" {
  type        = string
  description = "Service account ID for the worker workload (granted S3 read/write on media bucket)."
}

variable "api_sa_id" {
  type        = string
  description = "Service account ID for the api workload (granted S3 uploader on media bucket)."
}
