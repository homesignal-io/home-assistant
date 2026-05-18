package notifications

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"strings"
	"time"

	"github.com/homesignal-io/homesignal-home-assistant-app/backend/internal/domain/alerting"
)

const (
	StatusPending    Status = "pending"
	StatusClaimed    Status = "claimed"
	StatusSent       Status = "sent"
	StatusFailed     Status = "failed"
	StatusSuppressed Status = "suppressed"
	StatusCanceled   Status = "canceled"

	ChannelEmail = "email"

	TemplateAlertEmail = "alert_email_v0"
)

type Status string

type NotificationAttempt struct {
	NotificationAttemptID    string
	AlertID                  string
	AlertRecipientID         string
	AccountID                string
	SiteID                   string
	DeviceID                 string
	Family                   alerting.SubscriptionFamily
	Channel                  string
	Status                   Status
	TemplateKey              string
	RecipientEmail           string
	RecipientEmailNormalized string
	Subject                  string
	BodyText                 string
	BodyHTML                 string
	Provider                 string
	ProviderMessageID        string
	IdempotencyKey           string
	AttemptCount             int
	LastError                string
	TemplateData             json.RawMessage
	NotBefore                *time.Time
	ClaimedAt                *time.Time
	SentAt                   *time.Time
	FailedAt                 *time.Time
	CreatedAt                time.Time
	UpdatedAt                time.Time
}

type EmailMessage struct {
	From           string
	To             string
	Subject        string
	Text           string
	HTML           string
	IdempotencyKey string
}

type ProviderResult struct {
	Provider          string
	ProviderMessageID string
	Metadata          json.RawMessage
}

type Repository interface {
	CreateAttempt(ctx context.Context, attempt NotificationAttempt) error
	ClaimPendingAttempts(ctx context.Context, now time.Time, limit int) ([]NotificationAttempt, error)
	SaveAttempt(ctx context.Context, attempt NotificationAttempt) error
	IsSuppressed(ctx context.Context, alertRecipientID string, scopeKey string, family alerting.SubscriptionFamily, now time.Time) (bool, error)
}

type EmailProvider interface {
	SendEmail(ctx context.Context, message EmailMessage) (ProviderResult, error)
}

type Renderer interface {
	Render(ctx context.Context, attempt NotificationAttempt) (EmailMessage, error)
}

type IDGenerator func() string
type Clock func() time.Time

type Service struct {
	Repository  Repository
	Provider    EmailProvider
	Renderer    Renderer
	FromAddress string
	IDGenerator IDGenerator
	Clock       Clock
}

func (s Service) EnqueueAlertEmail(ctx context.Context, alert alerting.Alert, recipient alerting.AlertRecipient) (NotificationAttempt, error) {
	if s.Repository == nil {
		return NotificationAttempt{}, fmt.Errorf("notification repository is required")
	}
	if recipient.Status != alerting.RecipientStatusVerified || recipient.Channel != ChannelEmail {
		return NotificationAttempt{}, fmt.Errorf("recipient is not eligible for product alert email")
	}
	family, ok := alertingSubscriptionFamily(alert.Family)
	if !ok {
		return NotificationAttempt{}, fmt.Errorf("alert family is not email-notifiable")
	}
	now := s.now()
	attemptID, err := s.newAttemptID()
	if err != nil {
		return NotificationAttempt{}, err
	}
	scopeKey := alertScopeKey(alert)
	suppressed, err := s.Repository.IsSuppressed(ctx, recipient.AlertRecipientID, scopeKey, family, now)
	if err != nil {
		return NotificationAttempt{}, fmt.Errorf("check notification suppression: %w", err)
	}
	status := StatusPending
	if suppressed {
		status = StatusSuppressed
	}
	templateData := mustJSON(map[string]any{
		"alert_id":    alert.AlertID,
		"family":      alert.Family,
		"severity":    alert.Severity,
		"title":       alert.Title,
		"detail":      alert.Detail,
		"reason_code": alert.ReasonCode,
	})
	attempt := NotificationAttempt{
		NotificationAttemptID:    attemptID,
		AlertID:                  alert.AlertID,
		AlertRecipientID:         recipient.AlertRecipientID,
		AccountID:                alert.AccountID,
		SiteID:                   alert.SiteID,
		DeviceID:                 alert.DeviceID,
		Family:                   family,
		Channel:                  ChannelEmail,
		Status:                   status,
		TemplateKey:              TemplateAlertEmail,
		RecipientEmail:           recipient.Email,
		RecipientEmailNormalized: recipient.EmailNormalized,
		IdempotencyKey:           "alert:" + alert.AlertID + ":recipient:" + recipient.AlertRecipientID,
		TemplateData:             templateData,
		CreatedAt:                now,
		UpdatedAt:                now,
	}
	if err := validateAttempt(attempt); err != nil {
		return NotificationAttempt{}, err
	}
	if err := s.Repository.CreateAttempt(ctx, attempt); err != nil {
		return NotificationAttempt{}, fmt.Errorf("create notification attempt: %w", err)
	}
	return attempt, nil
}

