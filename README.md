# HomeSignal

HomeSignal is the Home Assistant app and platform codebase for the HomeSignal project.

## Add To Home Assistant

[![Open your Home Assistant instance and add this app repository.](https://my.home-assistant.io/badges/supervisor_add_addon_repository.svg)](https://my.home-assistant.io/redirect/supervisor_add_addon_repository/?repository_url=https%3A%2F%2Fgithub.com%2Fhomesignal-io%2Fhomesignal-home-assistant-app)

This button adds the HomeSignal app repository to Home Assistant. Home Assistant must be able to access the repository URL, so the repository needs to be public or otherwise reachable by the Home Assistant Supervisor.

The first implemented package is the Home Assistant app:

```text
homesignal
```

See `homesignal/README.md` for install and development notes.

## First Staging Deploy

The first deployable backend slices are:

- Go control-plane skeleton in `backend/`, deployed through Lambda/API Gateway.
- Go telemetry-ingest skeleton in `telemetry-ingest/`, deployed as one small
  ECS/Fargate task with in-memory dedupe for unchanged telemetry.
- React portal skeleton in `portal/`, built from the public read-model contract
  fixtures and ready to swap from fixtures to `/api/v1` routes.

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
cd portal && npm ci && npm run dev
scripts/deploy.sh staging
scripts/smoke.sh staging
scripts/logs.sh staging
scripts/logs.sh staging telemetry-ingest
```

The portal uses contract fixtures unless `VITE_HOMESIGNAL_API_BASE_URL` is set
to the public API `/api/v1` base. For authenticated staging reads, also set
`VITE_HOMESIGNAL_API_TOKEN` to a short-lived Cognito access token. The staging
smoke script accepts the same kind of token through
`HOMESIGNAL_STAGING_API_BEARER_TOKEN`.

The staging deploy is pinned to AWS `us-east-1`. Before running
`scripts/deploy.sh staging`, use a named AWS deploy principal and provide either
`HOMESIGNAL_BUDGET_ALERT_EMAIL` so Terraform can create the staging budget
guardrail, or `HOMESIGNAL_BUDGET_GUARDRAIL_CONFIRMED=1` if the guardrail already
exists.
