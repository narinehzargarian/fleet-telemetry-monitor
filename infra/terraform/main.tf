# AWS infrastructure for the platform: private networking, managed Postgres (RDS),
# managed Redis (ElastiCache), and the ingest token in Secrets Manager.
# The services themselves run on Kubernetes (EKS or any cluster) using deploy/k8s.

terraform {
  required_version = ">= 1.6"
  required_providers {
    aws    = { source = "hashicorp/aws", version = "~> 5.0" }
    random = { source = "hashicorp/random", version = "~> 3.6" }
  }
}

provider "aws" {
  region = var.region
}

data "aws_availability_zones" "available" {
  state = "available"
}

locals {
  azs  = slice(data.aws_availability_zones.available.names, 0, 2)
  tags = { Project = "fleet-telemetry-monitor", Environment = var.environment }
}

# ---------- Network: databases live only in private subnets ----------
resource "aws_vpc" "main" {
  cidr_block           = "10.40.0.0/16"
  enable_dns_hostnames = true
  tags                 = merge(local.tags, { Name = "fleet-${var.environment}" })
}

resource "aws_subnet" "private" {
  count             = length(local.azs)
  vpc_id            = aws_vpc.main.id
  cidr_block        = cidrsubnet(aws_vpc.main.cidr_block, 8, count.index)
  availability_zone = local.azs[count.index]
  tags              = merge(local.tags, { Name = "fleet-private-${count.index}" })
}

# Only workloads inside the VPC (the app's security group) may reach the data stores.
resource "aws_security_group" "app" {
  name   = "fleet-app-${var.environment}"
  vpc_id = aws_vpc.main.id
  tags   = local.tags
}

resource "aws_security_group" "data" {
  name   = "fleet-data-${var.environment}"
  vpc_id = aws_vpc.main.id
  tags   = local.tags

  ingress {
    description     = "Postgres from app"
    from_port       = 5432
    to_port         = 5432
    protocol        = "tcp"
    security_groups = [aws_security_group.app.id]
  }
  ingress {
    description     = "Redis from app"
    from_port       = 6379
    to_port         = 6379
    protocol        = "tcp"
    security_groups = [aws_security_group.app.id]
  }
}

# ---------- PostgreSQL ----------
resource "random_password" "db" {
  length  = 32
  special = false
}

resource "aws_db_subnet_group" "main" {
  name       = "fleet-${var.environment}"
  subnet_ids = aws_subnet.private[*].id
  tags       = local.tags
}

resource "aws_db_instance" "postgres" {
  identifier              = "fleet-${var.environment}"
  engine                  = "postgres"
  engine_version          = "16"
  instance_class          = var.db_instance_class
  allocated_storage       = 20
  db_name                 = "fleet"
  username                = "fleet"
  password                = random_password.db.result
  db_subnet_group_name    = aws_db_subnet_group.main.name
  vpc_security_group_ids  = [aws_security_group.data.id]
  storage_encrypted       = true
  backup_retention_period = 7
  publicly_accessible     = false
  skip_final_snapshot     = var.environment != "prod"
  tags                    = local.tags
}

# ---------- Redis ----------
resource "aws_elasticache_subnet_group" "main" {
  name       = "fleet-${var.environment}"
  subnet_ids = aws_subnet.private[*].id
}

resource "aws_elasticache_cluster" "redis" {
  cluster_id         = "fleet-${var.environment}"
  engine             = "redis"
  node_type          = var.redis_node_type
  num_cache_nodes    = 1
  port               = 6379
  subnet_group_name  = aws_elasticache_subnet_group.main.name
  security_group_ids = [aws_security_group.data.id]
  tags               = local.tags
}

# ---------- Secrets: generated here, never stored in git ----------
resource "random_password" "ingest_token" {
  length  = 40
  special = false
}

resource "aws_secretsmanager_secret" "app" {
  name = "fleet/${var.environment}/app"
  tags = local.tags
}

resource "aws_secretsmanager_secret_version" "app" {
  secret_id = aws_secretsmanager_secret.app.id
  secret_string = jsonencode({
    DATABASE_URL = "postgres://fleet:${random_password.db.result}@${aws_db_instance.postgres.address}:5432/fleet?sslmode=require"
    REDIS_ADDR   = "${aws_elasticache_cluster.redis.cache_nodes[0].address}:6379"
    INGEST_TOKEN = random_password.ingest_token.result
  })
}
