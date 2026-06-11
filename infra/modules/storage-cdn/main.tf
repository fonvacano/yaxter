# Object Storage buckets + optional CDN.
#
# media bucket — private; application access via pre-signed URLs and IAM SA keys.
# web bucket   — static website hosting; error document = index.html for SPA client routing.
# CDN          — created only when cdn_enabled=true (prod); same hostname, DNS change only.

# ─── Media Bucket ────────────────────────────────────────────────────────────

resource "yandex_storage_bucket" "media" {
  bucket    = var.media_bucket_name
  folder_id = var.folder_id
  acl       = "private"

  versioning {
    enabled = false
  }

  cors_rule {
    allowed_headers = ["*"]
    allowed_methods = ["GET", "PUT"]
    allowed_origins = ["*"]
    expose_headers  = ["ETag"]
    max_age_seconds = 3600
  }

  lifecycle_rule {
    id      = "expire-originals"
    enabled = false # Enable in prod with status = "Enabled" to move originals to cold storage.

    transition {
      days          = 90
      storage_class = "COLD"
    }
  }
}

# ─── Web Bucket (SPA static website) ─────────────────────────────────────────

resource "yandex_storage_bucket" "web" {
  bucket    = var.web_bucket_name
  folder_id = var.folder_id
  acl       = "public-read"

  website {
    index_document = "index.html"
    # Redirect all 404s to index.html for client-side SPA routing.
    error_document = "index.html"
  }

  cors_rule {
    allowed_headers = ["*"]
    allowed_methods = ["GET", "HEAD"]
    allowed_origins = ["*"]
    max_age_seconds = 86400
  }
}

# ─── CDN (conditional on cdn_enabled) ────────────────────────────────────────

resource "yandex_cdn_origin_group" "media" {
  count = var.cdn_enabled ? 1 : 0

  name      = "${var.app_name}-media-cdn-origins"
  folder_id = var.folder_id

  origin {
    source = "${var.media_bucket_name}.storage.yandexcloud.net"
    backup = false
  }
}

resource "yandex_cdn_resource" "media" {
  count = var.cdn_enabled ? 1 : 0

  cname             = "media-cdn.${var.app_name}.internal"
  folder_id         = var.folder_id
  active            = true
  origin_group_id   = yandex_cdn_origin_group.media[0].id
  origin_protocol   = "https"

  options {
    allowed_http_methods   = ["GET", "HEAD", "OPTIONS"]
    browser_cache_settings = 86400
    cache_http_headers     = ["ETag", "Last-Modified", "Content-Type"]
    cors                   = ["*"]
    gzip_on                = true
    ignore_query_params    = false
    static_request_headers = {}
    static_response_headers = {}
  }

  ssl_certificate {
    type = "not_managed"
  }
}
