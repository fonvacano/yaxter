terraform {
  required_providers {
    yandex = {
      source  = "yandex-cloud/yandex"
      version = "~> 0.122"
    }
    random = {
      source  = "hashicorp/random"
      version = "~> 3.6"
    }
  }
}

# ─── Service Accounts ─────────────────────────────────────────────────────────

# api SA — signs pre-upload S3 URLs + reads Lockbox secrets.
resource "yandex_iam_service_account" "api" {
  name        = "${var.app_name}-api"
  description = "Service account for the api workload."
  folder_id   = var.folder_id
}

# workers SA — reads and writes Object Storage (media variants).
resource "yandex_iam_service_account" "worker" {
  name        = "${var.app_name}-worker"
  description = "Service account for the worker workload."
  folder_id   = var.folder_id
}

# CI SA — pushes container images and runs Helm deploys.
resource "yandex_iam_service_account" "ci" {
  name        = "${var.app_name}-ci"
  description = "Service account for CI/CD pipelines."
  folder_id   = var.folder_id
}

# ─── IAM Roles (least-privilege) ─────────────────────────────────────────────

# api: storage.uploader scoped at folder level (pre-sign PUT URLs for media uploads).
resource "yandex_resourcemanager_folder_iam_member" "api_storage_uploader" {
  folder_id = var.folder_id
  role      = "storage.uploader"
  member    = "serviceAccount:${yandex_iam_service_account.api.id}"
}

# api: read Lockbox secret payloads (JWT keys, OAuth secrets, DB DSNs).
resource "yandex_resourcemanager_folder_iam_member" "api_lockbox_viewer" {
  folder_id = var.folder_id
  role      = "lockbox.payloadViewer"
  member    = "serviceAccount:${yandex_iam_service_account.api.id}"
}

# worker: full Object Storage read/write (download originals, write variants).
resource "yandex_resourcemanager_folder_iam_member" "worker_storage_editor" {
  folder_id = var.folder_id
  role      = "storage.editor"
  member    = "serviceAccount:${yandex_iam_service_account.worker.id}"
}

# worker: read Lockbox secrets (DB DSNs, Kafka credentials).
resource "yandex_resourcemanager_folder_iam_member" "worker_lockbox_viewer" {
  folder_id = var.folder_id
  role      = "lockbox.payloadViewer"
  member    = "serviceAccount:${yandex_iam_service_account.worker.id}"
}

# CI: push container images to Container Registry.
resource "yandex_resourcemanager_folder_iam_member" "ci_cr_pusher" {
  folder_id = var.folder_id
  role      = "container-registry.images.pusher"
  member    = "serviceAccount:${yandex_iam_service_account.ci.id}"
}

# CI: deploy Helm charts to the k8s cluster (editor scope on the folder).
resource "yandex_resourcemanager_folder_iam_member" "ci_k8s_editor" {
  folder_id = var.folder_id
  role      = "k8s.editor"
  member    = "serviceAccount:${yandex_iam_service_account.ci.id}"
}

# ─── Random passwords (stored only in Lockbox — not in Terraform state) ───────

resource "random_password" "pg" {
  length           = 32
  special          = true
  override_special = "!#$%&*()-_=+[]{}<>:?"
}

resource "random_password" "jwt_ed25519_seed" {
  length  = 64
  special = false
}

resource "random_password" "refresh_token_secret" {
  length  = 64
  special = false
}

# ─── Lockbox Secrets ─────────────────────────────────────────────────────────

# api workload secret — JWT keys, DB DSN template, OAuth client secrets.
resource "yandex_lockbox_secret" "api" {
  name      = "${var.app_name}-api"
  folder_id = var.folder_id
}

resource "yandex_lockbox_secret_version" "api" {
  secret_id = yandex_lockbox_secret.api.id

  entries {
    key        = "PG_PASSWORD"
    text_value = random_password.pg.result
  }

  entries {
    key        = "JWT_ED25519_SEED"
    text_value = random_password.jwt_ed25519_seed.result
  }

  entries {
    key        = "REFRESH_TOKEN_SECRET"
    text_value = random_password.refresh_token_secret.result
  }

  # Placeholder entries — replaced with real values after OAuth app registration.
  entries {
    key        = "YANDEX_OAUTH_CLIENT_ID"
    text_value = "replace-me"
  }

  entries {
    key        = "YANDEX_OAUTH_CLIENT_SECRET"
    text_value = "replace-me"
  }

  entries {
    key        = "GOOGLE_OAUTH_CLIENT_ID"
    text_value = "replace-me"
  }

  entries {
    key        = "GOOGLE_OAUTH_CLIENT_SECRET"
    text_value = "replace-me"
  }
}

# worker workload secret — shares PG password, Kafka credentials.
resource "yandex_lockbox_secret" "worker" {
  name      = "${var.app_name}-worker"
  folder_id = var.folder_id
}

resource "yandex_lockbox_secret_version" "worker" {
  secret_id = yandex_lockbox_secret.worker.id

  entries {
    key        = "PG_PASSWORD"
    text_value = random_password.pg.result
  }
}

# oauth secret — separate secret for OAuth client credentials (easier rotation).
resource "yandex_lockbox_secret" "oauth" {
  name      = "${var.app_name}-oauth"
  folder_id = var.folder_id
}

resource "yandex_lockbox_secret_version" "oauth" {
  secret_id = yandex_lockbox_secret.oauth.id

  entries {
    key        = "YANDEX_OAUTH_CLIENT_ID"
    text_value = "replace-me"
  }

  entries {
    key        = "YANDEX_OAUTH_CLIENT_SECRET"
    text_value = "replace-me"
  }

  entries {
    key        = "GOOGLE_OAUTH_CLIENT_ID"
    text_value = "replace-me"
  }

  entries {
    key        = "GOOGLE_OAUTH_CLIENT_SECRET"
    text_value = "replace-me"
  }
}
