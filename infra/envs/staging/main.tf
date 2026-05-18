data "aws_caller_identity" "current" {}

locals {
  environment     = "staging"
  project         = "homesignal"
  resource_prefix = "${local.project}-${local.environment}"

  tags = {
    Project     = local.project
    Environment = local.environment
    Boundary    = "control-plane"
    ManagedBy   = "iac"
    Owner       = var.owner_tag
  }
}

resource "aws_cloudwatch_log_group" "control_plane" {
  name              = "/homesignal/staging/control-plane"
  retention_in_days = 7
}

resource "aws_cloudwatch_log_group" "public_api_access" {
  name              = "/homesignal/staging/public-api/access"
  retention_in_days = 7
}

resource "aws_iam_role" "control_plane_runtime" {
  name = "${local.resource_prefix}-control-plane-runtime-role"

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Effect = "Allow"
        Principal = {
          Service = "lambda.amazonaws.com"
        }
        Action = "sts:AssumeRole"
      }
    ]
  })
}

resource "aws_iam_role_policy_attachment" "lambda_basic" {
  role       = aws_iam_role.control_plane_runtime.name
  policy_arn = "arn:aws:iam::aws:policy/service-role/AWSLambdaBasicExecutionRole"
}

resource "aws_lambda_function" "control_plane" {
  function_name = "${local.resource_prefix}-control-plane-runtime"
  role          = aws_iam_role.control_plane_runtime.arn
  runtime       = "provided.al2023"
  handler       = "bootstrap"
  architectures = ["arm64"]

  filename         = var.lambda_package_path
  source_code_hash = filebase64sha256(var.lambda_package_path)

  memory_size = 128
  timeout     = 10

  environment {
    variables = {
      HOMESIGNAL_ENV          = local.environment
      HOMESIGNAL_AWS_REGION   = var.aws_region
      HOMESIGNAL_SERVICE_NAME = "control-plane"
      HOMESIGNAL_VERSION      = var.artifact_version
    }
  }

  logging_config {
    log_format = "JSON"
    log_group  = aws_cloudwatch_log_group.control_plane.name
  }

  depends_on = [
    aws_cloudwatch_log_group.control_plane,
    aws_iam_role_policy_attachment.lambda_basic,
  ]
}

resource "aws_apigatewayv2_api" "public" {
  name          = "${local.resource_prefix}-public-api"
  protocol_type = "HTTP"
}

resource "aws_apigatewayv2_integration" "control_plane" {
  api_id                 = aws_apigatewayv2_api.public.id
  integration_type       = "AWS_PROXY"
  integration_method     = "POST"
  integration_uri        = aws_lambda_function.control_plane.invoke_arn
  payload_format_version = "2.0"
  timeout_milliseconds   = 10000
}

resource "aws_apigatewayv2_route" "control_plane" {
  for_each = toset([
    "GET /healthz",
    "GET /readyz",
    "GET /version",
    "ANY /api/v1/{proxy+}",
  ])

  api_id    = aws_apigatewayv2_api.public.id
  route_key = each.value
  target    = "integrations/${aws_apigatewayv2_integration.control_plane.id}"
}

resource "aws_apigatewayv2_stage" "default" {
  api_id      = aws_apigatewayv2_api.public.id
  name        = "$default"
  auto_deploy = true

  access_log_settings {
    destination_arn = aws_cloudwatch_log_group.public_api_access.arn
    format = jsonencode({
      requestId      = "$context.requestId"
      ip             = "$context.identity.sourceIp"
      requestTime    = "$context.requestTime"
      httpMethod     = "$context.httpMethod"
      routeKey       = "$context.routeKey"
      status         = "$context.status"
      protocol       = "$context.protocol"
      responseLength = "$context.responseLength"
    })
  }
}

resource "aws_lambda_permission" "allow_public_api" {
  statement_id  = "AllowExecutionFromAPIGateway"
  action        = "lambda:InvokeFunction"
  function_name = aws_lambda_function.control_plane.function_name
  principal     = "apigateway.amazonaws.com"
  source_arn    = "${aws_apigatewayv2_api.public.execution_arn}/*/*"
}

resource "aws_budgets_budget" "staging_monthly" {
  count = var.create_budget && var.budget_alert_email != "" ? 1 : 0

  name         = "${local.resource_prefix}-monthly-cost"
  budget_type  = "COST"
  limit_amount = format("%.2f", var.monthly_budget_amount)
  limit_unit   = "USD"
  time_unit    = "MONTHLY"

  notification {
    comparison_operator        = "GREATER_THAN"
    threshold                  = 80
    threshold_type             = "PERCENTAGE"
    notification_type          = "ACTUAL"
    subscriber_email_addresses = [var.budget_alert_email]
  }

  notification {
    comparison_operator        = "GREATER_THAN"
    threshold                  = 100
    threshold_type             = "PERCENTAGE"
    notification_type          = "ACTUAL"
    subscriber_email_addresses = [var.budget_alert_email]
  }

  notification {
    comparison_operator        = "GREATER_THAN"
    threshold                  = 100
    threshold_type             = "PERCENTAGE"
    notification_type          = "FORECASTED"
    subscriber_email_addresses = [var.budget_alert_email]
  }
}
