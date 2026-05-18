# Telemetry Ingest Build And Deploy Plan

This plan turns `telemetry-ingest-architecture.md` into buildable work. It intentionally separates code stems from cloud deployment so the service can be developed and tested before AWS resources exist.

## Build Stance

- Telemetry Ingest is the only separately deployable v0 backend service; API/domain services stay in the control-plane monolith.
- Build the service as one small process with explicit internal interfaces.
- Keep AWS-specific code behind adapters.
- Start with contract-example/unit inputs and fake alert receiver for service tests.
- Treat full end-to-end harness as a separate capability.
- Do not deploy AWS resources as part of the first coding pass.
- Do not implement telemetry time-series, commands, diagnostics, backups, release orchestration, or runtime schema transformation.

## Implementation Defaults

Use these defaults unless a later decision changes scope.

### Enrollment And Registration

Default claim path remains:

```text
unclaimed app
  -> HomeSignal API enrollment endpoints over HTTPS
  -> HomeSignal-authorized AWS IoT CSR signing/provisioning
  -> HomeSignal finalization
```

Telemetry Ingest remains post-claim only.

If an IoT-based bootstrap/registration topic is added later, it is a separate deploy path:

```text
bootstrap/registration topic
  -> registration/enrollment service

runtime topic
  -> Telemetry Ingest
```

Unclaimed-device flood risk is bounded by HTTPS enrollment rate limits and by keeping runtime AWS IoT credentials unavailable until HomeSignal authorizes claim finalization.

### Runtime Message Boundaries

Device-to-cloud telemetry/events use the mTLS Agent HTTPS API in v0:

```text
POST /agent/telemetry
POST /agent/events
```

This reuses the same AWS IoT-signed device certificate identity used by `/agent/*` artifact flows. The HTTPS edge validates the certificate chain; HomeSignal authorizes the exact certificate fingerprint/serial stored during claim and derives `device_id -> site_id -> org_id` from app DB records.

`telemetry` carries reported current/latest state. The v0 root contract is
`schema_type=device.health_snapshot`, whose payload is namespaced under
`agent`, `home_assistant`, `ha_apps`, and `runtime_log_summary`.

`event` carries occurrences such as `ha_event`, `agent_alarm`, future
`command_lifecycle`, and `system_event`.

Runtime events are allowlisted and budgeted. Cloud policy provisions the app's local publish budget; the app enforces the last accepted budget hard, and cloud ingest enforces current server policy immediately. Normal durable policy convergence belongs behind `edge-state-adapter.md` and AWS IoT named shadows. Scoped `refresh_publish_policy` can accelerate or repair local convergence but is not required for correctness.

The app receives resolved effective publish policy, not plan/tier labels. It
reports `applied_publish_policy_version` with every claimed-device runtime
publish. Conservative defaults allow low-rate health snapshot and strict-budget
`agent_alarm`, while disabling live `ha_event` and paid/live event behavior.
Paid/live events are not v0, but the policy shape should allow them soon.

If a future runtime family deliberately moves to MQTT/Basic Ingest, it must use
narrow runtime topic families and must not use broad catch-all rules. The
detailed optional/future AWS IoT topic/rule contract lives in
`aws-iot-routing-contract.md`.

Default routing:

```text
Agent HTTPS request
  -> mTLS edge validates certificate chain
  -> backend resolves exact certificate fingerprint/serial
  -> receiver gets authenticated device context

payload
  -> business data plus optional duplicate self-description for archive/replay
```

Missing, duplicate, or invalid HomeSignal envelope fields are rejected or quarantined before product state changes.

Cloud-to-device commands use the normal MQTT broker path because the app must subscribe to command topics. Do not move command delivery to Basic Ingest.

### Device Identity And Credential Identity

Canonical device lifecycle, trust, and authority rules live in `workstreams/device-lifecycle.md`. This build plan applies those rules to the ingest implementation.

Use two explicit identity layers:

```text
HomeSignal device_id
  durable product identity
  stable across AWS IoT reconnects
  may survive credential rotation or re-pairing

trusted transport credential identity
  Thing name, certificate ID/ARN, principalIdentifier
  certificate fingerprint/serial for Agent HTTPS mTLS authorization
  optional/future MQTT auth and topic-policy provenance
  replaceable
```

Telemetry Ingest must resolve trusted credential identity to HomeSignal
`device_id` before updating product state. For v0 Agent HTTPS, this resolution
uses exact certificate fingerprint/serial stored during claim. AWS IoT
connection session fields such as `clientId`, `sessionIdentifier`, and
`versionNumber` are provenance for a connection, not durable product identity.

