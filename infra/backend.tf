# Remote backend: Yandex Cloud Object Storage + YDB document table for locking.
# Before using this backend, run the bootstrap module once (infra/bootstrap/)
# to create the bucket and YDB database.
#
# Configure credentials via environment variables:
#   export YC_STORAGE_ACCESS_KEY=<static key from bootstrap output>
#   export YC_STORAGE_SECRET_KEY=<static secret from bootstrap output>
#
# Or pass them in a .backend.hcl file (do not commit it):
#   access_key = "..."
#   secret_key = "..."

terraform {
  backend "s3" {
    # Yandex Cloud Object Storage is S3-compatible.
    endpoint = "https://storage.yandexcloud.net"

    # Values below are intentionally left as placeholders so the file can be
    # committed. Override with -backend-config flags or a .backend.hcl file.
    bucket = "yaxter-tf-state"
    key    = "yaxter/terraform.tfstate"
    region = "ru-central1"

    # YDB document table used as DynamoDB-compatible lock table.
    # Point dynamodb_endpoint at the YDB serverless endpoint from bootstrap output.
    dynamodb_table    = "tf-state-lock"
    dynamodb_endpoint = "https://docapi.serverless.yandexcloud.net/ru-central1/<folder-id>/<db-id>"

    skip_region_validation      = true
    skip_credentials_validation = true
    skip_metadata_api_check     = true
    force_path_style            = true
  }
}
