package pipeline

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var testReceivedAt = time.Date(2026, 5, 14, 12, 1, 0, 0, time.UTC)

func TestRuntimePipelineAcceptsHealthSnapshotFixture(t *testing.T) {
	for _, name := range []string{
		"agent_https_telemetry_device_health_snapshot_v1_valid.json",
		"agent_https_telemetry_device_health_snapshot_v1_degraded.json",
	} {
		t.Run(name, func(t *testing.T) {
			writer := &MemoryWriter{}
			failures := &MemoryFailureSink{}
			runtimePipeline := NewRuntimePipeline(writer, failures)
			runtimePipeline.Clock = func() time.Time { return testReceivedAt }

			result, err := runtimePipeline.Ingest(context.Background(), IngestRequest{
				Route:  RouteTelemetry,
				Device: testDeviceContext(),
				Body:   readContractFixture(t, name),
			})
			if err != nil {
				t.Fatalf("ingest health snapshot: %v", err)
			}
			if !result.Accepted {
				t.Fatal("expected accepted result")
			}
			if writer.Count() != 1 {
				t.Fatalf("expected one write, got %d", writer.Count())
			}
			if failures.Count() != 0 {
				t.Fatalf("expected no failures, got %d", failures.Count())
			}
		})
	}
}

func TestRuntimePipelineAcceptsAgentAlarmFixture(t *testing.T) {
	writer := &MemoryWriter{}
	runtimePipeline := NewRuntimePipeline(writer, &MemoryFailureSink{})

	result, err := runtimePipeline.Ingest(context.Background(), IngestRequest{
		Route:  RouteEvents,
		Device: testDeviceContext(),
		Body:   readContractFixture(t, "agent_https_event_agent_alarm_v1_valid.json"),
	})
	if err != nil {
		t.Fatalf("ingest agent alarm: %v", err)
	}
	if !result.Accepted || writer.Count() != 1 {
		t.Fatalf("expected accepted alarm write, result=%#v writes=%d", result, writer.Count())
	}
}

func TestRuntimePipelineSuppressesDuplicateMessage(t *testing.T) {
	writer := &MemoryWriter{}
	runtimePipeline := NewRuntimePipeline(writer, &MemoryFailureSink{})
	body := readContractFixture(t, "agent_https_telemetry_device_health_snapshot_v1_valid.json")

	first, err := runtimePipeline.Ingest(context.Background(), IngestRequest{
		Route:  RouteTelemetry,
		Device: testDeviceContext(),
		Body:   body,
	})
	if err != nil {
		t.Fatalf("ingest first message: %v", err)
	}
	second, err := runtimePipeline.Ingest(context.Background(), IngestRequest{
		Route:  RouteTelemetry,
		Device: testDeviceContext(),
		Body:   body,
	})
	if err != nil {
		t.Fatalf("ingest duplicate message: %v", err)
	}

	if !first.Written || first.Suppressed {
		t.Fatalf("expected first write, result=%#v", first)
	}
	if !second.Accepted || !second.Suppressed || second.SuppressionReason != "duplicate_message" {
		t.Fatalf("expected duplicate suppression, result=%#v", second)
	}
	if writer.Count() != 1 {
		t.Fatalf("expected one write after duplicate, got %d", writer.Count())
	}
}

func TestRuntimePipelineSuppressesUnchangedTelemetryMaterial(t *testing.T) {
	writer := &MemoryWriter{}
	runtimePipeline := NewRuntimePipeline(writer, &MemoryFailureSink{})
	firstBody := readContractFixture(t, "agent_https_telemetry_device_health_snapshot_v1_valid.json")
	secondBody := bytes.ReplaceAll(
		firstBody,
		[]byte(`"message_id": "01J00000000000000000000000"`),
		[]byte(`"message_id": "01J00000000000000000000009"`),
	)

	first, err := runtimePipeline.Ingest(context.Background(), IngestRequest{
		Route:  RouteTelemetry,
		Device: testDeviceContext(),
		Body:   firstBody,
	})
	if err != nil {
		t.Fatalf("ingest first message: %v", err)
	}
	second, err := runtimePipeline.Ingest(context.Background(), IngestRequest{
		Route:  RouteTelemetry,
		Device: testDeviceContext(),
		Body:   secondBody,
	})
	if err != nil {
		t.Fatalf("ingest unchanged message: %v", err)
	}

	if !first.Written || first.MaterialHash == "" {
		t.Fatalf("expected first material write, result=%#v", first)
	}
	if !second.Accepted || !second.Suppressed || second.SuppressionReason != "unchanged_material" {
		t.Fatalf("expected unchanged suppression, result=%#v", second)
	}
	if first.MaterialHash != second.MaterialHash {
		t.Fatalf("expected same material hash, first=%s second=%s", first.MaterialHash, second.MaterialHash)
	}
	if writer.Count() != 1 {
		t.Fatalf("expected one write after unchanged telemetry, got %d", writer.Count())
	}
}

func TestRuntimePipelineWritesMaterialChange(t *testing.T) {
	writer := &MemoryWriter{}
	runtimePipeline := NewRuntimePipeline(writer, &MemoryFailureSink{})

	first, err := runtimePipeline.Ingest(context.Background(), IngestRequest{
		Route:  RouteTelemetry,
		Device: testDeviceContext(),
		Body:   readContractFixture(t, "agent_https_telemetry_device_health_snapshot_v1_valid.json"),
	})
	if err != nil {
		t.Fatalf("ingest first message: %v", err)
	}
	second, err := runtimePipeline.Ingest(context.Background(), IngestRequest{
		Route:  RouteTelemetry,
		Device: testDeviceContext(),
		Body:   readContractFixture(t, "agent_https_telemetry_device_health_snapshot_v1_degraded.json"),
	})
	if err != nil {
		t.Fatalf("ingest changed message: %v", err)
	}

	if !first.Written || !second.Written {
		t.Fatalf("expected both material states to write, first=%#v second=%#v", first, second)
	}
	if first.MaterialHash == second.MaterialHash {
		t.Fatalf("expected material hash to change")
	}
	if writer.Count() != 2 {
		t.Fatalf("expected two writes after material change, got %d", writer.Count())
	}
}