Session storage default:

```text
device_presence.current_session_identifier
  replaced by the latest accepted active session

device_lifecycle_events.session_identifier
  append-only history
```

Payload device identifiers are allowed for raw storage and replay usefulness,
but not for authority. Mismatches between resolved credential identity, optional
future topic device ID, and payload device annotations are identity drift.

### Alert Candidate Idempotency

Alert candidates are not durable product truth and should not be deduped only in Telemetry Ingest memory.

Default:

```text
Ingest:
  may rate-limit repeated candidates in memory

Alerting:
  owns durable idempotency with candidate_id and DB uniqueness
```

This allows ingest restarts without creating duplicate alert authority.

### Environments

Launch has two cloud environments: staging and production.

Default:

```text
staging stack with real AWS IoT Core, Agent HTTPS receiver, DB, and control-plane integration
production stack with the same IaC modules and production config
production deploy only after staging validation
```

Avoid hand-created one-off AWS resources. Use infrastructure as code so disaster recovery and rebuilds are possible from the start. Use `workstreams/deployment-readiness-matrix.md` for the v0 resource inventory, smoke checks, and production gate.

### Secrets And Config

Use platform runtime secret injection.

Default:

```text
secrets from AWS Secrets Manager or Systems Manager Parameter Store
non-secret config from environment variables or parameter store
no application secrets baked into images
```

### CI/CD

CI/CD is a separate capability, but the deploy plan should assume the shared
script-first path.

Default direction:

```text
repo-owned scripts are the build/test/migrate/deploy/smoke interface
AWS CodeBuild is the preferred AWS-heavy runner
GitHub Actions may trigger or report CodeBuild but should not own AWS deploy behavior
CodePipeline remains optional until promotion/approval orchestration justifies it
```

Do not block the ingest service design on full CI/CD implementation. Do require
that deployment resources are described in IaC so CI/CD can adopt them later.
Use `workstreams/deployment-readiness-matrix.md` for the v0 CI/CD stages and
gates.

## Development Phases

Implementation checkpoint, 2026-05-14:

- `telemetry-ingest/` now exists as the cloud-side Go service skeleton.
- Shared runtime fixtures live at `testdata/contracts/runtime/`.
- Phase 1 has an initial local service shell with health/readiness/version and
  Agent HTTPS-shaped telemetry/event routes.
- Phase 2 has initial typed pipeline seams for receiver, envelope parsing,
  schema catalog, authority resolution, lifecycle/state evaluation, dedupe,
  persistence, alert candidates, and failures.
- Phase 3 has an initial schema catalog for
  `telemetry/device.health_snapshot/v1` and `event/agent_alarm/v1`, backed by
  fixture tests. Remaining Phase 3 work is implementation depth: policy-budget
  enforcement, quarantine persistence, material hashing, and refresh-policy
  remediation hooks.
- Phase 5 has a first Postgres writer path for accepted latest-state upserts,
  sparse event inserts, failure rows, and telemetry last-seen updates. The
  current staging smoke seeds and cleans a `dev_smoke-*` device fixture before
  posting telemetry so persistence is FK-backed. Full authority resolution and
  production-grade backpressure remain later phases.

### Phase 1: Service Skeleton And Contract Example Tests

Goal: run Telemetry Ingest locally with contract-example input and fake persistence. This is not the full end-to-end harness.

Build:

- `telemetry-ingest` service entrypoint.
- Config loader for local/prod settings.
- Structured logging.
- Health endpoint.
- Contract example receiver that can feed unit/integration messages.
- No AWS dependency.

Acceptance:

- Service starts locally.
- Contract examples can be submitted to the pipeline in tests.
- Health endpoint reports ready/unready based on dependencies.

### Phase 2: Pipeline Interfaces

Goal: create the stable seams before adding logic.

Build interfaces:

- `Receiver`
- `EnvelopeParser`
- `SchemaCatalog`
- `SchemaHandler`
- `DeviceAuthorityResolver`
- `LifecycleEvaluator`
- `StateEvaluator`
- `DedupeStore`
- `PersistenceWriter`
- `AlertCandidateSink`
- `FailureSink`

Acceptance:

- Unit tests can run each stage independently.
- Pipeline passes typed structs between stages, not raw JSON maps.
- No stage imports AWS SDK directly except AWS adapters later.

### Phase 3: Schema Catalog MVP

Goal: support first versioned HomeSignal message contracts.

Build:

