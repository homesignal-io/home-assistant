# HomeSignal Implementation Plan

This is the execution map for turning the current architecture docs into code.
It is not a new architecture source of truth. If this plan conflicts with a
canonical architecture doc, update this plan to match the architecture rather
than silently choosing a local shortcut.

Before reopening any broad architecture question, read
`architectural-decision-log.md`, then the canonical docs named there.

## Purpose

This plan is meant to let Codex pick up one small slice, implement it, test it,
and stop without wandering. Each slice should have:

- a narrow owner boundary
- docs to read first
- explicit dependencies
- test fixtures or local tests
- acceptance criteria
- clear things not to do

The target product is the v0 architecture currently documented:

- Home Assistant app exists locally and owns local identity, private key, and
  bounded local behavior.
- Control-plane API and logical domain services run as one v0 monolith.
- Telemetry Ingest is separately deployable in v0.
- Routine telemetry/events use the mTLS Agent HTTPS API.
- AWS IoT Core owns command notices, notifications, lifecycle presence, device
  certificate auth, Thing registry, and named shadows.
- Postgres is product truth.
- Object storage holds large artifacts and backup bytes.
- Alerting plus Notification Service handle v0 customer email alerts.

## How To Use This Plan

For every implementation task:

1. Read the slice's "Read first" docs.
2. Check `architectural-decision-log.md` before asking broad questions.
3. Confirm the slice dependencies already exist or stub them behind an
   interface.
4. Implement only the slice.
5. Add or update local tests and contract fixtures.
6. Run the smallest relevant verification command.
7. Update this plan only if implementation sequencing changes.

Do not use this plan to smuggle architecture changes. If code reveals a real
architecture conflict, report the conflict with file/line citations and stop the
slice until the owning doc is reconciled.

## Recommended Repo Shape

The current repo already has the Home Assistant app under `homesignal/` and a
React design mock under `design-mock/`. The cloud implementation does not exist
yet.

Recommended default unless deliberately changed:

```text
backend/
  cmd/control-plane/
  internal/platform/
  internal/services/
  internal/storage/
  internal/adapters/
  migrations/

telemetry-ingest/
  cmd/telemetry-ingest/
  internal/pipeline/
  internal/adapters/
  internal/storage/
  testdata/fixtures/

portal/
  React/Vite app promoted from design-mock when ready

infra/
  envs/staging/
  envs/production/
  modules/

scripts/
  test.sh
  build.sh
  migrate.sh
  deploy.sh
  smoke.sh
  logs.sh
  rotate-db-credentials.sh
  cleanup-staging-fixtures.sh

testdata/contracts/
  shared contract examples used by more than one package
```

Backend default: Go services, because the app is already Go, the deployment
shape favors small binaries, and the architecture relies on typed service
contracts and adapters. Portal default: React/Vite, evolving from the current
mock. IaC default: OpenTofu unless Terraform materially reduces v0 friction.

If the backend stack changes, update only this section and the setup slices; the
service boundaries and acceptance criteria should remain intact.

## Global Build Rules

These rules apply to every slice.

- Keep API route handlers thin. They authenticate, build request context,
  validate route envelopes, and delegate.
- Domain services own state transitions. Route handlers must not directly
  read/write Postgres.
- Keep logical services extractable. In-process calls should use typed request,
  context, response, and error shapes that can later become HTTP/internal RPC.
- Use adapters for AWS, email providers, object storage, Cognito, and Postgres.
  Domain logic should test against fakes.
- Use typed IDs and typed enums for lifecycle, command, credential, alert, and
  policy states.
- Use explicit reason codes for rejected commands, ingest failures, auth
  failures, and artifact failures.
- Put durable schema changes in migrations. Prefer additive-first migrations.
- Keep fixtures deterministic: fixed timestamps, stable IDs, no secrets.
- Redact private keys, claim invite codes, claim verification tokens, cert PEMs,
  signed URLs, Authorization headers, cookies, and raw secrets from logs.
- Every service entrypoint gets structured logs, `/healthz`, `/readyz`, version
  metadata, request/correlation ID handling, and coarse metrics hooks.
- Do not add a queue on the v0 telemetry ingest path unless the architecture doc
  is changed first.
- Do not use AWS IoT Basic Ingest for v0 routine telemetry/events.
- Do not create generic artifact upload. Every artifact flow needs an owning
  product service or debug/diagnostic flow.
- Do not make internal routes internet-facing.
- Do not add hidden support god-mode or arbitrary local host execution.

## Definition Of Done For A Slice

A slice is done when all applicable items are true:

- The owning docs listed by the slice were read.
- Tests cover the happy path and at least one meaningful failure path.
- Contract fixtures were added or updated for boundary changes.
- Public `/api/v1` routes exist in source-controlled OpenAPI before route
  implementation.
- Any migration has a local verification path.
- Any AWS/email/object-store behavior is behind an adapter with a fake.
- Logs and errors follow redaction rules.
- Sensitive state changes emit audit events or have a documented reason not to.
- The final response lists files changed and verification run.

## Milestone Order

The plan is ordered by dependency, not by feature glamour.

| Milestone | Goal | Unlocks |
| --- | --- | --- |
| M0 | Repo, scripts, backend skeleton, migration path | All backend work |
| M1 | Core data model and repository seams | Auth, sites, devices, commands |
| M2 | Control-plane API shell and authorization foundation | Portal and enrollment routes |
| M3 | Enrollment and Device Registry | Real devices, certificates, IoT identity |
| M4 | AWS IoT provisioning adapter and staging wiring | Real claim path |
| M5 | Agent HTTPS mTLS identity boundary | Telemetry, commands, artifacts |
| M6 | Telemetry Ingest v0 | Device status, health, alert candidates |
| M7 | Edge State Adapter and Publish Policy | Device policy convergence |
| M8 | Command lifecycle and device notification path | Backup, diagnostics, update checks |
| M9 | Backup, Artifact Broker, Diagnostics/debug | Offsite backups, support flows |
| M10 | Release/update orchestration | App version intent/status |
| M11 | Alerting and transactional email | Customer email alert surface |
| M12 | Portal implementation and read models | Usable integrator and internal UI |
| M13 | App runtime expansion | Claimed runtime behavior |
| M14 | Deployment, staging canary, production gates | Launch readiness |

## M0: Repo And Tooling Foundation

Read first:

- `platform-doctrine.md`
- `workstreams/deployment.md`
- `workstreams/deployment-readiness-matrix.md`
- `workstreams/operator-prerequisites.md` for operator-owned deploy inputs
- `workstreams/first-deploy-greenlight.md` when the user has greenlit first deploy work
- `workstreams/migration-strategy.md`
- `workstreams/test-environments-and-fixtures.md`

### M0.1 Backend Workspace Skeleton

Scope:

- Create `backend/` Go module.
- Add `cmd/control-plane`.
- Add `/healthz`, `/readyz`, `/version`.
- Add config loader with `HOMESIGNAL_ENV`.
- Add structured logging with service/environment/version fields.

Tests:

- Unit test config defaults and required env validation.
- HTTP tests for health/readiness/version.

Acceptance:

- `backend` runs locally without AWS.
- No database or Cognito dependency is required for liveness.

Do not:

- Implement domain routes yet.
- Add real AWS clients yet.

### M0.2 Script-First Command Surface

Scope:

- Add repo scripts named by `workstreams/deployment-readiness-matrix.md`:
  test, build, migrate, deploy, smoke, logs, database credential rotation, and
  staging fixture cleanup.
