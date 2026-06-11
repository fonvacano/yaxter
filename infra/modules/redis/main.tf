# Managed Redis cluster.
# Demo: 1 host, b2.medium, no persistence, no sharding.
# Prod: 6 shards × 3 replicas, s3.medium, sharded=true.
#
# Redis is intentionally stateless cache — no persistence configured.
# On eviction or restart, the application rebuilds from PostgreSQL (§2.3).

locals {
  # Number of shards: 6 in prod (sharded=true), 1 in demo.
  shard_count = var.sharded ? 6 : 1

  # Build the list of hosts:
  # sharded=false → 1 host in az_names[0]
  # sharded=true  → shard_count * replica_count hosts distributed across AZs
  hosts = [
    for idx in range(local.shard_count * var.replica_count) : {
      zone      = var.az_names[idx % length(var.az_names)]
      subnet_id = var.subnet_ids[idx % length(var.subnet_ids)]
      shard_name = var.sharded ? "shard${floor(idx / var.replica_count)}" : "shard0"
    }
  ]
}

resource "yandex_mdb_redis_cluster" "this" {
  name        = "${var.app_name}-redis"
  folder_id   = var.folder_id
  environment = "PRODUCTION"
  network_id  = var.network_id
  sharded     = var.sharded

  security_group_ids = var.security_group_ids

  config {
    version  = var.redis_version
    password = "" # YC Managed Redis uses TLS without password in private network; override if needed

    # No persistence — Redis is a cache layer; source of truth is PostgreSQL.
    # maxmemory-policy matches the caching use-case.
    maxmemory_policy = "allkeys-lru"
  }

  resources {
    resource_preset_id = var.resource_preset
    disk_size          = 16
    disk_type_id       = "network-ssd"
  }

  dynamic "host" {
    for_each = local.hosts

    content {
      zone       = host.value.zone
      subnet_id  = host.value.subnet_id
      shard_name = host.value.shard_name
    }
  }

  maintenance_window {
    type = "WEEKLY"
    day  = "SUN"
    hour = 4
  }
}
