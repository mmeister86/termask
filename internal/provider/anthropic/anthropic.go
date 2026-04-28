package anthropic

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

const defaultBaseURL = "https://api.anthropic.com/v1/messages"

// Provider implements provider.Provider for Anthropic Claude models.
type Provider struct{}

func New() *Provider { return &Provider{} }

func (p *Provider) Name() string { return "anthropic" }

func (p *Provider) Models(_ context.Context, _ provider.ProviderConfig) ([]provider.Model, error) {
	return []provider.Model{
		{ID: "claude-sonnet-4-6", Description: "Sonnet 4.6 — best speed/intelligence balance"},
		{ID: "claude-opus-4-6", Description: "Opus 4.6 — most capable"},
		{ID: "claude-haiku-4-5-20251001", Description: "Haiku 4.5 — fastest & cheapest"},
	}, nil
}

// ── Streaming request/response types ────────────────────────────────────────

type reqMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type apiRequest struct {
	Model     string       `json:"model"`
	MaxTokens int          `json:"max_tokens"`
	System    string       `json:"system,omitempty"`
	Messages  []reqMessage `json:"messages"`
	Stream    bool         `json:"stream"`
}

type streamEvent struct {
	Type  string `json:"type"`
	Delta *struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"delta"`
}

// ── Ask ──────────────────────────────────────────────────────────────────────

func (p *Provider) Ask(
	ctx context.Context,
	cfg provider.ProviderConfig,
	systemPrompt, prompt string,
	out io.Writer,
) error {
	return p.AskMessages(ctx, cfg, systemPrompt, []provider.Message{{Role: "user", Content: prompt}}, out)
}

func (p *Provider) AskMessages(
	ctx context.Context,
	cfg provider.ProviderConfig,
	systemPrompt string,
	messages []provider.Message,
	out io.Writer,
) error {
	if cfg.APIKey == "" {
		return fmt.Errorf("anthropic: api_key is not set")
	}

	model := cfg.Model
	if model == "" {
		model = "claude-sonnet-4-6"
	}

	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = defaultBaseURL
	}

	apiMessages := make([]reqMessage, 0, len(messages))
	for _, msg := range messages {
		if msg.Role == "system" {
			continue
		}
		apiMessages = append(apiMessages, reqMessage{Role: msg.Role, Content: msg.Content})
	}
	body, err := json.Marshal(apiRequest{
		Model:     model,
		MaxTokens: 2048,
		System:    systemPrompt,
		Messages:  apiMessages,
		Stream:    true,
	})
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", cfg.APIKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("anthropic API %d: %s", resp.StatusCode, raw)
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
		var evt streamEvent
		if err := json.Unmarshal([]byte(data), &evt); err != nil {
			continue
		}
		if evt.Type == "content_block_delta" && evt.Delta != nil && evt.Delta.Type == "text_delta" {
			fmt.Fprint(out, evt.Delta.Text)
		}
	}
	return scanner.Err()
}
