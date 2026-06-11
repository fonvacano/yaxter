# Managed Kubernetes cluster.
# Demo: zonal master (ru-central1-a), 1 node group, 2 preemptible s2.small nodes.
# Prod: regional master (3 AZs), 3 node groups, autoscale 6→30 s2.large non-preemptible.

# Service accounts for the cluster and its node groups.
resource "yandex_iam_service_account" "k8s_cluster" {
  name        = "${var.app_name}-k8s-cluster"
  description = "Service account for the Managed K8s cluster control plane."
  folder_id   = var.folder_id
}

resource "yandex_iam_service_account" "k8s_nodes" {
  name        = "${var.app_name}-k8s-nodes"
  description = "Service account used by Kubernetes node VMs."
  folder_id   = var.folder_id
}

resource "yandex_resourcemanager_folder_iam_member" "k8s_cluster_agent" {
  folder_id = var.folder_id
  role      = "k8s.clusters.agent"
  member    = "serviceAccount:${yandex_iam_service_account.k8s_cluster.id}"
}

resource "yandex_resourcemanager_folder_iam_member" "k8s_vpc_public_admin" {
  folder_id = var.folder_id
  role      = "vpc.publicAdmin"
  member    = "serviceAccount:${yandex_iam_service_account.k8s_cluster.id}"
}

resource "yandex_resourcemanager_folder_iam_member" "k8s_nodes_cr_puller" {
  folder_id = var.folder_id
  role      = "container-registry.images.puller"
  member    = "serviceAccount:${yandex_iam_service_account.k8s_nodes.id}"
}

resource "yandex_resourcemanager_folder_iam_member" "k8s_nodes_viewer" {
  folder_id = var.folder_id
  role      = "k8s.viewer"
  member    = "serviceAccount:${yandex_iam_service_account.k8s_nodes.id}"
}

# ─── Cluster ─────────────────────────────────────────────────────────────────

resource "yandex_kubernetes_cluster" "this" {
  name       = "${var.app_name}-k8s"
  folder_id  = var.folder_id
  network_id = var.network_id

  service_account_id      = yandex_iam_service_account.k8s_cluster.id
  node_service_account_id = yandex_iam_service_account.k8s_nodes.id

  release_channel = "REGULAR"

  dynamic "master" {
    for_each = var.master_type == "zonal" ? [1] : []

    content {
      version = var.k8s_version
      zonal {
        zone      = var.az_names[0]
        subnet_id = var.public_subnet_ids[0]
      }

      public_ip = true

      maintenance_policy {
        auto_upgrade = true

        maintenance_window {
          day        = "sunday"
          start_time = "03:00"
          duration   = "3h"
        }
      }
    }
  }

  dynamic "master" {
    for_each = var.master_type == "regional" ? [1] : []

    content {
      version = var.k8s_version
      regional {
        region = "ru-central1"

        dynamic "location" {
          for_each = var.az_names

          content {
            zone      = location.value
            subnet_id = var.public_subnet_ids[location.key]
          }
        }
      }

      public_ip = true

      maintenance_policy {
        auto_upgrade = true

        maintenance_window {
          day        = "sunday"
          start_time = "03:00"
          duration   = "3h"
        }
      }
    }
  }

  depends_on = [
    yandex_resourcemanager_folder_iam_member.k8s_cluster_agent,
    yandex_resourcemanager_folder_iam_member.k8s_vpc_public_admin,
  ]
}

# ─── Node Groups ─────────────────────────────────────────────────────────────
# One node group per AZ in prod (az_count=3 → 3 groups, 2 nodes each = 6 total min).
# Demo: az_count=1 → 1 group, initial=2, min=2, max=4.

resource "yandex_kubernetes_node_group" "this" {
  count      = length(var.az_names)
  cluster_id = yandex_kubernetes_cluster.this.id
  name       = "${var.app_name}-ng-${var.az_names[count.index]}"
  version    = var.k8s_version

  instance_template {
    platform_id = var.node_platform

    resources {
      cores         = var.node_cores
      memory        = var.node_memory
      core_fraction = 100
    }

    boot_disk {
      size = var.node_disk_size
      type = var.node_disk_type
    }

    network_interface {
      subnet_ids         = [var.private_subnet_ids[count.index]]
      nat                = false
      security_group_ids = [var.node_sg_id]
    }

    scheduling_policy {
      preemptible = var.node_preemptible
    }

    container_runtime {
      type = "containerd"
    }
  }

  scale_policy {
    auto_scale {
      min     = var.node_min
      max     = var.node_max
      initial = var.node_initial
    }
  }

  allocation_policy {
    location {
      zone = var.az_names[count.index]
    }
  }

  maintenance_policy {
    auto_upgrade = true
    auto_repair  = true

    maintenance_window {
      day        = "sunday"
      start_time = "04:00"
      duration   = "2h"
    }
  }

  node_labels = {
    "app"  = var.app_name
    "zone" = var.az_names[count.index]
  }
}
