package agent

import (
	"bytes"
	"context"
	"fmt"
	"strings"

	"github.com/yourusername/termask/internal/config"
	"github.com/yourusername/termask/internal/provider"
)

type ProviderModel struct {
	providerName string
	provider     provider.ConversationProvider
	cfg          provider.ProviderConfig
	model        string
}

func NewProviderModel(ctx context.Context, cfg config.Config, providerName string) (*ProviderModel, error) {
	name, pcfg, err := cfg.ActiveProviderConfig(providerName)
	if err != nil {
		return nil, err
	}
	p, err := provider.Get(name)
	if err != nil {
		return nil, err
	}
	cp, ok := p.(provider.ConversationProvider)
	if !ok {
		return nil, fmt.Errorf("%s does not support conversations", name)
	}
	model := pcfg.Model
	if model == "" {
		models, err := p.Models(ctx, pcfg)
		if err == nil && len(models) > 0 {
			model = models[0].ID
		}
	}
	return &ProviderModel{providerName: name, provider: cp, cfg: pcfg, model: model}, nil
}

func (m *ProviderModel) Generate(ctx context.Context, messages []Message) (string, error) {
	var system []string
	providerMessages := make([]provider.Message, 0, len(messages))
	for _, msg := range messages {
		if msg.Role == "system" {
			system = append(system, msg.Content)
			continue
		}
		providerMessages = append(providerMessages, provider.Message{Role: msg.Role, Content: msg.Content})
	}
	var out bytes.Buffer
	err := m.provider.AskMessages(ctx, m.cfg, strings.Join(system, "\n\n"), providerMessages, &out)
	return out.String(), err
}

func (m *ProviderModel) ProviderName() string {
	return m.providerName
}

func (m *ProviderModel) ModelName() string {
	return m.model
}
