package agent

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadFileRejectsPathOutsideWorkspace(t *testing.T) {
	workspace := t.TempDir()
	outside := filepath.Join(t.TempDir(), "secret.txt")
	if err := os.WriteFile(outside, []byte("secret"), 0644); err != nil {
		t.Fatal(err)
	}

	tools := NewToolset(workspace)
	result := tools.Execute(context.Background(), ToolCall{
		Tool: "read_file",
		Args: map[string]string{"path": outside},
	})

	if result.OK {
		t.Fatal("read_file outside workspace succeeded")
	}
	if !strings.Contains(result.Error, "outside workspace") {
		t.Fatalf("error = %q, want outside workspace", result.Error)
	}
}

func TestReadFileRejectsBinaryAndOversizedFiles(t *testing.T) {
	workspace := t.TempDir()
	binaryPath := filepath.Join(workspace, "bin.dat")
	largePath := filepath.Join(workspace, "large.txt")
	if err := os.WriteFile(binaryPath, []byte{'a', 0, 'b'}, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(largePath, []byte("abcdef"), 0644); err != nil {
		t.Fatal(err)
	}

	tools := NewToolset(workspace)
	tools.MaxFileBytes = 3

	binary := tools.Execute(context.Background(), ToolCall{
		Tool: "read_file",
		Args: map[string]string{"path": "bin.dat"},
	})
	if binary.OK || !strings.Contains(binary.Error, "binary") {
		t.Fatalf("binary result = %+v, want binary rejection", binary)
	}

	large := tools.Execute(context.Background(), ToolCall{
		Tool: "read_file",
		Args: map[string]string{"path": "large.txt"},
	})
	if large.OK || !strings.Contains(large.Error, "over limit") {
		t.Fatalf("large result = %+v, want size rejection", large)
	}
}

func TestRunCheckAllowsOnlyGoTestCommands(t *testing.T) {
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "go.mod"), []byte("module example.com/test\n\ngo 1.25\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "main_test.go"), []byte("package test\n\nimport \"testing\"\n\nfunc TestExample(t *testing.T) {}\n"), 0644); err != nil {
		t.Fatal(err)
	}

	tools := NewToolset(workspace)
	rejected := tools.Execute(context.Background(), ToolCall{
		Tool: "run_check",
		Args: map[string]string{"command": "rm -rf ."},
	})
	if rejected.OK {
		t.Fatal("mutating command succeeded")
	}
	if !strings.Contains(rejected.Error, "not allowlisted") {
		t.Fatalf("error = %q, want allowlist rejection", rejected.Error)
	}

	allowed := tools.Execute(context.Background(), ToolCall{
		Tool: "run_check",
		Args: map[string]string{"command": "go test ./..."},
	})
	if !allowed.OK {
		t.Fatalf("go test command failed: %+v", allowed)
	}
}

func TestToolsetSymlinkEscapeDenied(t *testing.T) {
	workspace := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside-secret.txt")
	if err := os.WriteFile(outside, []byte("topsecret"), 0644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(workspace, "link.txt")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatal(err)
	}

	tools := NewToolset(workspace)
	result := tools.Execute(context.Background(), ToolCall{
		Tool: "read_file",
		Args: map[string]string{"path": "link.txt"},
	})
	if result.OK {
		t.Fatal("read_file via symlink escape succeeded")
	}
	if !strings.Contains(result.Error, "outside workspace") {
		t.Fatalf("error = %q, want outside workspace", result.Error)
	}
}

func TestToolsetSecretFileDenied(t *testing.T) {
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, ".env"), []byte("KEY=secret"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "ok.txt"), []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}

	tools := NewToolset(workspace)
	denied := tools.Execute(context.Background(), ToolCall{
		Tool: "read_file",
		Args: map[string]string{"path": ".env"},
	})
	if denied.OK {
		t.Fatal("read_file .env succeeded")
	}

	allowed := tools.Execute(context.Background(), ToolCall{
		Tool: "read_file",
		Args: map[string]string{"path": "ok.txt"},
	})
	if !allowed.OK {
		t.Fatalf("normal file read failed: %+v", allowed)
	}
	if !strings.Contains(allowed.Output, "hello") {
		t.Fatalf("normal file output = %q, want hello", allowed.Output)
	}
}

func TestToolsetSearchTextExcludesSecrets(t *testing.T) {
	workspace := t.TempDir()
	secretValue := "SUPERSECRET-9f8a7b6c5d"
	tomlSecret := "TOMLSECRET-1a2b3c4d5e"
	if err := os.WriteFile(filepath.Join(workspace, ".env"), []byte("KEY="+secretValue+"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	// config.toml is a non-hidden secret basename: rg finds it workspace-wide.
	if err := os.WriteFile(filepath.Join(workspace, "config.toml"), []byte("token = \""+tomlSecret+"\"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "ok.txt"), []byte("nothing interesting\n"), 0644); err != nil {
		t.Fatal(err)
	}

	tools := NewToolset(workspace)
	for _, secret := range []string{secretValue, tomlSecret} {
		wide := tools.Execute(context.Background(), ToolCall{
			Tool: "search_text",
			Args: map[string]string{"pattern": secret},
		})
		if strings.Contains(wide.Output, secret) {
			t.Fatalf("workspace search leaked secret: %q", wide.Output)
		}
	}

	explicit := tools.Execute(context.Background(), ToolCall{
		Tool: "search_text",
		Args: map[string]string{"pattern": secretValue, "path": ".env"},
	})
	if explicit.OK {
		t.Fatalf("search_text with explicit secret path succeeded: %q", explicit.Output)
	}
}

func TestToolsetSearchTextPatternAsFlag(t *testing.T) {
	workspace := t.TempDir()
	marker := "MARKER-7f3a9b2c1e"
	if err := os.WriteFile(filepath.Join(workspace, "config.toml"), []byte("token = \""+marker+"\"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "ok.txt"), []byte("nothing interesting\n"), 0644); err != nil {
		t.Fatal(err)
	}

	tools := NewToolset(workspace)
	// Pattern that looks like an rg flag must be treated as a literal pattern.
	injected := tools.Execute(context.Background(), ToolCall{
		Tool: "search_text",
		Args: map[string]string{"pattern": "--no-filename", "path": "."},
	})
	if strings.Contains(injected.Output, marker) {
		t.Fatalf("pattern-as-flag leaked secret: %q", injected.Output)
	}
}

func TestIsSecretPathPatterns(t *testing.T) {
	denied := []string{".env", ".env.local", "server.pem", "server.key", "id_rsa", "id_rsa.pub", "config.toml", "sub/dir/.env"}
	for _, p := range denied {
		if !isSecretPath(p) {
			t.Errorf("isSecretPath(%q) = false, want true", p)
		}
	}
	allowed := []string{"ok.txt", "env.go", "mykey.txt", "my_id_rsa.txt"}
	for _, p := range allowed {
		if isSecretPath(p) {
			t.Errorf("isSecretPath(%q) = true, want false", p)
		}
	}
}

func TestUnknownToolReturnsToolError(t *testing.T) {
	tools := NewToolset(t.TempDir())
	result := tools.Execute(context.Background(), ToolCall{Tool: "write_file"})

	if result.OK {
		t.Fatal("unknown tool succeeded")
	}
	if !strings.Contains(result.Error, "unknown tool") {
		t.Fatalf("error = %q, want unknown tool", result.Error)
	}
}
