package commands

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

const (
	CommandTopicPrefix       = "homesignal/devices"
	DefaultNoticeMaxBytes    = 8 * 1024
	DefaultNoticePayloadMax  = 4 * 1024
	DefaultCommandNoticeQoS  = 1
	DefaultNoticeContentType = "application/json"
)

type MQTTMessage struct {
	Topic       string
	Payload     []byte
	QoS         int
	ContentType string
}

type MQTTMessagePublisher interface {
	Publish(ctx context.Context, message MQTTMessage) error
}

type CommandNoticePublisher struct {
	Publisher       MQTTMessagePublisher
	MaxNoticeBytes  int
	MaxPayloadBytes int
}

type CommandNotice struct {
	CommandID   string          `json:"command_id"`
	CommandType CommandType     `json:"command_type"`
	DeviceID    string          `json:"device_id"`
	IssuedAt    time.Time       `json:"issued_at"`
	ExpiresAt   time.Time       `json:"expires_at"`
	Payload     json.RawMessage `json:"payload,omitempty"`
}

func (p CommandNoticePublisher) Publish(ctx context.Context, command Command) error {
	if p.Publisher == nil {
		return fmt.Errorf("mqtt publisher is required")
	}
	message, err := BuildCommandNoticeMessage(command, p.maxNoticeBytes(), p.maxPayloadBytes())
	if err != nil {
		return err
	}
	if err := p.Publisher.Publish(ctx, message); err != nil {
		return fmt.Errorf("publish command notice: %w", err)
	}
	return nil
}

func BuildCommandNoticeMessage(command Command, maxNoticeBytes int, maxPayloadBytes int) (MQTTMessage, error) {
	if maxNoticeBytes <= 0 {
		maxNoticeBytes = DefaultNoticeMaxBytes
	}
	if maxPayloadBytes <= 0 {
		maxPayloadBytes = DefaultNoticePayloadMax
	}
	notice, err := BuildCommandNotice(command, maxPayloadBytes)
	if err != nil {
		return MQTTMessage{}, err
	}
	payload, err := json.Marshal(notice)
	if err != nil {
		return MQTTMessage{}, fmt.Errorf("marshal command notice: %w", err)
	}
	if len(payload) > maxNoticeBytes {
		return MQTTMessage{}, fmt.Errorf("command notice exceeds %d bytes", maxNoticeBytes)
	}
	topic, err := CommandTopic(command.DeviceID)
	if err != nil {
		return MQTTMessage{}, err
	}
	return MQTTMessage{
		Topic:       topic,
		Payload:     payload,
		QoS:         DefaultCommandNoticeQoS,
		ContentType: DefaultNoticeContentType,
	}, nil
}

func BuildCommandNotice(command Command, maxPayloadBytes int) (CommandNotice, error) {
	if strings.TrimSpace(command.CommandID) == "" {
		return CommandNotice{}, fmt.Errorf("command_id is required")
	}
	if strings.TrimSpace(command.DeviceID) == "" {
		return CommandNotice{}, fmt.Errorf("device_id is required")
	}
	if command.CommandType == "" {
		return CommandNotice{}, fmt.Errorf("command_type is required")
	}
	if command.AckDeadlineAt.IsZero() {
		return CommandNotice{}, fmt.Errorf("ack_deadline_at is required")
	}
	issuedAt := command.UpdatedAt
	if command.SentAt != nil {
		issuedAt = *command.SentAt
	}
	if issuedAt.IsZero() {
		return CommandNotice{}, fmt.Errorf("issued_at is required")
	}
	payload := normalizePayload(command.Payload)
	if err := validateNoticePayload(payload, maxPayloadBytes); err != nil {
		return CommandNotice{}, err
	}
	return CommandNotice{
		CommandID:   strings.TrimSpace(command.CommandID),
		CommandType: command.CommandType,
		DeviceID:    strings.TrimSpace(command.DeviceID),
		IssuedAt:    issuedAt.UTC(),
		ExpiresAt:   command.AckDeadlineAt.UTC(),
		Payload:     payload,
	}, nil
}

func CommandTopic(deviceID string) (string, error) {
	deviceID = strings.TrimSpace(deviceID)
	if deviceID == "" {
		return "", fmt.Errorf("device_id is required")
	}
	if strings.ContainsAny(deviceID, "/#+") {
		return "", fmt.Errorf("device_id contains MQTT topic separators or wildcards")
	}
	return fmt.Sprintf("%s/%s/commands", CommandTopicPrefix, deviceID), nil
}

func validateNoticePayload(payload json.RawMessage, maxPayloadBytes int) error {
	if maxPayloadBytes <= 0 {
		maxPayloadBytes = DefaultNoticePayloadMax
	}
	if len(payload) > maxPayloadBytes {
		return fmt.Errorf("command notice payload exceeds %d bytes", maxPayloadBytes)
	}
	if !json.Valid(payload) {
		return fmt.Errorf("command notice payload must be valid JSON")
	}
	var decoded any
	if err := json.Unmarshal(payload, &decoded); err != nil {
		return fmt.Errorf("decode command notice payload: %w", err)
	}
	if forbidden, ok := findForbiddenPayloadKey(decoded); ok {
		return fmt.Errorf("command notice payload contains forbidden key %q", forbidden)
	}
	return nil
}

func findForbiddenPayloadKey(value any) (string, bool) {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			normalized := normalizePayloadKey(key)
			if forbiddenPayloadKeys[normalized] {
				return key, true
			}
			if forbidden, ok := findForbiddenPayloadKey(child); ok {
				return forbidden, true
			}
		}
	case []any:
		for _, child := range typed {
			if forbidden, ok := findForbiddenPayloadKey(child); ok {
				return forbidden, true
			}
		}
	}
	return "", false
}

func normalizePayloadKey(key string) string {
	key = strings.ToLower(strings.TrimSpace(key))
	var buf bytes.Buffer
	for _, r := range key {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			buf.WriteRune(r)
		}
	}
	return buf.String()
}

func (p CommandNoticePublisher) maxNoticeBytes() int {
	if p.MaxNoticeBytes > 0 {
		return p.MaxNoticeBytes
	}
	return DefaultNoticeMaxBytes
}

func (p CommandNoticePublisher) maxPayloadBytes() int {
	if p.MaxPayloadBytes > 0 {
		return p.MaxPayloadBytes
	}
	return DefaultNoticePayloadMax
}

var forbiddenPayloadKeys = map[string]bool{
	"password":       true,
	"privatekey":     true,
	"privatekeypem":  true,
	"secret":         true,
	"signedurl":      true,
	"signedurls":     true,
	"token":          true,
	"certificatepem": true,
}
