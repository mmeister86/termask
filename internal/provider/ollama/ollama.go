// Package ollama implements the provider.Provider interface for Ollama,
// allowing termask to query locally-running models without any API key.
package ollama

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/mmeister86/termask/internal/provider"
)

const defaultBaseURL = "http://localhost:11434"

type Provider struct{}

func New() *Provider { return &Provider{} }

func (p *Provider) Name() string { return "ollama" }

// Models queries the running Ollama instance for available models.
func (p *Provider) Models(ctx context.Context, cfg provider.ProviderConfig) ([]provider.Model, error) {
	base := baseURL(cfg)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/api/tags", nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ollama not reachable at %s — is it running?", base)
	}
	defer resp.Body.Close()

	var result struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	models := make([]provider.Model, len(result.Models))
	for i, m := range result.Models {
		models[i] = provider.Model{ID: m.Name, Description: "local"}
	}
	return models, nil
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

type chatChunk struct {
	Message struct {
		Content string `json:"content"`
	} `json:"message"`
	Done  bool   `json:"done"`
	Error string `json:"error"`
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
	model := cfg.Model
	if model == "" {
		return fmt.Errorf("ollama: no model set — run `termask providers add ollama --model llama3`")
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

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL(cfg)+"/api/chat", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("ollama not reachable — is it running? (%w)", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("ollama %d: %s", resp.StatusCode, raw)
	}

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	done := false
	for scanner.Scan() {
		var chunk chatChunk
		if err := json.Unmarshal(scanner.Bytes(), &chunk); err != nil {
			continue
		}
		if chunk.Error != "" {
			return fmt.Errorf("ollama: %s", chunk.Error)
		}
		fmt.Fprint(out, chunk.Message.Content)
		if chunk.Done {
			done = true
			break
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	if !done {
		return fmt.Errorf("ollama: stream ended before done")
	}
	return nil
}

func baseURL(cfg provider.ProviderConfig) string {
	if cfg.BaseURL != "" {
		return cfg.BaseURL
	}
	return defaultBaseURL
}
