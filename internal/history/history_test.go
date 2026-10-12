package history

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
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

func TestConcurrentSavesKeepAllSessions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.jsonl")
	a := NewStore(path)
	b := NewStore(path)

	sessions := make([]Session, 0, 20)
	for i := 0; i < 20; i++ {
		s := NewSession("anthropic", "model")
		s.AddUser(fmt.Sprintf("session %d", i))
		sessions = append(sessions, s)
	}

	var wg sync.WaitGroup
	errs := make(chan error, len(sessions))
	for i, s := range sessions {
		store := a
		if i%2 == 1 {
			store = b
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := store.Save(s); err != nil {
				errs <- err
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("Save() error = %v", err)
	}

	got, err := a.List()
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(got) != len(sessions) {
		t.Fatalf("len = %d, want %d (lost updates without cross-process lock)", len(got), len(sessions))
	}
	want := map[string]bool{}
	for _, s := range sessions {
		want[s.ID] = true
	}
	for _, s := range got {
		delete(want, s.ID)
	}
	if len(want) != 0 {
		t.Fatalf("missing %d sessions after concurrent saves", len(want))
	}
}

func TestWriteAllUniqueTmp(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "history.jsonl")
	store := NewStore(path)

	session := NewSession("anthropic", "model")
	if err := store.Save(session); err != nil {
		t.Fatal(err)
	}
	leftovers, err := filepath.Glob(filepath.Join(dir, ".history-*.tmp"))
	if err != nil {
		t.Fatal(err)
	}
	if len(leftovers) != 0 {
		t.Fatalf("tmp leftovers: %v", leftovers)
	}
}
