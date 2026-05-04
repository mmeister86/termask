package tui

import (
	"context"
	"fmt"
	"image/color"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/yourusername/termask/internal/agent"
	"github.com/yourusername/termask/internal/ask"
	"github.com/yourusername/termask/internal/config"
	"github.com/yourusername/termask/internal/history"
	"github.com/yourusername/termask/internal/provider"
)

type command = tea.Cmd

type mode int

const (
	modeChat mode = iota
	modeAgent
)

func (m mode) String() string {
	if m == modeAgent {
		return "Agent"
	}
	return "Chat"
}

type chatRunner interface {
	RunChat(context.Context, config.Config, ask.Request) (ask.Response, error)
}

type agentRunner interface {
	RunAgent(context.Context, agent.Request, func(agent.Event)) (agent.Response, error)
}

type realChatRunner struct{}

func (realChatRunner) RunChat(ctx context.Context, cfg config.Config, req ask.Request) (ask.Response, error) {
	return ask.Run(ctx, cfg, req)
}

type realAgentRunner struct {
	runner *agent.Agent
}

func newRealAgentRunner(ctx context.Context, cfg config.Config, providerName, workspace string) (realAgentRunner, error) {
	model, err := agent.NewProviderModel(ctx, cfg, providerName)
	if err != nil {
		return realAgentRunner{}, err
	}
	return realAgentRunner{runner: agent.New(model, agent.NewToolset(workspace))}, nil
}

func (r realAgentRunner) RunAgent(ctx context.Context, req agent.Request, emit func(agent.Event)) (agent.Response, error) {
	return r.runner.RunStream(ctx, req, emit)
}

type modelOptions struct {
	chatRunner  chatRunner
	agentRunner agentRunner
	workspace   string
	store       historyStore
}

type historyStore interface {
	Save(history.Session) error
	List() ([]history.Session, error)
}

type transcriptItem struct {
	Role string
	Text string
}

type model struct {
	ctx context.Context
	cfg config.Config

	mode         mode
	providerName string
	modelName    string
	workspace    string
	gitBranch    string
	version      string

	width  int
	height int
	input  textarea.Model
	spin   spinner.Model

	busy             bool
	status           string
	errText          string
	paletteOpen      bool
	paletteCursor    int
	transcriptScroll int
	transcript       []transcriptItem

	chatRunner  chatRunner
	agentRunner agentRunner
	store       historyStore
	session     history.Session
	agentPrior  []agent.Message
	stream      <-chan tea.Msg
}

type chatDoneMsg struct {
	Query    string
	Response ask.Response
	Err      error
}

type chatStreamStartedMsg struct {
	Events <-chan tea.Msg
}

type chatDeltaMsg struct {
	Text string
}

type agentStreamStartedMsg struct {
	Events <-chan tea.Msg
}

type agentEventMsg struct {
	Event agent.Event
}

type agentDoneMsg struct {
	Goal     string
	Response agent.Response
	Err      error
}

type teaMsgWriter struct {
	ch chan<- tea.Msg
}

func (w teaMsgWriter) Write(p []byte) (int, error) {
	if len(p) > 0 {
		w.ch <- chatDeltaMsg{Text: string(p)}
	}
	return len(p), nil
}

var _ io.Writer = teaMsgWriter{}

func Run(ctx context.Context, cfg config.Config) error {
	m, err := newModel(ctx, cfg, modelOptions{})
	if err != nil {
		return err
	}
	_, err = tea.NewProgram(m).Run()
	return err
}

