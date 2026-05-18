package email

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/homesignal-io/homesignal-home-assistant-app/backend/internal/domain/notifications"
)

const DefaultResendAPIURL = "https://api.resend.com/emails"

type ResendConfig struct {
	APIKey      string
	FromAddress string
	APIURL      string
}

type ResendProvider struct {
	Client *http.Client
	Config ResendConfig
}

func NewResendProvider(config ResendConfig, client *http.Client) (*ResendProvider, error) {
	if strings.TrimSpace(config.APIKey) == "" {
		return nil, fmt.Errorf("RESEND_API_KEY is required")
	}
	if strings.TrimSpace(config.FromAddress) == "" {
		return nil, fmt.Errorf("EMAIL_FROM is required")
	}
	if strings.TrimSpace(config.APIURL) == "" {
		config.APIURL = DefaultResendAPIURL
	}
	if client == nil {
		client = http.DefaultClient
	}
	return &ResendProvider{Client: client, Config: config}, nil
}

func (p ResendProvider) SendEmail(ctx context.Context, message notifications.EmailMessage) (notifications.ProviderResult, error) {
	if strings.TrimSpace(p.Config.APIKey) == "" {
		return notifications.ProviderResult{}, fmt.Errorf("RESEND_API_KEY is required")
	}
	from := strings.TrimSpace(message.From)
	if from == "" {
		from = strings.TrimSpace(p.Config.FromAddress)
	}
	if from == "" {
		return notifications.ProviderResult{}, fmt.Errorf("EMAIL_FROM is required")
	}
	payload := map[string]any{
		"from":    from,
		"to":      []string{strings.TrimSpace(message.To)},
		"subject": strings.TrimSpace(message.Subject),
	}
	if strings.TrimSpace(message.HTML) != "" {
		payload["html"] = message.HTML
	}
	if strings.TrimSpace(message.Text) != "" {
		payload["text"] = message.Text
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return notifications.ProviderResult{}, fmt.Errorf("marshal Resend email: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.Config.APIURL, bytes.NewReader(body))
	if err != nil {
		return notifications.ProviderResult{}, fmt.Errorf("build Resend email request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(p.Config.APIKey))
	req.Header.Set("Content-Type", "application/json")
	if strings.TrimSpace(message.IdempotencyKey) != "" {
		req.Header.Set("Idempotency-Key", strings.TrimSpace(message.IdempotencyKey))
	}
	resp, err := p.Client.Do(req)
	if err != nil {
		return notifications.ProviderResult{}, fmt.Errorf("send Resend email: %w", err)
	}
	defer resp.Body.Close()
	var response struct {
		ID string `json:"id"`
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return notifications.ProviderResult{}, fmt.Errorf("send Resend email: status %d", resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return notifications.ProviderResult{}, fmt.Errorf("decode Resend email response: %w", err)
	}
	if strings.TrimSpace(response.ID) == "" {
		return notifications.ProviderResult{}, fmt.Errorf("Resend response missing id")
	}
	return notifications.ProviderResult{Provider: "resend", ProviderMessageID: strings.TrimSpace(response.ID)}, nil
}
