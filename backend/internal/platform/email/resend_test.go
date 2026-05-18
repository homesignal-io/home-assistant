package email

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/homesignal-io/homesignal-home-assistant-app/backend/internal/domain/notifications"
)

func TestNewResendProviderFailsClosedWithoutConfig(t *testing.T) {
	if _, err := NewResendProvider(ResendConfig{}, nil); err == nil {
		t.Fatalf("expected missing config error")
	}
	if _, err := NewResendProvider(ResendConfig{APIKey: "re_test"}, nil); err == nil {
		t.Fatalf("expected missing from address error")
	}
}

func TestResendProviderSendsEmailAndMapsMessageID(t *testing.T) {
	var gotIDempotency string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotIDempotency = r.Header.Get("Idempotency-Key")
		if r.Header.Get("Authorization") != "Bearer re_test" {
			t.Fatalf("missing authorization header")
		}
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode payload: %v", err)
		}
		if payload["from"] != "HomeSignal <alerts@example.com>" || payload["subject"] != "Subject" {
			t.Fatalf("unexpected payload %#v", payload)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"email_123"}`))
	}))
	defer server.Close()
	provider, err := NewResendProvider(ResendConfig{
		APIKey:      "re_test",
		FromAddress: "HomeSignal <alerts@example.com>",
		APIURL:      server.URL,
	}, server.Client())
	if err != nil {
		t.Fatalf("new provider: %v", err)
	}

	result, err := provider.SendEmail(context.Background(), notifications.EmailMessage{
		To:             "owner@example.com",
		Subject:        "Subject",
		Text:           "Body",
		IdempotencyKey: "notif_123",
	})
	if err != nil {
		t.Fatalf("send email: %v", err)
	}
	if result.Provider != "resend" || result.ProviderMessageID != "email_123" {
		t.Fatalf("unexpected provider result %#v", result)
	}
	if gotIDempotency != "notif_123" {
		t.Fatalf("expected idempotency key, got %q", gotIDempotency)
	}
}