func newModel(ctx context.Context, cfg config.Config, opts modelOptions) (model, error) {
	providerName, pcfg, err := cfg.ActiveProviderConfig("")
	if err != nil {
		return model{}, err
	}
	workspace := opts.workspace
	if workspace == "" {
		workspace, err = os.Getwd()
		if err != nil {
			return model{}, err
		}
	}
	ta := textarea.New()
	ta.Placeholder = `Ask anything... "What is the tech stack of this project?"`
	ta.ShowLineNumbers = false
	ta.Prompt = ""
	ta.SetWidth(72)
	ta.SetHeight(3)
	ta.Focus()
	ta.SetStyles(textareaStyles())

	s := spinner.New(spinner.WithSpinner(spinner.MiniDot), spinner.WithStyle(accentStyle))
	chat := opts.chatRunner
	if chat == nil {
		chat = realChatRunner{}
	}
	agentRun := opts.agentRunner
	if agentRun == nil {
		real, err := newRealAgentRunner(ctx, cfg, providerName, workspace)
		if err != nil {
			return model{}, err
		}
		agentRun = real
	}
	store := opts.store
	if store == nil && cfg.HistoryEnabled {
		path, err := history.DefaultPath()
		if err != nil {
			return model{}, err
		}
		store = history.NewStore(path)
	}

	return model{
		ctx:          ctx,
		cfg:          cfg,
		mode:         modeChat,
		providerName: providerName,
		modelName:    pcfg.Model,
		workspace:    workspace,
		gitBranch:    gitBranch(workspace),
		version:      "dev",
		width:        100,
		height:       30,
		input:        ta,
		spin:         s,
		status:       "Ready",
		chatRunner:   chat,
		agentRunner:  agentRun,
		store:        store,
		session:      history.NewSession(providerName, pcfg.Model),
	}, nil
}

func (m model) Init() tea.Cmd {
	return tea.Batch(textarea.Blink, m.spin.Tick)
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	next, cmd := m.updateMessage(msg)
	return next, cmd
}

func (m model) updateMessage(msg tea.Msg) (model, command) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.resizeInput()
		return m, nil
	case tea.KeyPressMsg:
		key := normalizedKey(msg)
		if m.paletteOpen || isShortcutKey(key) {
			return m.handleKey(key)
		}
	case tea.MouseWheelMsg:
		switch msg.Mouse().Button {
		case tea.MouseWheelUp:
			m.scrollTranscript(scrollStep())
		case tea.MouseWheelDown:
			m.scrollTranscript(-scrollStep())
		}
		return m, nil
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spin, cmd = m.spin.Update(msg)
		if m.busy {
			return m, cmd
		}
		return m, nil
	case chatStreamStartedMsg:
		m.stream = msg.Events
		return m, waitForStream(msg.Events)
	case chatDeltaMsg:
		m.appendAssistantDelta(msg.Text)
		if m.stream != nil {
			return m, waitForStream(m.stream)
		}
		return m, nil
	case chatDoneMsg:
		return m.applyChatDone(msg), nil
	case agentStreamStartedMsg:
		m.stream = msg.Events
		return m, waitForStream(msg.Events)
	case agentEventMsg:
		m.applyAgentEvent(msg.Event)
		if m.stream != nil {
			return m, waitForStream(m.stream)
		}
		return m, nil
	case agentDoneMsg:
		return m.applyAgentDone(msg), nil
	}
	if !m.busy && !m.paletteOpen {
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m model) handleKey(key string) (model, command) {
	if m.paletteOpen {
		return m.handlePaletteKey(key)
	}
	switch key {
	case "ctrl+c", "esc":
		return m, tea.Quit
	case "tab":
		if m.mode == modeChat {
			m.mode = modeAgent
		} else {
			m.mode = modeChat
		}
		m.status = "Switched to " + m.mode.String()
		return m, nil
	case "ctrl+p":
		m.paletteOpen = true
		m.paletteCursor = 0
		return m, nil
	case "pgup":
		m.scrollTranscript(m.transcriptPageSize())
		return m, nil
	case "pgdown":
		m.scrollTranscript(-m.transcriptPageSize())
		return m, nil
	case "home":
		m.scrollTranscript(m.maxTranscriptScroll())
		return m, nil
	case "end":
		m.transcriptScroll = 0
		return m, nil
	case "alt+enter", "ctrl+j":
		m.input.InsertString("\n")
		return m, nil
	case "enter":
		return m.submit()
	}
	return m, nil
}

func (m model) handlePaletteKey(key string) (model, command) {
	switch key {
	case "esc", "ctrl+c", "ctrl+p":
		m.paletteOpen = false
		return m, nil
	case "up", "ctrl+k":
		if m.paletteCursor > 0 {
			m.paletteCursor--
		}
		return m, nil
	case "down", "ctrl+j":
		if m.paletteCursor < len(m.paletteItems())-1 {
			m.paletteCursor++
		}
		return m, nil
	case "enter":
		items := m.paletteItems()
		if len(items) == 0 {
			m.paletteOpen = false
			return m, nil
		}
		m.paletteOpen = false
		return m.runCommand(items[m.paletteCursor].Command)
	}
	return m, nil
}

