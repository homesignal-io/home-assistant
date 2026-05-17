output "aws_account_id" {
  description = "AWS account ID resolved by the staging deployment."
  value       = data.aws_caller_identity.current.account_id
}

output "aws_region" {
  description = "AWS region used by the staging deployment."
  value       = var.aws_region
}

output "control_plane_log_group" {
  description = "CloudWatch log group for the staging control-plane Lambda."
  value       = aws_cloudwatch_log_group.control_plane.name
}

output "resource_prefix" {
  description = "Resource name prefix used by staging infrastructure."
  value       = local.resource_prefix
}

output "staging_base_url" {
  description = "Base URL for the staging HTTP API."
  value       = aws_apigatewayv2_api.public.api_endpoint
}

output "telemetry_ingest_ecr_repository_url" {
  description = "ECR repository URL for telemetry-ingest images."
  value       = aws_ecr_repository.telemetry_ingest.repository_url
}

output "telemetry_ingest_cluster_name" {
  description = "ECS cluster that runs telemetry-ingest."
  value       = aws_ecs_cluster.runtime.name
}

output "telemetry_ingest_service_name" {
  description = "ECS service name for telemetry-ingest."
  value       = aws_ecs_service.telemetry_ingest.name
}

output "telemetry_ingest_log_group" {
  description = "CloudWatch log group for telemetry-ingest."
  value       = aws_cloudwatch_log_group.telemetry_ingest.name
}

output "iot_device_policy_name" {
  description = "AWS IoT device policy name for claimed runtime devices."
  value       = aws_iot_policy.device.name
}

output "iot_lifecycle_rule_name" {
  description = "AWS IoT lifecycle topic rule name."
  value       = aws_iot_topic_rule.lifecycle.name
}

output "iot_lifecycle_log_group" {
  description = "CloudWatch log group for AWS IoT lifecycle events."
  value       = aws_cloudwatch_log_group.iot_lifecycle.name
}
