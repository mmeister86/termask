package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

const DefaultMaxSteps = 8

type Message struct {
	Role    string
	Content string
}

type Model interface {
	Generate(ctx context.Context, messages []Message) (string, error)
}

type Agent struct {
	model Model
	tools *Toolset
}

type Request struct {
	Goal     string
	MaxSteps int
	Context  string
	History  []Message
}

type Response struct {
	Text  string
	Steps []Step
}

type Step struct {
	Tool string
	Args map[string]string
	OK   bool
}

type ToolCall struct {
	Tool string            `json:"tool"`
	Args map[string]string `json:"args,omitempty"`
}

type ToolResult struct {
	Tool   string `json:"tool"`
	OK     bool   `json:"ok"`
	Output string `json:"output,omitempty"`
	Error  string `json:"error,omitempty"`
}

type EventType string

const (
	EventAgentStart  EventType = "agent_start"
	EventModelStart  EventType = "model_start"
	EventToolStart   EventType = "tool_start"
	EventToolEnd     EventType = "tool_end"
	EventAnswerDelta EventType = "answer_delta"
	EventAnswerDone  EventType = "answer_done"
	EventError       EventType = "error"
)

type Event struct {
	Type   EventType
	Step   int
	Tool   string
	Args   map[string]string
	Text   string
	Result ToolResult
	Err    error
}

func New(model Model, tools *Toolset) *Agent {
	return &Agent{model: model, tools: tools}
}

func (a *Agent) Run(ctx context.Context, req Request) (Response, error) {
	return a.RunStream(ctx, req, nil)
}

func (a *Agent) RunStream(ctx context.Context, req Request, emit func(Event)) (Response, error) {
	maxSteps := req.MaxSteps
	if maxSteps <= 0 {
		maxSteps = DefaultMaxSteps
	}
	emitEvent(emit, Event{Type: EventAgentStart})

	messages := []Message{
		{Role: "system", Content: systemPrompt()},
	}
	messages = append(messages, req.History...)
	messages = append(messages, Message{Role: "user", Content: userPrompt(req)})
	var steps []Step
	seenTools := map[string]bool{}
	if call, ok := initialToolCall(req.Goal); ok {
		seenTools[toolCallSignature(call)] = true
		emitEvent(emit, Event{Type: EventToolStart, Step: len(steps) + 1, Tool: call.Tool, Args: call.Args})
		result := a.tools.Execute(ctx, call)
		steps = append(steps, Step{Tool: call.Tool, Args: call.Args, OK: result.OK})
		emitEvent(emit, Event{Type: EventToolEnd, Step: len(steps), Tool: call.Tool, Args: call.Args, Result: result})
		if shouldAnswerFileListDirectly(req.Goal, call, result) {
			answer := formatFileListAnswer(result.Output)
			emitAnswer(emit, answer)
			return Response{Text: answer, Steps: steps}, nil
		}
		payload, err := json.Marshal(result)
		if err != nil {
			emitEvent(emit, Event{Type: EventError, Err: err})
			return Response{}, err
		}
		messages = append(messages, Message{Role: "user", Content: "Initial read-only tool result:\n" + string(payload)})
	}
	for step := 0; step < maxSteps; step++ {
		emitEvent(emit, Event{Type: EventModelStart, Step: step + 1})
		text, err := a.model.Generate(ctx, messages)
		if err != nil {
			emitEvent(emit, Event{Type: EventError, Step: step + 1, Err: err})
			return Response{}, err
		}
		call, ok, err := parseToolCall(text)
		if err != nil {
			emitEvent(emit, Event{Type: EventError, Step: step + 1, Err: err})
			return Response{}, err
		}
		if !ok {
			if asksPermissionForReadOnlyTool(text) {
				messages = append(messages,
					Message{Role: "assistant", Content: strings.TrimSpace(text)},
					Message{Role: "user", Content: "Do not ask the user for permission to use read-only tools in this one-shot CLI. If read-only context is needed, return exactly one tool call JSON object now. For current-folder or directory listing questions, use {\"tool\":\"list_files\",\"args\":{}}."},
				)
				continue
			}
			if isUnhelpfulFollowupDeflection(req, text) {
				messages = append(messages,
					Message{Role: "assistant", Content: strings.TrimSpace(text)},
					Message{Role: "user", Content: "Answer the current follow-up directly using the conversation history and any read-only tools you need. Do not say you already answered, do not offer to answer later, and do not ask which file to inspect unless the current request is genuinely ambiguous. If the user asks what the listed files do, summarize the files or path groups by purpose now."},
				)
				continue
			}
			answer := strings.TrimSpace(text)
			emitAnswer(emit, answer)
			return Response{Text: answer, Steps: steps}, nil
		}
		signature := toolCallSignature(call)
		if seenTools[signature] {
			messages = append(messages,
				Message{Role: "assistant", Content: strings.TrimSpace(text)},
				Message{Role: "user", Content: fmt.Sprintf("Do not call %s again with the same arguments. You already have that read-only tool result in the conversation. Answer the current user request now using the existing context; if the user asks what this project is about, summarize the project purpose from the file list/history.", call.Tool)},
			)
			continue
		}
		seenTools[signature] = true
		emitEvent(emit, Event{Type: EventToolStart, Step: len(steps) + 1, Tool: call.Tool, Args: call.Args})
		result := a.tools.Execute(ctx, call)
		steps = append(steps, Step{Tool: call.Tool, Args: call.Args, OK: result.OK})
		emitEvent(emit, Event{Type: EventToolEnd, Step: len(steps), Tool: call.Tool, Args: call.Args, Result: result})
		payload, err := json.Marshal(result)
		if err != nil {
			emitEvent(emit, Event{Type: EventError, Step: step + 1, Err: err})
			return Response{}, err
		}
		messages = append(messages,
			Message{Role: "assistant", Content: strings.TrimSpace(text)},
			Message{Role: "user", Content: "Tool result:\n" + string(payload)},
		)
	}
	err := fmt.Errorf("agent stopped after max steps (%d) without a final answer", maxSteps)
	emitEvent(emit, Event{Type: EventError, Err: err})
	return Response{Steps: steps}, err
}