func (m model) submit() (model, command) {
	if m.busy {
		return m, nil
	}
	query := strings.TrimSpace(m.input.Value())
	if query == "" {
		return m, nil
	}
	m.input.SetValue("")
	if strings.HasPrefix(query, "/") {
		return m.runCommand(query)
	}
	m.transcript = append(m.transcript, transcriptItem{Role: "user", Text: query})
	m.transcriptScroll = 0
	m.busy = true
	m.errText = ""
	if m.mode == modeAgent {
		m.status = "Agent thinking"
		return m, m.runAgent(query)
	}
	m.status = "Chat thinking"
	return m, m.runChat(query)
}

func (m model) runChat(query string) command {
	historyMessages := sessionMessages(m.session)
	return func() tea.Msg {
		ch := make(chan tea.Msg, 32)
		go func() {
			resp, err := m.chatRunner.RunChat(m.ctx, m.cfg, ask.Request{
				ProviderName: m.providerName,
				Query:        query,
				History:      historyMessages,
				Out:          teaMsgWriter{ch: ch},
			})
			ch <- chatDoneMsg{Query: query, Response: resp, Err: err}
			close(ch)
		}()
		return chatStreamStartedMsg{Events: ch}
	}
}

func (m model) runAgent(goal string) command {
	prior := append([]agent.Message{}, m.agentPrior...)
	return func() tea.Msg {
		ch := make(chan tea.Msg, 32)
		go func() {
			resp, err := m.agentRunner.RunAgent(m.ctx, agent.Request{
				Goal:     goal,
				MaxSteps: agent.DefaultMaxSteps,
				History:  prior,
			}, func(event agent.Event) {
				ch <- agentEventMsg{Event: event}
			})
			ch <- agentDoneMsg{Goal: goal, Response: resp, Err: err}
			close(ch)
		}()
		return agentStreamStartedMsg{Events: ch}
	}
}

func waitForStream(ch <-chan tea.Msg) tea.Cmd {
	return func() tea.Msg {
		msg, ok := <-ch
		if !ok {
			return nil
		}
		return msg
	}
}

func (m model) applyChatDone(msg chatDoneMsg) model {
	m.busy = false
	m.stream = nil
	if msg.Err != nil {
		m.errText = msg.Err.Error()
		m.status = "Chat error"
		m.transcript = append(m.transcript, transcriptItem{Role: "error", Text: msg.Err.Error()})
		return m
	}
	m.providerName = msg.Response.ProviderName
	m.modelName = msg.Response.Model
	m.session.Provider = msg.Response.ProviderName
	m.session.Model = msg.Response.Model
	m.session.AddUser(msg.Query)
	m.session.AddAssistant(msg.Response.Text)
	if m.store != nil {
		if err := m.store.Save(m.session); err != nil {
			m.transcript = append(m.transcript, transcriptItem{Role: "error", Text: "history: " + err.Error()})
		}
	}
	m.ensureAssistantText(msg.Response.Text)
	m.status = "Ready"
	return m
}

func (m *model) applyAgentEvent(event agent.Event) {
	switch event.Type {
	case agent.EventModelStart:
		m.transcript = append(m.transcript, transcriptItem{Role: "status", Text: fmt.Sprintf("thinking step %d/%d", event.Step, agent.DefaultMaxSteps)})
	case agent.EventToolStart:
		m.transcript = append(m.transcript, transcriptItem{Role: "tool", Text: fmt.Sprintf("-> %s%s", event.Tool, formatArgs(event.Args))})
	case agent.EventToolEnd:
		if event.Result.OK {
			m.transcript = append(m.transcript, transcriptItem{Role: "tool", Text: "done " + event.Tool})
		} else {
			m.transcript = append(m.transcript, transcriptItem{Role: "error", Text: fmt.Sprintf("failed %s: %s", event.Tool, event.Result.Error)})
		}
	case agent.EventAnswerDelta:
		m.appendAssistantDelta(event.Text)
	case agent.EventError:
		if event.Err != nil {
			m.transcript = append(m.transcript, transcriptItem{Role: "error", Text: event.Err.Error()})
		}
	}
}

