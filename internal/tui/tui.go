package tui

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/yourusername/termask/internal/ask"
	"github.com/yourusername/termask/internal/config"
	"github.com/yourusername/termask/internal/history"
	"github.com/yourusername/termask/internal/markdown"
	"github.com/yourusername/termask/internal/provider"
)

func Run(ctx context.Context, cfg config.Config) error {
	reader := bufio.NewScanner(os.Stdin)
	providerName, pcfg, err := cfg.ActiveProviderConfig("")
	if err != nil {
		return err
	}
	session := history.NewSession(providerName, pcfg.Model)
	storePath, err := history.DefaultPath()
	if err != nil {
		return err
	}
	store := history.NewStore(storePath)

	clear()
	fmt.Printf("termask TUI  [%s / %s]\n", providerName, pcfg.Model)
	fmt.Println(strings.Repeat("-", 72))
	fmt.Println("Commands: /provider <name>, /new, /history, /quit")

	for {
		fmt.Print("\ntermask › ")
		if !reader.Scan() {
			break
		}
		query := strings.TrimSpace(reader.Text())
		if query == "" {
			continue
		}
		if strings.HasPrefix(query, "/") {
			var quit bool
			session, providerName, quit, err = handleCommand(query, cfg, session, providerName, store)
			if err != nil {
				fmt.Fprintf(os.Stderr, "error: %v\n", err)
			}
			if quit {
				return nil
			}
			continue
		}
		fmt.Println()
		resp, err := ask.Run(ctx, cfg, ask.Request{
			ProviderName: providerName,
			Query:        query,
			History:      sessionMessages(session),
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "\nerror: %v\n", err)
			continue
		}
		fmt.Print(markdown.Render(resp.Text))
		session.Provider = resp.ProviderName
		session.Model = resp.Model
		session.AddUser(query)
		session.AddAssistant(resp.Text)
		if cfg.HistoryEnabled {
			_ = store.Save(session)
		}
	}
	return reader.Err()
}

func handleCommand(query string, cfg config.Config, session history.Session, currentProvider string, store history.Store) (history.Session, string, bool, error) {
	fields := strings.Fields(query)
	switch fields[0] {
	case "/quit", "/exit", "/q":
		return session, currentProvider, true, nil
	case "/new":
		_, pcfg, err := cfg.ActiveProviderConfig(currentProvider)
		if err != nil {
			return session, currentProvider, false, err
		}
		session = history.NewSession(currentProvider, pcfg.Model)
		fmt.Println("New session started.")
	case "/provider":
		if len(fields) != 2 {
			return session, currentProvider, false, fmt.Errorf("usage: /provider <name>")
		}
		if _, _, err := cfg.ActiveProviderConfig(fields[1]); err != nil {
			return session, currentProvider, false, err
		}
		currentProvider = fields[1]
		_, pcfg, _ := cfg.ActiveProviderConfig(currentProvider)
		session = history.NewSession(currentProvider, pcfg.Model)
		fmt.Printf("Provider switched to %s / %s\n", currentProvider, pcfg.Model)
	case "/history":
		sessions, err := store.List()
		if err != nil {
			return session, currentProvider, false, err
		}
		for _, s := range sessions {
			fmt.Printf("%s  %s/%s  %d messages\n", s.ID, s.Provider, s.Model, len(s.Messages))
		}
	default:
		return session, currentProvider, false, fmt.Errorf("unknown command %q", fields[0])
	}
	return session, currentProvider, false, nil
}

func sessionMessages(session history.Session) []provider.Message {
	messages := make([]provider.Message, 0, len(session.Messages))
	for _, msg := range session.Messages {
		messages = append(messages, provider.Message{Role: msg.Role, Content: msg.Content})
	}
	return messages
}

func clear() {
	fmt.Print("\033[H\033[2J")
}
