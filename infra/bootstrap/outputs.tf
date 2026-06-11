output "state_bucket_name" {
  value       = yandex_storage_bucket.tf_state.bucket
  description = "Name of the Object Storage bucket holding Terraform state."
}

output "ydb_endpoint" {
  value       = yandex_ydb_database_serverless.tf_lock.ydb_full_endpoint
  description = "YDB full endpoint used as the dynamodb_endpoint in the root backend config."
}

output "ydb_database_path" {
  value       = yandex_ydb_database_serverless.tf_lock.database_path
  description = "YDB database path for the state lock table."
}

output "tf_state_sa_id" {
  value       = yandex_iam_service_account.tf_state.id
  description = "Service account ID owning the state bucket."
}

output "tf_state_access_key" {
  value       = yandex_iam_service_account_static_access_key.tf_state.access_key
  description = "Static access key for the state bucket SA (set as YC_STORAGE_ACCESS_KEY)."
  sensitive   = true
}

output "tf_state_secret_key" {
  value       = yandex_iam_service_account_static_access_key.tf_state.secret_key
  description = "Static secret key for the state bucket SA (set as YC_STORAGE_SECRET_KEY)."
  sensitive   = true
}
