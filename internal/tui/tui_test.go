package tui

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"testing"

	"github.com/yourusername/termask/internal/agent"
	"github.com/yourusername/termask/internal/ask"
	"github.com/yourusername/termask/internal/config"
	"github.com/yourusername/termask/internal/provider"
)

func TestModelSwitchesModesAndCommandPalette(t *testing.T) {
	m := newTestModel(t)

	m, _ = m.handleKey("tab")
	if m.mode != modeAgent {
		t.Fatalf("mode = %v, want agent", m.mode)
	}

	m, _ = m.handleKey("ctrl+p")
	if !m.paletteOpen {
		t.Fatal("palette should open")
	}
	if len(m.paletteItems()) == 0 {
		t.Fatal("palette should contain commands")
	}

	m, _ = m.handleKey("esc")
	if m.paletteOpen {
		t.Fatal("palette should close on esc")
	}
}

func TestModelSendsChatPromptThroughRunner(t *testing.T) {
	m := newTestModel(t)
	m.chatRunner = fakeChatRunner(func(req ask.Request) (ask.Response, error) {
		if req.Query != "hello" {
			t.Fatalf("query = %q, want hello", req.Query)
		}
		return ask.Response{ProviderName: "anthropic", Model: "claude-test", Text: "hi back"}, nil
	})
	m.setInputValue("hello")

	var cmd command
	m, cmd = m.handleKey("enter")
	if cmd == nil {
		t.Fatal("enter should produce a chat command")
	}
	if !m.busy {
		t.Fatal("model should be busy while chat command runs")
	}

	msg := cmd()
	m, _ = m.updateMessage(msg)
	if m.busy {
		t.Fatal("model should no longer be busy after chat result")
	}
	if got := transcriptText(m); !strings.Contains(got, "hello") || !strings.Contains(got, "hi back") {
		t.Fatalf("transcript = %q, want user and assistant text", got)
	}
}

func TestModelHandlesSlashCommands(t *testing.T) {
	m := newTestModel(t)
	m.transcript = append(m.transcript, transcriptItem{Role: "assistant", Text: "old"})
	m.setInputValue("/provider openai")

	var cmd command
	m, cmd = m.handleKey("enter")
	if cmd != nil {
		t.Fatal("provider slash command should not start async request")
	}
	if m.providerName != "openai" || m.modelName != "gpt-test" {
		t.Fatalf("provider/model = %s/%s, want openai/gpt-test", m.providerName, m.modelName)
	}

	m.setInputValue("/new")
	m, _ = m.handleKey("enter")
	if len(m.transcript) != 1 || !strings.Contains(m.transcript[0].Text, "New chat session") {
		t.Fatalf("transcript after /new = %+v, want reset notice", m.transcript)
	}

	m, _ = m.runCommand("/mode agent")
	if m.mode != modeAgent {
		t.Fatalf("mode = %v, want agent after /mode agent", m.mode)
	}
	m, _ = m.runCommand("/mode chat")
	if m.mode != modeChat {
		t.Fatalf("mode = %v, want chat after /mode chat", m.mode)
	}
}

func TestModelInsertsMultilineWithAltEnterAndCtrlJ(t *testing.T) {
	m := newTestModel(t)
	m.setInputValue("line one")

	m, _ = m.handleKey("alt+enter")
	if got := m.inputValue(); got != "line one\n" {
		t.Fatalf("input after alt+enter = %q, want newline", got)
	}

	m.setInputValue("line two")
	m, _ = m.handleKey("ctrl+j")
	if got := m.inputValue(); got != "line two\n" {
		t.Fatalf("input after ctrl+j = %q, want newline", got)
	}
}

