data "aws_vpc" "default" {
  default = true
}

data "aws_subnets" "default" {
  filter {
    name   = "vpc-id"
    values = [data.aws_vpc.default.id]
  }
}

resource "aws_ecr_repository" "telemetry_ingest" {
  name         = "${local.resource_prefix}-telemetry-ingest"
  force_delete = true

  image_scanning_configuration {
    scan_on_push = true
  }

  tags = {
    Boundary = "telemetry-ingest"
  }
}

resource "aws_cloudwatch_log_group" "telemetry_ingest" {
  name              = "/homesignal/staging/telemetry-ingest"
  retention_in_days = 7

  tags = {
    Boundary = "telemetry-ingest"
  }
}

resource "aws_ecs_cluster" "runtime" {
  name = "${local.resource_prefix}-runtime-cluster"

  setting {
    name  = "containerInsights"
    value = "disabled"
  }

  tags = {
    Boundary = "platform"
  }
}

resource "aws_iam_role" "telemetry_ingest_task_execution" {
  name = "${local.resource_prefix}-telemetry-ingest-exec-role"

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Effect = "Allow"
        Principal = {
          Service = "ecs-tasks.amazonaws.com"
        }
        Action = "sts:AssumeRole"
      }
    ]
  })

  tags = {
    Boundary = "telemetry-ingest"
  }
}

resource "aws_iam_role_policy_attachment" "telemetry_ingest_task_execution" {
  role       = aws_iam_role.telemetry_ingest_task_execution.name
  policy_arn = "arn:aws:iam::aws:policy/service-role/AmazonECSTaskExecutionRolePolicy"
}

resource "aws_iam_role" "telemetry_ingest_task" {
  name = "${local.resource_prefix}-telemetry-ingest-task-role"

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Effect = "Allow"
        Principal = {
          Service = "ecs-tasks.amazonaws.com"
        }
        Action = "sts:AssumeRole"
      }
    ]
  })

  tags = {
    Boundary = "telemetry-ingest"
  }
}

resource "aws_security_group" "telemetry_ingest" {
  name        = "${local.resource_prefix}-telemetry-ingest-sg"
  description = "Temporary direct staging access to telemetry-ingest skeleton"
  vpc_id      = data.aws_vpc.default.id

  ingress {
    description = "Temporary staging telemetry-ingest skeleton HTTP"
    from_port   = 8080
    to_port     = 8080
    protocol    = "tcp"
    cidr_blocks = var.telemetry_ingest_ingress_cidr_blocks
  }

  egress {
    description = "Allow outbound access for image pulls and future service calls"
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }

  tags = {
    Boundary = "telemetry-ingest"
  }
}

resource "aws_ecs_task_definition" "telemetry_ingest" {
  family                   = "${local.resource_prefix}-telemetry-ingest-runtime"
  requires_compatibilities = ["FARGATE"]
  network_mode             = "awsvpc"
  cpu                      = tostring(var.telemetry_ingest_cpu)
  memory                   = tostring(var.telemetry_ingest_memory)
  execution_role_arn       = aws_iam_role.telemetry_ingest_task_execution.arn
  task_role_arn            = aws_iam_role.telemetry_ingest_task.arn

  runtime_platform {
    operating_system_family = "LINUX"
    cpu_architecture        = "ARM64"
  }

  container_definitions = jsonencode([
    {
      name      = "telemetry-ingest"
      image     = var.telemetry_ingest_image
      essential = true

      portMappings = [
        {
          containerPort = 8080
          hostPort      = 8080
          protocol      = "tcp"
        }
      ]

      environment = [
        {
          name  = "HOMESIGNAL_ENV"
          value = local.environment
        },
        {
          name  = "HOMESIGNAL_AWS_REGION"
          value = var.aws_region
        },
        {
          name  = "HOMESIGNAL_SERVICE_NAME"
          value = "telemetry-ingest"
        },
        {
          name  = "HOMESIGNAL_VERSION"
          value = var.artifact_version
        }
      ]

      logConfiguration = {
        logDriver = "awslogs"
        options = {
          awslogs-group         = aws_cloudwatch_log_group.telemetry_ingest.name
          awslogs-region        = var.aws_region
          awslogs-stream-prefix = "telemetry-ingest"
        }
      }
    }
  ])

  tags = {
    Boundary = "telemetry-ingest"
  }
}

resource "aws_ecs_service" "telemetry_ingest" {
  name            = "${local.resource_prefix}-telemetry-ingest-runtime"
  cluster         = aws_ecs_cluster.runtime.id
  task_definition = aws_ecs_task_definition.telemetry_ingest.arn
  desired_count   = var.telemetry_ingest_desired_count
  launch_type     = "FARGATE"

  deployment_maximum_percent         = 200
  deployment_minimum_healthy_percent = 0

  network_configuration {
    subnets          = data.aws_subnets.default.ids
    security_groups  = [aws_security_group.telemetry_ingest.id]
    assign_public_ip = true
  }

  depends_on = [
    aws_iam_role_policy_attachment.telemetry_ingest_task_execution,
  ]

  tags = {
    Boundary = "telemetry-ingest"
  }
}
