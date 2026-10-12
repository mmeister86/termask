package config

import (
	"os"
	"path/filepath"
	"testing"
)

// Regression guard for the F11 deletion of Config.RenderMarkdown: TOML files
// in the wild may still contain `render_markdown` and must keep parsing
// (BurntSushi toml ignores unknown keys unless Strict mode is used).
func TestConfigWithLegacyRenderMarkdownParses(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	dir := filepath.Join(home, ".config", "termask")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	legacy := "default_provider = \"openai\"\nrender_markdown = true\n"
	if err := os.WriteFile(filepath.Join(dir, "config.toml"), []byte(legacy), 0600); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load with legacy render_markdown: %v", err)
	}
	if cfg.DefaultProvider != "openai" {
		t.Fatalf("DefaultProvider = %q, want openai", cfg.DefaultProvider)
	}
}
