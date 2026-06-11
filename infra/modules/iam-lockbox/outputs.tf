output "api_sa_id" {
  value       = yandex_iam_service_account.api.id
  description = "Service account ID for the api workload."
}

output "worker_sa_id" {
  value       = yandex_iam_service_account.worker.id
  description = "Service account ID for the worker workload."
}

output "ci_sa_id" {
  value       = yandex_iam_service_account.ci.id
  description = "Service account ID for CI/CD pipelines."
}

output "api_secret_id" {
  value       = yandex_lockbox_secret.api.id
  description = "Lockbox secret ID for the api workload."
}

output "worker_secret_id" {
  value       = yandex_lockbox_secret.worker.id
  description = "Lockbox secret ID for the worker workload."
}

output "oauth_secret_id" {
  value       = yandex_lockbox_secret.oauth.id
  description = "Lockbox secret ID for OAuth provider credentials."
}

output "pg_password" {
  value       = random_password.pg.result
  description = "Generated PostgreSQL password (also stored in Lockbox)."
  sensitive   = true
}