- Embedded schema catalog loaded on boot.
- `message_type=telemetry`, `schema_type=device.health_snapshot`,
  `schema_version=1` handler.
- Payload projection for `agent`, `home_assistant`, `ha_apps`, and
  `runtime_log_summary` namespaces.
- `message_type=event`, `schema_type=agent_alarm`, `schema_version=1` handler
  for `potential_abuse_detected`, `publish_policy_apply_failed`,
  `publish_policy_rejected_suspicious`, and `artifact_upload_failed`.
- Event allowlist and publish-budget validation hooks.
- Over-budget drop/count path for routine events.
- Quarantine path for structurally suspicious or security-relevant events.
- Internal security/abuse signal threshold for sustained over-budget publishing.
- Idempotent, rate-limited `refresh_publish_policy` remediation hook.
- Quarantine path for runtime events that include oversized logs/stack traces, raw policy blobs, signed URLs, secrets, or local file contents beyond the bounded redacted excerpt.
- Guardrail that `artifact_upload_failed` does not trigger automatic `error_log_bundle` requests.
- Unsupported schema failure path.
- Canonical normalization and material/sidecar field selection.

Acceptance:

- Supported schema validates and projects fields.
- Non-allowlisted event categories/types are rejected or quarantined.
- Routine over-budget events are dropped/counted.
- Suspicious or security-relevant events are quarantined.
- Sustained over-budget publishing can request scoped publish-policy refresh without changing unrelated app config.
- Unsupported schema writes/returns quarantine failure and does not update latest state.
- Material hash ignores noisy envelope/sample fields.
- Schema catalog exposes supported schema list for internal diagnostics.

### Phase 4: Dedupe And State Evaluation

Goal: suppress unchanged writes while preserving honest freshness.

Build:

- `MemoryDedupeStore`.
- `message_hash`, `material_hash`, `sidecar_hash`.
- StateEvaluator write decisions.
- Periodic refresh rule for latest state.

Acceptance:

- Duplicate messages are idempotent.
- Unchanged samples are suppressed.
- Material changes produce DB write intents.
- Cache is updated only after persistence success.

### Phase 5: Persistence Model

Goal: write the first DB-backed read models.

Build tables/migrations:

- `device_presence`
- `device_lifecycle_events`
- `device_latest_state`
- `device_telemetry_events`
- `telemetry_ingest_failures`

Build writer:

- Upsert latest state.
- Append sparse events.
- Write failures/quarantine rows.
- Update `last_accepted_telemetry_at` for accepted telemetry.

Acceptance:

- API-readable latest state rows are created/updated.
- Sparse event history does not store every unchanged sample.
- Persistence failures prevent input ack.

### Phase 6: Device Authority Resolution

Goal: ensure ingest only accepts messages from known, active devices.

Build:

- Registry lookup from Postgres.
- Short-lived in-memory registry cache.
- Unknown/revoked/released device rejection.
- Credential/thing/client ID validation hooks.
- AWS credential identity to internal device record mapping.
- Identity drift detection for unknown, revoked, mismatched, or stale credentials.
- Payload device annotations treated as consistency checks, not authority.

Acceptance:

- Unknown device is rejected/quarantined.
- Revoked device does not update product state.
- Credential identity resolves to a `devices` row before runtime state is accepted.
- Topic/payload device ID mismatches are treated as identity drift, not authority.
- Identity drift writes `telemetry_ingest_failures` and can emit `device.identity_drift_detected`.
- Registry cache expires and refreshes.

### Phase 7: AWS Lifecycle Presence Logic

Goal: use AWS IoT lifecycle events as connectivity authority.

Build:

- Lifecycle event parser.
- Connected evaluator.
- Disconnected pending-candidate storage.
- Delayed verification worker/timer.
- `connect_failed` handling.
- Ordering/idempotency using `clientId`, `sessionIdentifier`, `versionNumber`, event timestamp, and stored presence.

Acceptance:

- Connected event marks device online when newer.
- Disconnected event never immediately marks offline.
- Reconnect during debounce prevents false offline.
- Duplicate/stale lifecycle events do not corrupt presence.

### Phase 8: Alert Candidate Handoff

Goal: send logical alert candidates without requiring SQS yet.

Build:

- `AlertCandidateSink` interface.
- Phase 1 HTTP or in-process adapter to `POST /internal/alert-candidates`.
- Candidate idempotency key.
- Best-effort delivery semantics.
- DB-state-first rule.

Initial candidates:

