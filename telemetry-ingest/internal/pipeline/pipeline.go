package pipeline

import (
	"context"
	"fmt"
	"time"
)

type RuntimePipeline struct {
	Parser      EnvelopeParser
	Catalog     *Catalog
	DedupeStore DedupeStore
	Writer      PersistenceWriter
	FailureSink FailureSink
	Clock       func() time.Time
}

func NewRuntimePipeline(writer PersistenceWriter, failureSink FailureSink) *RuntimePipeline {
	return &RuntimePipeline{
		Parser:      RuntimeEnvelopeParser{},
		Catalog:     NewDefaultCatalog(),
		DedupeStore: NewMemoryDedupeStore(),
		Writer:      writer,
		FailureSink: failureSink,
		Clock:       func() time.Time { return time.Now().UTC() },
	}
}

func (p *RuntimePipeline) Ingest(ctx context.Context, request IngestRequest) (IngestResult, error) {
	p = p.withDefaults()
	receivedAt := request.ReceivedAt
	if receivedAt.IsZero() {
		receivedAt = p.Clock().UTC()
	}

	envelope, err := p.Parser.Parse(request.Route, request.Body)
	if err != nil {
		p.recordFailure(ctx, request.Device, RuntimeEnvelope{}, "parse", err.Error(), receivedAt)
		return IngestResult{Accepted: false, ReceivedAt: receivedAt}, err
	}

	projection, err := p.Catalog.Validate(envelope)
	if err != nil {
		p.recordFailure(ctx, request.Device, envelope, "schema", err.Error(), receivedAt)
		return IngestResult{Accepted: false, MessageID: envelope.MessageID, SchemaType: envelope.SchemaType, ReceivedAt: receivedAt}, err
	}

	materialHash := MaterialHash(projection.Material)
	messageKey := MessageDedupeKey{DeviceID: request.Device.DeviceID, MessageID: envelope.MessageID}
	if p.DedupeStore != nil {
		seen, err := p.DedupeStore.SeenMessage(ctx, messageKey)
		if err != nil {
			p.recordFailure(ctx, request.Device, envelope, "dedupe", err.Error(), receivedAt)
			return IngestResult{Accepted: false, MessageID: envelope.MessageID, SchemaType: envelope.SchemaType, MaterialHash: materialHash, ReceivedAt: receivedAt}, fmt.Errorf("check message dedupe: %w", err)
		}
		if seen {
			return IngestResult{
				Accepted:          true,
				Written:           false,
				Suppressed:        true,
				SuppressionReason: "duplicate_message",
				MessageID:         envelope.MessageID,
				SchemaType:        envelope.SchemaType,
				MaterialHash:      materialHash,
				ReceivedAt:        receivedAt,
			}, nil
		}
	}

	message := ValidatedMessage{
		Device:     request.Device,
		Envelope:   envelope,
		Projection: projection,
		ReceivedAt: receivedAt,
	}
	written := false
	suppressed := false
	suppressionReason := ""
	if p.shouldSuppressUnchangedTelemetry(ctx, message, materialHash) {
		suppressed = true
		suppressionReason = "unchanged_material"
	} else if p.Writer != nil {
		if err := p.Writer.WriteLatest(ctx, message); err != nil {
			p.recordFailure(ctx, request.Device, envelope, "persistence", err.Error(), receivedAt)
			return IngestResult{Accepted: false, MessageID: envelope.MessageID, SchemaType: envelope.SchemaType, ReceivedAt: receivedAt}, fmt.Errorf("write latest state: %w", err)
		}
		written = true
		if p.DedupeStore != nil && envelope.MessageType == MessageTypeTelemetry {
			if err := p.DedupeStore.RecordMaterialHash(ctx, stateDedupeKey(message), materialHash); err != nil {
				p.recordFailure(ctx, request.Device, envelope, "dedupe", err.Error(), receivedAt)
				return IngestResult{Accepted: false, MessageID: envelope.MessageID, SchemaType: envelope.SchemaType, MaterialHash: materialHash, ReceivedAt: receivedAt}, fmt.Errorf("record material hash: %w", err)
			}
		}
	}
	if p.DedupeStore != nil {
		if err := p.DedupeStore.RecordMessage(ctx, messageKey); err != nil {
			p.recordFailure(ctx, request.Device, envelope, "dedupe", err.Error(), receivedAt)
			return IngestResult{Accepted: false, MessageID: envelope.MessageID, SchemaType: envelope.SchemaType, MaterialHash: materialHash, ReceivedAt: receivedAt}, fmt.Errorf("record message dedupe: %w", err)
		}
	}

	return IngestResult{
		Accepted:          true,
		Written:           written,
		Suppressed:        suppressed,
		SuppressionReason: suppressionReason,
		MessageID:         envelope.MessageID,
		SchemaType:        envelope.SchemaType,
		MaterialHash:      materialHash,
		ReceivedAt:        receivedAt,
	}, nil
}

func (p *RuntimePipeline) withDefaults() *RuntimePipeline {
	if p == nil {
		return NewRuntimePipeline(nil, nil)
	}
	if p.Parser == nil {
		p.Parser = RuntimeEnvelopeParser{}
	}
	if p.Catalog == nil {
		p.Catalog = NewDefaultCatalog()
	}
	if p.DedupeStore == nil {
		p.DedupeStore = NewMemoryDedupeStore()
	}
	if p.Clock == nil {
		p.Clock = func() time.Time { return time.Now().UTC() }
	}
	return p
}

func (p *RuntimePipeline) shouldSuppressUnchangedTelemetry(ctx context.Context, message ValidatedMessage, materialHash string) bool {
	if p.DedupeStore == nil || message.Envelope.MessageType != MessageTypeTelemetry {
		return false
	}
	previous, ok, err := p.DedupeStore.LastMaterialHash(ctx, stateDedupeKey(message))
	if err != nil {
		return false
	}
	return ok && previous == materialHash
}

func stateDedupeKey(message ValidatedMessage) StateDedupeKey {
	return StateDedupeKey{
		DeviceID:      message.Device.DeviceID,
		SchemaType:    message.Envelope.SchemaType,
		SchemaVersion: message.Envelope.SchemaVersion,
	}
}

func (p *RuntimePipeline) recordFailure(ctx context.Context, device AuthenticatedDeviceContext, envelope RuntimeEnvelope, stage, reason string, receivedAt time.Time) {
	if p.FailureSink == nil {
		return
	}
	_ = p.FailureSink.RecordFailure(ctx, IngestFailure{
		Device:     device,
		MessageID:  envelope.MessageID,
		SchemaType: envelope.SchemaType,
		Stage:      stage,
		Reason:     reason,
		ReceivedAt: receivedAt,
	})
}
