package commands

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"
)

func TestBuildCommandNoticeMessageMatchesContractFixture(t *testing.T) {
	sentAt := time.Date(2026, 5, 18, 12, 0, 1, 0, time.UTC)
	command := Command{
		CommandID:     "cmd_123",
		DeviceID:      "dev_123",
		CommandType:   TypeRefreshPublishPolicy,
		Status:        StatusSent,
		Payload:       json.RawMessage(`{"reason":"over_budget"}`),
		AckDeadlineAt: sentAt.Add(15 * time.Second),
		SentAt:        &sentAt,
		UpdatedAt:     sentAt,
	}

	message, err := BuildCommandNoticeMessage(command, 0, 0)
	if err != nil {
		t.Fatalf("build command notice: %v", err)
	}
	if message.Topic != "homesignal/devices/dev_123/commands" {
		t.Fatalf("unexpected topic %q", message.Topic)
	}
	if message.QoS != 1 {
		t.Fatalf("expected QoS 1, got %d", message.QoS)
	}
	if message.ContentType != "application/json" {
		t.Fatalf("unexpected content type %q", message.ContentType)
	}
	expected, err := os.ReadFile("testdata/command_notice_refresh_publish_policy.json")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	if got := string(message.Payload); got != strings.TrimSpace(string(expected)) {
		t.Fatalf("notice payload mismatch\nwant: %s\n got: %s", strings.TrimSpace(string(expected)), got)
	}
}

func TestCommandNoticePublisherPublishesThroughAdapter(t *testing.T) {
	sentAt := time.Date(2026, 5, 18, 12, 0, 1, 0, time.UTC)
	fake := &fakeMQTTPublisher{}
	publisher := CommandNoticePublisher{Publisher: fake}

	err := publisher.Publish(context.Background(), Command{
		CommandID:     "cmd_123",
		DeviceID:      "dev_123",
		CommandType:   TypeTriggerBackup,
		Status:        StatusSent,
		AckDeadlineAt: sentAt.Add(15 * time.Second),
		SentAt:        &sentAt,
		UpdatedAt:     sentAt,
	})
	if err != nil {
		t.Fatalf("publish notice: %v", err)
	}
	if len(fake.messages) != 1 {
		t.Fatalf("expected one message, got %d", len(fake.messages))
	}
	if fake.messages[0].Topic != "homesignal/devices/dev_123/commands" {
		t.Fatalf("unexpected topic %q", fake.messages[0].Topic)
	}
}

func TestCommandNoticeRejectsTopicWildcards(t *testing.T) {
	if _, err := CommandTopic("dev/123"); err == nil {
		t.Fatalf("expected slash in device id to fail")
	}
	if _, err := CommandTopic("dev+123"); err == nil {
		t.Fatalf("expected wildcard in device id to fail")
	}
}

func TestCommandNoticeRejectsSecretsAndSignedURLs(t *testing.T) {
	sentAt := time.Date(2026, 5, 18, 12, 0, 1, 0, time.UTC)
	tests := []json.RawMessage{
		json.RawMessage(`{"signed_url":"https://example.com/download"}`),
		json.RawMessage(`{"nested":{"privateKeyPem":"nope"}}`),
		json.RawMessage(`{"token":"nope"}`),
	}

	for _, payload := range tests {
		_, err := BuildCommandNoticeMessage(Command{
			CommandID:     "cmd_123",
			DeviceID:      "dev_123",
			CommandType:   TypeRefreshPublishPolicy,
			Status:        StatusSent,
			Payload:       payload,
			AckDeadlineAt: sentAt.Add(15 * time.Second),
			SentAt:        &sentAt,
			UpdatedAt:     sentAt,
		}, 0, 0)
		if err == nil || !strings.Contains(err.Error(), "forbidden key") {
			t.Fatalf("expected forbidden payload key error for %s, got %v", payload, err)
		}
	}
}

func TestCommandNoticeRejectsOversizePayload(t *testing.T) {
	sentAt := time.Date(2026, 5, 18, 12, 0, 1, 0, time.UTC)
	_, err := BuildCommandNoticeMessage(Command{
		CommandID:     "cmd_123",
		DeviceID:      "dev_123",
		CommandType:   TypeRefreshPublishPolicy,
		Status:        StatusSent,
		Payload:       json.RawMessage(`{"reason":"over_budget"}`),
		AckDeadlineAt: sentAt.Add(15 * time.Second),
		SentAt:        &sentAt,
		UpdatedAt:     sentAt,
	}, 0, 8)
	if err == nil || !strings.Contains(err.Error(), "exceeds 8 bytes") {
		t.Fatalf("expected payload size error, got %v", err)
	}
}

type fakeMQTTPublisher struct {
	messages []MQTTMessage
}

func (p *fakeMQTTPublisher) Publish(_ context.Context, message MQTTMessage) error {
	p.messages = append(p.messages, message)
	return nil
}