- Scripts may initially print "not implemented" for deploy/logs, but the names
  should be stable.

Tests:

- Script smoke test that validates local test command dispatches.

Acceptance:

- Future Codex work can run one documented command instead of inventing local
  command lines.

### M0.3 Migration Tooling

Status: implemented for the first staging database slice. `backend/migrations`
contains Goose-style SQL, `backend/cmd/migrate` runs plan/status/up, and
`scripts/migrate.sh` is the shared command surface.

Scope:

- Add Goose-style SQL migration runner under `backend/migrations`.
- Add local migration script.
- Add a migration table to Postgres through the tool's normal mechanism.

Tests:

- Local migration test against a disposable Postgres instance or a fake dry-run
  parser if local Postgres is not available yet.

Acceptance:

- Migrations are ordered and scriptable.
- Rollback expectations are documented per migration.

Do not:

- Depend on Neon-specific application behavior.

### M0.4 Contract Fixture Layout

Scope:

- Create service-local fixture conventions.
- Create shared `testdata/contracts/README.md`.
- Use `testdata/contracts/runtime/` as the canonical shared fixture location
  for runtime telemetry/event envelopes.

Tests:

- None beyond repository checks.

Acceptance:

- Runtime/enrollment fixtures have clear shared or service-local locations.

## M1: Core Data Model And Storage Seams

Read first:

- `service-map.md`
- `account-site-service.md`
- `auth-service.md`
- `enrollment-claiming-contract.md`
- `command-lifecycle.md`
- `telemetry-ingest-architecture.md`
- `artifact-upload-broker.md`
- `workstreams/observability.md`

### M1.1 Database Connection And Repository Pattern

Status: implemented as a first platform seam. `backend/internal/platform/database`
owns Postgres connection config and transaction helpers.
`backend/internal/domain/ports` owns repository interfaces and fakes for the
first logical service boundaries. SQL adapters remain the next step once Neon is
available or a disposable Postgres harness is added.

Scope:

- Add DB connection package.
- Add transaction helper.
- Add repository interfaces per logical service.
- Keep SQL implementation under storage adapters.

Tests:

- Unit tests for transaction helper behavior.
- Repository fake examples.

Acceptance:

- Domain services can be tested without Postgres.

### M1.2 Identity/Auth Tables

Status: base schema implemented in
`backend/migrations/000002_core_domain_schema.sql`. Seeded system roles and
role permissions are data-backed. Membership invitation and service-account
tables remain owner-specific follow-up when their API slices start.

Scope:

- `users`
- `account_memberships`
- `roles`
- `role_permissions`
- `membership_invitations`
- `access_grants`
- service account subject tables if needed for internal service identity

Tests:

- Migration applies.
- Seeded default roles and permissions are queryable.

Acceptance:

- Backend supports seeded roles that are configurable in data, not hard-coded in
  route handlers.

### M1.3 Account/Site Tables

Status: base schema implemented in
`backend/migrations/000002_core_domain_schema.sql`: customer records, site
relationships, buildings, zones, and generic resources exist alongside the
earlier account/site tables.

Scope:

- `accounts`
- `customers`
- `sites`
- `buildings`
- `zones`
- site relationship tables

Tests:

- Migration applies.
- Default building/default zone seed behavior tested at service layer later.

Acceptance:

- Account / Site owns account records and status.
- Site is the durable installation/data container.
- Customer record and site are distinct.

### M1.4 Device Registry And Enrollment Tables

Status: base schema implemented in
`backend/migrations/000002_core_domain_schema.sql`: claim invites,
verifications, invite email deliveries, device claim fields, credential AWS
metadata, and active-session uniqueness constraints exist.

Scope:

- `devices`
- `device_credentials`
- `device_claim_invites`
- `device_claim_verifications`
- `device_claim_invite_email_deliveries`
- recognition signal storage

Tests:

- Migration applies.
- Unique constraints prevent accidental active duplicate code/session usage.

Acceptance:

- `devices.device_id` can equal AWS IoT Thing name.
- Credential records can hold fingerprint, serial, issuer, certificate ID/ARN,
  active/revoked status, and primary/secondary overlap metadata.

### M1.5 Audit Table

Status: base schema implemented in
`backend/migrations/000002_core_domain_schema.sql`. Domain insert/query tests
belong with the first service that emits audit events.

Scope:

- `audit_events`
- minimal indexed fields: actor, action, resource, account/site/device scope,
  occurred_at, request/correlation ID, redacted metadata

Tests:

- Insert/query test.

Acceptance:

- Sensitive flows can write audit records before v0 features expand.

### M1.6 Command Tables

Status: base schema implemented in
`backend/migrations/000002_core_domain_schema.sql`, with command, progress, and
terminal result tables separated.

Scope:

- `commands`
- `command_results`
- optional `command_progress_events`

Tests:

- State transition constraints tested in service layer later.

Acceptance:

- ACK and terminal result can be stored separately.

### M1.7 Runtime State Tables

Status: base schema implemented across
`backend/migrations/000001_initial_v0_platform.sql` and
`backend/migrations/000002_core_domain_schema.sql`.

Scope:

- `device_presence`
- `device_latest_state`
- `device_telemetry_events`
- `device_lifecycle_events`
- `telemetry_ingest_failures`
- `device_desired_state`
- `device_edge_state_projection`

Tests:

- Migration applies.

Acceptance:

- Latest state, sparse history, failures, desired product state, and edge
  convergence projection have separate homes.
- `device_desired_state` stores HomeSignal product desired-state pointers and
  targets. It is not a full AWS shadow mirror.

### M1.8 Owner-Specific Schema Deferral

Scope:

- Confirm owner-service table families and foreign-key expectations.
- Do not create all backup, artifact, alert, notification, release, and debug
  tables in the first base schema wave unless a concrete earlier slice needs a
  shared FK target.
- Owner milestones add their own migrations when the owning service is built.

Tests:

- None beyond schema-plan review.

Acceptance:

- Base schema does not frontload product tables before the owning service slice.
- Later owner migrations can add their tables without changing base ownership.

## M2: Control-Plane API And Authorization Foundation

Read first:

- `api-facade.md`
- `auth-service.md`
- `workstreams/identity-and-authorization.md`
- `workstreams/observability.md`

### M2.1 Request Context And Error Envelope

Status: implemented as a first platform/API seam. `backend/internal/platform/api`
creates request/correlation IDs and standard error envelopes; the control-plane
skeleton uses it for operational routes.

Scope:

- Common request context with request ID, actor subject, account/site scope,
  service name, and correlation ID.
- Standard JSON error envelope with stable reason codes.

Tests:

- HTTP middleware tests.
- Redaction tests for headers/body snippets.

Acceptance:

- All routes can produce consistent errors without exposing secrets.

### M2.2 AuthN Adapter

Status: implemented as an interface/fake seam. `backend/internal/platform/authn`
parses bearer credentials, verifies through an adapter interface, and maps
Cognito subjects to local `users` through `AuthRepository`. Live Cognito JWKS
configuration remains an environment/provider wiring step.

Scope:

- Cognito JWT verifier interface.
- Local/dev fake verifier.
- Subject mapping to local `users`.

Tests:

- Fake verifier route tests.
- Invalid token path.

Acceptance:

- Human authentication is separable from app authorization.

### M2.3 Authorization Service

Status: first evaluator seam implemented in
`backend/internal/domain/authorization`: validates permission action shape and
checks additive permission keys through `AuthRepository`. Relationship-aware
permission expansion remains the SQL/service adapter follow-up.

Scope:

