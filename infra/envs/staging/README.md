# HomeSignal Staging Deployment

This is the first deployable staging slice for HomeSignal. It creates the
control-plane smoke runtime, the telemetry-ingest runtime skeleton, and the
first AWS IoT Core routing resources:

- Lambda custom runtime for the Go `control-plane` service.
- HTTP API routes for `GET /healthz`, `GET /readyz`, and `GET /version`.
- ECR repository and one ECS/Fargate `telemetry-ingest` task.
- Temporary direct staging HTTP access to telemetry-ingest on port `8080` for
  smoke tests until Agent HTTPS mTLS is wired.
- AWS IoT device policy, Thing type, lifecycle topic rule, and lifecycle log
  group.
- CloudWatch log groups with short staging retention.
- Runtime IAM roles scoped to Lambda logging, ECS execution, and IoT lifecycle
  logging.
- Optional AWS Budget when `HOMESIGNAL_CREATE_STAGING_BUDGET=1` is set and
  payer-account Budgets are enabled for member-account creation.

The first deploy is intentionally pinned to `us-east-1` to stay colocated with
the staging Neon region that was validated from the voice extraction API repo.

## Run

```bash
scripts/deploy.sh staging
scripts/smoke.sh staging
```

Set these environment variables before deploying when applicable:

- `AWS_PROFILE`: named staging deploy principal.
- `HOMESIGNAL_BUDGET_ALERT_EMAIL`: email recipient for the staging budget
  guardrail task.
- `HOMESIGNAL_CREATE_STAGING_BUDGET=1`: create the budget from this workspace
  only after payer-account Budgets are enabled for member accounts.
- `HOMESIGNAL_BUDGET_GUARDRAIL_CONFIRMED=1`: use only when the budget guardrail
  already exists outside this Terraform state.
- `HOMESIGNAL_STAGING_BUDGET_AMOUNT`: monthly budget amount, default `25`.
- `HOMESIGNAL_OWNER_TAG`: owner tag value, default `platform`.
- `HOMESIGNAL_TELEMETRY_IMAGE_TAG`: optional telemetry-ingest image tag,
  defaults to the deploy version.

## State

This first slice uses the Terraform or OpenTofu local state default. Before CI/CD
or production, add a remote state backend with state locking.
