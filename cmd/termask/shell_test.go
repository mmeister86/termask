package main

import (
	"strings"
	"testing"

	agentpkg "github.com/yourusername/termask/internal/agent"
)

func TestAskRendersByDefaultUnlessPlain(t *testing.T) {
	if !shouldRenderAskOutput(false) {
		t.Fatal("ask should render by default")
	}
	if shouldRenderAskOutput(true) {
		t.Fatal("ask --plain should disable rendering")
	}
}

func TestShellPluginsUseDefaultRenderedAskOutput(t *testing.T) {
	if !strings.Contains(zshPlugin(), "termask ask ") {
		t.Fatal("zsh plugin should call termask ask")
	}
	if strings.Contains(zshPlugin(), "--render") {
		t.Fatal("zsh plugin should not need --render anymore")
	}
	if strings.Contains(bashPlugin(), "--render") {
		t.Fatal("bash plugin should not need --render anymore")
	}
}

func TestAgentCommandExposesExpectedFlags(t *testing.T) {
	cmd := agentCmd()

	if cmd.Use != "agent [goal]" {
		t.Fatalf("Use = %q", cmd.Use)
	}
	for _, name := range []string{"provider", "max-steps", "file", "plain"} {
		if cmd.Flags().Lookup(name) == nil {
			t.Fatalf("agent command missing --%s flag", name)
		}
	}
}

func TestAgentRendersByDefaultUnlessPlain(t *testing.T) {
	if !shouldRenderAgentOutput(false) {
		t.Fatal("agent should render by default")
	}
	if shouldRenderAgentOutput(true) {
		t.Fatal("agent --plain should disable rendering")
	}
}

func TestAgentRunsSessionOnTerminalUnlessPlain(t *testing.T) {
	if !shouldRunAgentSession(false, false, true) {
		t.Fatal("agent without initial goal should start a terminal session")
	}
	if !shouldRunAgentSession(true, false, true) {
		t.Fatal("agent with initial goal should stay in a terminal session")
	}
	if shouldRunAgentSession(true, true, true) {
		t.Fatal("agent --plain should remain one-shot")
	}
	if shouldRunAgentSession(true, false, false) {
		t.Fatal("agent with piped stdin should remain one-shot")
	}
}

func TestFormatAgentStatusShowsThinkingAndToolEvents(t *testing.T) {
	model := formatAgentStatus(agentpkg.Event{Type: agentpkg.EventModelStart, Step: 2}, 8)
	if !strings.Contains(model, "thinking") {
		t.Fatalf("model status = %q, want thinking", model)
	}

	tool := formatAgentStatus(agentpkg.Event{
		Type: agentpkg.EventToolStart,
		Tool: "read_file",
		Args: map[string]string{"path": "README.md"},
	}, 8)
	if !strings.Contains(tool, "read_file") || !strings.Contains(tool, "README.md") {
		t.Fatalf("tool status = %q, want tool and args", tool)
	}

	done := formatAgentStatus(agentpkg.Event{
		Type:   agentpkg.EventToolEnd,
		Tool:   "read_file",
		Result: agentpkg.ToolResult{OK: true},
	}, 8)
	if !strings.Contains(done, "done") || !strings.Contains(done, "read_file") {
		t.Fatalf("tool done status = %q, want done", done)
	}

	answer := formatAgentStatus(agentpkg.Event{Type: agentpkg.EventAnswerDelta, Text: "hello"}, 8)
	if answer != "" {
		t.Fatalf("answer delta status = %q, want no status text", answer)
	}
}
