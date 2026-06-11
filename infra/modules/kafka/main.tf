# Managed Kafka cluster.
# Demo: 1 broker, s2.micro, 32 GB, RF=1, min.insync=1.
# Prod: 3+ brokers across AZs, s3.medium, RF=3, min.insync=2.
#
# Topics are fixed in name; only partition counts vary by tfvars.
# Topic names follow the versioned contract: <domain>.v<N>.
# Partition scaling is online (Kafka only adds partitions); key→partition
# affinity is contractually not assumed (see ARCHITECTURE.md §2.4).

locals {
  # Brokers are distributed round-robin across available AZs.
  broker_hosts = [
    for idx in range(var.broker_count) : {
      zone      = var.az_names[idx % length(var.az_names)]
      subnet_id = var.subnet_ids[idx % length(var.subnet_ids)]
    }
  ]
}

resource "yandex_mdb_kafka_cluster" "this" {
  name        = "${var.app_name}-kafka"
  folder_id   = var.folder_id
  environment = "PRODUCTION"
  network_id  = var.network_id

  security_group_ids = var.security_group_ids

  config {
    version          = var.kafka_version
    brokers_count    = var.broker_count
    zones            = var.az_names
    assign_public_ip = false

    kafka {
      resources {
        resource_preset_id = var.broker_resource_preset
        disk_size          = var.broker_disk_size
        disk_type_id       = var.broker_disk_type
      }

      kafka_config = {
        compression_type                = "LZ4"
        default_replication_factor      = tostring(var.replication_factor)
        min_insync_replicas             = tostring(var.min_insync_replicas)
        num_partitions                  = "3"
        auto_create_topics_enable       = false
        log_flush_interval_messages     = "9223372036854775807"
        log_flush_interval_ms           = "1000"
        log_retention_bytes             = "-1"
        log_retention_hours             = "168"
        log_segment_bytes               = "1073741824"
        message_max_bytes               = "1048588"
        replica_fetch_max_bytes         = "1048576"
        socket_receive_buffer_bytes     = "102400"
        socket_send_buffer_bytes        = "102400"
      }
    }
  }

  maintenance_window {
    type = "WEEKLY"
    day  = "SUN"
    hour = 5
  }
}

# ─── Topics ──────────────────────────────────────────────────────────────────
# Fixed topic names; partition counts driven by tfvars.

resource "yandex_mdb_kafka_topic" "tweets_v1" {
  cluster_id         = yandex_mdb_kafka_cluster.this.id
  name               = "tweets.v1"
  partitions         = var.partitions_tweets
  replication_factor = var.replication_factor

  topic_config = {
    min_insync_replicas = tostring(var.min_insync_replicas)
    retention_ms        = "604800000" # 7 days
    segment_bytes       = "1073741824"
  }
}

resource "yandex_mdb_kafka_topic" "engagements_v1" {
  cluster_id         = yandex_mdb_kafka_cluster.this.id
  name               = "engagements.v1"
  partitions         = var.partitions_engagements
  replication_factor = var.replication_factor

  topic_config = {
    min_insync_replicas = tostring(var.min_insync_replicas)
    retention_ms        = "604800000"
    segment_bytes       = "1073741824"
  }
}

resource "yandex_mdb_kafka_topic" "follows_v1" {
  cluster_id         = yandex_mdb_kafka_cluster.this.id
  name               = "follows.v1"
  partitions         = var.partitions_follows
  replication_factor = var.replication_factor

  topic_config = {
    min_insync_replicas = tostring(var.min_insync_replicas)
    retention_ms        = "604800000"
    segment_bytes       = "1073741824"
  }
}

resource "yandex_mdb_kafka_topic" "media_v1" {
  cluster_id         = yandex_mdb_kafka_cluster.this.id
  name               = "media.v1"
  partitions         = var.partitions_media
  replication_factor = var.replication_factor

  topic_config = {
    min_insync_replicas = tostring(var.min_insync_replicas)
    retention_ms        = "604800000"
    segment_bytes       = "1073741824"
  }
}

# ─── Kafka Users ─────────────────────────────────────────────────────────────

# api user — producer only (publishes via outbox relay).
resource "yandex_mdb_kafka_user" "relay" {
  cluster_id = yandex_mdb_kafka_cluster.this.id
  name       = "relay"
  password   = "change-me-via-lockbox"

  permission {
    topic_name  = "tweets.v1"
    role        = "ACCESS_ROLE_PRODUCER"
    allow_hosts = []
  }

  permission {
    topic_name  = "engagements.v1"
    role        = "ACCESS_ROLE_PRODUCER"
    allow_hosts = []
  }

  permission {
    topic_name  = "follows.v1"
    role        = "ACCESS_ROLE_PRODUCER"
    allow_hosts = []
  }

  permission {
    topic_name  = "media.v1"
    role        = "ACCESS_ROLE_PRODUCER"
    allow_hosts = []
  }
}

# worker user — consumers only.
resource "yandex_mdb_kafka_user" "worker" {
  cluster_id = yandex_mdb_kafka_cluster.this.id
  name       = "worker"
  password   = "change-me-via-lockbox"

  permission {
    topic_name  = "tweets.v1"
    role        = "ACCESS_ROLE_CONSUMER"
    allow_hosts = []
  }

  permission {
    topic_name  = "engagements.v1"
    role        = "ACCESS_ROLE_CONSUMER"
    allow_hosts = []
  }

  permission {
    topic_name  = "follows.v1"
    role        = "ACCESS_ROLE_CONSUMER"
    allow_hosts = []
  }

  permission {
    topic_name  = "media.v1"
    role        = "ACCESS_ROLE_CONSUMER"
    allow_hosts = []
  }
}