func (m model) applyAgentDone(msg agentDoneMsg) model {
	m.busy = false
	m.stream = nil
	if msg.Err != nil {
		m.errText = msg.Err.Error()
		m.status = "Agent error"
		m.transcript = append(m.transcript, transcriptItem{Role: "error", Text: msg.Err.Error()})
		return m
	}
	m.agentPrior = append(m.agentPrior,
		agent.Message{Role: "user", Content: msg.Goal},
		agent.Message{Role: "assistant", Content: msg.Response.Text},
	)
	m.session.AddUser(msg.Goal)
	m.session.AddAssistant(agentHistorySummary(msg.Response))
	if m.store != nil {
		if err := m.store.Save(m.session); err != nil {
			m.transcript = append(m.transcript, transcriptItem{Role: "error", Text: "history: " + err.Error()})
		}
	}
	m.status = "Ready"
	return m
}

func (m *model) appendAssistantDelta(text string) {
	if len(m.transcript) > 0 && m.transcript[len(m.transcript)-1].Role == "assistant" {
		m.transcript[len(m.transcript)-1].Text += text
		m.transcriptScroll = clamp(m.transcriptScroll, 0, m.maxTranscriptScroll())
		return
	}
	m.transcript = append(m.transcript, transcriptItem{Role: "assistant", Text: text})
	m.transcriptScroll = clamp(m.transcriptScroll, 0, m.maxTranscriptScroll())
}

func (m *model) ensureAssistantText(text string) {
	if len(m.transcript) > 0 && m.transcript[len(m.transcript)-1].Role == "assistant" {
		m.transcript[len(m.transcript)-1].Text = text
		m.transcriptScroll = clamp(m.transcriptScroll, 0, m.maxTranscriptScroll())
		return
	}
	m.transcript = append(m.transcript, transcriptItem{Role: "assistant", Text: text})
	m.transcriptScroll = 0
}

func (m model) runCommand(query string) (model, command) {
	fields := strings.Fields(query)
	if len(fields) == 0 {
		return m, nil
	}
	switch fields[0] {
	case "/quit", "/exit", "/q":
		return m, tea.Quit
	case "/new":
		m.transcript = []transcriptItem{{Role: "system", Text: "New chat session started."}}
		m.transcriptScroll = 0
		m.session = history.NewSession(m.providerName, m.modelName)
		m.agentPrior = nil
		m.status = "New session"
	case "/provider":
		if len(fields) != 2 {
			m.transcript = append(m.transcript, transcriptItem{Role: "error", Text: "usage: /provider <name>"})
			return m, nil
		}
		if err := m.switchProvider(fields[1]); err != nil {
			m.transcript = append(m.transcript, transcriptItem{Role: "error", Text: err.Error()})
			return m, nil
		}
	case "/history":
		return m.showHistory()
	case "/mode":
		if len(fields) != 2 {
			m.transcript = append(m.transcript, transcriptItem{Role: "error", Text: "usage: /mode <chat|agent>"})
			return m, nil
		}
		switch fields[1] {
		case "chat":
			m.mode = modeChat
			m.status = "Switched to Chat"
		case "agent":
			m.mode = modeAgent
			m.status = "Switched to Agent"
		default:
			m.transcript = append(m.transcript, transcriptItem{Role: "error", Text: "usage: /mode <chat|agent>"})
		}
	default:
		m.transcript = append(m.transcript, transcriptItem{Role: "error", Text: fmt.Sprintf("unknown command %q", fields[0])})
	}
	return m, nil
}

func (m *model) switchProvider(name string) error {
	providerName, pcfg, err := m.cfg.ActiveProviderConfig(name)
	if err != nil {
		return err
	}
	var rebuilt agentRunner
	if _, ok := m.agentRunner.(realAgentRunner); ok {
		runner, err := newRealAgentRunner(m.ctx, m.cfg, providerName, m.workspace)
		if err != nil {
			return err
		}
		rebuilt = runner
	}
	m.providerName = providerName
	m.modelName = pcfg.Model
	m.session = history.NewSession(providerName, pcfg.Model)
	m.agentPrior = nil
	if rebuilt != nil {
		m.agentRunner = rebuilt
	}
	m.transcript = append(m.transcript, transcriptItem{Role: "system", Text: fmt.Sprintf("Provider switched to %s / %s", providerName, pcfg.Model)})
	m.status = "Provider switched"
	return nil
}

