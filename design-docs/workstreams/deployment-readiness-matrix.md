# V0 Deployment Readiness Matrix

This is the architecture checklist for non-functional launch readiness. It is
not an implementation plan and does not require the resources to exist before
feature work starts. It defines the target shape future implementation must
satisfy so CI/CD, infra, secrets, testing, observability, and operations do not
invent local policy.

Canonical owners remain:

- deployment policy: `deployment.md`
- secrets/config: `secrets-and-config.md`
- observability: `observability.md`
- test fixtures/staging: `test-environments-and-fixtures.md`
- migrations/rollback: `migration-strategy.md`
- operator prerequisites: `operator-prerequisites.md`
- service ownership: `../service-map.md`

## Readiness Grade Target

The architecture target for every launch-critical non-functional area is
`B+` or better before implementation begins, and `A-` or better before
production launch.

Grade meanings:

| Grade | Meaning |
| --- | --- |
| A | The area has a clear policy, concrete inventory, acceptance gates, and named operational owner/runbook. Implementation should be mostly mechanical. |
| A- | The area is architecturally complete except for external account/provider values or minor implementation sequencing. |
| B+ | The policy and default path are clear; a future implementer may still need to enumerate exact resource names or script arguments. |
| B | The direction is acceptable, but missing inventory/gates could cause implementation drift. |
| Below B | Not acceptable for launch-critical non-functional architecture; close the gap before implementation depends on it. |

## Area Grades After This Matrix

These are architecture-readiness grades, not implementation-status grades.

| Area | Grade | Why |
| --- | --- | --- |
| Environment model | A | `staging` and `production` are the only launch cloud environments, with production-shaped staging. |
| Deployment topology | A- | V0 uses one control-plane monolith plus separately deployable Telemetry Ingest; exact compute is defaulted below. |
| Runtime compute target | A | API Gateway plus Lambda is selected for the first control-plane deploy; Telemetry Ingest defaults to a small long-lived/Fargate-style runtime so hot dedupe, coalescing, and DB write batching are preserved. |
| CI/CD policy | A- | Script-first, CodeBuild-preferred, thin GitHub Actions, staging automation, production approval. |
| IaC policy | A- | OpenTofu/Terraform-style IaC with env directories and no undocumented production resources. |
| Secrets/config | A | Standard paths, env vars, injection posture, rotation defaults, and service-level inventory are defined. |
| Database/migrations | A- | Goose-style migrations, additive-first policy, pre/post checks, and rollback/forward-fix posture. |
| Test strategy | A- | Local tests, contract fixtures, simulator, staging canary, live test marking, and smoke gates are named. |
| Observability | A- | CloudWatch-first logs/metrics/alarms, health/readiness, coarse dimensions, and runbook hooks are named. |
| Staging strategy | A- | Real AWS IoT, real Agent HTTPS mTLS, long-lived canary, and ephemeral lifecycle fixtures are required. |
| Production gate | A- | Staging smoke, explicit approval, migration checks, deploy record, and rollback/forward-fix note are required. |
| Runtime resource inventory | A- | Required resource classes are enumerated; exact provider IDs/domain names remain external inputs. |
| Email/notifications | A- | Resend-backed provider adapter, outbox, delivery attempts, sandbox/staging verification, and secrets are defined. |
| Operational runbooks | A- | Required runbooks are named and tied to launch gates. |
| Security boundaries | A- | Public/internal/agent boundaries, mTLS, IAM/service identity, no generic uploads, and no public internal routes are settled. |

Overall non-functional architecture readiness after this matrix: `A-`.

## First Deploy Implementation Status

The first deployable slice now has source-controlled implementation entry
points:

- Control-plane skeleton: `backend/cmd/control-plane`
- Staging IaC: `infra/envs/staging`
- Script surface: `scripts/test.sh`, `scripts/build.sh`, `scripts/deploy.sh`,
  `scripts/smoke.sh`, `scripts/logs.sh`, `scripts/migrate.sh`,
  `scripts/rotate-db-credentials.sh`, and
  `scripts/cleanup-staging-fixtures.sh`