- `device.presence_changed`
- `device.connect_failed`
- `device.identity_drift_detected`
- `device.telemetry_health_changed`
- `device.backup_summary_changed`
- `device.schema_rejected`
- `device.connected_but_telemetry_stale`
- `device.telemetry_rate_limited`
- `device.telemetry_ingest_degraded`

Acceptance:

- Alert candidate is sent only after DB write succeeds, except candidates that describe ingest degradation itself.
- Alert sink failure does not roll back persisted device state.
- Candidate payload includes idempotency key and enough context for alerting to verify from DB.

### Phase 9: Backpressure, Limits, And Observability

Goal: keep one small worker honest under load.

Build:

- `max_inflight_messages`.
- `input_batch_size`, if the receiver batches.
- `db_write_batch_size`.
- per-device token bucket.
- per-account token bucket.
- schema reject budget.
- DB circuit breaker.
- alert sink circuit breaker.
- metrics/logs for pipeline stage outcomes.

Acceptance:

- Noisy device/account cannot exhaust the process.
- Lifecycle/material changes are prioritized over unchanged samples.
- Postgres degradation stops intake or allows retry rather than growing memory forever.
- Metrics expose received vs persisted writes.

### Phase 10: Cloud Receiver Adapter

Goal: add the real cloud input adapter after service logic works locally.

Build:

- Agent HTTPS receiver adapter for the chosen target.
- HTTP status and bounded device retry behavior for Agent HTTPS telemetry/events.
- Reject/quarantine metadata mapping for invalid or suspicious messages.
- Local integration test against a fake Agent HTTPS receiver.

Acceptance:

- Accepted messages return success only after required validation and durable
  writes complete.
- Retryable failures return bounded retryable HTTP responses and emit coarse
  failure metrics.
- Non-retryable invalid payloads are rejected or quarantined according to
  policy.

## Dependency List

Code dependencies:

- A backend/service workspace for Telemetry Ingest.
- Postgres connection and migration path.
- Shared device registry schema from enrollment/device management.
- Shared internal auth for alert candidate route if HTTP is used.
- JSON canonicalization helper.
- Hashing helper.
- Clock abstraction for debounce/timer tests.
- Structured logging/metrics package.

Product/data dependencies:

- Claimed devices table exists.
- Device credential metadata includes AWS Thing/cert/client identity mapping.
- Device registry distinguishes durable HomeSignal `device_id` from replaceable AWS IoT credential identity.
- Account/site/device relationships are queryable by ingest.
- Alerting handler route exists or fake sink is available for tests.

AWS deployment dependencies:

- AWS IoT Core endpoint.
- Claimed device certificates and policies.
- Enrollment remains HTTPS in v0; any bootstrap/registration IoT topic family is
  optional/future and must stay separate from Telemetry Ingest.
- Agent HTTPS routes for HomeSignal telemetry/event intake.
- IoT Rules for AWS lifecycle topics.
- No queue is used on the v0 telemetry/event ingest path unless the owning
  architecture docs are changed first.
- Runtime target follows `workstreams/deployment-readiness-matrix.md`.
- IAM permissions for IoT lifecycle Rules to invoke/write to the receiver,
  service logs/metrics, and any future S3 snapshot.

## Deployment Plan

Deployment is a separate step from code implementation.

### Deploy Step 1: Runtime Shell

Resources:

- Immutable service artifact or image repository, depending on chosen runtime.
- Runtime definition following `workstreams/deployment-readiness-matrix.md`.
- Log group.
- Security group/networking only when the chosen runtime needs it.
- Secret/config injection for DB and internal auth.

Validation:

- Service boots.
- Health endpoint passes.
- Service can reach Postgres.
- No IoT traffic required yet.

### Deploy Step 2: Database

Resources:

- Postgres migrations for ingest tables.
- Ingest DB user/role with least necessary permissions.

Validation:

- Migrations apply.
- Ingest can upsert latest state and write failures in staging.

### Deploy Step 3: Local/Fake Input In Staging

Resources:

- Internal test route or one-shot job to feed contract-example messages.
- Fake/HTTP alert receiver.

Validation:

- Contract telemetry message updates DB.
- Contract lifecycle connect/disconnect updates presence after debounce.
- Unsupported schema is rejected/quarantined.

### Deploy Step 4: Agent HTTPS Routes And Ingest Receiver

Resources:

- API Gateway/edge routes for `POST /agent/telemetry` and `POST /agent/events`.
- mTLS client-certificate context forwarding to the receiver or API Facade.
- Device certificate fingerprint/serial authorization lookup.
- IoT Rule for `$aws/events/presence/connected/+`.
- IoT Rule for `$aws/events/presence/disconnected/+`.
- IoT Rule for `$aws/events/presence/connect_failed/+`.
- IAM role allowing the chosen HTTPS edge/backend to invoke/write to the receiver.

