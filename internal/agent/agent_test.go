package agent

import (
	"context"
	"os"
	"strings"
	"testing"
)

type scriptedModel struct {
	responses []string
	calls     int
	messages  [][]Message
}

func (m *scriptedModel) Generate(_ context.Context, messages []Message) (string, error) {
	cp := append([]Message{}, messages...)
	m.messages = append(m.messages, cp)
	resp := m.responses[m.calls]
	m.calls++
	return resp, nil
}

func TestRunReturnsFinalAnswerWithoutToolCall(t *testing.T) {
	model := &scriptedModel{responses: []string{"Final answer"}}
	agent := New(model, NewToolset(t.TempDir()))

	resp, err := agent.Run(context.Background(), Request{Goal: "explain", MaxSteps: 4})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Text != "Final answer" {
		t.Fatalf("Text = %q", resp.Text)
	}
	if len(resp.Steps) != 0 {
		t.Fatalf("Steps = %+v, want none", resp.Steps)
	}
}

func TestRunStreamEmitsAnswerDeltasAndDone(t *testing.T) {
	model := &scriptedModel{responses: []string{"Hello streamed world"}}
	agent := New(model, NewToolset(t.TempDir()))
	var events []Event

	resp, err := agent.RunStream(context.Background(), Request{Goal: "explain", MaxSteps: 4}, func(event Event) {
		events = append(events, event)
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Text != "Hello streamed world" {
		t.Fatalf("Text = %q", resp.Text)
	}
	if !hasEvent(events, EventModelStart) {
		t.Fatalf("events = %+v, want model_start", events)
	}
	if got := collectEventText(events, EventAnswerDelta); got != "Hello streamed world" {
		t.Fatalf("answer deltas = %q", got)
	}
	if !hasEvent(events, EventAnswerDone) {
		t.Fatalf("events = %+v, want answer_done", events)
	}
}

func TestRunStreamPreservesAnswerWhitespace(t *testing.T) {
	answer := "Hier sind Dateien:\n- README.md\n- go.mod\n"
	model := &scriptedModel{responses: []string{answer}}
	agent := New(model, NewToolset(t.TempDir()))
	var events []Event

	_, err := agent.RunStream(context.Background(), Request{Goal: "list", MaxSteps: 4}, func(event Event) {
		events = append(events, event)
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := collectEventText(events, EventAnswerDelta); got != strings.TrimSpace(answer) {
		t.Fatalf("answer deltas = %q, want original whitespace %q", got, strings.TrimSpace(answer))
	}
}

func TestRunStreamEmitsToolEventsAndDoesNotLeakToolJSON(t *testing.T) {
	model := &scriptedModel{responses: []string{
		`{"tool":"list_files","args":{}}`,
		"Done after tool",
	}}
	agent := New(model, NewToolset(t.TempDir()))
	var events []Event

	resp, err := agent.RunStream(context.Background(), Request{Goal: "inspect", MaxSteps: 4}, func(event Event) {
		events = append(events, event)
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Steps) != 1 || resp.Steps[0].Tool != "list_files" {
		t.Fatalf("Steps = %+v, want list_files", resp.Steps)
	}
	if !hasEvent(events, EventToolStart) || !hasEvent(events, EventToolEnd) {
		t.Fatalf("events = %+v, want tool start/end", events)
	}
	if got := collectEventText(events, EventAnswerDelta); strings.Contains(got, `"tool":"list_files"`) {
		t.Fatalf("answer deltas leaked tool JSON: %q", got)
	}
	if got := collectEventText(events, EventAnswerDelta); got != "Done after tool" {
		t.Fatalf("answer deltas = %q", got)
	}
}

func TestRunIncludesPriorHistory(t *testing.T) {
	model := &scriptedModel{responses: []string{"Follow-up answer"}}
	agent := New(model, NewToolset(t.TempDir()))

	_, err := agent.Run(context.Background(), Request{
		Goal:     "und jetzt?",
		MaxSteps: 4,
		History: []Message{
			{Role: "user", Content: "welche dateien sind hier?"},
			{Role: "assistant", Content: "README.md"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	firstCall := model.messages[0]
	joined := ""
	for _, msg := range firstCall {
		joined += msg.Role + ":" + msg.Content + "\n"
	}
	if !strings.Contains(joined, "user:welche dateien sind hier?") ||
		!strings.Contains(joined, "assistant:README.md") ||
		!strings.Contains(joined, "user:und jetzt?") {
		t.Fatalf("messages missing history/current turn:\n%s", joined)
	}
}

func hasEvent(events []Event, typ EventType) bool {
	for _, event := range events {
		if event.Type == typ {
			return true
		}
	}
	return false
}

func collectEventText(events []Event, typ EventType) string {
	var out strings.Builder
	for _, event := range events {
		if event.Type == typ {
			out.WriteString(event.Text)
		}
	}
	return out.String()
}

func TestRunExecutesToolCallAndFeedsResultBackToModel(t *testing.T) {
	model := &scriptedModel{responses: []string{
		`{"tool":"list_files","args":{}}`,
		"Final answer with files",
	}}
	agent := New(model, NewToolset(t.TempDir()))

	resp, err := agent.Run(context.Background(), Request{Goal: "inspect", MaxSteps: 4})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Text != "Final answer with files" {
		t.Fatalf("Text = %q", resp.Text)
	}
	if len(resp.Steps) != 1 || resp.Steps[0].Tool != "list_files" {
		t.Fatalf("Steps = %+v, want list_files step", resp.Steps)
	}
	if model.calls != 2 {
		t.Fatalf("model calls = %d, want 2", model.calls)
	}
	lastMessages := model.messages[1]
	if got := lastMessages[len(lastMessages)-1].Content; !strings.Contains(got, `"tool":"list_files"`) {
		t.Fatalf("last tool result message = %q", got)
	}
}

func TestRunPrefetchesFileListForCurrentFolderQuestion(t *testing.T) {
	workspace := t.TempDir()
	if err := os.WriteFile(workspace+"/README.md", []byte("# Test\n"), 0644); err != nil {
		t.Fatal(err)
	}
	model := &scriptedModel{responses: []string{"README.md is present"}}
	agent := New(model, NewToolset(workspace))

	resp, err := agent.Run(context.Background(), Request{Goal: "welche dateien sind in diesem ordner", MaxSteps: 4})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Text != "Hier sind die Dateien im aktuellen Ordner:\n- `README.md`" {
		t.Fatalf("Text = %q", resp.Text)
	}
	if len(resp.Steps) != 1 || resp.Steps[0].Tool != "list_files" {
		t.Fatalf("Steps = %+v, want prefetched list_files", resp.Steps)
	}
	if model.calls != 0 {
		t.Fatalf("model calls = %d, want deterministic file-list answer", model.calls)
	}
}

func TestRunAnswersCurrentFolderFileListWithoutModelOverreach(t *testing.T) {
	workspace := t.TempDir()
	if err := os.WriteFile(workspace+"/README.md", []byte("# Test\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(workspace+"/go.mod", []byte("module example.com/test\n"), 0644); err != nil {
		t.Fatal(err)
	}
	model := &scriptedModel{responses: []string{
		"Das ist offenbar ein Go-Projekt mit KI-Agenten.",
	}}
	agent := New(model, NewToolset(workspace))

	resp, err := agent.Run(context.Background(), Request{Goal: "welche dateien sind in diesem ordner", MaxSteps: 4})
	if err != nil {
		t.Fatal(err)
	}
	if model.calls != 0 {
		t.Fatalf("model calls = %d, want deterministic file-list answer without model", model.calls)
	}
	if !strings.Contains(resp.Text, "- `README.md`") || !strings.Contains(resp.Text, "- `go.mod`") {
		t.Fatalf("Text = %q, want file list bullets", resp.Text)
	}
	if strings.Contains(resp.Text, "Go-Projekt") || strings.Contains(resp.Text, "KI-Agent") {
		t.Fatalf("Text = %q, want no project analysis", resp.Text)
	}
}

func TestRunExecutesEmbeddedToolCall(t *testing.T) {
	model := &scriptedModel{responses: []string{
		"Ich kann das prüfen. Wenn du möchtest: {\"tool\":\"list_files\",\"args\":{}}",
		"Final answer after listing files",
	}}
	agent := New(model, NewToolset(t.TempDir()))

	resp, err := agent.Run(context.Background(), Request{Goal: "welche dateien", MaxSteps: 4})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Text != "Final answer after listing files" {
		t.Fatalf("Text = %q", resp.Text)
	}
	if len(resp.Steps) != 1 || resp.Steps[0].Tool != "list_files" {
		t.Fatalf("Steps = %+v, want embedded list_files step", resp.Steps)
	}
}

func TestRunRetriesWhenModelAsksPermissionForReadOnlyTool(t *testing.T) {
	model := &scriptedModel{responses: []string{
		"Ich kann dir die Dateien anzeigen, aber ich brauche dafür einmal eine Abfrage. Wenn du möchtest, mache ich einen Verzeichnis-Scan.",
		`{"tool":"list_files","args":{}}`,
		"Final answer after retry",
	}}
	agent := New(model, NewToolset(t.TempDir()))

	resp, err := agent.Run(context.Background(), Request{Goal: "prüfe den verzeichnisinhalt", MaxSteps: 4})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Text != "Final answer after retry" {
		t.Fatalf("Text = %q", resp.Text)
	}
	if len(resp.Steps) != 1 || resp.Steps[0].Tool != "list_files" {
		t.Fatalf("Steps = %+v, want list_files after retry", resp.Steps)
	}
	if model.calls != 3 {
		t.Fatalf("model calls = %d, want corrective retry plus final", model.calls)
	}
	lastFirstTurn := model.messages[1][len(model.messages[1])-1].Content
	if !strings.Contains(lastFirstTurn, "Do not ask the user for permission") {
		t.Fatalf("corrective message = %q, want no-permission instruction", lastFirstTurn)
	}
}

func TestRunRetriesUnhelpfulFollowupDeflection(t *testing.T) {
	model := &scriptedModel{responses: []string{
		"Ich habe den Inhalt des Ordners bereits aufgelistet. Wenn du magst, kann ich eine bestimmte Datei genauer erklären.",
		"README.md documents the project, go.mod defines the module, and internal/ contains implementation packages.",
	}}
	agent := New(model, NewToolset(t.TempDir()))

	resp, err := agent.Run(context.Background(), Request{
		Goal:     "was bewirken diese dateien",
		MaxSteps: 4,
		History: []Message{
			{Role: "user", Content: "welche dateien sind in diesem ordner"},
			{Role: "assistant", Content: "README.md\ngo.mod\ninternal/agent/agent.go"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(resp.Text, "README.md documents") {
		t.Fatalf("Text = %q, want direct follow-up answer", resp.Text)
	}
	if model.calls != 2 {
		t.Fatalf("model calls = %d, want corrective retry", model.calls)
	}
	lastFirstTurn := model.messages[1][len(model.messages[1])-1].Content
	if !strings.Contains(lastFirstTurn, "Answer the current follow-up directly") {
		t.Fatalf("corrective message = %q, want direct follow-up instruction", lastFirstTurn)
	}
}

func TestRunStopsRepeatedToolCallAndRequestsFinalAnswer(t *testing.T) {
	model := &scriptedModel{responses: []string{
		`{"tool":"list_files","args":{}}`,
		`{"tool":"list_files","args":{}}`,
		"This project is a Go CLI for AI-assisted terminal workflows.",
	}}
	agent := New(model, NewToolset(t.TempDir()))

	resp, err := agent.Run(context.Background(), Request{
		Goal:     "worum geht in diesem ordner/projekt",
		MaxSteps: 6,
		History: []Message{
			{Role: "user", Content: "welche dateien sind in diesem ordner"},
			{Role: "assistant", Content: "README.md\ngo.mod\ncmd/termask/main.go\ninternal/agent/agent.go"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Text != "This project is a Go CLI for AI-assisted terminal workflows." {
		t.Fatalf("Text = %q", resp.Text)
	}
	if len(resp.Steps) != 1 {
		t.Fatalf("Steps = %+v, want only first list_files execution", resp.Steps)
	}
	if model.calls != 3 {
		t.Fatalf("model calls = %d, want repeated tool correction plus final", model.calls)
	}
	correction := model.messages[2][len(model.messages[2])-1].Content
	if !strings.Contains(correction, "Do not call list_files again") {
		t.Fatalf("correction = %q, want repeated-tool instruction", correction)
	}
}

func TestRunRejectsInvalidToolJSON(t *testing.T) {
	model := &scriptedModel{responses: []string{`{"tool":`}}
	agent := New(model, NewToolset(t.TempDir()))

	_, err := agent.Run(context.Background(), Request{Goal: "inspect", MaxSteps: 4})
	if err == nil {
		t.Fatal("Run() error = nil, want invalid JSON error")
	}
	if !strings.Contains(err.Error(), "invalid tool call JSON") {
		t.Fatalf("error = %v, want invalid JSON", err)
	}
}

func TestRunStopsAtMaxSteps(t *testing.T) {
	model := &scriptedModel{responses: []string{
		`{"tool":"list_files","args":{}}`,
		`{"tool":"list_files","args":{}}`,
	}}
	agent := New(model, NewToolset(t.TempDir()))

	_, err := agent.Run(context.Background(), Request{Goal: "inspect", MaxSteps: 2})
	if err == nil {
		t.Fatal("Run() error = nil, want max steps error")
	}
	if !strings.Contains(err.Error(), "max steps") {
		t.Fatalf("error = %v, want max steps", err)
	}
}
