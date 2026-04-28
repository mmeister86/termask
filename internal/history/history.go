package history

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type Message struct {
	Role      string    `json:"role"`
	Content   string    `json:"content"`
	Timestamp time.Time `json:"timestamp"`
}

type Session struct {
	ID        string    `json:"id"`
	Provider  string    `json:"provider"`
	Model     string    `json:"model"`
	Messages  []Message `json:"messages"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Store struct {
	path string
}

func DefaultPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".local", "share", "termask", "history.jsonl"), nil
}

func NewStore(path string) Store {
	return Store{path: path}
}

func NewSession(providerName, model string) Session {
	now := time.Now().UTC()
	return Session{
		ID:        fmt.Sprintf("%d-%s", now.UnixNano(), randomSuffix()),
		Provider:  providerName,
		Model:     model,
		CreatedAt: now,
		UpdatedAt: now,
	}
}

func randomSuffix() string {
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "00000000"
	}
	return hex.EncodeToString(b[:])
}

func (s *Session) AddUser(content string) {
	s.add("user", content)
}

func (s *Session) AddAssistant(content string) {
	s.add("assistant", content)
}

func (s *Session) add(role, content string) {
	now := time.Now().UTC()
	s.Messages = append(s.Messages, Message{Role: role, Content: content, Timestamp: now})
	if s.CreatedAt.IsZero() {
		s.CreatedAt = now
	}
	s.UpdatedAt = now
}

func (s Store) Save(session Session) error {
	sessions, err := s.List()
	if err != nil {
		return err
	}
	replaced := false
	for i := range sessions {
		if sessions[i].ID == session.ID {
			sessions[i] = session
			replaced = true
			break
		}
	}
	if !replaced {
		sessions = append(sessions, session)
	}
	return s.writeAll(sessions)
}

func (s Store) Get(id string) (Session, error) {
	sessions, err := s.List()
	if err != nil {
		return Session{}, err
	}
	for _, session := range sessions {
		if session.ID == id {
			return session, nil
		}
	}
	return Session{}, fmt.Errorf("session %q not found", id)
}

func (s Store) Latest() (Session, error) {
	sessions, err := s.List()
	if err != nil {
		return Session{}, err
	}
	if len(sessions) == 0 {
		return Session{}, errors.New("no history sessions found")
	}
	return sessions[0], nil
}

func (s Store) List() ([]Session, error) {
	file, err := os.Open(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer file.Close()

	byID := map[string]Session{}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var session Session
		if err := json.Unmarshal([]byte(line), &session); err != nil {
			return nil, fmt.Errorf("parse history: %w", err)
		}
		byID[session.ID] = session
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	sessions := make([]Session, 0, len(byID))
	for _, session := range byID {
		sessions = append(sessions, session)
	}
	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].UpdatedAt.After(sessions[j].UpdatedAt)
	})
	return sessions, nil
}

func (s Store) Clear() error {
	if err := os.Remove(s.path); errors.Is(err, os.ErrNotExist) {
		return nil
	} else {
		return err
	}
}

func (s Store) writeAll(sessions []Session) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0700); err != nil {
		return err
	}
	file, err := os.OpenFile(s.path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	defer file.Close()
	enc := json.NewEncoder(file)
	for _, session := range sessions {
		if err := enc.Encode(session); err != nil {
			return err
		}
	}
	return nil
}
