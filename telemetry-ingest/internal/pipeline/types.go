package pipeline

import (
	"context"
	"encoding/json"
	"time"
)

type RuntimeRoute string

const (
	RouteTelemetry RuntimeRoute = "telemetry"
	RouteEvents    RuntimeRoute = "events"
)

type MessageType string

const (
	MessageTypeTelemetry     MessageType = "telemetry"
	MessageTypeEvent         MessageType = "event"
	MessageTypeCommandACK    MessageType = "command_ack"
	MessageTypeCommandResult MessageType = "command_result"
)

const (
	SchemaTypeDeviceHealthSnapshot = "device.health_snapshot"
	SchemaTypeAgentAlarm           = "agent_alarm"

	RuntimeSchemaVersionV1 = 1
)

type SchemaKey struct {
	MessageType MessageType
	SchemaType  string
	Version     int
}

type RuntimeEnvelope struct {
	MessageType                 MessageType     `json:"message_type"`
	SchemaType                  string          `json:"schema_type"`
	SchemaVersion               int             `json:"schema_version"`
	MessageID                   string          `json:"message_id"`
	AppliedPublishPolicyVersion string          `json:"applied_publish_policy_version,omitempty"`
	ObservedAt                  time.Time       `json:"observed_at"`
	Payload                     json.RawMessage `json:"payload"`
}

func (e RuntimeEnvelope) Key() SchemaKey {
	return SchemaKey{
		MessageType: e.MessageType,
		SchemaType:  e.SchemaType,
		Version:     e.SchemaVersion,
	}
}

type AuthenticatedDeviceContext struct {
	DeviceID               string
	SiteID                 string
	OrgID                  string
	CertificateFingerprint string
	CertificateSerial      string
}

type IngestRequest struct {
	Route      RuntimeRoute
	Device     AuthenticatedDeviceContext
	Body       []byte
	ReceivedAt time.Time
}

type IngestResult struct {
	Accepted          bool      `json:"accepted"`
	Written           bool      `json:"written"`
	Suppressed        bool      `json:"suppressed"`
	SuppressionReason string    `json:"suppression_reason,omitempty"`
	MessageID         string    `json:"message_id,omitempty"`
	SchemaType        string    `json:"schema_type,omitempty"`
	MaterialHash      string    `json:"material_hash,omitempty"`
	ReceivedAt        time.Time `json:"received_at"`
}

type ValidatedMessage struct {
	Device     AuthenticatedDeviceContext
	Envelope   RuntimeEnvelope
	Projection Projection
	ReceivedAt time.Time
}

type Projection struct {
	Material map[string]string
	Sidecar  map[string]string
}

type IngestFailure struct {
	Device     AuthenticatedDeviceContext
	MessageID  string
	SchemaType string
	Stage      string
	Reason     string
	ReceivedAt time.Time
}

type TransportCredential struct {
	CertificateFingerprint string
	CertificateSerial      string
	CertificateIssuer      string
}

type LifecycleEvent struct {
	EventType         string
	ClientID          string
	SessionIdentifier string
	ObservedAt        time.Time
}

type StateDecision struct {
	WriteLatest    bool
	WriteHistory   bool
	MaterialChange bool
	Reason         string
}

type MessageDedupeKey struct {
	DeviceID  string
	MessageID string
}

type StateDedupeKey struct {
	DeviceID      string
	SchemaType    string
	SchemaVersion int
}

type AlertCandidate struct {
	DeviceID    string
	CandidateID string
	ReasonCode  string
	Severity    string
	ObservedAt  time.Time
}

type Receiver interface {
	ReceiveRuntime(ctx context.Context, request IngestRequest) (IngestResult, error)
}

type EnvelopeParser interface {
	Parse(route RuntimeRoute, body []byte) (RuntimeEnvelope, error)
}

type DeviceAuthorityResolver interface {
	ResolveDevice(ctx context.Context, credential TransportCredential) (AuthenticatedDeviceContext, error)
}

type SchemaHandler interface {
	Key() SchemaKey
	Validate(envelope RuntimeEnvelope) (Projection, error)
}

type DedupeStore interface {
	SeenMessage(ctx context.Context, key MessageDedupeKey) (bool, error)
	RecordMessage(ctx context.Context, key MessageDedupeKey) error
	LastMaterialHash(ctx context.Context, key StateDedupeKey) (string, bool, error)
	RecordMaterialHash(ctx context.Context, key StateDedupeKey, materialHash string) error
}

type LifecycleEvaluator interface {
	EvaluateLifecycle(ctx context.Context, event LifecycleEvent) error
}

type StateEvaluator interface {
	EvaluateState(ctx context.Context, message ValidatedMessage) (StateDecision, error)
}

type PersistenceWriter interface {
	WriteLatest(ctx context.Context, message ValidatedMessage) error
}

type AlertCandidateSink interface {
	EmitCandidate(ctx context.Context, candidate AlertCandidate) error
}

type FailureSink interface {
	RecordFailure(ctx context.Context, failure IngestFailure) error
}