- `AuthorizationService.can(subject, action, resource, context)`.
- Seed default roles.
- Enforce lower_snake_case `resource:action` action names.

Tests:

- Permission matrix tests.
- Disabled user/site relationship ended cases.

Acceptance:

- No sensitive route performs local role checks directly.

### M2.4 API Facade Route Shell

Status: implemented as route-family shell plus public OpenAPI guardrail.
`backend/openapi/public-v1.yaml` owns public `/api/v1` operation IDs, and
`backend/internal/controlplane` rejects route-family calls at the correct auth
boundary before returning not-implemented stubs. Staging exposes `/api/v1/*`
through API Gateway while keeping `/internal/*` unexposed.

Scope:

- `/api/v1`
- `/agent`
- `/internal`
- route registration without full domain behavior
- source-controlled OpenAPI scaffold for public `/api/v1` routes
- guardrail that public routes are not implemented until represented in OpenAPI

Tests:

- Route existence and auth boundary tests.
- Public route test fixture references an OpenAPI operation ID.

Acceptance:

- Public `/api/v1` implementation follows OpenAPI-first discipline.
- `/internal` routes are not public in deployment config later.
- `/agent` routes require the mTLS identity middleware once M5 exists.

### M2.5 Idempotency And Rate Limit Skeleton

Status: implemented as a first in-memory API facade guardrail.
Retry-prone public mutation shells now enforce `Idempotency-Key`, replay
same-request duplicates, reject changed-body key reuse, and return
`RATE_LIMITED` with `Retry-After` from route buckets.

Scope:

- Idempotency key middleware for approved methods/routes.
- In-memory v0 rate limiter for API routes.

Tests:

- Duplicate request behavior.
- Rate limit reason code.

Acceptance:

- In-memory is acceptable before a load balancer/multi-instance topology.

### M2.6 Internal Service Identity

Status: implemented as a local/dev static service-principal adapter.
`/internal/*` shells reject browser/user auth and unknown service principals;
known service callers map to HomeSignal service subjects until the IAM/SigV4
verifier is wired.

Scope:

- IAM/SigV4 verifier interface for AWS-hosted `/internal/*` callers.
- Local/dev fake service identity adapter.
- Map verified AWS principals to HomeSignal service subjects such as
  `service:telemetry-ingest`.
- Ensure internal service subjects still use app-level authorization for
  sensitive actions.

Tests:

- Valid fake service principal maps to expected service subject.
- Unknown service principal rejects.
- Internal route cannot be called by browser/app auth.

Acceptance:

- Physically split Telemetry Ingest can call internal control-plane routes
  without static shared internal API keys.

## M3: Enrollment And Device Registry

Read first:

- `enrollment-claiming-contract.md`
- `workstreams/device-lifecycle.md`
- `api-facade.md`
- `workstreams/local-cloud-trust-boundaries.md`

### M3.1 Portal Claim Invite Creation

Scope:

- `POST /api/v1/sites/{site_id}/device-claim-invites`
- Authenticated user creates a site/customer-bound claim invite.
- Return the GUID-style claim code once and optionally send the HomeSignal email.
- Store only claim code hash/fingerprint.
- Enforce v0 creation budgets before writing/sending.
- Require `Idempotency-Key`; retries must not create duplicate invites or email
  sends.
- Persist bounded display snapshot/hash generated from canonical account/site,
  customer, creator, and support-contact records.
- Route exists in OpenAPI before implementation.

Tests:

- Authorized user succeeds.
- Unauthorized user gets audited denial.
- User/account/site/recipient creation limits return 429 with `Retry-After`.
- Raw claim code is never logged or used as a metrics label.
- Immediate idempotent retry can return the cached raw claim code; retry after
  cache loss returns `CLAIM_INVITE_ALREADY_OPEN` without raw code.
- Email uses canonical account/site/customer details, not free-form sender text.

Acceptance:

- Web flow can express site-bound claim intent before the customer opens the app.

### M3.2 App Claim Invite Verification

Scope:

- `POST /api/v1/device-enrollment/claim-invites/verify`
- App submits claim code, installation ID, CSR hash, agent version, and recognition signals.
- API validates invite state, expiry, creator authority, and rate limits.
- API returns integrator, creator, site, and customer display details plus a short verification token.
- API returns only generic unavailable errors for invalid, unknown, expired,
  cancelled, or already-used codes.
- Route is POST-only, `Cache-Control: no-store`, and does not support broad
  browser CORS.
- Requesting verification does not issue runtime credentials or finalize claim authority.
- Route exists in OpenAPI before implementation.

Tests:

- Valid invite returns bounded display details.
- Invalid/expired/cancelled/used invite returns stable errors without unrelated account/site data.
- Verification token is stored hashed and never logged.
- `claim_details_hash` covers the exact canonical display payload returned to
  the app.
- Recognition matches do not expose old-account details.
- Repeated failures are rate-limited by source IP, installation ID, and claim code fingerprint.

Acceptance:

- App can show "confirm this integrator/site/customer" before committing.

### M3.3 App Claim Confirmation With Fake Provisioner

Scope:

- `POST /api/v1/device-enrollment/claim-verifications/{claim_verification_id}/confirm`
- Verification-token auth.
- Idempotency-key required.
- Confirm same verification, installation ID, CSR hash, and presented details hash.
- Fail with `CLAIM_DETAILS_STALE` if the presented details hash no longer
  matches the invite/verification snapshot.
- Backend selects AWS region/endpoint/template from configuration, not from app request fields.
- Accept optional initial `local_management_policy` metadata without treating it
  as claim authority.
- Create `device_id`.
- Set AWS IoT Thing name equal to `device_id`.
- Store credential metadata including fingerprint/serial/issuer.
- Return certificate PEM and IoT config to the app response.
- Route exists in OpenAPI before implementation.

Tests:

- Device and credential records created in one transaction when the fake provisioner succeeds.
- Private key is never accepted or persisted.
- App-provided AWS region/endpoint/template fields are ignored or rejected.
- Failure leaves remediable state.
- Expired or mismatched verification cannot confirm.
- Duplicate/racing confirmation fails safely or returns the idempotent prior response.
- Transaction locks the invite row before marking it used and creating device
  records.

Acceptance:

- The app can complete a two-step verify/confirm claim without live AWS.

### M3.4 Claim Invite Guardrails And Audit

Scope:

- Configurable limits with v0 defaults:
  - 5 claim invite creations per creating user per rolling 24 hours.
  - 10 claim invite creations per account per rolling 24 hours.
  - 3 open, unexpired claim invites per site.
  - 1 open invite per site/recipient email pair.
- Layered/conjunctive buckets for creator, account, site, recipient email hash,
  recipient domain, source IP/network, installation ID, claim code fingerprint,
  and verification ID.
- Claim conversion ratio metric by creator/account/site:
  successful claimed devices divided by currently open claim invites, plus a
  rolling created-to-used conversion rate for accounts with low invite volume.
- Progressive delay/backoff for verification failures before hard lockout.
- Invite replacement/support-review path after repeated failures against one
  claim code fingerprint.
- Audit invite creation, email send attempt, cancellation, expiry, verification
  attempt/outcome, confirmation attempt/outcome, and AWS provisioning conflict.
- Admin/support read model can show invite status without raw codes.
- Scheduled GC expires old invites/verifications and purges token/code hashes
  and unused PII according to `enrollment-claiming-contract.md`.

Tests:

