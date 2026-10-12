package openai

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/yourusername/termask/internal/provider"
)

func TestAskMessages_StreamsTwoDeltas(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w,
			"data: {\"choices\":[{\"delta\":{\"content\":\"Hello \"}}]}\n\n"+
				"data: {\"choices\":[{\"delta\":{\"content\":\"world\"}}]}\n\n"+
				"data: [DONE]\n\n")
	}))
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
