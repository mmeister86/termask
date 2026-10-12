package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/mmeister86/termask/internal/ask"
	"github.com/mmeister86/termask/internal/config"
	"github.com/mmeister86/termask/internal/contextfiles"
	"github.com/mmeister86/termask/internal/history"
	"github.com/mmeister86/termask/internal/markdown"
	"github.com/mmeister86/termask/internal/provider"
	"github.com/mmeister86/termask/internal/safety"
	prompttpl "github.com/mmeister86/termask/internal/template"
	"github.com/mmeister86/termask/internal/tui"
	"github.com/mmeister86/termask/internal/ui"
)

// ── ask ───────────────────────────────────────────────────────────────────────

func askCmd() *cobra.Command {
	var providerName string
	var modelOverride string
	var plainOutput bool
	var continueLast bool
	var templateName string
	var files []string
	var savePath string
	var safetyMode bool

	cmd := &cobra.Command{
		Use:   "ask [question]",
		Short: "Ask a question (streams the answer)",
		Example: `  termask ask "ffmpeg: alle mkv zu mp4 konvertieren"
  termask ask --provider openai "erkläre goroutines"
  termask ask --template shell "alle mkv zu mp4 konvertieren"
  termask ask --file main.go "review"
  echo "was ist ein inode?" | termask ask`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				ui.Err.Fprintf(os.Stderr, "Config: %v\n", err)
				ui.Warn.Fprintln(os.Stderr, "→ Führe `termask init` aus.")
				os.Exit(1)
			}
			registerConfiguredProviders(cfg)

			// Collect query from args or stdin
			var query string
			if len(args) > 0 {
				query = strings.Join(args, " ")
			} else {
				scanner := bufio.NewScanner(os.Stdin)
				scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
				var lines []string
				for scanner.Scan() {
					lines = append(lines, scanner.Text())
				}
				if err := scanner.Err(); err != nil {
					return err
				}
				query = strings.Join(lines, "\n")
			}
			if strings.TrimSpace(query) == "" {
				return fmt.Errorf("keine Frage angegeben")
			}
			if len(files) > 0 {
				ctxText, err := contextfiles.Build(files, 128*1024)
				if err != nil {
					return err
				}
				query = query + "\n\nContext files:\n" + ctxText
			}
			if templateName != "" {
				tpl, err := resolveTemplate(cfg, templateName)
				if err != nil {
					return err
				}
				query, err = prompttpl.Render(tpl.Prompt, query)
				if err != nil {
					return err
				}
			}

			// Resolve provider
			pName, pcfg, err := cfg.ActiveProviderConfig(providerName)
			if err != nil {
				return err
			}
			if modelOverride != "" {
				pcfg.Model = modelOverride
			}
			if _, err := provider.Get(pName); err != nil {
				return err
			}

			if err := continueError(cfg, continueLast); err != nil {
				return err
			}

			ctx := rootCtx()
			var prior []provider.Message
			var session history.Session
			var store history.Store
			if cfg.HistoryEnabled {
				historyPath, err := history.DefaultPath()
				if err != nil {
					return err
				}
				store = history.NewStoreWithLimit(historyPath, cfg.HistoryLimit)
				if continueLast {
					session, err = store.Latest()
					if err != nil {
						return err
					}
					if providerName == "" {
						providerName = session.Provider
						pName = session.Provider
					}
					// Reload so header/session model match the continued conversation.
					if _, fresh, err := cfg.ActiveProviderConfig(pName); err == nil {
						pcfg = fresh
					}
					// --model wins over the reloaded config copy for this run only.
					if modelOverride != "" {
						pcfg.Model = modelOverride
					}
					for _, msg := range session.Messages {
						prior = append(prior, provider.Message{Role: msg.Role, Content: msg.Content})
					}
				} else {
					session = history.NewSession(pName, pcfg.Model)
				}
			}

			if !plainOutput {
				ui.PrintHeader(query, pName, pcfg.Model)
			}

			renderOutput := shouldRender(plainOutput)
			out := io.Writer(os.Stdout)
			if renderOutput {
				out = nil
			}
			resp, err := ask.Run(ctx, cfg, ask.Request{
				ProviderName:  providerName,
				Query:         query,
				History:       prior,
				Out:           out,
				ModelOverride: modelOverride,
			})
			if err != nil {
				return fmt.Errorf("%s: %w", pName, err)
			}
			if renderOutput {
				fmt.Print(markdown.Render(resp.Text))
			}

			if !plainOutput {
				ui.PrintFooter()
			} else {
				fmt.Println()
			}
			if cfg.HistoryEnabled {
				if session.ID == "" {
					session = history.NewSession(resp.ProviderName, resp.Model)
				}
				session.Provider = resp.ProviderName
				session.Model = resp.Model
				session.AddUser(query)
				session.AddAssistant(resp.Text)
				if err := store.Save(session); err != nil {
					ui.Warn.Fprintf(os.Stderr, "history: %v\n", err)
				}
			}
			if savePath != "" {
				if err := os.WriteFile(savePath, []byte(resp.Text), 0600); err != nil {
					return err
				}
			}
			if safetyMode {
				printSafety(safety.AnalyzeShell(resp.Text))
			} else if !plainOutput {
				if report := safety.AnalyzeShell(resp.Text); hasRiskyCommands(report) {
					printSafety(report)
				}
			}
			return nil
		},
	}

	cmd.Flags().StringVarP(&providerName, "provider", "p", "", "Provider to use (overrides default)")
	cmd.Flags().StringVar(&modelOverride, "model", "", "Model to use for this run only (overrides config)")
	cmd.Flags().BoolVar(&plainOutput, "plain", false, "Plain output — no borders (for scripting)")
	cmd.Flags().BoolVar(&continueLast, "continue", false, "Continue the latest saved conversation")
	cmd.Flags().StringVar(&templateName, "template", "", "Prompt template to apply")
	cmd.Flags().StringArrayVar(&files, "file", nil, "Attach a text file as explicit context")
	cmd.Flags().StringVar(&savePath, "save", "", "Save the answer text to a file")
	cmd.Flags().BoolVar(&safetyMode, "safety", false, "Analyze shell commands in the answer for risky patterns")
	return cmd
}