- Limits are configurable and default to the v0 values.
- Request is rejected when any relevant bucket is exhausted.
- Poor sustained successful-claim-to-open-claim ratio lowers creation headroom
  or requires support review without exposing raw claim codes.
- Public verification failures do not reveal whether the code existed.
- Cancelling or expiring an invite does not reset the rolling creation budget.
- Audit rows contain actor/resource/result/reason but never raw claim codes or verification tokens.
- Support lookup can use bounded fingerprint only.
- Expiry job marks `OPEN` invites expired and expires active verifications.
- Terminal invite cleanup clears `verification_token_hash`, then
  `claim_code_hash`, then unused display PII on the documented TTLs.

Acceptance:

- Invite abuse guardrails exist before email delivery is enabled broadly.

### M3.5 Device Registry Service

Scope:

- Device lookup by `device_id`.
- Credential lookup by fingerprint/serial.
- Credential status transitions.
- Release/revoke cloud authority state.

Tests:

- Active credential authorizes.
- Revoked credential rejects.
- Primary/secondary overlap maps to same `device_id`.

Acceptance:

- M5 and M6 can derive identity from certificate metadata.

## M4: AWS IoT Provisioning Adapter

Read first:

- `enrollment-claiming-contract.md`
- `workstreams/device-lifecycle.md`
- `aws-iot-routing-contract.md`
- `workstreams/secrets-and-config.md`
- `workstreams/deployment.md`

### M4.1 Provisioner Interface

Scope:

- Interface for CSR signing, Thing binding, policy attachment, and metadata
  return.
- Fake implementation already used by M3 remains the local default.

Tests:

- Contract test for returned certificate PEM, cert ID, cert ARN, and parsed
  metadata.

Acceptance:

- Domain flow does not import AWS SDK directly.

### M4.2 AWS IoT Adapter

Scope:

- `CreateCertificateFromCsr`.
- Thing creation/binding.
- IoT policy attachment.
- Return certificate PEM plus metadata.

Tests:

- Unit tests with AWS client mocks.
- Live staging test marked separately.

Acceptance:

- HomeSignal coordinates provisioning but never handles the device private key.

### M4.3 Orphan Remediation

Scope:

- Detect partial success when AWS provisioning succeeds but DB finalization
  fails.
- Store remediation record or retry-safe status.
- Add cleanup script or admin command stub.

Tests:

- Simulated DB failure after AWS success records remediation.

Acceptance:

- Claiming service failure does not create invisible AWS credential mess.

## M5: Agent HTTPS mTLS Boundary

Read first:

- `api-facade.md`
- `artifact-upload-broker.md`
- `telemetry-ingest-architecture.md`
- `workstreams/device-lifecycle.md`

### M5.1 Client Certificate Metadata Middleware

Scope:

- Define edge contract for forwarded client cert metadata.
- Extract fingerprint/serial/issuer from headers or TLS state depending on
  runtime.
- Reject missing or malformed cert metadata.

Tests:

- Valid cert metadata resolves.
- Unknown/revoked cert rejects.
- Body-provided `device_id`, `site_id`, `org_id` ignored for authority.

Acceptance:

- `/agent/*` identity is always derived from certificate metadata and DB.

Do not:

- Trust a body/header `device_id` unless it came from trusted edge cert parsing
  and was resolved through the DB.

### M5.2 Agent Runtime Envelope Parser

Scope:

- Shared Agent HTTPS runtime-envelope parser for telemetry, events, command
  ACK, and command result.
- Validate `message_type`, `schema_type`, `schema_version`, `message_id`,
  `applied_publish_policy_version`, `observed_at`, and typed JSON payload
  shape.
- Keep route-specific ownership while sharing envelope parsing.

Tests:

- Valid telemetry, event, ACK, and result envelopes parse through the common
  parser.
- Missing or duplicate envelope fields reject before domain delegation.
- Blob payloads are rejected from JSON runtime routes and routed to artifact
  flow instead.

Acceptance:

- Agent HTTPS routes use one contract parser for the shared runtime envelope.

### M5.3 Agent Command Detail/ACK/Result Routes

Scope:

- `GET /agent/commands/{command_id}`
- `POST /agent/commands/{command_id}/ack`
- `POST /agent/commands/{command_id}/result`

Tests:

- Command must belong to authenticated device.
- ACK accepted/rejected window recorded.
- Terminal result is separate from ACK.

Acceptance:

- Commands can complete over HTTPS after MQTT wake-up.

### M5.4 Agent Telemetry/Event Route Adapter

Scope:

- `POST /agent/telemetry`
- `POST /agent/events`
- Normalize authenticated device context plus envelope.
- Handoff to Telemetry Ingest adapter.

Tests:

- Valid route envelope accepted into fake receiver.
- Missing envelope field rejected.
- Payload ID mismatch quarantined by ingest once M6 exists.

Acceptance:

- Agent HTTPS becomes the v0 routine telemetry/event ingress.

### M5.5 Artifact Upload Agent Routes

Scope:

- `POST /agent/commands/{command_id}/artifact-upload`
- `POST /agent/artifact-uploads/{upload_id}/complete`

Tests:

- Signed URL never appears in IoT command records.
- Upload command must belong to authenticated device.

Acceptance:

- Artifact broker can issue upload capability through HTTPS only.

## M6: Telemetry Ingest

Read first:

- `telemetry-ingest-architecture.md`
- `telemetry-ingest-build-plan.md`
- `aws-iot-routing-contract.md`
- `workstreams/observability.md`
- `platform-health-monitoring-service.md`

Use `telemetry-ingest-build-plan.md` as the detailed child plan. The slices
below are the top-level dependency path.

### M6.1 Service Skeleton

Scope:

- `telemetry-ingest/` module and process.
- Config, health/readiness, structured logging.
- Fake receiver test harness.

Tests:

- Health/readiness tests.
- Contract example can feed pipeline.

Acceptance:

- No AWS dependency for local service tests.

### M6.2 Pipeline Interfaces

Scope:

- Receiver, envelope parser, schema catalog, authority resolver, dedupe store,
  persistence writer, alert candidate sink, failure sink.

Tests:

- Unit tests for each stage with typed structs.

Acceptance:

- AWS SDK and DB details stay outside core pipeline logic.

### M6.3 Schema Catalog MVP

Scope:

- Health, Home Assistant, app, backup summary, update summary, storage
  telemetry.
- Agent alarm events.
- Policy version validation hooks.

Tests:

- Valid schema fixtures.
- Unsupported schema rejection.
- Oversized/log-secret quarantine fixtures.

Acceptance:

- Ingest writes product state only after schema and authority pass.

### M6.4 Persistence And Dedupe

Scope:

- Latest-state upserts.
- Sparse material history.
- Failure/quarantine rows.
- Material hash and message hash.
- Runtime-local or shared hot dedupe/coalescing path that prevents unchanged
  telemetry from becoming one DB write per report.

Tests:

- Duplicate message idempotency.
- Material change writes; unchanged sample suppresses.
- Received-message versus persisted-write metric proves unchanged suppression.

Acceptance:

- Postgres is not an unbounded raw time-series store.
- Telemetry Ingest is not implemented as per-message Lambda direct-to-Postgres;
  the cloud runtime preserves hot dedupe, coalescing, and batched writes.

### M6.5 Authority Resolution

Scope:

- Resolve device from cert fingerprint/serial for HTTPS messages.
- Reject unknown/revoked/released credentials.
- Detect identity drift.

Tests:

- Unknown cert rejected.
- Payload `device_id` mismatch does not update product state.

Acceptance:

- No runtime ingest depends on mutable payload identity.

