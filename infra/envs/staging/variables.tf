variable "aws_region" {
  description = "AWS region for the staging deployment."
  type        = string
  default     = "us-east-1"

  validation {
    condition     = var.aws_region == "us-east-1"
    error_message = "The first staging deployment must run in us-east-1."
  }
}

variable "budget_alert_email" {
  description = "Email recipient for staging budget alerts. Used only when create_budget is true."
  type        = string
  default     = ""
}

variable "create_budget" {
  description = "Whether this workspace should create the AWS Budget. Keep false for Organizations member accounts unless payer-account budget creation is enabled."
  type        = bool
  default     = false
}

variable "lambda_package_path" {
  description = "Path to the packaged Lambda bootstrap zip."
  type        = string
  default     = "../../../backend/dist/control-plane/bootstrap.zip"
}

variable "monthly_budget_amount" {
  description = "Monthly staging budget amount in USD."
  type        = number
  default     = 25

  validation {
    condition     = var.monthly_budget_amount > 0
    error_message = "monthly_budget_amount must be greater than zero."
  }
}

variable "owner_tag" {
  description = "Owner tag value applied to taggable staging resources."
  type        = string
  default     = "platform"
}

variable "artifact_version" {
  description = "Version string exposed by the control-plane /version endpoint."
  type        = string
  default     = "dev"
}

variable "telemetry_ingest_image" {
  description = "Fully qualified telemetry-ingest container image URI."
  type        = string
}

variable "telemetry_ingest_cpu" {
  description = "Fargate CPU units for the staging telemetry-ingest task."
  type        = number
  default     = 256
}

variable "telemetry_ingest_memory" {
  description = "Fargate memory MiB for the staging telemetry-ingest task."
  type        = number
  default     = 512
}

variable "telemetry_ingest_desired_count" {
  description = "Desired staging telemetry-ingest task count."
  type        = number
  default     = 1
}

variable "telemetry_ingest_ingress_cidr_blocks" {
  description = "Temporary staging CIDR blocks allowed to call the telemetry-ingest skeleton directly on port 8080 until Agent HTTPS mTLS is wired."
  type        = list(string)
  default     = ["0.0.0.0/0"]
}