- Operator-safe staging database secret helper:
  `scripts/set-staging-database-url.sh`
- DB-backed staging smoke fixture helper:
  `scripts/staging-fixtures.sh`
- Database migration surface: `backend/migrations`, `backend/cmd/migrate`, and
  AWS secret metadata for `/homesignal/staging/platform/database_url`

The first cloud deploy remains gated by local toolchain and operator/provider
inputs: Go, AWS CLI, OpenTofu or Terraform, a named AWS deploy principal, and a
confirmed staging budget guardrail or alert email. Applying database migrations
also requires a HomeSignal Neon database URL stored in the staging database
secret or exported as `HOMESIGNAL_DATABASE_URL`.

An owned domain is not a gate for the Stage 0 skeleton smoke deploy. It is a
gate for Stage 1 staging flows that depend on browser origin trust, including
public pairing pages, localStorage/postMessage bridge behavior, HA App staging
environment profiles, claim UX, and email links.

## Environment Defaults

Launch environments:

- `staging`
- `production`

Environment posture:

- The first cloud environment is `staging`.
- Do not create a cloud `dev` environment for launch. Local development and
  disposable tests use local config, fakes, and explicit staging fixtures.
- If a personal or preview cloud environment is needed later, treat it as a
  temporary sandbox with an owner, TTL, cost limit, and cleanup path. It is not
  part of the launch environment model.

Staging phase gates:

| Phase | Endpoint posture | Allowed work |
| --- | --- | --- |
| Stage 0 skeleton smoke | Generated AWS endpoint allowed | Build/deploy/smoke scripts, IaC, logs, `/healthz`, `/readyz`, `/version`. |
| Stage 1 domain-backed staging | Stable owned HTTPS domain required | Public pairing page, browser bridge, HA App staging profile, claim UX, email links, canary pairing, and human-visible staging workflows. |

Default Stage 1 DNS shape:

- Start with `staging.<owned-root-domain>` for the first public HomeSignal web
  origin.
- Split `app.staging.<root>` and `api.staging.<root>` only when routing,
  cookie, or certificate boundaries need separate hostnames.
- Generated AWS endpoints must not be stored in durable HA App environment
  config.

Account model:

- Production should run in a production AWS account before customer launch.
- Staging should run in a separate staging AWS account when practical.
- A single AWS account with environment-scoped names is acceptable only as a
  temporary pre-production shortcut; it is not the target production boundary.

Region model:

- Use one primary AWS region per environment.
- Selected v0 region is `us-east-1`.
- The selection aligns HomeSignal AWS resources with the currently verified
  Neon database region from the existing voice-extraction staging database
  connection, whose Neon host suffix is `us-east-1.aws.neon.tech`.
- Treat "together" as same AWS region. Do not target a specific availability
  zone or physical data center across Neon and the HomeSignal AWS account.
- Keep future Neon/Postgres projects in the same AWS region when practical.

Naming:

- Canonical environment names are `staging` and `production`.
- Use lowercase kebab-case for AWS resource names where the service allows it.
- Use this default shape:
  `homesignal-{environment}-{boundary}-{purpose}`.
- Use full environment names in durable resources. Avoid `prod` and `stg`
  except where a hard length constraint requires an abbreviation.
- Durable boundary slugs:
  `control-plane`, `telemetry-ingest`, `notification`, `agent-api`,
  `public-api`, `artifact-broker`, `backup`, `diagnostics`, `release`,
  `platform`, `ops`, and `ci`.
- The first deploy uses only the `control-plane`, `public-api`, `platform`,
  `ops`, and `ci` slugs as needed.
- Ephemeral staging resources should use
  `hs-stg-<purpose>-<yyyymmdd>-<shortid>` and carry cleanup metadata.

Canonical examples:

| Resource class | Naming pattern |
| --- | --- |
| Control-plane runtime | `homesignal-{environment}-control-plane-runtime` |
| Public API Gateway/API | `homesignal-{environment}-public-api` |
| Agent HTTPS API Gateway/API | `homesignal-{environment}-agent-api` |
| Telemetry Ingest runtime | `homesignal-{environment}-telemetry-ingest-runtime` |
| Notification worker/runtime | `homesignal-{environment}-notification-worker` |
| Artifact/product bucket | `homesignal-{environment}-{boundary}-{purpose}-{account_id}` |
| IaC state bucket | `homesignal-{environment}-platform-iac-state-{account_id}` or shared infra account equivalent |
| Artifact/build bucket or registry | `homesignal-{environment}-ci-artifacts-{account_id}` |
| CloudWatch log group | `/homesignal/{environment}/{boundary}` |
| IAM runtime role | `homesignal-{environment}-{boundary}-runtime-role` |
| IAM deploy role | `homesignal-{environment}-ci-deploy-role` |
| CodeBuild test/build project | `homesignal-{environment}-ci-test-build` |
| CodeBuild deploy project | `homesignal-{environment}-ci-deploy` |
| Secrets path | `/homesignal/{environment}/{service}/{secret_name}` |
| Config path | `/homesignal/{environment}/{service}/config/{config_name}` |
| IoT policy | `homesignal-{environment}-device-policy` |
| IoT Thing name / HomeSignal device ID | durable `device_id` generated by HomeSignal; do not include environment in the device ID |
| IoT rule | `homesignal-{environment}-{purpose}-rule` |
| Alarm | `homesignal-{environment}-{boundary}-{condition}` |
| Dashboard | `homesignal-{environment}-ops` |

S3 bucket names must include the AWS account ID or another globally unique
suffix. IAM role names, Lambda function names, and CodeBuild project names have
length limits; if a name would exceed a provider limit, shorten the boundary or
purpose while preserving `homesignal`, environment, and owner meaning.

Tagging:

- All taggable AWS resources should include:
  `Project=homesignal`, `Environment=<staging|production>`,
  `Boundary=<boundary>`, `ManagedBy=iac`, and `Owner=platform`.
- Add account-specific cost allocation tags if the operator provides them.
- Ephemeral resources must also include cleanup metadata such as `ExpiresAt`,
  `Purpose`, and `CreatedBy`.
- Do not encode customer, site, or device identifiers in AWS tags.

## Default Runtime Target

Default v0 compute:

- Control-plane monolith: Go service deployed behind API Gateway. The first deploy
  may use Lambda while the control-plane surface is still skeletal.
- Telemetry Ingest: separately deployable Go service behind the Agent HTTPS
  receiver and AWS IoT lifecycle integration. Its default cloud runtime is a
  small long-lived service, such as ECS/Fargate, because hot dedupe,
  coalescing, and batched Postgres writes are architecture requirements.
- Async/background work: start as process-local or scheduled workers only when
  durability is not required; use database-backed outbox rows for notification
  delivery before introducing a queue.

Preferred AWS substrate for the first launch architecture:

- Use API Gateway for public product/API routes and Agent HTTPS mTLS routes.
- Use Lambda or another API Gateway-native integration for the first
  control-plane deploy while it hosts only low-rate API/smoke behavior.
- Use ECS/Fargate or an equivalent long-lived runtime for Telemetry Ingest unless
  a later architecture decision adds a shared dedupe/batching layer that gives a
  serverless adapter the same write-suppression behavior.
- Do not implement Telemetry Ingest as per-message Lambda direct-to-Postgres.
  A function adapter can receive requests only if it delegates to the ingest
  runtime or to shared hot-state infrastructure that preserves `received
  messages vs persisted writes` suppression.
- Move control-plane work to ECS/Fargate when a concrete need appears, such as
  long-running workers, persistent connections, heavy local dependencies, or
  Lambda/API Gateway limits.

Rules:

- `/internal/*` routes must not be exposed through the public API Gateway.
- If an internal route is needed before services are physically split, keep it
  in-process or make it reachable only through IAM-authenticated internal
  integration.
- Do not introduce NAT Gateway/private-egress complexity solely for v0 polish.

## Required Resource Inventory

These resources must be represented in IaC before production launch.

### Platform And CI/CD

