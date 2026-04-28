// Package openai implements the provider.Provider interface for OpenAI and any
// OpenAI-compatible API endpoint (Groq, Together AI, Mistral, etc.).
// Set base_url in the provider config to point at a different host.
package openai

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/yourusername/termask/internal/provider"
)

const defaultBaseURL = "https://api.openai.com/v1/chat/completions"

// Provider implements provider.Provider for OpenAI-compatible APIs.
// The same struct is reused for Groq, Together AI, Mistral, etc. by
// setting a different Name() and BaseURL in the config.
type Provider struct {
	name           string
	defaultBaseURL string
	staticModels   []provider.Model
}

func New() *Provider {
	return &Provider{
		name:           "openai",
		defaultBaseURL: defaultBaseURL,
		staticModels: []provider.Model{
			{ID: "gpt-5.4-mini", Description: "GPT-5.4 mini — fast & efficient"},
			{ID: "gpt-5.5", Description: "GPT-5.5 — frontier reasoning for complex work"},
			{ID: "gpt-5.4", Description: "GPT-5.4 — strong reasoning, lower cost"},
			{ID: "gpt-5.4-nano", Description: "GPT-5.4 nano — cheapest high-volume option"},
			{ID: "gpt-4.1", Description: "GPT-4.1 — non-reasoning, long context"},
		},
	}
}

// NewCompatible creates a provider for any OpenAI-compatible API.
// name is the registry key (e.g. "groq"), baseURL overrides the endpoint.
func NewCompatible(name, baseURL string, models []provider.Model) *Provider {
	return &Provider{
		name:           name,
		defaultBaseURL: baseURL,
		staticModels:   models,
	}
}

func (p *Provider) Name() string { return p.name }

func (p *Provider) Models(_ context.Context, _ provider.ProviderConfig) ([]provider.Model, error) {
	return p.staticModels, nil
}

// ── Streaming types ──────────────────────────────────────────────────────────

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatRequest struct {
	Model    string        `json:"model"`
	Messages []chatMessage `json:"messages"`
	Stream   bool          `json:"stream"`
}

type streamChunk struct {
	Choices []struct {
		Delta struct {
			Content string `json:"content"`
		} `json:"delta"`
	} `json:"choices"`
}

// ── Ask ──────────────────────────────────────────────────────────────────────

func (p *Provider) Ask(
	ctx context.Context,
	cfg provider.ProviderConfig,
	systemPrompt, prompt string,
	out io.Writer,
) error {
	messages := []provider.Message{}
	if systemPrompt != "" {
		messages = append(messages, provider.Message{Role: "system", Content: systemPrompt})
	}
	messages = append(messages, provider.Message{Role: "user", Content: prompt})
	return p.AskMessages(ctx, cfg, "", messages, out)
}

func (p *Provider) AskMessages(
	ctx context.Context,
	cfg provider.ProviderConfig,
	systemPrompt string,
	messages []provider.Message,
	out io.Writer,
) error {
	if cfg.APIKey == "" {
		return fmt.Errorf("%s: api_key is not set", p.name)
	}

	model := cfg.Model
	if model == "" && len(p.staticModels) > 0 {
		model = p.staticModels[0].ID
	}

	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = p.defaultBaseURL
	}

	apiMessages := []chatMessage{}
	if systemPrompt != "" {
		apiMessages = append(apiMessages, chatMessage{Role: "system", Content: systemPrompt})
	}
	for _, msg := range messages {
		apiMessages = append(apiMessages, chatMessage{Role: msg.Role, Content: msg.Content})
	}

	body, err := json.Marshal(chatRequest{
		Model:    model,
		Messages: apiMessages,
		Stream:   true,
	})
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+cfg.APIKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("%s API %d: %s", p.name, resp.StatusCode, raw)
	}

	return readSSE(resp.Body, out)
}

func readSSE(r io.Reader, out io.Writer) error {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			break
		}
		var chunk streamChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue
		}
		if len(chunk.Choices) > 0 {
			fmt.Fprint(out, chunk.Choices[0].Delta.Content)
		}
	}
	return scanner.Err()
}
