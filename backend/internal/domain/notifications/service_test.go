package notifications

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/homesignal-io/homesignal-home-assistant-app/backend/internal/domain/alerting"
)

func TestServiceEnqueuesAlertEmailAttempt(t *testing.T) {
	repo := newFakeRepository()
	service := testService(repo, &FakeProvider{})

	attempt, err := service.EnqueueAlertEmail(context.Background(), activeAlert(), verifiedRecipient())
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	if attempt.Status != StatusPending || attempt.IdempotencyKey == "" {
		t.Fatalf("unexpected attempt %#v", attempt)
	}
	if repo.attempts[attempt.NotificationAttemptID].AlertID != "alert_123" {
		t.Fatalf("attempt was not stored")
	}
}

func TestServiceSuppressesAttemptDuringCooldown(t *testing.T) {
	repo := newFakeRepository()
	repo.suppressed = true
	service := testService(repo, &FakeProvider{})

	attempt, err := service.EnqueueAlertEmail(context.Background(), activeAlert(), verifiedRecipient())
	if err != nil {
		t.Fatalf("enqueue suppressed: %v", err)
	}
	if attempt.Status != StatusSuppressed {
		t.Fatalf("expected suppressed attempt, got %#v", attempt)
	}
}

func TestServiceRejectsUnverifiedRecipient(t *testing.T) {
	service := testService(newFakeRepository(), &FakeProvider{})
	recipient := verifiedRecipient()
	recipient.Status = alerting.RecipientStatusPendingVerification

	_, err := service.EnqueueAlertEmail(context.Background(), activeAlert(), recipient)
	if err == nil || !strings.Contains(err.Error(), "not eligible") {
		t.Fatalf("expected ineligible recipient error, got %v", err)
	}
}

func TestServiceProcessesPendingAttemptWithFakeProviderSuccess(t *testing.T) {
	repo := newFakeRepository()
	provider := &FakeProvider{MessageID: "msg_123"}
	service := testService(repo, provider)
	attempt, err := service.EnqueueAlertEmail(context.Background(), activeAlert(), verifiedRecipient())
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}

	processed, err := service.ProcessPending(context.Background(), 10)
	if err != nil {
		t.Fatalf("process: %v", err)
	}
	if len(processed) != 1 || processed[0].Status != StatusSent || processed[0].ProviderMessageID != "msg_123" {
		t.Fatalf("expected sent attempt, got %#v", processed)
	}
	if provider.Messages[0].IdempotencyKey != attempt.IdempotencyKey {
		t.Fatalf("provider did not receive idempotency key")
	}
}

func TestServiceRecordsProviderFailureWithoutAlertMutation(t *testing.T) {
	repo := newFakeRepository()
	provider := &FakeProvider{Err: errors.New("provider unavailable")}
	service := testService(repo, provider)
	if _, err := service.EnqueueAlertEmail(context.Background(), activeAlert(), verifiedRecipient()); err != nil {
		t.Fatalf("enqueue: %v", err)
	}

	processed, err := service.ProcessPending(context.Background(), 10)
	if err != nil {
		t.Fatalf("process: %v", err)
	}
	if processed[0].Status != StatusFailed || !strings.Contains(processed[0].LastError, "provider unavailable") {
		t.Fatalf("expected failed attempt, got %#v", processed[0])
	}
	if repo.alertMutations != 0 {
		t.Fatalf("notification provider failure must not mutate alert authority")
	}
}

func testService(repo *fakeRepository, provider EmailProvider) Service {
	return Service{
		Repository:  repo,
		Provider:    provider,
		FromAddress: "HomeSignal <alerts@example.com>",
		IDGenerator: func() string {
			return "notif_123"
		},
		Clock: fixedClock,
	}
}

func fixedClock() time.Time {
	return time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC)
}

func activeAlert() alerting.Alert {
	return alerting.Alert{
		AlertID:    "alert_123",
		AccountID:  "acct_123",
		SiteID:     "site_123",
		DeviceID:   "dev_123",
		Family:     alerting.FamilyDeviceDisconnected,
		Severity:   alerting.SeverityCritical,
		Status:     alerting.StatusActive,
		Title:      "Device disconnected",
		Detail:     "HomeSignal has not seen this device recently.",
		ReasonCode: "presence_offline",
	}
}

func verifiedRecipient() alerting.AlertRecipient {
	return alerting.AlertRecipient{
		AlertRecipientID: "recipient_123",
		AccountID:        "acct_123",
		SiteID:           "site_123",
		Email:            "owner@example.com",
		EmailNormalized:  "owner@example.com",
		Channel:          "email",
		Status:           alerting.RecipientStatusVerified,
	}
}

type fakeRepository struct {
	attempts       map[string]NotificationAttempt
	suppressed     bool
	alertMutations int
}

func newFakeRepository() *fakeRepository {
	return &fakeRepository{attempts: map[string]NotificationAttempt{}}
}

func (r *fakeRepository) CreateAttempt(_ context.Context, attempt NotificationAttempt) error {
	r.attempts[attempt.NotificationAttemptID] = attempt
	return nil
}

func (r *fakeRepository) ClaimPendingAttempts(_ context.Context, now time.Time, limit int) ([]NotificationAttempt, error) {
	var attempts []NotificationAttempt
	for id, attempt := range r.attempts {
		if len(attempts) >= limit {
			break
		}
		if attempt.Status != StatusPending {
			continue
		}
		if attempt.NotBefore != nil && attempt.NotBefore.After(now) {
			continue
		}
		attempt.Status = StatusClaimed
		attempt.ClaimedAt = &now
		r.attempts[id] = attempt
		attempts = append(attempts, attempt)
	}
	return attempts, nil
}

func (r *fakeRepository) SaveAttempt(_ context.Context, attempt NotificationAttempt) error {
	r.attempts[attempt.NotificationAttemptID] = attempt
	return nil
}

func (r *fakeRepository) IsSuppressed(_ context.Context, _ string, _ string, _ alerting.SubscriptionFamily, _ time.Time) (bool, error) {
	return r.suppressed, nil
}
