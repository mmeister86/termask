package tui

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
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

func TestUpdateOpensCommandPaletteFromCtrlPKeyEvent(t *testing.T) {
	tests := []struct {
		name string
		key  tea.Key
	}{
		{name: "modified p", key: tea.Key{Code: 'p', Mod: tea.ModCtrl}},
		{name: "modified shifted p", key: tea.Key{Code: 'p', ShiftedCode: 'P', Mod: tea.ModCtrl}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newTestModel(t)
			m, _ = m.updateMessage(tea.KeyPressMsg(tt.key))
			if !m.paletteOpen {
				t.Fatal("ctrl+p key event should open the command palette")
			}
		})
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

	m, _ = runCommandToCompletion(t, m, cmd)
	if m.busy {
		t.Fatal("model should no longer be busy after chat result")
	}
	if got := transcriptText(m); !strings.Contains(got, "hello") || !strings.Contains(got, "hi back") {
		t.Fatalf("transcript = %q, want user and assistant text", got)
	}
}

func TestModelStreamsChatDeltasBeforeCompletion(t *testing.T) {
	m := newTestModel(t)
	release := make(chan struct{})
	m.chatRunner = fakeChatRunner(func(req ask.Request) (ask.Response, error) {
		if req.Out == nil {
			t.Fatal("chat request should provide a streaming writer")
		}
		if _, err := req.Out.Write([]byte("hel")); err != nil {
			t.Fatalf("write delta: %v", err)
		}
		<-release
		if _, err := req.Out.Write([]byte("lo")); err != nil {
			t.Fatalf("write delta: %v", err)
		}
		return ask.Response{ProviderName: "anthropic", Model: "claude-test", Text: "hello"}, nil
	})
	m.setInputValue("stream please")

	var cmd command
	m, cmd = m.handleKey("enter")
	start := cmd()
	m, cmd = m.updateMessage(start)
	if cmd == nil {
		t.Fatal("stream start should wait for chat events")
	}
	m, cmd = m.updateMessage(cmd())
	if got := transcriptText(m); !strings.Contains(got, "assistant:hel") {
		t.Fatalf("transcript after first delta = %q, want partial assistant text", got)
	}
	if !m.busy {
		t.Fatal("model should stay busy until chat completion")
	}

	close(release)
	for m.busy {
		if cmd == nil {
			t.Fatal("expected stream wait command while chat is busy")
		}
		var msg tea.Msg
		msg = cmd()
		m, cmd = m.updateMessage(msg)
	}
	if got := transcriptText(m); !strings.Contains(got, "assistant:hello") {
		t.Fatalf("transcript after completion = %q, want full streamed assistant text", got)
	}
	if len(m.session.Messages) != 2 || m.session.Messages[1].Content != "hello" {
		t.Fatalf("session messages = %+v, want saved full assistant response", m.session.Messages)
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

func TestModelPassesPrintableKeysToTextarea(t *testing.T) {
	m := newTestModel(t)

	m, _ = m.updateMessage(tea.KeyPressMsg(tea.Key{Text: "a", Code: 'a'}))
	m, _ = m.updateMessage(tea.KeyPressMsg(tea.Key{Text: "b", Code: 'b'}))

	if got := m.inputValue(); got != "ab" {
		t.Fatalf("input value = %q, want typed characters", got)
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

func TestRenderFillsWideTerminalViewport(t *testing.T) {
	m := newTestModel(t)
	m.width = 180
	m.height = 36

	out := stripANSI(m.render())
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) != m.height {
		t.Fatalf("rendered height = %d, want %d\n%s", len(lines), m.height, out)
	}
	for i, line := range lines {
		if lipgloss.Width(line) != m.width {
			t.Fatalf("line %d width = %d, want %d: %q", i, lipgloss.Width(line), m.width, line)
		}
	}
	if longestContentLine(out) > 132 {
		t.Fatalf("content stretches too wide: longest visible line = %d\n%s", longestContentLine(out), out)
	}
}

func TestRenderDoesNotResetStyledLinesBeforeTrailingPadding(t *testing.T) {
	m := newTestModel(t)
	m.width = 180
	m.height = 36

	view := m.render()
	for i, line := range strings.Split(strings.TrimRight(view, "\n"), "\n") {
		if hasUnstyledTrailingPadding(line) {
			t.Fatalf("line %d resets colors before trailing padding, causing terminal-background gaps: %q", i, line)
		}
	}
}

func TestRenderInputCentersHintsUnderInputBox(t *testing.T) {
	m := newTestModel(t)

	input := stripANSI(m.renderInput(104))
	for _, line := range strings.Split(input, "\n") {
		if !strings.Contains(line, "ctrl+p commands") {
			continue
		}
		leading := len(line) - len(strings.TrimLeft(line, " "))
		trailing := len(line) - len(strings.TrimRight(line, " "))
		if diff := abs(leading - trailing); diff > 1 {
			t.Fatalf("hint line should be centered under input box, leading=%d trailing=%d: %q", leading, trailing, line)
		}
		return
	}
	t.Fatalf("input hints missing from render:\n%s", input)
}

func TestTranscriptCanScrollBackThroughOlderOutput(t *testing.T) {
	m := newTestModel(t)
	for i := 0; i < 24; i++ {
		m.transcript = append(m.transcript, transcriptItem{Role: "assistant", Text: "message " + twoDigits(i)})
	}

	bottom := stripANSI(m.renderTranscript(80, 6))
	if strings.Contains(bottom, "message 00") {
		t.Fatalf("bottom transcript should start near the newest output:\n%s", bottom)
	}

	m, _ = m.updateMessage(tea.KeyPressMsg(tea.Key{Code: tea.KeyPgUp}))
	scrolled := stripANSI(m.renderTranscript(80, 6))
	if !strings.Contains(scrolled, "message 09") {
		t.Fatalf("page up should reveal older transcript output:\n%s", scrolled)
	}
}

func TestTranscriptMouseWheelScrollsOutput(t *testing.T) {
	m := newTestModel(t)
	for i := 0; i < 24; i++ {
		m.transcript = append(m.transcript, transcriptItem{Role: "assistant", Text: "message " + twoDigits(i)})
	}

	m, _ = m.updateMessage(tea.MouseWheelMsg(tea.Mouse{Button: tea.MouseWheelUp}))
	scrolled := stripANSI(m.renderTranscript(80, 6))
	if !strings.Contains(scrolled, "message 15") {
		t.Fatalf("mouse wheel up should reveal older transcript output:\n%s", scrolled)
	}

	m, _ = m.updateMessage(tea.MouseWheelMsg(tea.Mouse{Button: tea.MouseWheelDown}))
	bottom := stripANSI(m.renderTranscript(80, 6))
	if !strings.Contains(bottom, "message 23") {
		t.Fatalf("mouse wheel down should return toward newest transcript output:\n%s", bottom)
	}
}

func TestTranscriptUsesCompactMessageSpacing(t *testing.T) {
	m := newTestModel(t)
	m.transcript = []transcriptItem{
		{Role: "user", Text: "first"},
		{Role: "assistant", Text: "second"},
		{Role: "user", Text: "third"},
	}

	out := stripANSI(m.renderTranscript(80, 8))
	if hasBlankVisualLineBetweenContent(out) {
		t.Fatalf("transcript should not insert blank lines between short messages:\n%s", out)
	}
}

func TestTranscriptKeepsMarkdownCodeBlocksNearIntroText(t *testing.T) {
	m := newTestModel(t)
	m.transcript = []transcriptItem{
		{Role: "user", Text: "Wie wird das Wetter in Crimmitschau"},
		{Role: "assistant", Text: "Ich kann das aktuelle Wetter nicht direkt abrufen.\n\nz. B.:\n\n```bash\ncurl wttr.in/Crimmitschau\n```"},
	}

	out := stripANSI(m.renderTranscript(100, 18))
	lines := strings.Split(out, "\n")
	introLine := lineIndexContaining(lines, "z. B.:")
	codeLine := lineIndexContaining(lines, "curl wttr.in/Crimmitschau")
	if introLine < 0 || codeLine < 0 {
		t.Fatalf("rendered transcript missing intro/code lines:\n%s", out)
	}
	if gap := codeLine - introLine; gap > 3 {
		t.Fatalf("code block is too far from intro, gap=%d:\n%s", gap, out)
	}
}

func TestRenderUsesCompactIdleLayoutOnShortTerminals(t *testing.T) {
	m := newTestModel(t)
	m.width = 92
	m.height = 18

	out := stripANSI(m.render())
	if strings.Contains(out, "████") {
		t.Fatalf("short terminal should use compact logo:\n%s", out)
	}
	for _, want := range []string{"termask", "Ask anything", "tab switch mode", "/tmp/termask-test"} {
		if !strings.Contains(out, want) {
			t.Fatalf("short render missing %q:\n%s", want, out)
		}
	}
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) != m.height {
		t.Fatalf("rendered height = %d, want %d\n%s", len(lines), m.height, out)
	}
}

func TestRenderKeepsNarrowLayoutInsideViewport(t *testing.T) {
	m := newTestModel(t)
	m.width = 42
	m.height = 16

	out := stripANSI(m.render())
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) != m.height {
		t.Fatalf("rendered height = %d, want %d\n%s", len(lines), m.height, out)
	}
	for i, line := range lines {
		if lipgloss.Width(line) != m.width {
			t.Fatalf("line %d width = %d, want %d: %q\n%s", i, lipgloss.Width(line), m.width, line, out)
		}
	}
	for _, want := range []string{"termask", "Chat", "ctrl+p"} {
		if !strings.Contains(out, want) {
			t.Fatalf("narrow render missing %q:\n%s", want, out)
		}
	}
}

func TestViewSetsTerminalBackgroundColor(t *testing.T) {
	m := newTestModel(t)

	view := m.View()
	if view.BackgroundColor == nil {
		t.Fatal("view should set a terminal background color to avoid default-background gaps")
	}
	if view.ForegroundColor == nil {
		t.Fatal("view should set a terminal foreground color for consistent reset behavior")
	}
	if view.MouseMode != tea.MouseModeCellMotion {
		t.Fatal("view should enable mouse wheel events for transcript scrolling")
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
	m, _ = runCommandToCompletion(t, m, cmd)

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

func runCommandToCompletion(t *testing.T, m model, cmd command) (model, command) {
	t.Helper()
	for cmd != nil {
		msg := cmd()
		m, cmd = m.updateMessage(msg)
		if !m.busy {
			return m, cmd
		}
	}
	return m, cmd
}

func stripANSI(s string) string {
	re := regexp.MustCompile(`\x1b\[[0-9;]*[A-Za-z]?`)
	return re.ReplaceAllString(s, "")
}

func longestContentLine(s string) int {
	longest := 0
	for _, line := range strings.Split(stripANSI(s), "\n") {
		visible := lipgloss.Width(strings.TrimSpace(line))
		if visible > longest {
			longest = visible
		}
	}
	return longest
}

func hasUnstyledTrailingPadding(line string) bool {
	re := regexp.MustCompile(`\x1b\[(?:0)?m {2,}(?:\x1b\[(?:0)?m)?$`)
	return re.MatchString(line)
}

func hasBlankVisualLineBetweenContent(out string) bool {
	sawContent := false
	sawBlankAfterContent := false
	for _, line := range strings.Split(out, "\n") {
		if strings.TrimSpace(line) == "" {
			if sawContent {
				sawBlankAfterContent = true
			}
			continue
		}
		if sawBlankAfterContent {
			return true
		}
		sawContent = true
	}
	return false
}

func lineIndexContaining(lines []string, needle string) int {
	for i, line := range lines {
		if strings.Contains(line, needle) {
			return i
		}
	}
	return -1
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func twoDigits(v int) string {
	if v < 10 {
		return "0" + string(rune('0'+v))
	}
	return string(rune('0'+v/10)) + string(rune('0'+v%10))
}
