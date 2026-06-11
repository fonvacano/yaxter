# Application Load Balancer.
# Path routing:
#   /v1/*   → api backend (k8s NodePort / target group referencing node IPs)
#   else    → web bucket static website
#
# TLS terminates at the ALB; all backend connections are internal.
# Demo: 1 AZ, 1 listener. Prod: 3 AZs, same config module (alb_az_count=3).

# ─── Backend Groups ───────────────────────────────────────────────────────────

# API backend — forwards to k8s NodePort service.
# Target group is managed by the k8s cloud controller via Service type=NodePort
# annotations; we create the backend group here and reference it by name in Helm.
resource "yandex_alb_backend_group" "api" {
  name      = "${var.app_name}-api-bg"
  folder_id = var.folder_id

  http_backend {
    name             = "api-backend"
    port             = 30080 # NodePort assigned to the api Service; override in Helm values.
    weight           = 100
    target_group_ids = [] # Populated by the k8s cloud controller; left empty for Terraform to manage the resource shape.

    healthcheck {
      timeout             = "10s"
      interval            = "2s"
      healthy_threshold   = 2
      unhealthy_threshold = 3

      http_healthcheck {
        path = "/healthz"
      }
    }

    http2 = false
  }
}

# Web SPA backend — S3 static website bucket as an HTTP backend.
resource "yandex_alb_backend_group" "web" {
  name      = "${var.app_name}-web-bg"
  folder_id = var.folder_id

  http_backend {
    name   = "web-backend"
    weight = 100
    port   = 80

    # S3 static website hostname.
    target_group_ids = []

    storageBucket = var.web_bucket_name

    healthcheck {
      timeout             = "10s"
      interval            = "2s"
      healthy_threshold   = 2
      unhealthy_threshold = 3

      http_healthcheck {
        path = "/index.html"
      }
    }

    http2 = false
  }
}

# ─── HTTP Router ─────────────────────────────────────────────────────────────

resource "yandex_alb_http_router" "this" {
  name      = "${var.app_name}-router"
  folder_id = var.folder_id
}

resource "yandex_alb_virtual_host" "this" {
  name           = "${var.app_name}-vhost"
  http_router_id = yandex_alb_http_router.this.id

  authority = [var.domain_name]

  # Route 1: API — /v1/* prefix.
  route {
    name = "api"

    http_route {
      http_match {
        path {
          prefix = "/v1/"
        }
      }

      http_route_action {
        backend_group_id = yandex_alb_backend_group.api.id
        timeout          = "60s"
      }
    }
  }

  # Route 2: SPA — catch-all for everything else.
  route {
    name = "web"

    http_route {
      http_match {
        path {
          prefix = "/"
        }
      }

      http_route_action {
        backend_group_id = yandex_alb_backend_group.web.id
        timeout          = "30s"
      }
    }
  }
}

# ─── Load Balancer ────────────────────────────────────────────────────────────

resource "yandex_alb_load_balancer" "this" {
  name      = "${var.app_name}-alb"
  folder_id = var.folder_id
  network_id = var.network_id

  security_group_ids = [var.alb_sg_id]

  # Allocate one node per AZ (alb_az_count drives how many).
  dynamic "allocation_policy" {
    for_each = [1]

    content {
      dynamic "location" {
        for_each = range(var.alb_az_count)

        content {
          zone_id   = var.az_names[location.value]
          subnet_id = var.public_subnet_ids[location.value]
        }
      }
    }
  }

  # HTTPS listener with TLS termination.
  listener {
    name = "https"

    endpoint {
      address {
        external_ipv4_address {}
      }
      ports = [443]
    }

    tls {
      default_handler {
        http_handler {
          http_router_id = yandex_alb_http_router.this.id
          http2_options {}
        }

        certificate_ids = [var.tls_certificate_id]
      }
    }
  }

  # HTTP listener — redirect to HTTPS.
  listener {
    name = "http"

    endpoint {
      address {
        external_ipv4_address {}
      }
      ports = [80]
    }

    http {
      redirects {
        http_to_https = true
      }
    }
  }
}