func TestModelAppliesAgentStreamEvents(t *testing.T) {
	m := newTestModel(t)
	m.mode = modeAgent
	m.busy = true

	m, _ = m.updateMessage(agentEventMsg{Event: agent.Event{Type: agent.EventModelStart, Step: 1}})
	m, _ = m.updateMessage(agentEventMsg{Event: agent.Event{
		Type: agent.EventToolStart,
		Tool: "read_file",
		Args: map[string]string{"path": "README.md"},
	}})
	m, _ = m.updateMessage(agentEventMsg{Event: agent.Event{Type: agent.EventAnswerDelta, Text: "partial answer"}})
	m, _ = m.updateMessage(agentDoneMsg{Response: agent.Response{Text: "partial answer"}})

	if m.busy {
		t.Fatal("model should no longer be busy after agent completion")
	}
	got := transcriptText(m)
	if !strings.Contains(got, "thinking step 1") || !strings.Contains(got, "read_file") || !strings.Contains(got, "partial answer") {
		t.Fatalf("transcript = %q, want status and answer deltas", got)
	}
}

func TestViewRendersResponsiveTermaskShell(t *testing.T) {
	m := newTestModel(t)
	m.width = 46
	m.height = 14
	out := stripANSI(m.render())

	for _, want := range []string{"termask", "Chat", "tab switch mode", "ctrl+p commands"} {
		if !strings.Contains(out, want) {
			t.Fatalf("view missing %q:\n%s", want, out)
		}
	}
	for _, line := range strings.Split(out, "\n") {
		visible := strings.TrimRight(line, " ")
		if len(visible) > 58 {
			t.Fatalf("line too wide (%d): %q\nfull view:\n%s", len(visible), line, out)
		}
	}
}

func TestChatErrorsReturnToInput(t *testing.T) {
	m := newTestModel(t)
	m.chatRunner = fakeChatRunner(func(req ask.Request) (ask.Response, error) {
		return ask.Response{}, errors.New("network down")
	})
	m.setInputValue("hello")

	var cmd command
	m, cmd = m.handleKey("enter")
	m, _ = m.updateMessage(cmd())

	if m.busy {
		t.Fatal("model should not remain busy after chat error")
	}
	if !strings.Contains(transcriptText(m), "network down") {
		t.Fatalf("transcript = %q, want error", transcriptText(m))
	}
}

type fakeChatRunner func(ask.Request) (ask.Response, error)

func (f fakeChatRunner) RunChat(_ context.Context, _ config.Config, req ask.Request) (ask.Response, error) {
	return f(req)
}

func newTestModel(t *testing.T) model {
	t.Helper()
	cfg := config.Default()
	cfg.DefaultProvider = "anthropic"
	cfg.Providers = map[string]provider.ProviderConfig{
		"anthropic": {Model: "claude-test"},
		"openai":    {Model: "gpt-test"},
	}
	m, err := newModel(context.Background(), cfg, modelOptions{
		chatRunner: fakeChatRunner(func(req ask.Request) (ask.Response, error) {
			return ask.Response{ProviderName: "anthropic", Model: "claude-test", Text: "ok"}, nil
		}),
		agentRunner: fakeAgentRunner{},
		workspace:   "/tmp/termask-test",
	})
	if err != nil {
		t.Fatal(err)
	}
	m.width = 80
	m.height = 24
	return m
}

type fakeAgentRunner struct{}

func (fakeAgentRunner) RunAgent(_ context.Context, _ agent.Request, emit func(agent.Event)) (agent.Response, error) {
	if emit != nil {
		emit(agent.Event{Type: agent.EventAnswerDelta, Text: "agent ok"})
	}
	return agent.Response{Text: "agent ok"}, nil
}

func transcriptText(m model) string {
	var out strings.Builder
	for _, item := range m.transcript {
		out.WriteString(item.Role)
		out.WriteString(":")
		out.WriteString(item.Text)
		out.WriteByte('\n')
	}
	return out.String()
}

func stripANSI(s string) string {
	re := regexp.MustCompile(`\x1b\[[0-9;]*[A-Za-z]`)
	return re.ReplaceAllString(s, "")
}
