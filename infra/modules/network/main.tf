# ─── VPC ─────────────────────────────────────────────────────────────────────

resource "yandex_vpc_network" "this" {
  name      = var.vpc_name
  folder_id = var.folder_id
}

# ─── Subnets ─────────────────────────────────────────────────────────────────
# az_count controls how many subnets are instantiated.
# All three CIDR blocks are always declared in tfvars; only the first az_count
# are created. This mirrors the §3 design note: "3 subnet sets declared,
# az_count=1 instantiates one."

resource "yandex_vpc_subnet" "private" {
  count = var.az_count

  name           = "${var.app_name}-private-${var.az_names[count.index]}"
  folder_id      = var.folder_id
  network_id     = yandex_vpc_network.this.id
  zone           = var.az_names[count.index]
  v4_cidr_blocks = [var.private_subnet_cidrs[count.index]]

  route_table_id = yandex_vpc_route_table.private[count.index].id
}

resource "yandex_vpc_subnet" "public" {
  count = var.az_count

  name           = "${var.app_name}-public-${var.az_names[count.index]}"
  folder_id      = var.folder_id
  network_id     = yandex_vpc_network.this.id
  zone           = var.az_names[count.index]
  v4_cidr_blocks = [var.public_subnet_cidrs[count.index]]
}

# ─── NAT Gateway ─────────────────────────────────────────────────────────────

resource "yandex_vpc_gateway" "nat" {
  name      = "${var.app_name}-nat-gw"
  folder_id = var.folder_id

  shared_egress_gateway {}
}

# ─── Route Tables ─────────────────────────────────────────────────────────────
# Each private subnet gets a route table pointing 0.0.0.0/0 to the NAT gateway.

resource "yandex_vpc_route_table" "private" {
  count = var.az_count

  name      = "${var.app_name}-rt-private-${var.az_names[count.index]}"
  folder_id = var.folder_id
  network_id = yandex_vpc_network.this.id

  static_route {
    destination_prefix = "0.0.0.0/0"
    gateway_id         = yandex_vpc_gateway.nat.id
  }
}

# ─── Security Groups ─────────────────────────────────────────────────────────

# K8s node security group — nodes talk to API server and data plane.
resource "yandex_vpc_security_group" "k8s_node" {
  name       = "${var.app_name}-k8s-node-sg"
  folder_id  = var.folder_id
  network_id = yandex_vpc_network.this.id

  # Allow all intra-node-group traffic (required by k8s pod networking).
  ingress {
    protocol          = "ANY"
    description       = "Intra-cluster pod/node traffic"
    predefined_target = "self_security_group"
  }

  # Allow node-to-data-plane traffic (rules added via data_sg).
  ingress {
    protocol       = "TCP"
    description    = "Health checks from ALB"
    port           = 10256
    v4_cidr_blocks = ["198.18.235.0/24", "198.18.248.0/24"] # YC ALB health-check ranges
  }

  ingress {
    protocol       = "TCP"
    description    = "NodePort range for ALB backend"
    from_port      = 30000
    to_port        = 32767
    v4_cidr_blocks = ["0.0.0.0/0"]
  }

  egress {
    protocol       = "ANY"
    description    = "Allow all egress (via NAT for external, direct for internal)"
    v4_cidr_blocks = ["0.0.0.0/0"]
  }
}

# ALB security group — accepts inbound HTTPS, forwards to nodes.
resource "yandex_vpc_security_group" "alb" {
  name       = "${var.app_name}-alb-sg"
  folder_id  = var.folder_id
  network_id = yandex_vpc_network.this.id

  ingress {
    protocol       = "TCP"
    description    = "HTTPS from internet"
    port           = 443
    v4_cidr_blocks = ["0.0.0.0/0"]
  }

  ingress {
    protocol       = "TCP"
    description    = "HTTP redirect"
    port           = 80
    v4_cidr_blocks = ["0.0.0.0/0"]
  }

  ingress {
    protocol       = "TCP"
    description    = "ALB health checks"
    from_port      = 0
    to_port        = 65535
    v4_cidr_blocks = ["198.18.235.0/24", "198.18.248.0/24"]
  }

  egress {
    protocol       = "ANY"
    description    = "Forward to node port range"
    v4_cidr_blocks = ["0.0.0.0/0"]
  }
}

# Data security group — controls which sources may reach data services.
# K8s nodes → PG port 6432 (PgBouncer), Redis 6380 (TLS), Kafka 9091 (TLS).
resource "yandex_vpc_security_group" "data" {
  name       = "${var.app_name}-data-sg"
  folder_id  = var.folder_id
  network_id = yandex_vpc_network.this.id

  # PostgreSQL (via PgBouncer which also uses 6432; native PG listens on 6432 in Managed PG).
  ingress {
    protocol          = "TCP"
    description       = "PostgreSQL from k8s nodes"
    port              = 6432
    predefined_target = "self_security_group" # refined below via k8s_node_sg reference
  }

  ingress {
    protocol       = "TCP"
    description    = "PostgreSQL from k8s node private subnets"
    port           = 6432
    v4_cidr_blocks = var.private_subnet_cidrs
  }

  # Redis TLS port.
  ingress {
    protocol       = "TCP"
    description    = "Redis TLS from k8s node private subnets"
    port           = 6380
    v4_cidr_blocks = var.private_subnet_cidrs
  }

  # Kafka TLS port (SASL_SSL listener).
  ingress {
    protocol       = "TCP"
    description    = "Kafka SASL_SSL from k8s node private subnets"
    port           = 9091
    v4_cidr_blocks = var.private_subnet_cidrs
  }

  egress {
    protocol       = "ANY"
    description    = "Managed service internal traffic"
    v4_cidr_blocks = ["0.0.0.0/0"]
  }
}