func toolCallSignature(call ToolCall) string {
	var out strings.Builder
	out.WriteString(call.Tool)
	if len(call.Args) == 0 {
		return out.String()
	}
	keys := make([]string, 0, len(call.Args))
	for key := range call.Args {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		out.WriteByte('\x00')
		out.WriteString(key)
		out.WriteByte('=')
		out.WriteString(call.Args[key])
	}
	return out.String()
}

func emitEvent(emit func(Event), event Event) {
	if emit != nil {
		emit(event)
	}
}

func emitAnswer(emit func(Event), answer string) {
	if emit == nil {
		return
	}
	for _, chunk := range answerChunks(answer) {
		emit(Event{Type: EventAnswerDelta, Text: chunk})
	}
	emit(Event{Type: EventAnswerDone})
}

func answerChunks(answer string) []string {
	if answer == "" {
		return nil
	}
	var chunks []string
	const chunkSize = 48
	for len(answer) > chunkSize {
		cut := chunkSize
		for cut > 0 && (answer[cut]&0xc0) == 0x80 {
			cut--
		}
		if cut == 0 {
			cut = chunkSize
		}
		chunks = append(chunks, answer[:cut])
		answer = answer[cut:]
	}
	if answer != "" {
		chunks = append(chunks, answer)
	}
	return chunks
}

func parseToolCall(text string) (ToolCall, bool, error) {
	trimmed := strings.TrimSpace(text)
	if strings.HasPrefix(trimmed, "{") {
		return decodeToolCall(trimmed, true)
	}
	for _, candidate := range jsonObjectCandidates(trimmed) {
		call, ok, err := decodeToolCall(candidate, false)
		if err != nil {
			continue
		}
		if ok {
			return call, true, nil
		}
	}
	return ToolCall{}, false, nil
}

func decodeToolCall(text string, strict bool) (ToolCall, bool, error) {
	var call ToolCall
	if err := json.Unmarshal([]byte(text), &call); err != nil {
		if !strict {
			return ToolCall{}, false, nil
		}
		return ToolCall{}, false, fmt.Errorf("invalid tool call JSON: %w", err)
	}
	if call.Tool == "" {
		if !strict {
			return ToolCall{}, false, nil
		}
		return ToolCall{}, false, fmt.Errorf("invalid tool call JSON: missing tool")
	}
	if call.Args == nil {
		call.Args = map[string]string{}
	}
	return call, true, nil
}

func jsonObjectCandidates(text string) []string {
	var candidates []string
	for start, r := range text {
		if r != '{' {
			continue
		}
		depth := 0
		inString := false
		escaped := false
		for end := start; end < len(text); end++ {
			ch := text[end]
			if inString {
				if escaped {
					escaped = false
					continue
				}
				if ch == '\\' {
					escaped = true
					continue
				}
				if ch == '"' {
					inString = false
				}
				continue
			}
			switch ch {
			case '"':
				inString = true
			case '{':
				depth++
			case '}':
				depth--
				if depth == 0 {
					candidates = append(candidates, text[start:end+1])
					end = len(text)
				}
			}
		}
	}
	return candidates
}