### M6.6 IoT Lifecycle Presence

Scope:

- Ingest AWS IoT lifecycle events.
- Update presence with debounce/ordering.

Tests:

- Connect marks online.
- Disconnect does not immediately false-alarm if reconnect follows.

Acceptance:

- UI can show online/disconnected using product read models.

### M6.7 Alert Candidate Sink

Scope:

- Emit candidates after DB write.
- In-process or internal HTTP adapter.
- If using internal HTTP, authenticate through M2.6 internal service identity.

Tests:

- Alert sink failure does not roll back device-state write.
- Candidate idempotency key stable.
- Internal HTTP adapter rejects unauthenticated service callers.

Acceptance:

- Alerting owns durable alert idempotency.

### M6.8 Hot Retention And Cold Archive Worker

Scope:

- Keep latest state in Postgres.
- Keep sparse telemetry hot for 7 days.
- Archive daily plain NDJSON under device-rooted object key.
- Prune archived DB history after hot window.
- Delete product telemetry history and cold archives after the 7-day operational
  grace period when a device is deleted.

Tests:

- Device-rooted key generation.
- Archive verification before prune.
- Device deletion cleanup removes DB history and object archives only after the
  grace period.

Acceptance:

- Device deletion/export is practical because device is path root.
- Audit, security, billing, and authority records remain governed separately and
  do not retain raw telemetry payloads.

## M7: Edge State Adapter And Publish Policy

Read first:

- `edge-state-adapter.md`
- `workstreams/state-change-and-policy-propagation.md`
- `command-lifecycle.md`
- `aws-iot-routing-contract.md`

### M7.1 Publish Policy Domain

Scope:

- Seed free/default policy as auditable record.
- Resolve per-device policy from managing plan/site relationship.
- Include observability policy, budgets, refresh_after, expires_at.

Tests:

- Default policy matches architecture values.
- Device receives resolved policy, not tier label.

Acceptance:

- Policy values are data/config records, not hidden constants.

### M7.2 Edge State Adapter Interface

Scope:

- Interface for writing desired `homesignal_edge` shadow.
- Store compact DB projection.
- Fake adapter for tests.

Tests:

- Product service writes product truth first, then requests edge projection.

Acceptance:

- Product services do not write IoT shadows directly.

### M7.3 AWS Named Shadow Adapter

Scope:

- Adapter for `homesignal_edge`.
- Desired sections: `publish_policy`, `update`.
- Reported projection observer.

Tests:

- Unit tests with AWS client mocks.
- Live staging test marked.

Acceptance:

- No full shadow mirror in Postgres.

### M7.4 App Publish Policy Application

Scope:

- App stores last accepted policy.
- Enforces local budget hard.
- Falls back conservative when missing/expired/invalid.
- Reports applied version through shadow reported state.

Tests:

- Expired policy falls conservative.
- Unknown event family dropped locally.
- Policy apply failure emits bounded agent alarm.

Acceptance:

- Device-side enforcement does not depend on cloud being reachable at that
  moment.

## M8: Command Lifecycle And Device Notification

Read first:

- `command-lifecycle.md`
- `workstreams/local-cloud-trust-boundaries.md`
- `workstreams/state-change-and-policy-propagation.md`
- `aws-iot-routing-contract.md`

### M8.1 Command Service Core

Scope:

- Create command records.
- Validate command allowlist.
- Track queued, sent, ACK, result, timeout, canceled.
- Enforce 15-second default ACK window.

Tests:

- Valid state transitions.
- Missing ACK behavior.
- Non-idempotent commands do not auto-retry.

Acceptance:

- ACK means accepted/rejected, not packet received.

### M8.2 MQTT Command Publisher Adapter

Scope:

- Publish tiny command notices to `homesignal/devices/{device_id}/commands`.
- No signed URLs or secrets in MQTT payload.

Tests:

- Payload contract fixture.
- AWS adapter mocked.

Acceptance:

- Device wake-up/control remains AWS IoT Core.

### M8.3 Notification Path

Scope:

- Fire-and-forget notification publisher for hints.
- No ACK/result.

Tests:

- Notification cannot mutate durable command state.

Acceptance:

- Notifications are never treated as proof of local state change.

### M8.4 App Command Receiver

Scope:

- App subscribes to command topic.
- Fetches command detail over mTLS Agent HTTPS.
- ACKs accepted/rejected.
- Rejects unknown or locally disallowed command types.

Tests:

- Fake MQTT command notice.
- Unknown command rejection.
- ACK over fake Agent HTTPS client.

Acceptance:

- Local command policy fails closed.

## M9: Backup, Artifact Broker, Diagnostics, Debug

Read first:

- `artifact-upload-broker.md`
- `app-runtime-error-and-artifact-contract.md`
- `command-lifecycle.md`
- `workstreams/local-cloud-trust-boundaries.md`
- `workstreams/observability.md`
- `service-map.md`

### M9.1 Artifact Broker Core

Scope:

- Add `artifact_uploads` migration.
- Upload slot records.
- Purpose/type registry.
- Server-generated object keys.
- TTL, size, content type, checksum fields.
- Fake signed URL issuer.

Tests:

- Unsupported purpose rejected.
- Object key server-generated.
- Signed URL not logged.

Acceptance:

- Broker grants temporary object capability only; it does not own product
  meaning.

### M9.2 S3/Object Storage Adapter

Scope:

- Pre-signed upload URLs.
- Metadata handoff.
- Completion validation hooks.

Tests:

- Mocked S3 signing.
- URL TTL and object key assertions.

Acceptance:

- No AWS credentials delivered to the app.

### M9.3 Backup Service MVP

Scope:

- Add `backups` migration.
- Backup policy/status record.
- Trigger backup command.
- Interpret backup command result.
- Track last success/failure, in-progress, overdue.
- Own offsite backup artifact product meaning.

Tests:

- Trigger authorized user.
- Command result updates backup status.
- Backup artifact visibility follows device/site authority.

Acceptance:

- Backup is a real logical service, even if initial behavior is small.

### M9.4 Diagnostics And Debug Session MVP

Scope:

- Add `debug_sessions` and `diagnostic_bundles` migrations.
- Debug session row and diagnostic bundle metadata.
- Internal/support-only start/stop route.
- Command issue for debug capture.
- Artifact request for debug/error log bundle.

Tests:

- Customer actor cannot start debug mode.
- TTL default 1 hour, hard max 24 hours.
- Audit start/stop/artifact request.
- Diagnostic bundle metadata references approved command/session/artifact
  records.

Acceptance:

- Debug is targeted, temporary, bounded, and not customer self-service.
- Diagnostics owns request lifecycle and redaction policy; Artifact Broker owns
  temporary upload capability.

### M9.5 App Artifact Upload Handler

Scope:

- Generate only allowlisted artifacts.
- Validate command detail and upload session.
- Upload bytes to signed URL.
- Report completion and command result over Agent HTTPS.
- Recursion guard for upload failures.

Tests:

- Unknown local artifact ref rejected.
- Upload failure emits one bounded alarm and does not request more logs.

Acceptance:

- App cannot be used as arbitrary file upload.

## M10: Release And Update Orchestration

Read first:

- `update-architecture.md`
- `edge-state-adapter.md`
- `workstreams/local-cloud-trust-boundaries.md`
- `workstreams/migration-strategy.md`

### M10.1 Release Catalog

Scope:

- Add `release_channels` and `release_artifacts` migrations.
- Release channels: stable, candidate, dev/internal.
- Release artifact metadata.
- Compatibility/protocol window records.

