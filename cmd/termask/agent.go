package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"sort"
	"strings"

	agentpkg "github.com/mmeister86/termask/internal/agent"
	"github.com/mmeister86/termask/internal/config"
	"github.com/mmeister86/termask/internal/contextfiles"
	"github.com/mmeister86/termask/internal/history"
	"github.com/mmeister86/termask/internal/markdown"
	"github.com/mmeister86/termask/internal/ui"
	"github.com/spf13/cobra"
)

// ── agent ────────────────────────────────────────────────────────────────────

func agentCmd() *cobra.Command {
	var providerName string
	var modelOverride string
	var maxSteps int
	var files []string
	var plainOutput bool

	cmd := &cobra.Command{
		Use:   "agent [goal]",
		Short: "Start a read-only local agent session",
		Example: `  termask agent "inspect this project and suggest next steps"
  termask agent --file README.md "summarize what this CLI can do"
  termask agent --max-steps 4 "find tests related to history"
  termask agent --plain "summarize this project for a script"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				ui.Err.Fprintf(os.Stderr, "Config: %v\n", err)
				ui.Warn.Fprintln(os.Stderr, "→ Führe `termask init` aus.")
				os.Exit(1)
			}
			registerConfiguredProviders(cfg)

			var goal string
			if len(args) > 0 {
				goal = strings.Join(args, " ")
			}

			var explicitContext string
			if len(files) > 0 {
				explicitContext, err = contextfiles.Build(files, 128*1024)
				if err != nil {
					return err
				}
			}

			setupCtx, stopSetup := turnCtx()
			model, err := agentpkg.NewProviderModel(setupCtx, cfg, providerName, modelOverride)
			stopSetup()
			if err != nil {
				return err
			}
			workspace, err := os.Getwd()
			if err != nil {
				return err
			}
			runner := agentpkg.New(model, agentpkg.NewToolset(workspace))
			if shouldRunAgentSession(len(args) > 0, plainOutput, stdinIsTerminal(os.Stdin)) {
				return runAgentSession(context.Background(), cfg, runner, model, goal, explicitContext, maxSteps)
			}

			if strings.TrimSpace(goal) == "" {
				scanner := bufio.NewScanner(os.Stdin)
				scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
				var lines []string
				for scanner.Scan() {
					lines = append(lines, scanner.Text())
				}
				if err := scanner.Err(); err != nil {
					return err
				}
				goal = strings.Join(lines, "\n")
			}
			if strings.TrimSpace(goal) == "" {
				return fmt.Errorf("kein Ziel angegeben")
			}

			resp, err := runner.Run(rootCtx(), agentpkg.Request{
				Goal:     goal,
				MaxSteps: maxSteps,
				Context:  explicitContext,
			})
			if err != nil {
				return err
			}
			printAgentResponse(resp, maxSteps, plainOutput)

			if cfg.HistoryEnabled {
				store, err := defaultHistoryStore(cfg.HistoryLimit)
				if err != nil {
					return err
				}
				session := history.NewSession(model.ProviderName(), model.ModelName())
				session.AddUser(goal)
				session.AddAssistant(agentHistorySummary(resp))
				if err := store.Save(session); err != nil {
					ui.Warn.Fprintf(os.Stderr, "history: %v\n", err)
				}
			}
			return nil
		},
	}
	cmd.Flags().StringVarP(&providerName, "provider", "p", "", "Provider to use (overrides default)")
	cmd.Flags().StringVar(&modelOverride, "model", "", "Model to use for this run only (overrides config)")
	cmd.Flags().IntVar(&maxSteps, "max-steps", agentpkg.DefaultMaxSteps, "Maximum read-only agent steps")
	cmd.Flags().StringArrayVar(&files, "file", nil, "Attach a text file as explicit context")
	cmd.Flags().BoolVar(&plainOutput, "plain", false, "Plain output — no step display or markdown rendering")
	return cmd
}

func runAgentSession(
	ctx context.Context,
	cfg config.Config,
	runner *agentpkg.Agent,
	model *agentpkg.ProviderModel,
	initialGoal string,
	explicitContext string,
	maxSteps int,
) error {
	fmt.Printf("termask agent  [%s / %s]\n", model.ProviderName(), model.ModelName())
	fmt.Println(strings.Repeat("-", 72))
	fmt.Println("Commands: /new, /history, /quit")

	var prior []agentpkg.Message
	var session history.Session
	var store history.Store
	if cfg.HistoryEnabled {
		var err error
		store, err = defaultHistoryStore(cfg.HistoryLimit)
		if err != nil {
			return err
		}
		session = history.NewSession(model.ProviderName(), model.ModelName())
	}

	runTurn := func(goal string) error {
		turnCtx, stop := turnCtx()
		resp, err := runner.RunStream(turnCtx, agentpkg.Request{
			Goal:     goal,
			MaxSteps: maxSteps,
			Context:  explicitContext,
			History:  prior,
		}, func(event agentpkg.Event) {
			renderAgentEvent(event, maxSteps)
		})
		stop()
		if err != nil {
			return err
		}
		summary := agentHistorySummary(resp)
		prior = append(prior,
			agentpkg.Message{Role: "user", Content: goal},
			agentpkg.Message{Role: "assistant", Content: summary},
		)
		if cfg.HistoryEnabled {
			session.AddUser(goal)
			session.AddAssistant(summary)
			if err := store.Save(session); err != nil {
				ui.Warn.Fprintf(os.Stderr, "history: %v\n", err)
			}
		}
		return nil
	}

	if strings.TrimSpace(initialGoal) != "" {
		fmt.Println()
		if err := runTurn(initialGoal); err != nil {
			return err
		}
	}

	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	for {
		fmt.Print("\ntermask agent › ")
		if !scanner.Scan() {
			return scanner.Err()
		}
		query := strings.TrimSpace(scanner.Text())
		if query == "" {
			continue
		}
		if strings.HasPrefix(query, "/") {
			switch strings.Fields(query)[0] {
			case "/quit", "/exit", "/q":
				return nil
			case "/new":
				prior = nil
				if cfg.HistoryEnabled {
					session = history.NewSession(model.ProviderName(), model.ModelName())
				}
				fmt.Println("New agent session started.")
			case "/history":
				if !cfg.HistoryEnabled {
					fmt.Println("History is disabled.")
					continue
				}
				sessions, err := store.List()
				if err != nil {
					fmt.Fprintf(os.Stderr, "error: %v\n", err)
					continue
				}
				for _, s := range sessions {
					fmt.Printf("%s  %s/%s  %d messages\n", s.ID, s.Provider, s.Model, len(s.Messages))
				}
			default:
				fmt.Fprintf(os.Stderr, "unknown command %q\n", strings.Fields(query)[0])
			}
			continue
		}
		fmt.Println()
		if err := runTurn(query); err != nil {
			fmt.Fprintf(os.Stderr, "\nerror: %v\n", err)
		}
	}
}

func shouldRunAgentSession(_ bool, plainOutput bool, stdinIsTerminal bool) bool {
	return stdinIsTerminal && !plainOutput
}
func stdinIsTerminal(file *os.File) bool {
	info, err := file.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}
func printAgentResponse(resp agentpkg.Response, maxSteps int, plainOutput bool) {
	if !plainOutput {
		for i, step := range resp.Steps {
			ui.Dim.Fprintf(os.Stderr, "step %d/%d: %s%s\n", i+1, effectiveMaxSteps(maxSteps), step.Tool, formatAgentArgs(step.Args))
		}
	}
	if shouldRender(plainOutput) {
		fmt.Print(markdown.Render(resp.Text))
		return
	}
	fmt.Print(resp.Text)
	if !strings.HasSuffix(resp.Text, "\n") {
		fmt.Println()
	}
}
func renderAgentEvent(event agentpkg.Event, maxSteps int) {
	switch event.Type {
	case agentpkg.EventAnswerDelta:
		fmt.Print(event.Text)
	case agentpkg.EventAnswerDone:
		fmt.Println()
	default:
		if status := formatAgentStatus(event, maxSteps); status != "" {
			ui.Dim.Fprintln(os.Stderr, status)
		}
	}
}
func formatAgentStatus(event agentpkg.Event, maxSteps int) string {
	switch event.Type {
	case agentpkg.EventModelStart:
		return fmt.Sprintf("thinking step %d/%d...", event.Step, effectiveMaxSteps(maxSteps))
	case agentpkg.EventToolStart:
		return fmt.Sprintf("-> %s%s", event.Tool, formatAgentArgs(event.Args))
	case agentpkg.EventToolEnd:
		if !event.Result.OK {
			return fmt.Sprintf("failed %s: %s", event.Tool, event.Result.Error)
		}
		return fmt.Sprintf("done %s", event.Tool)
	case agentpkg.EventError:
		if event.Err != nil {
			return "error: " + event.Err.Error()
		}
	}
	return ""
}
func effectiveMaxSteps(maxSteps int) int {
	if maxSteps <= 0 {
		return agentpkg.DefaultMaxSteps
	}
	return maxSteps
}
func formatAgentArgs(args map[string]string) string {
	if len(args) == 0 {
		return ""
	}
	keys := make([]string, 0, len(args))
	for key := range args {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, fmt.Sprintf(" %s=%q", key, args[key]))
	}
	return strings.Join(parts, "")
}
func agentHistorySummary(resp agentpkg.Response) string {
	var out strings.Builder
	if len(resp.Steps) > 0 {
		out.WriteString("Agent steps:\n")
		for i, step := range resp.Steps {
			fmt.Fprintf(&out, "%d. %s%s\n", i+1, step.Tool, formatAgentArgs(step.Args))
		}
		out.WriteByte('\n')
	}
	out.WriteString(resp.Text)
	return out.String()
}
