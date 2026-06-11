terraform {
  required_version = ">= 1.9"

  required_providers {
    yandex = {
      source  = "yandex-cloud/yandex"
      version = "~> 0.122"
    }
  }

  # Bootstrap uses local backend — it creates the remote state bucket.
  # After applying, configure the root module to use the remote backend.
  backend "local" {}
}

provider "yandex" {
  cloud_id  = var.cloud_id
  folder_id = var.folder_id
}