Tests:

- Unsupported version visible.
- Channel assignment validation.

Acceptance:

- Releases are immutable metadata, not floating "latest" assumptions.

### M10.2 Update Desired State

Scope:

- Add `rollouts` and `device_update_assignments` migrations.
- Assign desired app version/channel to device or cohort.
- Write `homesignal_edge.update` desired through Edge State Adapter.
- Do not deliver binaries over IoT.

Tests:

- Assignment writes DB truth and requests edge projection.
- Rollout status is auditable.

Acceptance:

- HomeSignal observes/converges update intent through Supervisor/app path.

### M10.3 Update Status/Repair Command

Scope:

- Bounded status/check/repair commands only.
- No v0 stage/apply binary install command unless update spec changes.

Tests:

- Command lifecycle follows M8.
- Reported update status updates projection/read model.

Acceptance:

- Update command language stays inside v0 trust boundary.

### M10.4 Home Assistant Version Catalog

Scope:

- Add a small adapter/cache for latest stable Home Assistant Core version.
- Refresh daily or through an explicit admin/maintenance job.
- Treat unavailable or ambiguous catalog data as "no advisory" rather than a
  customer warning.
- Feed the portal read model for Home Assistant update advisory only.

Tests:

- Current installed version compares against cached latest.
- Missing/stale catalog hides the advisory.

Acceptance:

- Home Assistant update drift is portal advisory data, not a cloud-initiated
  update action.

## M11: Alerting And Transactional Email

Read first:

- `service-map.md`
- `api-facade.md`
- `auth-service.md`
- `telemetry-ingest-architecture.md`
- `workstreams/observability.md`

### M11.1 Alert Candidate Intake

Scope:

- Internal route or in-process adapter for candidates.
- Alerting verifies current DB state before creating/updating alert.

Tests:

- Duplicate candidate idempotency.
- Stale candidate ignored when current state is healthy.

Acceptance:

- Alert candidates are wake-up signals, not authority.

### M11.2 Alert Lifecycle

Scope:

- Add `alerts` and `alert_events` migrations.
- Alert create/update/resolve/ack/snooze.
- Initial families: disconnected device, backup failed, app/update attention.

Tests:

- Disconnected candidate creates one active alert.
- Recovery resolves alert.

Acceptance:

- Customer-facing alerts are separate from platform-health findings.

### M11.3 Alert Recipients And Preferences

Scope:

- Add `alert_recipients` migration.
- Recipient CRUD.
- Per-email alert subscriptions.
- Optional site scope.
- Verification status: new recipients are pending until verified unless the
  address is the authenticated user's already-verified email.
- V0 alert families: disconnected device, backup failed/overdue, and app
  update attention.

Tests:

- User can configure recipient they are authorized to manage.
- Unverified recipient does not receive product alerts.
- Recipient settings drive notification eligibility.

Acceptance:

- Each email address can configure its alerts.

### M11.4 Notification Service

Scope:

- Add `notification_attempts` migration.
- Transactional email provider interface.
- Resend provider adapter as the v0 default connector.
- Fake provider for local/unit tests.
- Email outbox processor that claims pending notification attempts, renders
  templates, sends through provider, and records provider/result metadata.
- Render notification messages.
- Delivery attempts/result metadata.
- Suppression/cooldown.
- Config/env shape: `EMAIL_PROVIDER`, `EMAIL_FROM`, and `RESEND_API_KEY` or
  provider-specific equivalent secret.

Tests:

- Fake provider success/failure.
- Resend adapter maps successful response to provider message ID.
- Missing Resend config fails closed in non-fake provider mode.
- Provider failure does not mutate alert authority incorrectly.

Acceptance:

- Resend is the v0 default connector, but provider is generic and replaceable.

## M12: Portal Implementation

Read first:

- `design-mock/src/App.jsx`
- `api-facade.md`
- `account-site-service.md`
- `auth-service.md`
- `service-map.md`
- `ui-data-wiring-reconciliation.md`

### M12.0 Portal Read Model Contracts

Status: first contract fixtures added. `testdata/contracts/api/public-v1`
contains dashboard, devices, and activity read-model examples; backend tests
guard issue projection consistency and public activity filtering.

Scope:

- Add API facade read contracts/fixtures for `GET /api/v1/dashboard`,
  `GET /api/v1/devices`, and `GET /api/v1/activity`.
- Centralize issue projection fields: issue code, severity, label, detail,
  source area, sort priority, and primary action.
- V0 issue codes: disconnected device, backup failed, backup overdue,
  app update attention after 48 hour grace period, Home Assistant update
  advisory, and storage warning when reported.
- Centralize public activity feed fields: occurred time, category, action,
  subject, detail, severity, and actor label.
- Public activity excludes internal platform-health findings, provider-error
  noise, sensitive authorization denials, and debug-session details.
- Add optional `site_category` to fixtures/read models only as presentation
  data; UI falls back to the default Home Assistant/site icon when absent.

Tests:

- Dashboard and Devices fixtures count the same issues for the same source
  facts.
- Activity feed fixture renders from a mixed source set without exposing
  internal-only events.
- Missing latest Home Assistant version catalog data hides HA update advisory.

Acceptance:

- Portal components do not infer issue severity, issue counts, primary actions,
  or activity copy from raw service fragments.

### M12.1 Promote Mock To Portal App

Scope:

- Create `portal/` from the design mock or intentionally rename
  `design-mock/`.
- Preserve current page hierarchy learnings.
- Mark internal notes as non-product UI.

Tests:

- Build test.
- Basic route rendering tests if framework supports them.

Acceptance:

- Portal skeleton can become real UI without rewriting mock intent.

### M12.2 Devices Fleet View

Scope:

- Table/list rows modeled after the refined device row.
- Reuse one shared managed Home Assistant row component across Dashboard and
  Devices.
- Device status, last seen as hours/days, versions, backup summary, update
  attention.
- Inline site category icon immediately before site name when category is known;
  otherwise use the default Home Assistant/site icon.

Tests:

- Empty, connected, disconnected, backup failed, update attention states.

Acceptance:

- Integrators can live on the Devices page.

### M12.3 Device Detail View

Scope:

- Reported state.
- Backups associated with device.
- Version comparison.
- Advanced actions tucked away.

Tests:

- Device detail renders from API fixture.

Acceptance:

- IDs/Thing names are not primary customer-facing labels.

### M12.4 Enrollment Flow

Scope:

- Code entry.
- Claim context.
- Confirm page where the identifiable site name is directly above the primary
  action.
- Cancel path.

Tests:

- Early states do not show review/confirm UI.
- Confirm page replaces the whole page state.

Acceptance:

- User understands what site/environment is being paired.

### M12.5 Alerts UI

Scope:

- Email alert recipient settings.
- Per-recipient subscriptions.
- Offline alerting included as v0 UI.
- Recipient verification state and resend verification affordance.

Tests:

- Add recipient interaction.
- Per-address preferences render.

Acceptance:

- UI shape matches the architecture for Notification Service and Alerting.

### M12.6 Internal Admin Views

Scope:

- Internal-only surfaces for policy defaults, debug sessions, audit review,
  platform health source facts, and provider delivery failures.

Tests:

- Internal routes gated by authorization.

Acceptance:

- Internal controls do not leak into integrator/customer UI.

## M13: App Runtime Expansion

Read first:

- `app-skeleton.md`
- `homesignal/design-docs/app-security.md`
- `enrollment-claiming-contract.md`
- `workstreams/device-lifecycle.md`
- `app-runtime-error-and-artifact-contract.md`
- `edge-state-adapter.md`

