// Package provider defines the Provider interface and the central Registry
// that maps provider names to their implementations.
package provider

import (
	"context"
	"fmt"
	"io"
)

// ProviderConfig holds the per-provider settings from config.toml.
type ProviderConfig struct {
	APIKey  string  `toml:"api_key"`
	BaseURL string  `toml:"base_url"` // optional override (OpenAI-compatible)
	Model   string  `toml:"model"`
	Type    string  `toml:"type"`   // optional: openai-compatible
	Models  []Model `toml:"models"` // optional curated list for custom providers
}

// Model describes a single model offered by a provider.
type Model struct {
	ID          string `toml:"id"`
	Description string `toml:"description"`
}

// Provider is the interface every backend must satisfy.
type Provider interface {
	// Ask streams the answer for prompt into out. It returns the full text.
	Ask(ctx context.Context, cfg ProviderConfig, systemPrompt, prompt string, out io.Writer) error

	// Models returns the list of models available for this provider.
	// For local providers (Ollama) this queries the running instance.
	// For remote providers this may return a hardcoded curated list.
	Models(ctx context.Context, cfg ProviderConfig) ([]Model, error)

	// Name returns the canonical provider identifier, e.g. "anthropic".
	Name() string
}

type Message struct {
	Role    string
	Content string
}

type ConversationProvider interface {
	Provider
	AskMessages(ctx context.Context, cfg ProviderConfig, systemPrompt string, messages []Message, out io.Writer) error
}

// Registry holds all registered providers.
type Registry struct {
	providers map[string]Provider
}

var global = &Registry{providers: map[string]Provider{}}

// Register adds a provider to the global registry.
func Register(p Provider) {
	global.providers[p.Name()] = p
}

// Get returns the provider by name, or an error if unknown.
func Get(name string) (Provider, error) {
	p, ok := global.providers[name]
	if !ok {
		return nil, fmt.Errorf("unknown provider %q — run `termask providers` to see available ones", name)
	}
	return p, nil
}

// List returns all registered provider names in insertion order.
func List() []string {
	names := make([]string, 0, len(global.providers))
	for name := range global.providers {
		names = append(names, name)
	}
	return names
}
