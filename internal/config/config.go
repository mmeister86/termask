package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
	"github.com/yourusername/termask/internal/provider"
)

// Config is the top-level config file structure.
type Config struct {
	DefaultProvider string                             `toml:"default_provider"`
	SystemPrompt    string                             `toml:"system_prompt"`
	Providers       map[string]provider.ProviderConfig `toml:"providers"`
}

func Default() Config {
	return Config{
		DefaultProvider: "anthropic",
		SystemPrompt: `You are a helpful shell assistant.
Answer concisely and always prefer practical shell commands, scripts, and one-liners.
When writing scripts, use best practices: error handling, comments, portability.
Format code blocks with triple backticks and the language name.`,
		Providers: map[string]provider.ProviderConfig{
			"anthropic": {Model: "claude-sonnet-4-6"},
			"openai":    {Model: "gpt-5.4-mini"},
			"ollama":    {BaseURL: "http://localhost:11434", Model: "llama3"},
			"groq": {
				BaseURL: "https://api.groq.com/openai/v1/chat/completions",
				Model:   "openai/gpt-oss-120b",
			},
			"together": {
				BaseURL: "https://api.together.xyz/v1/chat/completions",
				Model:   "moonshotai/Kimi-K2.5",
			},
		},
	}
}

func ConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "termask", "config.toml"), nil
}

// Load reads the config file, overlaying env vars on top.
func Load() (Config, error) {
	cfg := Default()

	path, err := ConfigPath()
	if err != nil {
		return cfg, err
	}

	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		if key := os.Getenv("ANTHROPIC_API_KEY"); key != "" {
			pcfg := cfg.Providers["anthropic"]
			pcfg.APIKey = key
			cfg.Providers["anthropic"] = pcfg
			return cfg, nil
		}
		return cfg, fmt.Errorf("no config found — run `termask init` to set up")
	}

	if _, err := toml.DecodeFile(path, &cfg); err != nil {
		return cfg, fmt.Errorf("parse config: %w", err)
	}

	overlayEnvVars(&cfg)
	return cfg, nil
}

// ActiveProviderConfig returns the ProviderConfig for the given provider name,
// defaulting to cfg.DefaultProvider when name is empty.
func (c Config) ActiveProviderConfig(name string) (string, provider.ProviderConfig, error) {
	if name == "" {
		name = c.DefaultProvider
	}
	pcfg, ok := c.Providers[name]
	if !ok {
		return name, provider.ProviderConfig{}, fmt.Errorf(
			"provider %q not configured — run `termask init --provider %s`", name, name,
		)
	}
	return name, pcfg, nil
}

// Save writes the config back to disk.
func Save(cfg Config) error {
	path, err := ConfigPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	defer f.Close()
	return toml.NewEncoder(f).Encode(cfg)
}

// SetProviderKey sets or updates the API key / model for a named provider.
func SetProviderKey(providerName, apiKey, model string) error {
	cfg, err := loadOrDefault()
	if err != nil {
		return err
	}
	pcfg := cfg.Providers[providerName]
	if apiKey != "" {
		pcfg.APIKey = apiKey
	}
	if model != "" {
		pcfg.Model = model
	}
	cfg.Providers[providerName] = pcfg
	return Save(cfg)
}

// SetDefault changes the default provider and saves the config.
func SetDefault(providerName string) error {
	cfg, err := loadOrDefault()
	if err != nil {
		return err
	}
	cfg.DefaultProvider = providerName
	return Save(cfg)
}

// ── helpers ───────────────────────────────────────────────────────────────────

func loadOrDefault() (Config, error) {
	path, err := ConfigPath()
	if err != nil {
		return Config{}, err
	}
	cfg := Default()
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return cfg, nil
	}
	_, err = toml.DecodeFile(path, &cfg)
	return cfg, err
}

func overlayEnvVars(cfg *Config) {
	envMap := map[string]string{
		"ANTHROPIC_API_KEY": "anthropic",
		"OPENAI_API_KEY":    "openai",
		"GROQ_API_KEY":      "groq",
		"TOGETHER_API_KEY":  "together",
	}
	for env, name := range envMap {
		if key := os.Getenv(env); key != "" {
			pcfg := cfg.Providers[name]
			pcfg.APIKey = key
			cfg.Providers[name] = pcfg
		}
	}
}