The existing app already has local identity, enrollment state, CSR flow
pieces, local UI, and tests. Build on that rather than replacing it.

### M13.1 Recognition Signal Collection

Scope:

- Home Assistant instance UUID when available.
- Supervisor/machine ID when available and appropriate.
- HA/Supervisor/app versions.
- Hostname/environment facts.

Tests:

- Missing signals tolerated.
- Signals sent as advisory fields only.

Acceptance:

- Recognition supports claim invite context but never authorizes identity continuity by
  itself.

### M13.2 Certificate Persistence And mTLS Client

Scope:

- Store local private key and certificate under `/config/iot/*` with `0600`.
- Add mTLS HTTP client for `/agent/*`.

Tests:

- Private key remains local.
- Cert/key file permissions.
- Fake mTLS request includes client certificate.

Acceptance:

- Claimed app can authenticate to Agent HTTPS.

### M13.3 AWS IoT MQTT Client

Scope:

- Connect using Thing/client ID equal to `device_id`.
- Subscribe to commands and notifications.
- Connect to named shadow topics for `homesignal_edge`.

Tests:

- Fake MQTT broker/client tests.
- Client ID mismatch cannot be configured silently.

Acceptance:

- IoT Core is runtime control/session layer.

### M13.4 Telemetry Snapshot Publisher

Scope:

- Hourly default `device.health_snapshot` telemetry.
- Payload namespaces: `agent`, `home_assistant`, `ha_apps`, and
  `runtime_log_summary`.
- HA Core version, Supervisor version, agent version, observed app inventory,
  storage, backup/update summary when available.
- Agent HTTPS publish with common envelope.

Tests:

- Policy controls cadence and enabled categories.
- Snapshot omits secrets and raw config.
- Snapshot uses app arrays, not slug-keyed maps.
- Runtime log summary is collapsed and bounded, not a raw log stream.

Acceptance:

- V0 device history/status can be populated without requiring every unchanged
  telemetry snapshot to be persisted.

### M13.5 Routine Event And Agent Alarm Publisher

Scope:

- `agent_alarm` events and structured runtime log summaries under policy.
- 5 KB diagnostic excerpt cap.
- `more_logs_available` hint.

Tests:

- Budget enforcement.
- Over-budget local drop/count.
- Secret redaction.

Acceptance:

- Device can report useful errors without storming.

### M13.6 Shadow Reported State

Scope:

- Report only compact desired-state convergence facts for publish policy and
  update.
- No heartbeat/shadow spam.

Tests:

- Report on desired delta/apply/reject.
- No report for routine telemetry heartbeat.

Acceptance:

- Shadow costs stay tied to convergence, not routine reporting.

### M13.7 Backup, Diagnostics, Artifact Handlers

Scope:

- Trigger backup handler.
- Collect bounded diagnostics/debug.
- Generate allowlisted artifact bundles.
- Use artifact broker flow over Agent HTTPS.

Tests:

- Unknown command rejected.
- Artifact upload success/failure result paths.

Acceptance:

- Local behavior remains bounded and Home Assistant-safe.

## M14: Deployment, Staging, Production Readiness

Read first:

- `workstreams/deployment.md`
- `workstreams/deployment-readiness-matrix.md`
- `workstreams/operator-prerequisites.md`
- `workstreams/first-deploy-greenlight.md` when executing the first deploy path
- `workstreams/secrets-and-config.md`
- `workstreams/test-environments-and-fixtures.md`
- `workstreams/migration-strategy.md`

### M14.1 Infra Skeleton

Scope:

- OpenTofu/Terraform envs for staging and production.
- Resource inventory follows `workstreams/deployment-readiness-matrix.md`.
- Minimal VPC skeleton.
- No NAT/private-egress complexity unless needed.
- State/config documented.
- First-deploy greenlight exception: create only the staging skeleton needed by
  `workstreams/first-deploy-greenlight.md`; production IaC may be scaffolded but
  must not be applied unless the user explicitly approves production work.

Tests:

- IaC formatting/validation.

Acceptance:

- No production resource depends on undocumented manual creation.

### M14.2 Secrets And Config

Scope:

- `/homesignal/{environment}/{service}/{secret_name}`.
- `/homesignal/{environment}/{service}/config/{config_name}`.
- `HOMESIGNAL_*` env vars.
- Service secret/config inventory follows
  `workstreams/deployment-readiness-matrix.md`.
- Neon/Postgres credential rotation script/runbook.

Tests:

- Staging rotation dry run or scripted rehearsal.

Acceptance:

- Neon can be used without coupling domain code to Neon.

### M14.3 API Gateway mTLS Edge

Scope:

- Custom domain/truststore for `/agent/*`.
- Client certificate metadata forwarded to backend.
- Backend exact fingerprint/serial authorization.

Tests:

- Staging mTLS smoke test with a claimed device cert.

Acceptance:

- API edge validates chain; backend authorizes exact cert.

### M14.4 CodeBuild CI/CD

Scope:

- CodeBuild project for test/build.
- GitHub Actions may trigger or report.
- Scripts remain the real interface.
- CI/CD stages and gates follow `workstreams/deployment-readiness-matrix.md`.

Tests:

- CodeBuild dry run in staging account.

Acceptance:

- Codex/local/CI can run the same repo-owned commands.

### M14.5 Staging Canary

Scope:

- Long-lived real HA/app canary paired with staging AWS IoT.
- Ephemeral HA/app lifecycle test harness.
- Simulator device for contract tests.

Tests:

- Pair canary.
- Send telemetry.
- Command ACK/result.
- Shadow convergence.

Acceptance:

- Staging is production-shaped before production launch.

### M14.6 Production Gate

Scope:

- Production deploy requires explicit operator approval after staging smoke.
- Migration precheck/postcheck.
- Rollback or forward-fix note per deploy.
- Production deploy records include the fields named in
  `workstreams/deployment-readiness-matrix.md`.

Tests:

- Production dry-run/precheck command.

Acceptance:

- Launch operations are boring and scriptable.

### M14.7 Platform Health Source Fact Audit

Scope:

- Verify v0 services emit or store the Platform Health source facts named by
  `platform-health-monitoring-service.md`.
- Cover ingest reject/drop/quarantine counts, command ACK/result failures,
  artifact failures, Agent HTTPS auth rejects, edge-state convergence
  projection, coarse metrics, and short-retention logs.
- Do not build the future Platform Health evaluator or findings product.

Tests:

- Fixture or smoke check confirms each owner can produce at least one source
  fact.
- No Platform Health evaluation runs on the hot ingest path.

Acceptance:

- V0 leaves useful source facts for future platform-health correlation without
  productizing Platform Health findings.

## Cross-Service Data Ownership Matrix

| Data/fact | Owner | Implementation note |
| --- | --- | --- |
| Human auth session | Cognito/Auth adapter | App maps to local user before authorization |
| Roles/permissions | Auth/RBAC | Configurable backend data; no customer role UI in v0 |
| Account/customer/site | Account/Site | Site is durable data container |
| Device identity | Enrollment/Device Registry | `device_id` equals IoT Thing name |
| Device credential | Enrollment/Device Registry | Cert metadata only; private key local |
| Runtime telemetry | Telemetry Ingest | Agent HTTPS in v0 |
| IoT presence | Telemetry Ingest/Presence | Lifecycle events plus telemetry freshness |
| Desired edge state | Edge State Adapter | One named shadow: `homesignal_edge` |
| Publish policy | Publish Policy/Device Registry plus Edge State | Resolved per device; policy values auditable |
| Commands | Command Service | MQTT notice, HTTPS ACK/result |
| Backup meaning | Backup Service | Artifact Broker handles bytes only |
| Artifact capability | Artifact Upload Broker | Signed URL, TTL, object key, metadata |
| Diagnostics/debug metadata | Diagnostics/Observability | Internal/support only in v0 |
| Alerts | Alerting Service | Product/customer alert lifecycle |
| Email delivery | Notification Service | Resend-backed v0 transactional provider adapter behind generic interface |
| Audit | Audit Service | Authority history, not operational logs |
| Platform health findings | Future Platform Health | V0 emits source facts only |

