package agent

import (
	"bytes"
	"context"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const defaultMaxFileBytes int64 = 128 * 1024
const maxToolOutputBytes = 64 * 1024

type Toolset struct {
	Workspace    string
	MaxFileBytes int64
}

func NewToolset(workspace string) *Toolset {
	return &Toolset{Workspace: workspace, MaxFileBytes: defaultMaxFileBytes}
}

func (t *Toolset) Execute(ctx context.Context, call ToolCall) ToolResult {
	switch call.Tool {
	case "list_files":
		return t.listFiles(ctx, call.Args["pattern"])
	case "read_file":
		return t.readFile(call.Args["path"])
	case "search_text":
		return t.searchText(ctx, call.Args["pattern"], call.Args["path"])
	case "git_status":
		return t.gitStatus(ctx)
	case "git_diff":
		return t.gitDiff(ctx, call.Args["path"])
	case "run_check":
		return t.runCheck(ctx, call.Args["command"])
	default:
		return ToolResult{Tool: call.Tool, OK: false, Error: "unknown tool: " + call.Tool}
	}
}

func (t *Toolset) listFiles(ctx context.Context, pattern string) ToolResult {
	args := []string{"--files"}
	cmd := exec.CommandContext(ctx, "rg", args...)
	cmd.Dir = t.Workspace
	out, err := cmd.Output()
	if err != nil {
		out, err = t.walkFiles(pattern)
		if err != nil {
			return ToolResult{Tool: "list_files", OK: false, Error: err.Error()}
		}
	} else if pattern != "" {
		out = filterLines(out, pattern)
	}
	return ToolResult{Tool: "list_files", OK: true, Output: limitOutput(string(out))}
}

func (t *Toolset) readFile(path string) ToolResult {
	full, rel, err := t.resolvePath(path)
	if err != nil {
		return ToolResult{Tool: "read_file", OK: false, Error: err.Error()}
	}
	info, err := os.Stat(full)
	if err != nil {
		return ToolResult{Tool: "read_file", OK: false, Error: err.Error()}
	}
	if info.IsDir() {
		return ToolResult{Tool: "read_file", OK: false, Error: rel + " is a directory"}
	}
	if info.Size() > t.MaxFileBytes {
		return ToolResult{Tool: "read_file", OK: false, Error: fmt.Sprintf("%s is %d bytes, over limit %d", rel, info.Size(), t.MaxFileBytes)}
	}
	data, err := os.ReadFile(full)
	if err != nil {
		return ToolResult{Tool: "read_file", OK: false, Error: err.Error()}
	}
	if bytes.IndexByte(data, 0) >= 0 {
		return ToolResult{Tool: "read_file", OK: false, Error: rel + " appears to be binary"}
	}
	return ToolResult{Tool: "read_file", OK: true, Output: fmt.Sprintf("--- File: %s ---\n%s", rel, string(data))}
}

func (t *Toolset) searchText(ctx context.Context, pattern, path string) ToolResult {
	if strings.TrimSpace(pattern) == "" {
		return ToolResult{Tool: "search_text", OK: false, Error: "pattern is required"}
	}
	args := []string{"-n", pattern}
	if path != "" {
		full, rel, err := t.resolvePath(path)
		if err != nil {
			return ToolResult{Tool: "search_text", OK: false, Error: err.Error()}
		}
		info, err := os.Stat(full)
		if err != nil {
			return ToolResult{Tool: "search_text", OK: false, Error: err.Error()}
		}
		if info.IsDir() {
			args = append(args, rel)
		} else {
			args = append(args, rel)
		}
	}
	cmd := exec.CommandContext(ctx, "rg", args...)
	cmd.Dir = t.Workspace
	out, err := cmd.CombinedOutput()
	if err != nil && len(out) == 0 {
		return ToolResult{Tool: "search_text", OK: true, Output: ""}
	}
	return ToolResult{Tool: "search_text", OK: true, Output: limitOutput(string(out))}
}

func (t *Toolset) gitStatus(ctx context.Context) ToolResult {
	return t.runReadOnly(ctx, "git_status", "git", "status", "--short")
}

func (t *Toolset) gitDiff(ctx context.Context, path string) ToolResult {
	args := []string{"diff", "--"}
	if path != "" {
		_, rel, err := t.resolvePath(path)
		if err != nil {
			return ToolResult{Tool: "git_diff", OK: false, Error: err.Error()}
		}
		args = append(args, rel)
	}
	return t.runReadOnly(ctx, "git_diff", "git", args...)
}

func (t *Toolset) runCheck(ctx context.Context, command string) ToolResult {
	fields := strings.Fields(command)
	if !allowGoTest(fields) {
		return ToolResult{Tool: "run_check", OK: false, Error: "command is not allowlisted"}
	}
	return t.runReadOnly(ctx, "run_check", fields[0], fields[1:]...)
}

func (t *Toolset) runReadOnly(ctx context.Context, tool, name string, args ...string) ToolResult {
	runCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(runCtx, name, args...)
	cmd.Dir = t.Workspace
	out, err := cmd.CombinedOutput()
	if err != nil {
		return ToolResult{Tool: tool, OK: false, Output: limitOutput(string(out)), Error: err.Error()}
	}
	return ToolResult{Tool: tool, OK: true, Output: limitOutput(string(out))}
}

func (t *Toolset) resolvePath(path string) (string, string, error) {
	if strings.TrimSpace(path) == "" {
		return "", "", fmt.Errorf("path is required")
	}
	workspace, err := filepath.Abs(t.Workspace)
	if err != nil {
		return "", "", err
	}
	candidate := path
	if !filepath.IsAbs(candidate) {
		candidate = filepath.Join(workspace, candidate)
	}
	full, err := filepath.Abs(candidate)
	if err != nil {
		return "", "", err
	}
	rel, err := filepath.Rel(workspace, full)
	if err != nil {
		return "", "", err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || filepath.IsAbs(rel) {
		return "", "", fmt.Errorf("%s is outside workspace", path)
	}
	return full, filepath.ToSlash(rel), nil
}

func allowGoTest(fields []string) bool {
	if len(fields) < 2 || fields[0] != "go" || fields[1] != "test" {
		return false
	}
	expectRunPattern := false
	for _, field := range fields[2:] {
		if expectRunPattern {
			if strings.HasPrefix(field, "-") || strings.ContainsAny(field, ";&|><`$") {
				return false
			}
			expectRunPattern = false
			continue
		}
		switch {
		case field == "-run":
			expectRunPattern = true
		case field == "./..." || strings.HasPrefix(field, "./"):
			continue
		default:
			return false
		}
	}
	return !expectRunPattern
}

func (t *Toolset) walkFiles(pattern string) ([]byte, error) {
	var out strings.Builder
	err := filepath.WalkDir(t.Workspace, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", "node_modules", "target", "dist":
				return filepath.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(t.Workspace, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if pattern == "" || strings.Contains(rel, pattern) {
			out.WriteString(rel)
			out.WriteByte('\n')
		}
		return nil
	})
	return []byte(out.String()), err
}

func filterLines(data []byte, pattern string) []byte {
	var out strings.Builder
	for _, line := range strings.Split(string(data), "\n") {
		if line == "" {
			continue
		}
		if strings.Contains(line, pattern) {
			out.WriteString(line)
			out.WriteByte('\n')
		}
	}
	return []byte(out.String())
}

func limitOutput(text string) string {
	if len(text) <= maxToolOutputBytes {
		return text
	}
	return text[:maxToolOutputBytes] + "\n[output truncated]\n"
}
