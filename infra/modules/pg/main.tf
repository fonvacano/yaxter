# Managed PostgreSQL — one cluster per physical shard.
# Demo: physical_shards=1 → 1 cluster, 1 host, b2.medium.
# Prod: physical_shards=4 → 4 clusters, 3 hosts each (cross-AZ HA), s3.large.
#
# 256 logical shards are mapped across physical_shards clusters entirely in
# application config (shard map); no Terraform logic encodes that mapping.

locals {
  # Build a flat list of {shard_index, az_index} for multi-host clusters.
  # Each shard gets host_count hosts spread round-robin across AZs.
  shard_host_pairs = flatten([
    for shard_idx in range(var.physical_shards) : [
      for host_idx in range(var.host_count) : {
        shard_idx = shard_idx
        host_idx  = host_idx
        az_name   = var.az_names[host_idx % length(var.az_names)]
        subnet_id = var.subnet_ids[host_idx % length(var.subnet_ids)]
      }
    ]
  ])
}

resource "yandex_mdb_postgresql_cluster" "shard" {
  count = var.physical_shards

  name        = "${var.app_name}-pg-shard-${count.index}"
  folder_id   = var.folder_id
  environment = "PRODUCTION"
  network_id  = var.network_id

  security_group_ids = var.security_group_ids

  config {
    version = var.pg_version

    resources {
      resource_preset_id = var.resource_preset
      disk_size          = var.disk_size
      disk_type_id       = var.disk_type
    }

    postgresql_config = {
      # Enable logical replication (required for future CDC / outbox relay use).
      "wal_level"               = "LOGICAL"
      # Autovacuum tuning for high-churn outbox table.
      "autovacuum_vacuum_scale_factor"  = "0.01"
      "autovacuum_analyze_scale_factor" = "0.05"
    }

    access {
      web_sql = false
    }

    performance_diagnostics {
      enabled                      = true
      sessions_sampling_interval   = 60
      statements_sampling_interval = 600
    }
  }

  # Distribute hosts across AZs for HA; in demo host_count=1 so only az_names[0] used.
  dynamic "host" {
    for_each = [
      for pair in local.shard_host_pairs : pair
      if pair.shard_idx == count.index
    ]

    content {
      zone      = host.value.az_name
      subnet_id = host.value.subnet_id

      # First host is primary; additional hosts are replicas.
      assign_public_ip = false
      replication_source = host.value.host_idx == 0 ? "" : null
    }
  }

  database {
    name  = var.database_name
    owner = var.user_name
  }

  user {
    name     = var.user_name
    password = var.user_password

    permission {
      database_name = var.database_name
    }

    grants = ["pg_read_all_stats"]
  }

  maintenance_window {
    type = "WEEKLY"
    day  = "SUN"
    hour = 3
  }
}