## Testing Strategy

Local default tests:

- Pure unit tests for state machines, policy, authorization, and mappers.
- HTTP handler tests with fake services.
- Repository tests with disposable Postgres when available.
- Contract fixture tests for enrollment, Agent HTTPS envelopes, command
  ACK/result, telemetry schemas, artifact upload, alert candidates.
- App tests with fake HomeSignal API, fake provisioner, fake MQTT, fake
  Agent HTTPS.

Integration tests:

- Mark live tests explicitly, for example with a build tag or `TestLive...`.
- AWS IoT provisioning adapter live test only in staging.
- API Gateway mTLS live test only in staging.
- Staging canary tests for real HA/app pairing and telemetry.

Fixture naming:

```text
agent_https_telemetry_health_v1_valid.json
agent_https_telemetry_health_v1_identity_drift.json
enrollment_claim_context_cross_account_v1.json
command_ack_refresh_publish_policy_v1_accepted.json
artifact_upload_error_log_bundle_v1_complete.json
alert_candidate_device_disconnected_v1.json
```

## Code Hygiene Checklist For Future Codex Tasks

Use this checklist in every implementation PR or local task:

- Did I read the slice docs and ADL entry first?
- Did I avoid asking about a settled architecture topic?
- Did I keep route handlers thin?
- Did I keep AWS/provider code behind an adapter?
- Did I use typed request/response/error shapes?
- Did I add a fake adapter for tests?
- Did I add or update fixtures when a contract changed?
- Did I add at least one failure-path test?
- Did I avoid logging secrets, cert PEMs, claim verification tokens, signed
  URLs, and claim invite codes?
- Did I add audit for sensitive authority changes?
- Did I state migration rollback or forward-fix behavior?
- Did I run the smallest useful verification command?

## First Build Queue

This is the recommended near-term queue. Each item should be small enough for a
single focused Codex run.

If the user explicitly greenlights first deploy execution, follow
`workstreams/first-deploy-greenlight.md` instead of this normal product-build
queue. That protocol intentionally pulls a minimal staging subset of item 49
forward after the backend skeleton and script surface. It does not create
production resources or imply that product DB, auth, IoT, mTLS, object-storage,
email, or CI/CD slices are ready.

1. Create `backend/` Go module with health/readiness/version and config loader.
2. Add script-first command surface under `scripts/`.
3. Add migration tooling and first empty migration.
4. Add core platform packages: request context, errors, logging, clock, IDs.
5. Add Auth/RBAC, Account/Site, Device Registry, runtime state, and audit base
   migrations.
6. Add repository interfaces and fake repositories.
7. Add API facade route shell and public OpenAPI scaffold for `/api/v1`,
   `/agent`, and `/internal`.
8. Add Cognito verifier interface plus local fake auth.
9. Add AuthorizationService with seeded default roles and permission tests.
10. Add internal service identity interface for IAM/SigV4 and local fake
    service subjects.
11. Add Account/Site service create/read flows and default building/zone seed.
12. Add enrollment OpenAPI contract plus site claim-invite create route.
13. Add claim-invite verification route with recognition signals and CSR hash.
14. Add claim-verification confirmation route with fake provisioner and
    fixtures.
15. Add invite creation/verification/confirmation rate limits and audit rows.
16. Add app local claim persistence for confirmed invite responses.
17. Add Device Registry credential lookup by fingerprint/serial.
18. Add AWS IoT provisioner interface and mocked adapter tests.
19. Add real AWS IoT provisioner adapter behind staging-only live tests.
20. Add `/agent/*` mTLS identity middleware using forwarded cert metadata.
21. Add shared Agent HTTPS runtime-envelope parser.
22. Add Agent command detail/ACK/result routes.
23. Add Agent telemetry/events route adapter to fake ingest receiver.
24. Create `telemetry-ingest/` skeleton and pipeline interfaces.
25. Add telemetry schema fixtures and schema catalog MVP.
26. Add telemetry persistence/dedupe with latest-state tables and
    received-message versus persisted-write suppression metrics.
27. Add device authority resolution and identity-drift failures in ingest.
28. Add IoT lifecycle presence ingestion.
29. Add alert candidate sink and Alerting candidate intake.
30. Add telemetry hot-retention, cold archive, and deletion cleanup worker.
31. Add free/default publish-policy seed records.
32. Add Edge State Adapter interface and fake shadow adapter.
33. Add Command Service core lifecycle and MQTT publisher adapter.
34. Add app mTLS Agent HTTPS client.
35. Add app MQTT command/shadow client skeleton.
36. Add app telemetry snapshot publisher under publish policy.
37. Add Artifact Broker core with fake signed URL issuer.
38. Add Backup Service trigger/status MVP.
39. Add S3 signed URL adapter with mocked tests.
40. Add app artifact upload handler.
41. Add Diagnostics/debug session and diagnostic bundle metadata MVP.
42. Add Release/Update catalog and desired version assignment.
43. Add Home Assistant version catalog/advisory adapter.
44. Add Alert recipient routes and preference evaluation.
45. Add Notification Service with fake transactional email provider and Resend
    connector.
46. Add dashboard/devices/activity read-model contracts and API fixtures.
47. Promote design mock into `portal/` or explicitly keep it as mock until API
    reads exist.
48. Wire portal Devices, Enrollment, Backups, Updates, Alerts, Dashboard, and
    Activity to API fixtures.
49. Add infra skeleton for staging/production.
50. Add Secrets Manager/SSM path modules and DB credential rotation script.
51. Add API Gateway mTLS staging path.
52. Add CodeBuild test/build job.
53. Pair staging canary HA/app with real staging IoT.
54. Add staging smoke: claim, telemetry, command ACK/result, alert email fake or
    sandbox provider.
55. Add production deploy gate script with migration pre/post checks.
56. Add Platform Health source fact audit.
57. Run full reconciliation pass against docs before production hardening.

## Known Non-Goals For V0 Implementation

Do not build these as side effects:

- Generic remote shell or arbitrary Home Assistant service calls.
- Generic artifact upload/download outside approved product/debug flows.
- Customer self-service debug mode.
- Live paid event stream UX/pricing.
- Full Platform Health rule engine.
- Full topology product state.
- Remote access tunnel product.
- Custom Device Twin service replacing AWS IoT named shadows.
- Dedicated time-series database.
- Broad VPC/private networking with NAT Gateway complexity.
- Hidden support impersonation.

## When To Update Architecture Docs

Update canonical docs, not this plan, when implementation discovers:

- a route contract must change
- a state machine needs a new state
- a service owns a fact currently assigned elsewhere
- a migration changes compatibility posture
- a security boundary changes
- a v0 non-goal becomes required
- an AWS provider constraint invalidates the documented flow

Then update `architectural-decision-log.md` as a receipt pointing to the owning
doc, not as a duplicate architecture spec.