func (m model) showHistory() (model, command) {
	if m.store == nil {
		m.transcript = append(m.transcript, transcriptItem{Role: "system", Text: "History is disabled."})
		return m, nil
	}
	sessions, err := m.store.List()
	if err != nil {
		m.transcript = append(m.transcript, transcriptItem{Role: "error", Text: err.Error()})
		return m, nil
	}
	if len(sessions) == 0 {
		m.transcript = append(m.transcript, transcriptItem{Role: "system", Text: "No history sessions found."})
		return m, nil
	}
	var lines []string
	for i, s := range sessions {
		if i >= 6 {
			break
		}
		lines = append(lines, fmt.Sprintf("%s  %s/%s  %d messages", s.ID, s.Provider, s.Model, len(s.Messages)))
	}
	m.transcript = append(m.transcript, transcriptItem{Role: "system", Text: strings.Join(lines, "\n")})
	return m, nil
}

type paletteItem struct {
	Label   string
	Command string
}

func (m model) paletteItems() []paletteItem {
	items := []paletteItem{
		{Label: "New session", Command: "/new"},
		{Label: "Show history", Command: "/history"},
		{Label: "Switch to Chat", Command: "/mode chat"},
		{Label: "Switch to Agent", Command: "/mode agent"},
	}
	providers := make([]string, 0, len(m.cfg.Providers))
	for name := range m.cfg.Providers {
		providers = append(providers, name)
	}
	sort.Strings(providers)
	for _, name := range providers {
		items = append(items, paletteItem{Label: "Provider: " + name, Command: "/provider " + name})
	}
	items = append(items, paletteItem{Label: "Quit", Command: "/quit"})
	return items
}

func (m *model) resizeInput() {
	w := clamp(m.width-10, 32, 96)
	if m.width < 52 {
		w = clamp(m.width-4, 24, 48)
	}
	m.input.SetWidth(w)
	m.input.SetHeight(3)
}

func (m *model) resizeInputFor(width int) {
	inputWidth := clamp(width-7, 18, 90)
	m.input.SetWidth(inputWidth)
	m.input.SetHeight(3)
}

func (m *model) setInputValue(value string) {
	m.input.SetValue(value)
}

func (m model) inputValue() string {
	return m.input.Value()
}

func (m model) View() tea.View {
	v := tea.NewView(m.render())
	v.AltScreen = true
	v.WindowTitle = "termask tui"
	v.BackgroundColor = terminalBackgroundColor
	v.ForegroundColor = terminalForegroundColor
	v.MouseMode = tea.MouseModeCellMotion
	return v
}

func (m model) render() string {
	width := max(24, m.width)
	height := max(10, m.height)
	contentWidth := responsiveContentWidth(width)
	var body string
	inputLines := splitLines(m.renderInput(contentWidth))
	footer := m.renderFooter(contentWidth)
	bodyHeight := max(1, height-len(inputLines)-1)
	if len(m.transcript) == 0 {
		body = m.renderIdle(contentWidth, bodyHeight)
	} else {
		body = m.renderTranscript(contentWidth, bodyHeight)
	}
	lines := fitLines(splitLines(body), bodyHeight)
	lines = append(lines, inputLines...)
	lines = append(lines, footer)
	if m.paletteOpen {
		palette := splitLines(m.renderPalette(contentWidth))
		lines = append(palette, lines...)
	}
	lines = fitLines(lines, height)
	return screenStyle.Render(joinCanvas(lines, width, contentWidth))
}

func (m model) renderIdle(width, height int) string {
	logo := logoStyle.Render(compactLogo(width, height))
	sub := dimStyle.Render(wrapPlain(`Ask anything... "What is the tech stack of this project?"`, max(18, width-4)))
	content := lipgloss.JoinVertical(lipgloss.Center, logo, "", sub)
	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, content)
}

func (m model) renderTranscript(width, height int) string {
	inner := clamp(width-4, 30, 120)
	rendered := m.transcriptLines(inner)
	if m.busy {
		rendered = append(rendered, dimStyle.Render(m.spin.View()+" "+m.status))
	}
	rendered = visibleTranscriptLines(rendered, height, m.transcriptScroll)
	return transcriptStyle.Width(inner).Render(strings.Join(rendered, "\n"))
}

