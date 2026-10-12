package anthropic

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

func streamServer(t *testing.T, body string, capture *bytes.Buffer) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if capture != nil && r.Body != nil {
			raw, _ := io.ReadAll(r.Body)
			capture.Write(raw)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, body)
	}))
}

func TestAskMessages_StreamsTwoDeltas(t *testing.T) {
	var reqBody bytes.Buffer
	srv := streamServer(t,
		"data: {\"type\":\"content_block_delta\",\"delta\":{\"type\":\"text_delta\",\"text\":\"Hello \"}}\n\n"+
			"data: {\"type\":\"content_block_delta\",\"delta\":{\"type\":\"text_delta\",\"text\":\"world\"}}\n\n"+
			"data: [DONE]\n\n", &reqBody)
	defer srv.Close()

	var out bytes.Buffer
	p := New()
	err := p.AskMessages(context.Background(),
		provider.ProviderConfig{APIKey: "test", BaseURL: srv.URL},
		"", []provider.Message{{Role: "user", Content: "hi"}}, &out)
	if err != nil {
		t.Fatalf("AskMessages: %v", err)
	}
	if out.String() != "Hello world" {
		t.Fatalf("got %q", out.String())
	}

	var decoded struct {
		MaxTokens int `json:"max_tokens"`
	}
	if err := json.Unmarshal(reqBody.Bytes(), &decoded); err != nil {
		t.Fatalf("decode request: %v", err)
	}
	if decoded.MaxTokens != 8192 {
		t.Fatalf("max_tokens = %d, want 8192", decoded.MaxTokens)
	}
}

func TestAskMessages_MidStreamError(t *testing.T) {
	srv := streamServer(t,
		"data: {\"type\":\"content_block_delta\",\"delta\":{\"type\":\"text_delta\",\"text\":\"Hi\"}}\n\n"+
			"data: {\"type\":\"error\",\"error\":{\"type\":\"overloaded\",\"message\":\"server busy\"}}\n\n", nil)
	defer srv.Close()

	var out bytes.Buffer
	err := New().AskMessages(context.Background(),
		provider.ProviderConfig{APIKey: "test", BaseURL: srv.URL},
		"", []provider.Message{{Role: "user", Content: "hi"}}, &out)
	if err == nil || !strings.Contains(err.Error(), "server busy") {
		t.Fatalf("want error containing API message, got %v", err)
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
		provider.ProviderConfig{APIKey: "test", BaseURL: srv.URL},
		"", []provider.Message{{Role: "user", Content: "hi"}}, &out)
	if err == nil || !strings.Contains(err.Error(), "500") {
		t.Fatalf("want error containing 500, got %v", err)
	}
}
