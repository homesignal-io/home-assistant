# HomeSignal

HomeSignal is the Home Assistant add-on and platform codebase for the HomeSignal project.

## Add To Home Assistant

[![Open your Home Assistant instance and add this add-on repository.](https://my.home-assistant.io/badges/supervisor_add_addon_repository.svg)](https://my.home-assistant.io/redirect/supervisor_add_addon_repository/?repository_url=https%3A%2F%2Fgithub.com%2Fhomesignal-io%2Fhomesignal-home-assistant)

This button adds the HomeSignal add-on repository to Home Assistant. Home Assistant must be able to access the repository URL, so the repository needs to be public or otherwise reachable by the Home Assistant Supervisor.

The first implemented package is the Home Assistant add-on:

```text
homesignal
```

See `homesignal/README.md` for install and development notes.

## First Staging Deploy

The first deployable backend slices are:

- Go control-plane skeleton in `backend/`, deployed through Lambda/API Gateway.
- Go telemetry-ingest skeleton in `telemetry-ingest/`, deployed as one small
  ECS/Fargate task with in-memory dedupe for unchanged telemetry.

The control plane exposes only operational endpoints:

```text
GET /healthz
GET /readyz
GET /version
```

The script entry points are:

```bash
scripts/test.sh
scripts/build.sh
scripts/deploy.sh staging
scripts/smoke.sh staging
scripts/logs.sh staging
scripts/logs.sh staging telemetry-ingest
```

The staging deploy is pinned to AWS `us-east-1`. Before running
`scripts/deploy.sh staging`, use a named AWS deploy principal and provide either
`HOMESIGNAL_BUDGET_ALERT_EMAIL` so Terraform can create the staging budget
guardrail, or `HOMESIGNAL_BUDGET_GUARDRAIL_CONFIRMED=1` if the guardrail already
exists.
