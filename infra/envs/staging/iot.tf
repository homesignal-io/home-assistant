data "aws_partition" "current" {}

resource "aws_cloudwatch_log_group" "iot_lifecycle" {
  name              = "/homesignal/staging/iot/lifecycle"
  retention_in_days = 7

  tags = {
    Boundary = "device"
  }
}

resource "aws_iam_role" "iot_lifecycle_rule_logs" {
  name = "${local.resource_prefix}-iot-lifecycle-logs-role"

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Effect = "Allow"
        Principal = {
          Service = "iot.amazonaws.com"
        }
        Action = "sts:AssumeRole"
      }
    ]
  })

  tags = {
    Boundary = "device"
  }
}

resource "aws_iam_role_policy" "iot_lifecycle_rule_logs" {
  name = "${local.resource_prefix}-iot-lifecycle-logs"
  role = aws_iam_role.iot_lifecycle_rule_logs.id

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Effect = "Allow"
        Action = [
          "logs:CreateLogStream",
          "logs:DescribeLogStreams",
          "logs:PutLogEvents"
        ]
        Resource = "${aws_cloudwatch_log_group.iot_lifecycle.arn}:*"
      }
    ]
  })
}

resource "aws_iot_thing_type" "homesignal_device" {
  name = "${local.resource_prefix}-device"
}

resource "aws_iot_policy" "device" {
  name = "${local.resource_prefix}-device-policy"

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Effect = "Allow"
        Action = [
          "iot:Connect"
        ]
        Resource = "arn:${data.aws_partition.current.partition}:iot:${var.aws_region}:${data.aws_caller_identity.current.account_id}:client/$${iot:Connection.Thing.ThingName}"
        Condition = {
          Bool = {
            "iot:Connection.Thing.IsAttached" = "true"
          }
        }
      },
      {
        Effect = "Allow"
        Action = [
          "iot:Subscribe"
        ]
        Resource = [
          "arn:${data.aws_partition.current.partition}:iot:${var.aws_region}:${data.aws_caller_identity.current.account_id}:topicfilter/homesignal/devices/$${iot:Connection.Thing.ThingName}/+",
          "arn:${data.aws_partition.current.partition}:iot:${var.aws_region}:${data.aws_caller_identity.current.account_id}:topicfilter/$aws/things/$${iot:Connection.Thing.ThingName}/shadow/name/homesignal_edge/*"
        ]
      },
      {
        Effect = "Allow"
        Action = [
          "iot:Receive"
        ]
        Resource = [
          "arn:${data.aws_partition.current.partition}:iot:${var.aws_region}:${data.aws_caller_identity.current.account_id}:topic/homesignal/devices/$${iot:Connection.Thing.ThingName}/+",
          "arn:${data.aws_partition.current.partition}:iot:${var.aws_region}:${data.aws_caller_identity.current.account_id}:topic/$aws/things/$${iot:Connection.Thing.ThingName}/shadow/name/homesignal_edge/*"
        ]
      },
      {
        Effect = "Allow"
        Action = [
          "iot:Publish"
        ]
        Resource = [
          "arn:${data.aws_partition.current.partition}:iot:${var.aws_region}:${data.aws_caller_identity.current.account_id}:topic/$aws/things/$${iot:Connection.Thing.ThingName}/shadow/name/homesignal_edge/*"
        ]
      }
    ]
  })
}

resource "aws_iot_topic_rule" "lifecycle" {
  name        = "homesignal_staging_lifecycle_rule"
  description = "Route AWS IoT lifecycle presence events into staging logs until Telemetry Ingest presence handling is wired."
  enabled     = true
  sql         = "SELECT *, clientid() AS client_id, principal() AS principal_id, topic(4) AS lifecycle_event FROM '$aws/events/presence/+/+'"
  sql_version = "2016-03-23"

  cloudwatch_logs {
    log_group_name = aws_cloudwatch_log_group.iot_lifecycle.name
    role_arn       = aws_iam_role.iot_lifecycle_rule_logs.arn
  }
}
