package ollama

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mmeister86/termask/internal/provider"
)

func TestAskMessages_ChatAPIStreamsTwoChunks(t *testing.T) {
	var gotPath string
	var gotMessages []chatMessage
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		var req chatRequest
		raw, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(raw, &req); err != nil {
			t.Errorf("decode request: %v", err)
		}
		gotMessages = req.Messages
		io.WriteString(w,
			"{\"message\":{\"role\":\"assistant\",\"content\":\"Hello \"},\"done\":false}\n"+
				"{\"message\":{\"role\":\"assistant\",\"content\":\"world\"},\"done\":false}\n"+
				"{\"message\":{\"role\":\"assistant\",\"content\":\"\"},\"done\":true}\n")
	}))
	defer srv.Close()

	var out bytes.Buffer
	p := New()
	err := p.AskMessages(context.Background(),
		provider.ProviderConfig{BaseURL: srv.URL, Model: "llama3"},
		"be nice", []provider.Message{{Role: "user", Content: "hi"}}, &out)
	if err != nil {
		t.Fatalf("AskMessages: %v", err)
	}
	if out.String() != "Hello world" {
		t.Fatalf("got %q", out.String())
	}
	if gotPath != "/api/chat" {
		t.Fatalf("path = %q, want /api/chat", gotPath)
	}
	want := []chatMessage{{Role: "system", Content: "be nice"}, {Role: "user", Content: "hi"}}
	if len(gotMessages) != len(want) {
		t.Fatalf("messages = %+v, want %+v", gotMessages, want)
	}
	for i := range want {
		if gotMessages[i] != want[i] {
			t.Fatalf("messages = %+v, want %+v", gotMessages, want)
		}
	}
}

func TestAskMessages_HTTP500(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		io.WriteString(w, "boom")
	}))
	defer srv.Close()

	var out bytes.Buffer
	err := New().AskMessages(context.Background(),
		provider.ProviderConfig{BaseURL: srv.URL, Model: "llama3"},
		"", []provider.Message{{Role: "user", Content: "hi"}}, &out)
	if err == nil || !strings.Contains(err.Error(), "500") {
		t.Fatalf("want error containing 500, got %v", err)
	}
}