func (m model) transcriptLines(width int) []string {
	textWidth := max(12, width-14)
	var rendered []string
	for _, item := range m.transcript {
		label := roleStyle(item.Role).Render(item.Role)
		text := strings.TrimSpace(item.Text)
		if item.Role == "assistant" {
			text = compactAssistantMarkdown(text)
		}
		parts := compactTranscriptLines(splitLines(wrapPlain(text, textWidth)))
		if len(parts) == 0 {
			parts = []string{""}
		}
		indent := strings.Repeat(" ", lipgloss.Width(item.Role)+1)
		for i, part := range parts {
			if i == 0 {
				rendered = append(rendered, label+" "+part)
				continue
			}
			rendered = append(rendered, indent+part)
		}
	}
	return rendered
}

func visibleTranscriptLines(lines []string, height, scroll int) []string {
	if height <= 0 || len(lines) <= height {
		return lines
	}
	scroll = clamp(scroll, 0, len(lines)-height)
	end := len(lines) - scroll
	start := max(0, end-height)
	return lines[start:end]
}

func compactTranscriptLines(lines []string) []string {
	start := 0
	for start < len(lines) && strings.TrimSpace(lines[start]) == "" {
		start++
	}
	end := len(lines)
	for end > start && strings.TrimSpace(lines[end-1]) == "" {
		end--
	}
	out := make([]string, 0, end-start)
	blank := false
	for _, line := range lines[start:end] {
		isBlank := strings.TrimSpace(line) == ""
		if isBlank && blank {
			continue
		}
		out = append(out, line)
		blank = isBlank
	}
	return out
}

func compactAssistantMarkdown(text string) string {
	var out []string
	inFence := false
	for _, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") {
			inFence = !inFence
			continue
		}
		if !inFence {
			trimmed = strings.TrimPrefix(trimmed, "### ")
			trimmed = strings.TrimPrefix(trimmed, "## ")
			trimmed = strings.TrimPrefix(trimmed, "# ")
			trimmed = strings.ReplaceAll(trimmed, "**", "")
			trimmed = strings.ReplaceAll(trimmed, "__", "")
		}
		out = append(out, trimmed)
	}
	return strings.Join(compactTranscriptLines(out), "\n")
}

func (m model) renderInput(width int) string {
	inner := clamp(width, 24, 104)
	m.resizeInputFor(inner)
	meta := fmt.Sprintf("%s  %s / %s", accentStyle.Render(m.mode.String()), m.providerName, m.modelName)
	hints := dimStyle.Render(hintText(inner))
	boxContentWidth := max(12, inner-5)
	box := inputStyle.Width(boxContentWidth).Render(lipgloss.JoinVertical(lipgloss.Left, m.input.View(), "", meta))
	return lipgloss.JoinVertical(lipgloss.Center, box, hints)
}

func (m model) renderFooter(width int) string {
	pathLimit := max(10, width-24)
	if width > 70 {
		pathLimit = max(18, width-42)
	}
	path := compactPath(m.workspace, pathLimit)
	left := dimStyle.Render(path + branchSuffix(m.gitBranch))
	mid := statusStyle.Render("  " + m.status + "  ")
	right := ""
	if width > 58 {
		right = dimStyle.Render(m.version)
	}
	used := lipgloss.Width(left) + lipgloss.Width(mid) + lipgloss.Width(right)
	if used > width {
		left = dimStyle.Render(compactPath(m.workspace, max(6, width-lipgloss.Width(mid)-2)))
		used = lipgloss.Width(left) + lipgloss.Width(mid) + lipgloss.Width(right)
	}
	padding := max(1, width-used)
	return left + strings.Repeat(" ", padding) + mid + right
}

func (m model) renderPalette(width int) string {
	items := m.paletteItems()
	var lines []string
	for i, item := range items {
		cursor := "  "
		if i == m.paletteCursor {
			cursor = accentStyle.Render("› ")
		}
		lines = append(lines, cursor+item.Label)
	}
	return paletteStyle.Width(clamp(width-12, 30, 72)).Render(strings.Join(lines, "\n"))
}

func compactLogo(width, height int) string {
	if width < 92 || height < 13 {
		return "termask"
	}
	return strings.Join([]string{
		"  ████████  ████████  ██████   ███    ███   █████   ███████  ██   ██",
		"     ██     ██        ██   ██  ████  ████  ██   ██  ██       ██  ██ ",
		"     ██     ██████    ██████   ██ ████ ██  ███████  ███████  █████  ",
		"     ██     ██        ██   ██  ██  ██  ██  ██   ██       ██  ██  ██ ",
		"     ██     ████████  ██   ██  ██      ██  ██   ██  ███████  ██   ██",
	}, "\n")
}

