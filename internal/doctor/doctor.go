package doctor

import (
	"os"
	"slices"

	"github.com/yourusername/termask/internal/config"
)

type Level string

const (
	LevelOK    Level = "ok"
	LevelWarn  Level = "warn"
	LevelError Level = "error"
)

type Item struct {
	Level   Level
	Message string
}

type Result struct {
	OK    bool
	Items []Item
}

func CheckConfig(path string, cfg config.Config, registeredProviders []string) Result {
	result := Result{OK: true}
	add := func(level Level, msg string) {
		result.Items = append(result.Items, Item{Level: level, Message: msg})
		if level != LevelOK {
			result.OK = false
		}
	}

	if cfg.DefaultProvider == "" {
		add(LevelError, "default_provider is empty")
	} else if !slices.Contains(registeredProviders, cfg.DefaultProvider) {
		add(LevelError, "default_provider is not a registered provider")
	} else {
		add(LevelOK, "default provider is registered")
	}

	if path != "" {
		if info, err := os.Stat(path); err == nil {
			if info.Mode().Perm() != 0600 {
				add(LevelWarn, "config file permissions should be 0600")
			} else {
				add(LevelOK, "config file permissions are 0600")
			}
		} else {
			add(LevelWarn, "config file does not exist")
		}
	}

	if cfg.DefaultProvider != "" {
		pcfg, ok := cfg.Providers[cfg.DefaultProvider]
		if !ok {
			add(LevelError, "default provider has no config block")
		} else {
			if pcfg.Model == "" {
				add(LevelWarn, "default provider has no model configured")
			}
			if pcfg.APIKey == "" && cfg.DefaultProvider != "ollama" {
				add(LevelWarn, "default provider has no API key configured")
			}
		}
	}
	return result
}