| Resource | Purpose | Environment |
| --- | --- | --- |
| OpenTofu/Terraform state bucket and lock table | IaC state and locking | staging, production or shared infra account |
| Account-level AWS Budget or cost alarm | Early spend guardrail and notification | staging account before first deploy; production before production deploy |
| CodeBuild project: test/build | Repo test and artifact build | staging first, production-capable |
| CodeBuild project: deploy staging | Script-driven staging deploy | staging |
| CodeBuild project: deploy production | Script-driven production deploy after approval | production |
| Artifact bucket or registry | Immutable service build artifacts | staging, production |
| GHCR or equivalent app image registry | Home Assistant app images by stable/candidate/staging track | staging, production |
| Public Home Assistant app distribution repos | Thin generated install channels for stable, candidate, and staging | staging, production |
| IAM roles for CodeBuild/deploy | Least-privilege build/deploy authority | staging, production |
| Optional thin GitHub Actions workflow | Trigger/report CodeBuild status | repo-level |

### Public Edge And Auth

| Resource | Purpose | Environment |
| --- | --- | --- |
| API Gateway public API | Portal/client `/api/v1` routes | staging, production |
| API Gateway Agent HTTPS API/custom domain | mTLS `/agent/*` routes | staging, production |
| mTLS truststore object | Client certificate CA trust for Agent HTTPS edge | staging, production |
| ACM certificates | Public API and portal domains | staging, production |
| DNS/hosted zone records | API, agent, and portal names | staging, production |
| Cognito or equivalent auth pool/client | Human auth and JWT issuer | staging, production |

### Runtime Services

| Resource | Purpose | Environment |
| --- | --- | --- |
| Control-plane runtime | API Facade and logical domain services | staging, production |
| Telemetry Ingest ECS/Fargate service, task definition, and task role | Agent telemetry/events, hot dedupe/coalescing, IoT lifecycle processing, and batched persistence | staging, production |
| Notification/outbox worker | Transactional email attempts | staging, production |
| Optional scheduled cleanup worker | Expired sessions, debug TTL, artifact cleanup | staging, production |

### Data Stores

| Resource | Purpose | Environment |
| --- | --- | --- |
| Neon/Postgres database | Canonical product state | staging, production |
| Object storage bucket | Backup bytes, diagnostics/debug bundles, approved artifacts | staging, production |
| Optional archive bucket/prefix | Cold telemetry archive | staging, production |
| Secrets Manager/SSM paths | Runtime secrets and config | staging, production |

### AWS IoT Core

| Resource | Purpose | Environment |
| --- | --- | --- |
| IoT policies | Claimed device MQTT/shadow permissions | staging, production |
| Thing type or naming policy | Device registry convention | staging, production |
| CSR signing/provisioning IAM role | Certificate creation and binding | staging, production |
| Named shadow convention | `homesignal_edge` desired/reported state | staging, production |
| MQTT command topic convention | Cloud-to-device command delivery | staging, production |
| MQTT notification topic convention | Fire-and-forget convergence hints | staging, production |
| IoT lifecycle rule/integration | Presence/connect_failed routing | staging, production |

### Observability

| Resource | Purpose | Environment |
| --- | --- | --- |
| CloudWatch log groups | Structured logs with retention | staging, production |
| CloudWatch metrics/alarms | Health, errors, auth rejects, ingest failures | staging, production |
| Dashboard or alarm grouping | Operator view of launch-critical health | staging, production |
| Account budget/cost notification | Spend guardrail outside service health | staging, production |
| Audit event storage | Authority history in Postgres | staging, production |

### Email

| Resource | Purpose | Environment |
| --- | --- | --- |
| Resend account/API key | V0 transactional email provider | staging, production |
| Verified sending domain/address | Product alert email delivery | staging, production |
| Notification templates | HTML/text rendering inputs | app artifact or DB |
| Delivery attempt/outbox tables | Provider result metadata | staging, production |

## Required Secret And Config Inventory

Use these names as classes, not literal final variable names when a service
needs a narrower prefix. Store secrets at
`/homesignal/{environment}/{service}/{secret_name}` and non-secret config at
`/homesignal/{environment}/{service}/config/{config_name}`.

### Shared Runtime Config

Required for every service runtime:

