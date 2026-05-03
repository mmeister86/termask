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
