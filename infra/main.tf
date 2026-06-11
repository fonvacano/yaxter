# Root module — wires all child modules.
# Dependency order: network + iam_lockbox → pg/redis/kafka/k8s → storage_cdn + alb

module "network" {
  source = "./modules/network"

  folder_id            = var.folder_id
  app_name             = var.app_name
  vpc_name             = var.vpc_name
  az_count             = var.az_count
  az_names             = var.az_names
  private_subnet_cidrs = var.private_subnet_cidrs
  public_subnet_cidrs  = var.public_subnet_cidrs
}

module "iam_lockbox" {
  source = "./modules/iam-lockbox"

  folder_id = var.folder_id
  app_name  = var.app_name
}

module "pg" {
  source = "./modules/pg"

  folder_id       = var.folder_id
  app_name        = var.app_name
  network_id      = module.network.vpc_id
  subnet_ids      = module.network.private_subnet_ids
  security_group_ids = [module.network.data_sg_id]
  az_names        = slice(var.az_names, 0, var.az_count)

  pg_version          = var.pg_version
  resource_preset     = var.pg_resource_preset
  disk_size           = var.pg_disk_size
  disk_type           = var.pg_disk_type
  host_count          = var.pg_host_count
  physical_shards     = var.physical_shards
  database_name       = var.pg_database_name
  user_name           = var.pg_user_name
  user_password       = module.iam_lockbox.pg_password
}

module "redis" {
  source = "./modules/redis"

  folder_id          = var.folder_id
  app_name           = var.app_name
  network_id         = module.network.vpc_id
  subnet_ids         = module.network.private_subnet_ids
  security_group_ids = [module.network.data_sg_id]
  az_names           = slice(var.az_names, 0, var.az_count)

  redis_version   = var.redis_version
  resource_preset = var.redis_resource_preset
  sharded         = var.redis_sharded
  replica_count   = var.redis_replica_count
}

module "kafka" {
  source = "./modules/kafka"

  folder_id          = var.folder_id
  app_name           = var.app_name
  network_id         = module.network.vpc_id
  subnet_ids         = module.network.private_subnet_ids
  security_group_ids = [module.network.data_sg_id]
  az_names           = slice(var.az_names, 0, var.az_count)

  kafka_version            = var.kafka_version
  broker_count             = var.kafka_brokers
  broker_resource_preset   = var.kafka_broker_resource_preset
  broker_disk_size         = var.kafka_broker_disk_size
  broker_disk_type         = var.kafka_broker_disk_type
  replication_factor       = var.kafka_replication_factor
  min_insync_replicas      = var.kafka_min_insync_replicas
  partitions_tweets        = var.kafka_partitions_tweets
  partitions_engagements   = var.kafka_partitions_engagements
  partitions_follows       = var.kafka_partitions_follows
  partitions_media         = var.kafka_partitions_media
}

module "k8s" {
  source = "./modules/k8s"

  folder_id          = var.folder_id
  app_name           = var.app_name
  network_id         = module.network.vpc_id
  public_subnet_ids  = module.network.public_subnet_ids
  private_subnet_ids = module.network.private_subnet_ids
  node_sg_id         = module.network.k8s_node_sg_id
  az_names           = slice(var.az_names, 0, var.az_count)

  k8s_version        = var.k8s_version
  master_type        = var.k8s_master_type
  node_platform      = var.k8s_node_platform
  node_cores         = var.k8s_node_cores
  node_memory        = var.k8s_node_memory
  node_disk_size     = var.k8s_node_disk_size
  node_disk_type     = var.k8s_node_disk_type
  node_preemptible   = var.k8s_node_preemptible
  node_min           = var.k8s_node_min
  node_max           = var.k8s_node_max
  node_initial       = var.k8s_node_initial

  depends_on = [module.network, module.iam_lockbox]
}

module "storage_cdn" {
  source = "./modules/storage-cdn"

  folder_id        = var.folder_id
  app_name         = var.app_name
  media_bucket_name = var.media_bucket_name
  web_bucket_name  = var.web_bucket_name
  cdn_enabled      = var.cdn_enabled
  worker_sa_id     = module.iam_lockbox.worker_sa_id
  api_sa_id        = module.iam_lockbox.api_sa_id
}

module "alb" {
  source = "./modules/alb"

  folder_id          = var.folder_id
  app_name           = var.app_name
  network_id         = module.network.vpc_id
  public_subnet_ids  = module.network.public_subnet_ids
  alb_sg_id          = module.network.alb_sg_id
  az_names           = slice(var.az_names, 0, var.alb_az_count)
  alb_az_count       = var.alb_az_count

  domain_name           = var.domain_name
  tls_certificate_id    = var.tls_certificate_id
  web_bucket_name       = module.storage_cdn.web_bucket_name
  web_bucket_website_endpoint = module.storage_cdn.web_bucket_website_endpoint

  depends_on = [module.network, module.storage_cdn]
}