func responsiveContentWidth(width int) int {
	if width < 52 {
		return max(20, width-4)
	}
	if width < 100 {
		return width - 8
	}
	return clamp(width-16, 84, 132)
}

func hintText(width int) string {
	if width < 34 {
		return "tab mode  ctrl+p  alt+enter"
	}
	if width < 70 {
		return "tab switch mode   ctrl+p commands"
	}
	return "tab switch mode   ctrl+p commands   alt+enter newline"
}

func isShortcutKey(key string) bool {
	switch key {
	case "ctrl+c", "esc", "tab", "ctrl+p", "alt+enter", "ctrl+j", "enter", "pgup", "pgdown", "home", "end":
		return true
	default:
		return false
	}
}

func textareaStyles() textarea.Styles {
	styles := textarea.DefaultDarkStyles()
	base := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#cdd4f4")).
		Background(inputBackgroundColor)
	placeholder := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#777b92")).
		Background(inputBackgroundColor)
	prompt := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#caa7ff")).
		Background(inputBackgroundColor)
	styles.Focused.Base = base
	styles.Focused.Text = base
	styles.Focused.CursorLine = base
	styles.Focused.EndOfBuffer = base
	styles.Focused.Placeholder = placeholder
	styles.Focused.Prompt = prompt
	styles.Blurred.Base = base
	styles.Blurred.Text = base
	styles.Blurred.CursorLine = base
	styles.Blurred.EndOfBuffer = base
	styles.Blurred.Placeholder = placeholder
	styles.Blurred.Prompt = prompt
	styles.Cursor.Color = lipgloss.Color("#d8b4fe")
	return styles
}

func splitLines(s string) []string {
	if s == "" {
		return []string{""}
	}
	return strings.Split(strings.TrimRight(s, "\n"), "\n")
}

func fitLines(lines []string, height int) []string {
	if height <= 0 {
		return nil
	}
	out := append([]string{}, lines...)
	if len(out) > height {
		return out[len(out)-height:]
	}
	for len(out) < height {
		out = append(out, "")
	}
	return out
}

func joinCanvas(lines []string, width, contentWidth int) string {
	leftPad := max(0, (width-contentWidth)/2)
	rightPad := max(0, width-contentWidth-leftPad)
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		lineWidth := lipgloss.Width(line)
		contentPad := max(0, contentWidth-lineWidth)
		out = append(out, screenPad(leftPad)+line+screenPad(contentPad+rightPad))
	}
	return strings.Join(out, "\n")
}

func (m *model) scrollTranscript(delta int) {
	m.transcriptScroll = clamp(m.transcriptScroll+delta, 0, m.maxTranscriptScroll())
}

func (m model) maxTranscriptScroll() int {
	height := max(1, m.height)
	contentWidth := responsiveContentWidth(max(24, m.width))
	inputLines := splitLines(m.renderInput(contentWidth))
	transcriptHeight := max(1, height-len(inputLines)-1)
	lineCount := len(m.transcriptLines(clamp(contentWidth-4, 30, 120)))
	if m.busy {
		lineCount++
	}
	return max(0, lineCount-transcriptHeight)
}

func (m model) transcriptPageSize() int {
	height := max(1, m.height)
	contentWidth := responsiveContentWidth(max(24, m.width))
	inputLines := splitLines(m.renderInput(contentWidth))
	return max(1, height-len(inputLines)-2)
}

func scrollStep() int {
	return 3
}

func screenPad(width int) string {
	if width <= 0 {
		return ""
	}
	return screenStyle.Render(strings.Repeat(" ", width))
}

func normalizedKey(msg tea.KeyPressMsg) string {
	key := msg.Key()
	if key.Mod.Contains(tea.ModCtrl) {
		switch {
		case key.Code == 'p' || key.ShiftedCode == 'P' || key.BaseCode == 'p' || key.BaseCode == 'P':
			return "ctrl+p"
		case key.Code == 'j' || key.ShiftedCode == 'J' || key.BaseCode == 'j' || key.BaseCode == 'J':
			return "ctrl+j"
		case key.Code == 'c' || key.ShiftedCode == 'C' || key.BaseCode == 'c' || key.BaseCode == 'C':
			return "ctrl+c"
		}
	}
	return msg.Keystroke()
}

