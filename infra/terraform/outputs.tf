output "postgres_endpoint" {
  description = "RDS endpoint (private)"
  value       = aws_db_instance.postgres.address
}

output "redis_endpoint" {
  description = "ElastiCache endpoint (private)"
  value       = aws_elasticache_cluster.redis.cache_nodes[0].address
}

output "app_secret_arn" {
  description = "Secrets Manager ARN holding DATABASE_URL, REDIS_ADDR, INGEST_TOKEN"
  value       = aws_secretsmanager_secret.app.arn
}

output "app_security_group_id" {
  description = "Attach this security group to the app workloads so they can reach the data stores"
  value       = aws_security_group.app.id
}