func (s Service) ProcessPending(ctx context.Context, limit int) ([]NotificationAttempt, error) {
	if s.Repository == nil {
		return nil, fmt.Errorf("notification repository is required")
	}
	if s.Provider == nil {
		return nil, fmt.Errorf("email provider is required")
	}
	if limit <= 0 {
		return nil, fmt.Errorf("limit must be positive")
	}
	now := s.now()
	attempts, err := s.Repository.ClaimPendingAttempts(ctx, now, limit)
	if err != nil {
		return nil, fmt.Errorf("claim pending notification attempts: %w", err)
	}
	processed := make([]NotificationAttempt, 0, len(attempts))
	for _, attempt := range attempts {
		processedAttempt := s.processAttempt(ctx, attempt, now)
		processed = append(processed, processedAttempt)
	}
	return processed, nil
}

func (s Service) processAttempt(ctx context.Context, attempt NotificationAttempt, now time.Time) NotificationAttempt {
	attempt.AttemptCount++
	renderer := s.Renderer
	if renderer == nil {
		renderer = DefaultRenderer{FromAddress: s.FromAddress}
	}
	message, err := renderer.Render(ctx, attempt)
	if err != nil {
		return s.failAttempt(ctx, attempt, now, err)
	}
	result, err := s.Provider.SendEmail(ctx, message)
	if err != nil {
		return s.failAttempt(ctx, attempt, now, err)
	}
	attempt.Status = StatusSent
	attempt.Provider = strings.TrimSpace(result.Provider)
	attempt.ProviderMessageID = strings.TrimSpace(result.ProviderMessageID)
	attempt.Subject = message.Subject
	attempt.BodyText = message.Text
	attempt.BodyHTML = message.HTML
	attempt.SentAt = &now
	attempt.LastError = ""
	attempt.UpdatedAt = now
	if saveErr := s.Repository.SaveAttempt(ctx, attempt); saveErr != nil {
		attempt.Status = StatusFailed
		attempt.LastError = saveErr.Error()
	}
	return attempt
}

func (s Service) failAttempt(ctx context.Context, attempt NotificationAttempt, now time.Time, err error) NotificationAttempt {
	attempt.Status = StatusFailed
	attempt.LastError = err.Error()
	attempt.FailedAt = &now
	attempt.UpdatedAt = now
	_ = s.Repository.SaveAttempt(ctx, attempt)
	return attempt
}

type DefaultRenderer struct {
	FromAddress string
}

