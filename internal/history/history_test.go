package history

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestStoreCreatesAndUpdatesSessions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.jsonl")
	store := NewStore(path)

	session := NewSession("anthropic", "claude-sonnet-4-6")
	session.AddUser("hello")
	session.AddAssistant("hi there")

	if err := store.Save(session); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	session.AddUser("again")
	if err := store.Save(session); err != nil {
		t.Fatalf("Save() second error = %v", err)
	}

	got, err := store.Get(session.ID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if len(got.Messages) != 3 {
		t.Fatalf("messages = %d, want 3", len(got.Messages))
	}
	if got.Messages[2].Content != "again" {
		t.Fatalf("last message = %q, want again", got.Messages[2].Content)
	}
}

func TestStoreListReturnsNewestFirst(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.jsonl")
	store := NewStore(path)

	first := NewSession("openai", "gpt")
	second := NewSession("ollama", "llama")
	if err := store.Save(first); err != nil {
		t.Fatal(err)
	}
	if err := store.Save(second); err != nil {
		t.Fatal(err)
	}

	sessions, err := store.List()
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(sessions) != 2 {
		t.Fatalf("len = %d, want 2", len(sessions))
	}
	if sessions[0].ID != second.ID {
		t.Fatalf("newest ID = %q, want %q", sessions[0].ID, second.ID)
	}
}

func TestStoreClearRemovesHistoryFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.jsonl")
	store := NewStore(path)
	session := NewSession("anthropic", "model")
	if err := store.Save(session); err != nil {
		t.Fatal(err)
	}
	if err := store.Clear(); err != nil {
		t.Fatalf("Clear() error = %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("history file still exists or stat failed: %v", err)
	}
}

func TestHistoryRoundTripsLargeSession(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.jsonl")
	store := NewStore(path)

	session := NewSession("anthropic", "model")
	session.AddUser(strings.Repeat("x", 200*1024))
	if err := store.Save(session); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	got, err := store.Get(session.ID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if len(got.Messages) != 1 || len(got.Messages[0].Content) != 200*1024 {
		t.Fatalf("large message did not round-trip, got %d messages", len(got.Messages))
	}
}

func TestSavePrunesToLimit(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.jsonl")
	store := NewStoreWithLimit(path, 2)

	var ids []string
	for i := 0; i < 3; i++ {
		s := NewSession("anthropic", "model")
		s.UpdatedAt = time.Now().UTC().Add(time.Duration(i) * time.Second)
		s.CreatedAt = s.UpdatedAt
		if err := store.Save(s); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, s.ID)
		time.Sleep(time.Millisecond)
	}

	sessions, err := store.List()
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(sessions) != 2 {
		t.Fatalf("len = %d, want 2", len(sessions))
	}
	if sessions[0].ID != ids[2] || sessions[1].ID != ids[1] {
		t.Fatalf("prune kept wrong sessions: %v", sessions)
	}
}

func TestWriteAllLeavesNoTmpOnSuccess(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.jsonl")
	store := NewStore(path)

	session := NewSession("anthropic", "model")
	if err := store.Save(session); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Fatalf("tmp file left behind or stat failed: %v", err)
	}
}