func hasRiskyCommands(report safety.Report) bool {
	for _, cmd := range report.Commands {
		if cmd.Risk == safety.RiskMedium || cmd.Risk == safety.RiskHigh {
			return true
		}
	}
	return false
}

// ── chat ─────────────────────────────────────────────────────────────────────

func chatCmd() *cobra.Command {
	var providerName string
	var modelOverride string

	cmd := &cobra.Command{
		Use:   "chat",
		Short: "Start a simple multi-turn terminal chat",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			registerConfiguredProviders(cfg)
			pName, pcfg, err := cfg.ActiveProviderConfig(providerName)
			if err != nil {
				return err
			}
			if modelOverride != "" {
				pcfg.Model = modelOverride
			}
			session := history.NewSession(pName, pcfg.Model)
			storePath, err := history.DefaultPath()
			if err != nil {
				return err
			}
			store := history.NewStoreWithLimit(storePath, cfg.HistoryLimit)
			scanner := bufio.NewScanner(os.Stdin)
			scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
			ui.Prompt.Fprintf(os.Stderr, "termask chat [%s/%s] — /exit beendet, /new startet neu\n", pName, pcfg.Model)
			for {
				ui.Prompt.Fprint(os.Stderr, "\n› ")
				if !scanner.Scan() {
					break
				}
				query := strings.TrimSpace(scanner.Text())
				switch query {
				case "":
					continue
				case "/exit", "/quit":
					return nil
				case "/new":
					session = history.NewSession(pName, pcfg.Model)
					ui.Dim.Fprintln(os.Stderr, "Neue Session gestartet.")
					continue
				}
				prior := sessionMessages(session)
				turnCtx, stop := turnCtx()
				resp, err := ask.Run(turnCtx, cfg, ask.Request{
					ProviderName:  providerName,
					Query:         query,
					History:       prior,
					Out:           os.Stdout,
					ModelOverride: modelOverride,
				})
				stop()
				if err != nil {
					return err
				}
				if !strings.HasSuffix(resp.Text, "\n") {
					fmt.Println()
				}
				session.Provider = resp.ProviderName
				session.Model = resp.Model
				session.AddUser(query)
				session.AddAssistant(resp.Text)
				if cfg.HistoryEnabled {
					if err := store.Save(session); err != nil {
						ui.Warn.Fprintf(os.Stderr, "history: %v\n", err)
					}
				}
			}
			return scanner.Err()
		},
	}
	cmd.Flags().StringVarP(&providerName, "provider", "p", "", "Provider to use")
	cmd.Flags().StringVar(&modelOverride, "model", "", "Model to use for this run only (overrides config)")
	return cmd
}

// ── tui ──────────────────────────────────────────────────────────────────────

func tuiCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "tui",
		Short: "Start the optional terminal UI",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			registerConfiguredProviders(cfg)
			return tui.Run(rootCtx(), cfg)
		},
	}
}