- `HOMESIGNAL_ENV`
- `HOMESIGNAL_SERVICE_NAME`
- `HOMESIGNAL_VERSION`
- `HOMESIGNAL_AWS_REGION`
- `HOMESIGNAL_PUBLIC_API_BASE_URL` where applicable
- `HOMESIGNAL_AGENT_API_BASE_URL` where applicable

### Control Plane

Secrets:

- database connection URL or credentials
- Cognito/JWT verifier secrets only if the chosen auth provider requires them
- Resend API key if notification sending runs inside the monolith

Config:

- JWT issuer/audience
- public API domain
- agent API domain
- object storage bucket names
- IoT endpoint and policy names
- mTLS forwarded certificate metadata header names
- rate-limit defaults

### Telemetry Ingest

Secrets:

- database connection URL or credentials

Config:

- accepted runtime envelope versions
- schema catalog version
- ingest size limits
- hot dedupe/cache TTL
- input and DB write batch sizes
- quarantine/drop policy defaults
- object archive bucket/prefix when cold archive is enabled

### Notification Service

Secrets:

- `RESEND_API_KEY`

Config:

- `EMAIL_PROVIDER=resend`
- sender email/from name
- sandbox/test mode flag for staging
- retry/backoff defaults
- suppression/cooldown defaults

### IaC And CI/CD

Secrets:

- provider tokens only when AWS IAM is not enough, such as Neon provisioning API
  credentials if automated database creation is used.

Config:

- AWS account IDs
- AWS region
- hosted zone/domain names
- CodeBuild project names
- artifact bucket names

## CI/CD Stages And Gates

Repo scripts are the stable interface. CI systems call scripts rather than
owning bespoke deployment behavior.

Required script names:

- `scripts/test.sh`
- `scripts/build.sh`
- `scripts/migrate.sh`
- `scripts/deploy.sh`
- `scripts/smoke.sh`
- `scripts/logs.sh`
- `scripts/rotate-db-credentials.sh`
- `scripts/cleanup-staging-fixtures.sh`

Minimum stages:

| Stage | Required Gates | Notes |
| --- | --- | --- |
| Local/default test | unit tests, contract fixture tests, static checks that exist | No live AWS required. |
| Build | immutable service artifact, version metadata | Artifact must include commit/version. |
| Migration precheck | pending migration list, additive/destructive classification | Production requires explicit precheck output. |
| Staging deploy | tests pass, migration precheck, deploy, migration postcheck | May run automatically after tests pass. |
| Staging smoke | health, readiness, one core workflow, observable output | Must pass before production approval. |
| Production approval | human/operator approval, staging smoke reference, rollback/forward-fix note | Required before production deploy. |
| Production deploy | migration precheck, deploy, migration postcheck, smoke | Deploy record must be retained. |

Production deploy record must include:

- service/version/commit
- environment
- migration IDs applied or confirmed absent
- staging smoke result
- production smoke result
- operator approval
- rollback or forward-fix expectation

## Test Harness Gates

Local default tests:

- pure unit tests for policy/state machines
- HTTP handler tests with fake services
- repository tests with disposable Postgres when available
- contract fixture tests for runtime envelopes, enrollment, command
  ACK/result, artifact upload, alert candidates, and notification attempts
- fake adapters for AWS IoT, object storage, email, auth, and database where
  domain logic does not require live infrastructure

Simulator tests:

- Agent HTTPS telemetry/event envelope acceptance
- publish-policy behavior
- command ACK/result
- shadow convergence
- backup trigger/result shape
- alert candidate emission

Staging live tests:

- claim a long-lived HA/app canary through staging AWS IoT
- publish Agent HTTPS telemetry through mTLS
- process AWS IoT lifecycle presence
- deliver one command and record ACK/result
- apply/read `homesignal_edge` shadow desired/reported state
- send a notification through staging email sandbox/provider mode
- run cleanup for ephemeral lifecycle fixtures

Production smoke tests:

- `/healthz`
- `/readyz`
- version endpoint or metadata
- authenticated portal/API read
- Agent HTTPS mTLS auth reject path with invalid client identity
- one non-mutating database read path
- notification worker readiness without sending a real customer alert unless
  the smoke uses an approved test recipient

