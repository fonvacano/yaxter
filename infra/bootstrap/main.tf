# Bootstrap: creates the Object Storage bucket for Terraform remote state
# and the YDB serverless database / document table for state locking.
# Run this ONCE with a local backend before configuring the remote backend
# in the root module (infra/).

# Service account that owns the state bucket.
resource "yandex_iam_service_account" "tf_state" {
  name        = "yaxter-tf-state"
  description = "Service account for Terraform remote state access."
  folder_id   = var.folder_id
}

resource "yandex_resourcemanager_folder_iam_member" "tf_state_storage_admin" {
  folder_id = var.folder_id
  role      = "storage.admin"
  member    = "serviceAccount:${yandex_iam_service_account.tf_state.id}"
}

resource "yandex_iam_service_account_static_access_key" "tf_state" {
  service_account_id = yandex_iam_service_account.tf_state.id
  description        = "Static key for Terraform state bucket access."
}

# Object Storage bucket for Terraform state.
resource "yandex_storage_bucket" "tf_state" {
  bucket    = var.bucket_name
  acl       = "private"
  folder_id = var.folder_id

  access_key = yandex_iam_service_account_static_access_key.tf_state.access_key
  secret_key = yandex_iam_service_account_static_access_key.tf_state.secret_key

  versioning {
    enabled = true
  }

  # YC Object Storage encrypts data at rest by default; explicit KMS key can be
  # added via kms_master_key_id if a customer-managed key is required.
}

# YDB serverless database for state locking.
resource "yandex_ydb_database_serverless" "tf_lock" {
  name      = var.ydb_database_name
  folder_id = var.folder_id

  serverless_database {
    enable_throttling_rcu_limit = false
    provisioned_rcu_limit       = 0
    storage_size_limit          = 5
    throttling_rcu_limit        = 0
  }
}