func TestRuntimePipelineRejectsUnsupportedSchemaWithoutWriting(t *testing.T) {
	writer := &MemoryWriter{}
	failures := &MemoryFailureSink{}
	runtimePipeline := NewRuntimePipeline(writer, failures)
	payload := bytes.ReplaceAll(
		readContractFixture(t, "agent_https_telemetry_device_health_snapshot_v1_valid.json"),
		[]byte(`"schema_type": "device.health_snapshot"`),
		[]byte(`"schema_type": "device.unknown_snapshot"`),
	)

	_, err := runtimePipeline.Ingest(context.Background(), IngestRequest{
		Route:  RouteTelemetry,
		Device: testDeviceContext(),
		Body:   payload,
	})
	if err == nil || !strings.Contains(err.Error(), "unsupported runtime schema") {
		t.Fatalf("expected unsupported schema error, got %v", err)
	}
	if writer.Count() != 0 {
		t.Fatalf("expected no writes, got %d", writer.Count())
	}
	if failures.Count() != 1 {
		t.Fatalf("expected one failure, got %d", failures.Count())
	}
}

func TestRuntimePipelineRejectsOversizedDiagnosticExcerpt(t *testing.T) {
	writer := &MemoryWriter{}
	runtimePipeline := NewRuntimePipeline(writer, &MemoryFailureSink{})
	oversized := strings.Repeat("x", maxDiagnosticExcerptBytes+1)
	payload := bytes.ReplaceAll(
		readContractFixture(t, "agent_https_telemetry_device_health_snapshot_v1_degraded.json"),
		[]byte(`"diagnostic_excerpt": "Backup timed out after 30 minutes."`),
		[]byte(`"diagnostic_excerpt": "`+oversized+`"`),
	)

	_, err := runtimePipeline.Ingest(context.Background(), IngestRequest{
		Route:  RouteTelemetry,
		Device: testDeviceContext(),
		Body:   payload,
	})
	if err == nil || !strings.Contains(err.Error(), "diagnostic_excerpt exceeds") {
		t.Fatalf("expected oversized excerpt error, got %v", err)
	}
	if writer.Count() != 0 {
		t.Fatalf("expected no writes, got %d", writer.Count())
	}
}

func TestDefaultCatalogSupportedSchemas(t *testing.T) {
	keys := NewDefaultCatalog().Supported()
	if len(keys) != 2 {
		t.Fatalf("expected two supported schemas, got %#v", keys)
	}
	if keys[0].MessageType != MessageTypeEvent || keys[0].SchemaType != SchemaTypeAgentAlarm {
		t.Fatalf("expected agent_alarm first after sort, got %#v", keys[0])
	}
	if keys[1].MessageType != MessageTypeTelemetry || keys[1].SchemaType != SchemaTypeDeviceHealthSnapshot {
		t.Fatalf("expected health snapshot second after sort, got %#v", keys[1])
	}
}

func TestRuntimeEnvelopeRejectsDuplicateKeys(t *testing.T) {
	parser := RuntimeEnvelopeParser{}
	payload := bytes.Replace(
		readContractFixture(t, "agent_https_telemetry_device_health_snapshot_v1_valid.json"),
		[]byte(`"message_type": "telemetry",`),
		[]byte(`"message_type": "telemetry", "message_type": "telemetry",`),
		1,
	)

	_, err := parser.Parse(RouteTelemetry, payload)
	if err == nil || !strings.Contains(err.Error(), "duplicate json key") {
		t.Fatalf("expected duplicate key error, got %v", err)
	}
}

func TestRuntimeEnvelopeRejectsRouteMismatch(t *testing.T) {
	parser := RuntimeEnvelopeParser{}
	_, err := parser.Parse(RouteEvents, readContractFixture(t, "agent_https_telemetry_device_health_snapshot_v1_valid.json"))
	if err == nil || !strings.Contains(err.Error(), "route events requires message_type event") {
		t.Fatalf("expected route mismatch error, got %v", err)
	}
}

func TestRuntimeEnvelopeRejectsForbiddenPayloadTokens(t *testing.T) {
	parser := RuntimeEnvelopeParser{}
	payload := bytes.ReplaceAll(
		readContractFixture(t, "agent_https_telemetry_device_health_snapshot_v1_valid.json"),
		[]byte(`"sample_message": "Cloud reconnect backoff active"`),
		[]byte(`"sample_message": "Authorization: Bearer secret"`),
	)

	_, err := parser.Parse(RouteTelemetry, payload)
	if err == nil || !strings.Contains(err.Error(), "forbidden token") {
		t.Fatalf("expected forbidden token error, got %v", err)
	}
}

func readContractFixture(t *testing.T, name string) []byte {
	t.Helper()
	payload, err := os.ReadFile(filepath.Join("..", "..", "..", "testdata", "contracts", "runtime", name))
	if err != nil {
		t.Fatalf("read contract fixture %s: %v", name, err)
	}
	return payload
}

func testDeviceContext() AuthenticatedDeviceContext {
	return AuthenticatedDeviceContext{
		DeviceID:               "dev_01J00000000000000000000000",
		SiteID:                 "site_01J00000000000000000000000",
		OrgID:                  "org_01J00000000000000000000000",
		CertificateFingerprint: "SHA256:fixture",
		CertificateSerial:      "01J00000000000000000000000",
	}
}
