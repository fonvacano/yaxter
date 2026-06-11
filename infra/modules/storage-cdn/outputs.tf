output "media_bucket_name" {
  value       = yandex_storage_bucket.media.bucket
  description = "Name of the media Object Storage bucket."
}

output "web_bucket_name" {
  value       = yandex_storage_bucket.web.bucket
  description = "Name of the web SPA Object Storage bucket."
}

output "web_bucket_website_endpoint" {
  value       = yandex_storage_bucket.web.website_endpoint
  description = "HTTP website endpoint for the web SPA bucket."
}

output "cdn_endpoint" {
  value       = var.cdn_enabled ? yandex_cdn_resource.media[0].cname : ""
  description = "CDN endpoint CNAME (empty string when cdn_enabled=false)."
}

output "cdn_origin_group_id" {
  value       = var.cdn_enabled ? yandex_cdn_origin_group.media[0].id : ""
  description = "CDN origin group ID (empty string when cdn_enabled=false)."
}