## Smoke Check Inventory

Required smoke checks by milestone:

| Check | Staging | Production |
| --- | --- | --- |
| Control-plane liveness/readiness | required | required |
| Telemetry Ingest liveness/readiness | required | required |
| Database connectivity | required | required |
| Auth/JWT verification | required | required |
| Agent mTLS valid canary cert | required | optional after canary exists |
| Agent mTLS invalid cert reject | required | required |
| Pairing-session create/poll using non-customer fixture | required after enrollment exists | optional, non-mutating only |
| AWS IoT lifecycle presence | required after IoT wiring exists | canary-only |
| Command ACK/result | required after commands exist | canary-only |
| Shadow convergence | required after Edge State exists | canary-only |
| Backup status/trigger | required after Backup exists | canary-only |
| Email provider sandbox/test send | required after Notification exists | test recipient only |

## MVP Alarm Inventory

V0 alarms use coarse dimensions only. Do not use `device_id`, `site_id`, or
`account_id` as hot metric dimensions.

Account-level spend guardrails are required in addition to service alarms:

- staging monthly actual-spend budget notification before first cloud deploy
- staging forecasted-spend notification when supported by the selected alarm
  path
- production monthly actual and forecasted spend notifications before first
  production deploy
- alert target is an operator email address or SNS topic, never a customer
  notification channel

Required alarms:

- control-plane readiness failure
- control-plane elevated 5xx/error rate
- telemetry-ingest readiness failure
- telemetry-ingest elevated reject/quarantine/drop rate
- Agent HTTPS mTLS/cert authorization failures above baseline
- database connectivity/dependency failure
- command ACK/result failure spike
- artifact upload negotiation/completion failure spike
- notification provider failure spike
- notification outbox stuck/oldest pending age
- staging canary no telemetry within expected window
- staging canary command smoke failure
- object storage write failure for artifact/debug paths

Platform Health rule evaluation remains future. These alarms and stored facts
are the v0 substrate.

## Required Runbooks

Runbooks may be short and script-first, but they must exist before production
launch.

| Runbook | Required Before |
| --- | --- |
| Deploy staging | first staging deploy |
| Create or verify AWS budget/cost guardrail | first staging deploy |
| Deploy production | first production deploy |
| Roll back or forward-fix service deploy | first production deploy |
| Migration precheck/postcheck | first production migration |
| Rotate Neon/Postgres credentials | production launch |
| Rotate Resend API key | notification launch |
| Rotate Agent HTTPS truststore/CA | mTLS launch |
| Revoke/replace device credential | real device claim launch |
| Pair and repair staging canary | staging canary launch |
| Tail service logs by environment/service/correlation ID | staging launch |
| Investigate Agent HTTPS auth rejection | mTLS launch |
| Investigate telemetry reject/quarantine spike | telemetry launch |
| Investigate stuck notification outbox | notification launch |
| Clean up ephemeral staging fixtures | lifecycle test launch |

## External Inputs Still Needed

These are not architecture blockers. They are provider/account values that must
be filled during implementation or operator setup. The canonical task list is
`operator-prerequisites.md`.

- AWS account IDs
- account security baseline confirmation and deploy principal/profile
- budget/cost alert recipient and initial monthly threshold per AWS account
- optional cost allocation tag values if required by the AWS account
- explicit region override only if an operator rejects the selected `us-east-1`
  default before IaC is created
- hosted zone and final domain names before Stage 1 domain-backed staging
- GitHub organization/repository and image registry access for generated Home
  Assistant app distribution repos
- Neon organization/project/database names
- Resend account, verified sender domain, and test-recipient policy
- production operator approval mechanism or ticketing location

## Acceptance Criteria

The non-functional architecture is A-minus ready when:

- every launch-critical area is represented in this matrix
- no area below `B+` remains without an explicit external-input note
- future implementation can create infra/scripts without asking new broad
  architecture questions
- production launch requires staging smoke, approval, migration checks, and
  rollback/forward-fix expectations
- all secrets/config/resource/test/runbook inventories are specific enough to
  become implementation tasks
