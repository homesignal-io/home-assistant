package notifications

import (
	"context"
	"fmt"
)

type FakeProvider struct {
	ProviderName string
	MessageID    string
	Err          error
	Messages     []EmailMessage
}

func (p *FakeProvider) SendEmail(_ context.Context, message EmailMessage) (ProviderResult, error) {
	p.Messages = append(p.Messages, message)
	if p.Err != nil {
		return ProviderResult{}, p.Err
	}
	provider := p.ProviderName
	if provider == "" {
		provider = "fake"
	}
	messageID := p.MessageID
	if messageID == "" {
		messageID = fmt.Sprintf("fake_%d", len(p.Messages))
	}
	return ProviderResult{Provider: provider, ProviderMessageID: messageID}, nil
}