Runtime ingest routes must be narrow. Do not deploy broad catch-all routes as telemetry intake.

```text
homesignal/#
```

Runtime route examples:

```text
POST /agent/telemetry
POST /agent/events
```

The Agent HTTPS receiver matches only claimed-device runtime telemetry and event routes. Optional future Basic Ingest routing must be added deliberately and must not become a second authority for the same runtime family.

The Agent HTTPS edge/backend should enrich messages with authenticated device context before delivery:

```text
certificate_fingerprint/certificate_serial
device_id
site_id/org_id
received_at
message_type/schema_type/schema_version/message_id/applied_publish_policy_version/observed_at from HTTPS envelope fields
payload
```

Validation:

- Test mTLS HTTPS telemetry/event request reaches the receiver.
- Test lifecycle event reaches the receiver.
- Ingest consumes and writes DB.
- Failed device-origin HTTPS messages return bounded status codes and record
  reject/quarantine metadata where appropriate. Failed IoT lifecycle delivery
  follows the configured AWS integration retry/failure behavior and emits
  coarse alarms.

### Deploy Step 4A: Registration Topic Separation

This step is optional/future only. V0 enrollment uses HTTPS, so unclaimed-device
registration topics are not part of the launch-critical Telemetry Ingest path.
If a later architecture decision introduces bootstrap/registration IoT topics,
they must remain separate from Telemetry Ingest.

Resources:

- Registration/bootstrap topic family, for example `homesignal/registration/...`.
- Bootstrap/claim IoT policy, if this optional path is introduced, that can publish only registration/bootstrap topics.
- Registration IoT Rule that routes registration/bootstrap messages to the registration/enrollment service.
- Registration service input queue or handler.
- IAM role allowing registration IoT Rule to send only to registration resources.

Required separation:

```text
bootstrap/claim credential
  can publish registration/bootstrap topics only

durable claimed credential
  can call only its own authorized Agent HTTPS runtime routes

Telemetry Ingest Agent HTTPS routes
  never match registration/bootstrap topics

Registration IoT Rules
  never match runtime telemetry topics
```

Validation:

- Bootstrap credential cannot publish to runtime telemetry topics.
- Claimed device credential cannot publish to registration/bootstrap topics unless explicitly allowed for a future flow.
- Registration topic message reaches registration service, not Telemetry Ingest.
- Runtime telemetry message reaches Telemetry Ingest, not registration service.
- No IoT Rule uses a broad catch-all topic pattern.

### Deploy Step 5: Alert Handoff

Resources:

- `POST /internal/alert-candidates` route in API monolith, or alerting worker endpoint.
- Internal auth between ingest and API/alerting.

Validation:

- Ingest sends alert candidate after DB state change.
- Alert handler verifies DB before writing alert.
- Duplicate candidate is idempotent.
- Handler outage does not block DB state writes.

### Deploy Step 6: Operational Alarms

Resources:

- Receiver lag/error alarm.
- ingest error-rate alarm.
- DB write latency/error alarm.
- schema rejection anomaly alarm.
- rate-limited device/account alarm.
- Launch-critical alarm coverage follows
  `workstreams/deployment-readiness-matrix.md`.

Validation:

- Synthetic failures trigger expected alarms.

## Deferred Deployment Work

Do not deploy these for MVP unless a later decision changes scope:

- Redis/Valkey.
- SQLite/S3 dedupe snapshotting.
- EventBridge fanout.
- Kinesis/stream receiver.
- Time-series/metrics database for high-frequency telemetry.
- Runtime-editable schema registry.
- Transactional outbox.

## Settled Defaults For First Implementation

- **Receiver:** implement contract-example/local receiver first; add the first cloud receiver adapter after core pipeline tests pass. SQS remains an optional reliability upgrade, not a v0 requirement.
- **Alert handoff:** implement `POST /internal/alert-candidates` in the API monolith first; keep `AlertCandidateSink` abstract.
- **Connected-but-telemetry-stale threshold:** start at 3 hours, hard-coded/configured, then make plan-aware later.
- **Device identity:** resolve AWS credential identity to an internal `devices` row; do not use payload `device_id` as authority.
- **Claiming:** stays on HTTPS enrollment + HomeSignal-authorized AWS IoT CSR signing/provisioning unless a later registration-topic feature changes scope.

Everything else above is implementer-owned.
