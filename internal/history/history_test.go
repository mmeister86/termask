package history

import (
	"os"
	"path/filepath"
	"testing"
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