func systemPrompt() string {
	return `You are termask's read-only local agent.

You may either answer the user directly in Markdown, or request exactly one tool call by returning only compact JSON:
{"tool":"tool_name","args":{"key":"value"}}

Available tools:
- list_files: args optional {"pattern":"glob-like substring"}
- read_file: args {"path":"relative path"}
- search_text: args {"pattern":"text or regex","path":"optional relative path"}
- git_status: args {}
- git_diff: args optional {"path":"relative path"}
- run_check: args {"command":"go test ./..."}; only allowlisted go test commands run.

Rules:
- Use tools only to gather read-only evidence.
- Do not ask the user for permission to use read-only tools. This is a one-shot CLI, so ask for a tool call directly when read-only context is needed.
- If the user asks what files are in the current folder/directory, call list_files immediately.
- For follow-up questions, answer the current question directly from conversation history. Never respond with only "I already listed/explained that" or "if you want, I can...".
- If the user asks what previously listed files do, summarize those files by purpose or path group.
- If the user asks what the current folder/project is about and a file list is already in history, summarize the project purpose from that history before calling list_files again.
- Never request write, edit, delete, install, network, or arbitrary shell actions.
- When you have enough context, return the final answer as Markdown, not JSON.`
}

func userPrompt(req Request) string {
	if strings.TrimSpace(req.Context) == "" {
		return req.Goal
	}
	return req.Goal + "\n\nExplicit context files:\n" + req.Context
}

func initialToolCall(goal string) (ToolCall, bool) {
	if asksForCurrentFolderFiles(goal) {
		return ToolCall{Tool: "list_files", Args: map[string]string{}}, true
	}
	return ToolCall{}, false
}

func shouldAnswerFileListDirectly(goal string, call ToolCall, result ToolResult) bool {
	return call.Tool == "list_files" && result.OK && asksForCurrentFolderFiles(goal)
}

func asksForCurrentFolderFiles(goal string) bool {
	lower := strings.ToLower(goal)
	fileQuestion := strings.Contains(lower, "welche dateien") ||
		strings.Contains(lower, "dateien sind") ||
		strings.Contains(lower, "list files") ||
		strings.Contains(lower, "what files")
	folderTarget := strings.Contains(lower, "ordner") ||
		strings.Contains(lower, "verzeichnis") ||
		strings.Contains(lower, "folder") ||
		strings.Contains(lower, "directory")
	return fileQuestion && folderTarget
}

func formatFileListAnswer(output string) string {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	var out strings.Builder
	out.WriteString("Hier sind die Dateien im aktuellen Ordner:")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		out.WriteString("\n- `")
		out.WriteString(line)
		out.WriteString("`")
	}
	return out.String()
}

func asksPermissionForReadOnlyTool(text string) bool {
	lower := strings.ToLower(text)
	permissionPhrases := []string{
		"wenn du möchtest",
		"wenn du willst",
		"ich kann",
		"ich brauche dafür",
		"brauche dafür einmal",
	}
	toolHints := []string{
		"dateien",
		"ordner",
		"verzeichnis",
		"filesystem",
		"dateisystem",
		"abfrage",
		"scan",
		"auflisten",
	}
	hasPermissionPhrase := false
	for _, phrase := range permissionPhrases {
		if strings.Contains(lower, phrase) {
			hasPermissionPhrase = true
			break
		}
	}
	if !hasPermissionPhrase {
		return false
	}
	for _, hint := range toolHints {
		if strings.Contains(lower, hint) {
			return true
		}
	}
	return false
}

func isUnhelpfulFollowupDeflection(req Request, text string) bool {
	if len(req.History) == 0 {
		return false
	}
	lower := strings.ToLower(text)
	pastReference := strings.Contains(lower, "bereits") ||
		strings.Contains(lower, "schon") ||
		strings.Contains(lower, "already")
	offerInsteadOfAnswer := strings.Contains(lower, "wenn du magst") ||
		strings.Contains(lower, "wenn du möchtest") ||
		strings.Contains(lower, "kann ich") ||
		strings.Contains(lower, "i can")
	noDirectContent := !strings.Contains(lower, "readme") &&
		!strings.Contains(lower, "go.mod") &&
		!strings.Contains(lower, "internal/") &&
		!strings.Contains(lower, "cmd/")
	return pastReference && offerInsteadOfAnswer && noDirectContent
}
