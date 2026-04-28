package doctor

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/yourusername/termask/internal/config"
	"github.com/yourusername/termask/internal/provider"
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
	if result.OK {
		t.Fatal("OK = true, want false because permissions are loose")
	}
}