func roleStyle(role string) lipgloss.Style {
	switch role {
	case "user":
		return accentStyle
	case "assistant":
		return assistantStyle
	case "tool", "status", "system":
		return dimStyle
	case "error":
		return errorStyle
	default:
		return dimStyle
	}
}

func sessionMessages(session history.Session) []provider.Message {
	messages := make([]provider.Message, 0, len(session.Messages))
	for _, msg := range session.Messages {
		messages = append(messages, provider.Message{Role: msg.Role, Content: msg.Content})
	}
	return messages
}

func agentHistorySummary(resp agent.Response) string {
	if len(resp.Steps) == 0 {
		return resp.Text
	}
	var out strings.Builder
	for i, step := range resp.Steps {
		fmt.Fprintf(&out, "step %d: %s%s\n", i+1, step.Tool, formatArgs(step.Args))
	}
	if resp.Text != "" {
		out.WriteString(resp.Text)
	}
	return strings.TrimSpace(out.String())
}

func formatArgs(args map[string]string) string {
	if len(args) == 0 {
		return ""
	}
	keys := make([]string, 0, len(args))
	for key := range args {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var parts []string
	for _, key := range keys {
		parts = append(parts, fmt.Sprintf(" %s=%s", key, args[key]))
	}
	return strings.Join(parts, "")
}

func gitBranch(workspace string) string {
	cmd := exec.Command("git", "branch", "--show-current")
	cmd.Dir = workspace
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func branchSuffix(branch string) string {
	if branch == "" {
		return ""
	}
	return ":" + branch
}

func compactPath(path string, limit int) string {
	if path == "" {
		return "~"
	}
	home, err := os.UserHomeDir()
	if err == nil {
		if rel, err := filepath.Rel(home, path); err == nil && !strings.HasPrefix(rel, "..") {
			path = "~/" + rel
		}
	}
	if len(path) <= limit {
		return path
	}
	if limit < 8 {
		return path[len(path)-limit:]
	}
	return "…" + path[len(path)-limit+1:]
}

func wrapPlain(text string, width int) string {
	if width <= 0 || len(text) <= width {
		return text
	}
	var lines []string
	for _, original := range strings.Split(text, "\n") {
		words := strings.Fields(original)
		if len(words) == 0 {
			lines = append(lines, "")
			continue
		}
		line := words[0]
		for _, word := range words[1:] {
			if len(line)+1+len(word) > width {
				lines = append(lines, line)
				line = word
			} else {
				line += " " + word
			}
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

func stripStyle(s string) string {
	replacer := strings.NewReplacer("\x1b[0m", "")
	return replacer.Replace(s)
}

func clamp(v, minValue, maxValue int) int {
	if minValue > maxValue {
		return minValue
	}
	if v < minValue {
		return minValue
	}
	if v > maxValue {
		return maxValue
	}
	return v
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

var (
	terminalBackgroundColor = color.RGBA{R: 0x27, G: 0x28, B: 0x3c, A: 0xff}
	terminalForegroundColor = color.RGBA{R: 0xc9, G: 0xd1, B: 0xf2, A: 0xff}
	inputBackgroundColor    = lipgloss.Color("#111321")
	screenStyle             = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#c9d1f2")).
				Background(lipgloss.Color("#27283c"))
	logoStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#d8def8")).
			Bold(true)
	inputStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#cdd4f4")).
			Background(inputBackgroundColor).
			Border(lipgloss.NormalBorder(), false, false, false, true).
			BorderBackground(inputBackgroundColor).
			BorderForeground(lipgloss.Color("#caa7ff")).
			Padding(1, 2)
	transcriptStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#d4d8f0")).
			Padding(0, 1)
	paletteStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#d8def8")).
			Background(lipgloss.Color("#17192a")).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#caa7ff")).
			Padding(1, 2).
			MarginBottom(1)
	accentStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#d8b4fe")).Bold(true)
	assistantStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#cdd4f4")).Bold(true)
	statusStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#f0aac0"))
	errorStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("#ff8fa3")).Bold(true)
	dimStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("#a9adc6"))
)
