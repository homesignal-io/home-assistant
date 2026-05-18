package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/homesignal-io/homesignal-home-assistant-app/telemetry-ingest/internal/pipeline"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Writer struct {
	Pool *pgxpool.Pool
}

func Open(ctx context.Context, databaseURL string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse database url: %w", err)
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("open postgres pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}
	return pool, nil
}

func (w Writer) WriteLatest(ctx context.Context, message pipeline.ValidatedMessage) error {
	if w.Pool == nil {
		return fmt.Errorf("postgres pool is required")
	}

	materialJSON, err := json.Marshal(message.Projection.Material)
	if err != nil {
		return fmt.Errorf("marshal material: %w", err)
	}
	sidecarJSON, err := json.Marshal(message.Projection.Sidecar)
	if err != nil {
		return fmt.Errorf("marshal sidecar: %w", err)
	}
	payloadJSON := message.Envelope.Payload
	if len(payloadJSON) == 0 {
		payloadJSON = []byte("{}")
	}

	tx, err := w.Pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin telemetry transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	materialHash := pipeline.MaterialHash(message.Projection.Material)
	if _, err := tx.Exec(ctx, `
INSERT INTO device_latest_state (
  device_id,
  message_type,
  schema_type,
  schema_version,
  message_id,
  observed_at,
  received_at,
  material_hash,
  material,
  sidecar,
  payload,
  updated_at
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9::jsonb, $10::jsonb, $11::jsonb, now())
ON CONFLICT (device_id, message_type, schema_type, schema_version)
DO UPDATE SET
  message_id = EXCLUDED.message_id,
  observed_at = EXCLUDED.observed_at,
  received_at = EXCLUDED.received_at,
  material_hash = EXCLUDED.material_hash,
  material = EXCLUDED.material,
  sidecar = EXCLUDED.sidecar,
  payload = EXCLUDED.payload,
  updated_at = now()
`,
		message.Device.DeviceID,
		string(message.Envelope.MessageType),
		message.Envelope.SchemaType,
		message.Envelope.SchemaVersion,
		message.Envelope.MessageID,
		message.Envelope.ObservedAt,
		message.ReceivedAt,
		materialHash,
		string(materialJSON),
		string(sidecarJSON),
		string(payloadJSON),
	); err != nil {
		return fmt.Errorf("upsert latest telemetry state: %w", err)
	}

	if _, err := tx.Exec(ctx, `
INSERT INTO device_telemetry_events (
  device_id,
  message_id,
  message_type,
  schema_type,
  schema_version,
  observed_at,
  received_at,
  material_hash,
  material,
  sidecar,
  payload,
  created_at
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9::jsonb, $10::jsonb, $11::jsonb, now())
ON CONFLICT (device_id, message_id) DO NOTHING
`,
		message.Device.DeviceID,
		message.Envelope.MessageID,
		string(message.Envelope.MessageType),
		message.Envelope.SchemaType,
		message.Envelope.SchemaVersion,
		message.Envelope.ObservedAt,
		message.ReceivedAt,
		materialHash,
		string(materialJSON),
		string(sidecarJSON),
		string(payloadJSON),
	); err != nil {
		return fmt.Errorf("insert sparse telemetry event: %w", err)
	}

	if message.Envelope.MessageType == pipeline.MessageTypeTelemetry {
		if err := recordTelemetrySeen(ctx, tx, message.Device.DeviceID, message.ReceivedAt); err != nil {
			return err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit telemetry transaction: %w", err)
	}
	return nil
}

func (w Writer) RecordFailure(ctx context.Context, failure pipeline.IngestFailure) error {
	if w.Pool == nil {
		return fmt.Errorf("postgres pool is required")
	}
	rawContext, err := json.Marshal(map[string]string{
		"site_id":                 failure.Device.SiteID,
		"org_id":                  failure.Device.OrgID,
		"certificate_fingerprint": failure.Device.CertificateFingerprint,
		"certificate_serial":      failure.Device.CertificateSerial,
	})
	if err != nil {
		return fmt.Errorf("marshal failure context: %w", err)
	}
	if _, err := w.Pool.Exec(ctx, `
INSERT INTO telemetry_ingest_failures (
  device_id,
  message_id,
  schema_type,
  stage,
  reason,
  received_at,
  raw_context
)
VALUES ($1, $2, $3, $4, $5, $6, $7::jsonb)
`,
		failure.Device.DeviceID,
		failure.MessageID,
		failure.SchemaType,
		failure.Stage,
		failure.Reason,
		failure.ReceivedAt,
		string(rawContext),
	); err != nil {
		return fmt.Errorf("insert telemetry failure: %w", err)
	}
	return nil
}

func recordTelemetrySeen(ctx context.Context, tx pgx.Tx, deviceID string, receivedAt time.Time) error {
	if _, err := tx.Exec(ctx, `
UPDATE devices
SET last_seen_at = GREATEST(COALESCE(last_seen_at, $2), $2),
    updated_at = now()
WHERE device_id = $1
`, deviceID, receivedAt); err != nil {
		return fmt.Errorf("update device last seen: %w", err)
	}
	if _, err := tx.Exec(ctx, `
INSERT INTO device_presence (
  device_id,
  connection_state,
  last_seen_at,
  updated_at
)
VALUES ($1, 'unknown', $2, now())
ON CONFLICT (device_id)
DO UPDATE SET
  last_seen_at = GREATEST(COALESCE(device_presence.last_seen_at, EXCLUDED.last_seen_at), EXCLUDED.last_seen_at),
  updated_at = now()
`, deviceID, receivedAt); err != nil {
		return fmt.Errorf("upsert device presence: %w", err)
	}
	return nil
}
