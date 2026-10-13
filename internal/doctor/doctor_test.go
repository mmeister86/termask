package doctor

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mmeister86/termask/internal/config"
	"github.com/mmeister86/termask/internal/provider"
)

func TestCheckConfigFindsMissingDefaultProvider(t *testing.T) {
	result := CheckConfig("unused", config.Config{
		DefaultProvider: "missing",
		Providers:       map[string]provider.ProviderConfig{},
	}, []string{"anthropic"})
	if result.OK {
		t.Fatal("OK = true, want false")
	}
	if len(result.Items) == 0 {
		t.Fatal("items empty")
	}
}

func TestCheckConfigWarnsAboutLoosePermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte("default_provider = \"anthropic\"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	result := CheckConfig(path, config.Config{
		DefaultProvider: "anthropic",
		Providers: map[string]provider.ProviderConfig{
			"anthropic": {APIKey: "sk-test", Model: "claude"},
		},
	}, []string{"anthropic"})
	if !result.OK {
		t.Fatal("OK = false, want true: warnings must not fail doctor")
	}
	foundWarn := false
	for _, item := range result.Items {
		if item.Level == LevelWarn {
			foundWarn = true
		}
	}
	if !foundWarn {
		t.Fatal("no warn item, want permissions warning")
	}
}

func TestCheckConfigWarnOnlyStaysOK(t *testing.T) {
	result := CheckConfig("", config.Config{
		DefaultProvider: "anthropic",
		Providers: map[string]provider.ProviderConfig{
			"anthropic": {Model: "claude"},
		},
	}, []string{"anthropic"})
	if !result.OK {
		t.Fatalf("OK = false, want true for warn-only config: %+v", result.Items)
	}
}

type stubProvider struct {
	name   string
	models []provider.Model
	err    error
}

func (s stubProvider) Name() string { return s.name }
func (s stubProvider) Ask(_ context.Context, _ provider.ProviderConfig, _, _ string, _ io.Writer) error {
	return nil
}
func (s stubProvider) Models(_ context.Context, _ provider.ProviderConfig) ([]provider.Model, error) {
	return s.models, s.err
}

func TestCheckLiveFailureIsWarn(t *testing.T) {
	p := stubProvider{name: "ollama", err: errors.New("connection refused")}
	item := CheckLive(context.Background(), config.Config{
		DefaultProvider: "ollama",
		Providers:       map[string]provider.ProviderConfig{"ollama": {Model: "llama3"}},
	}, p)
	if item.Level != LevelWarn {
		t.Fatalf("Level = %q, want warn", item.Level)
	}
	if !strings.Contains(item.Message, "unreachable") {
		t.Fatalf("Message = %q, want unreachable", item.Message)
	}
}

func TestCheckLiveSuccessIsOK(t *testing.T) {
	p := stubProvider{name: "ollama", models: []provider.Model{{ID: "llama3"}}}
	item := CheckLive(context.Background(), config.Config{
		DefaultProvider: "ollama",
		Providers:       map[string]provider.ProviderConfig{"ollama": {Model: "llama3"}},
	}, p)
	if item.Level != LevelOK {
		t.Fatalf("Level = %q, want ok", item.Level)
	}
	if want := "live check: ollama: 1 models available"; item.Message != want {
		t.Fatalf("Message = %q, want %q", item.Message, want)
	}
}

type hangingProvider struct{ stubProvider }

func (s hangingProvider) Name() string { return "hang" }
func (s hangingProvider) Models(ctx context.Context, _ provider.ProviderConfig) ([]provider.Model, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

func TestCheckLiveTimeoutIsWarn(t *testing.T) {
	start := time.Now()
	item := CheckLive(context.Background(), config.Config{
		Providers: map[string]provider.ProviderConfig{"hang": {}},
	}, hangingProvider{})
	if elapsed := time.Since(start); elapsed > 10*time.Second {
		t.Fatalf("CheckLive took %v, want ~5s timeout bound", elapsed)
	}
	if item.Level != LevelWarn {
		t.Fatalf("Level = %q, want warn", item.Level)
	}
}