func (r DefaultRenderer) Render(_ context.Context, attempt NotificationAttempt) (EmailMessage, error) {
	from := strings.TrimSpace(r.FromAddress)
	if from == "" {
		return EmailMessage{}, fmt.Errorf("EMAIL_FROM is required")
	}
	if strings.TrimSpace(attempt.RecipientEmail) == "" {
		return EmailMessage{}, fmt.Errorf("recipient email is required")
	}
	var data struct {
		Title      string `json:"title"`
		Detail     string `json:"detail"`
		Severity   string `json:"severity"`
		ReasonCode string `json:"reason_code"`
	}
	if err := json.Unmarshal(attempt.TemplateData, &data); err != nil {
		return EmailMessage{}, fmt.Errorf("render alert email: %w", err)
	}
	subject := "HomeSignal alert: " + strings.TrimSpace(data.Title)
	text := strings.TrimSpace(data.Detail)
	if text == "" {
		text = "A HomeSignal alert needs attention."
	}
	htmlBody := "<p>" + html.EscapeString(text) + "</p>"
	return EmailMessage{
		From:           from,
		To:             attempt.RecipientEmail,
		Subject:        subject,
		Text:           text,
		HTML:           htmlBody,
		IdempotencyKey: attempt.IdempotencyKey,
	}, nil
}

func validateAttempt(attempt NotificationAttempt) error {
	if attempt.NotificationAttemptID == "" || attempt.AccountID == "" {
		return fmt.Errorf("notification_attempt_id and account_id are required")
	}
	if attempt.Channel != ChannelEmail {
		return fmt.Errorf("unsupported notification channel %q", attempt.Channel)
	}
	switch attempt.Status {
	case StatusPending, StatusClaimed, StatusSent, StatusFailed, StatusSuppressed, StatusCanceled:
	default:
		return fmt.Errorf("unsupported notification status %q", attempt.Status)
	}
	if attempt.TemplateKey == "" || attempt.RecipientEmail == "" || attempt.RecipientEmailNormalized == "" {
		return fmt.Errorf("template_key and recipient email are required")
	}
	if attempt.IdempotencyKey == "" {
		return fmt.Errorf("idempotency_key is required")
	}
	if !json.Valid(normalizeJSON(attempt.TemplateData)) {
		return fmt.Errorf("template data must be valid JSON")
	}
	return nil
}

func alertingSubscriptionFamily(family alerting.Family) (alerting.SubscriptionFamily, bool) {
	switch family {
	case alerting.FamilyDeviceDisconnected:
		return alerting.SubscriptionDeviceDisconnected, true
	case alerting.FamilyBackupFailed, alerting.FamilyBackupOverdue:
		return alerting.SubscriptionBackupFailedOrOverdue, true
	case alerting.FamilyAppUpdateAttention:
		return alerting.SubscriptionAppUpdateAttention, true
	default:
		return "", false
	}
}

func alertScopeKey(alert alerting.Alert) string {
	if strings.TrimSpace(alert.DeviceID) != "" {
		return "device:" + strings.TrimSpace(alert.DeviceID)
	}
	if strings.TrimSpace(alert.SiteID) != "" {
		return "site:" + strings.TrimSpace(alert.SiteID)
	}
	return "account:" + strings.TrimSpace(alert.AccountID)
}

func normalizeJSON(value json.RawMessage) json.RawMessage {
	if len(value) == 0 {
		return json.RawMessage(`{}`)
	}
	return value
}

func mustJSON(value any) json.RawMessage {
	payload, err := json.Marshal(value)
	if err != nil {
		return json.RawMessage(`{}`)
	}
	return payload
}

func (s Service) newAttemptID() (string, error) {
	if s.IDGenerator == nil {
		return "", fmt.Errorf("notification attempt id generator is required")
	}
	id := strings.TrimSpace(s.IDGenerator())
	if id == "" {
		return "", fmt.Errorf("notification attempt id is required")
	}
	return id, nil
}

func (s Service) now() time.Time {
	if s.Clock != nil {
		return s.Clock().UTC()
	}
	return time.Now().UTC()
}
