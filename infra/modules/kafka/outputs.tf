output "cluster_id" {
  value       = yandex_mdb_kafka_cluster.this.id
  description = "Kafka cluster ID."
}

output "bootstrap_brokers" {
  value = join(",", [
    for host in yandex_mdb_kafka_cluster.this.host :
    "${host.name}:9091"
    if host.role == "KAFKA"
  ])
  description = "Comma-separated list of Kafka broker bootstrap addresses (SASL_SSL port 9091)."
  sensitive   = true
}

output "topic_tweets_v1" {
  value       = yandex_mdb_kafka_topic.tweets_v1.name
  description = "Name of the tweets topic."
}

output "topic_engagements_v1" {
  value       = yandex_mdb_kafka_topic.engagements_v1.name
  description = "Name of the engagements topic."
}

output "topic_follows_v1" {
  value       = yandex_mdb_kafka_topic.follows_v1.name
  description = "Name of the follows topic."
}

output "topic_media_v1" {
  value       = yandex_mdb_kafka_topic.media_v1.name
  description = "Name of the media topic."
}
