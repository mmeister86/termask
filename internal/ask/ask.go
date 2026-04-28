package ask

import (
	"bytes"
	"context"
	"io"

	"github.com/yourusername/termask/internal/config"
	"github.com/yourusername/termask/internal/provider"
)

type Request struct {
	ProviderName string
	Query        string
	History      []provider.Message
	Out          io.Writer
}

type Response struct {
	ProviderName string
	Model        string
	Text         string
}

func Run(ctx context.Context, cfg config.Config, req Request) (Response, error) {
	providerName, pcfg, err := cfg.ActiveProviderConfig(req.ProviderName)
	if err != nil {
		return Response{}, err
	}
	p, err := provider.Get(providerName)
	if err != nil {
		return Response{}, err
	}
	model := pcfg.Model
	if model == "" {
		models, err := p.Models(ctx, pcfg)
		if err == nil && len(models) > 0 {
			model = models[0].ID
		}
	}

	var buf bytes.Buffer
	out := io.Writer(&buf)
	if req.Out != nil {
		out = io.MultiWriter(req.Out, &buf)
	}
	if len(req.History) > 0 {
		if cp, ok := p.(provider.ConversationProvider); ok {
			messages := append([]provider.Message{}, req.History...)
			messages = append(messages, provider.Message{Role: "user", Content: req.Query})
			if err := cp.AskMessages(ctx, pcfg, cfg.SystemPrompt, messages, out); err != nil {
				return Response{}, err
			}
			return Response{ProviderName: providerName, Model: model, Text: buf.String()}, nil
		}
	}
	if err := p.Ask(ctx, pcfg, cfg.SystemPrompt, req.Query, out); err != nil {
		return Response{}, err
	}
	return Response{ProviderName: providerName, Model: model, Text: buf.String()}, nil
}
