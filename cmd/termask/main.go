package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"github.com/yourusername/termask/internal/ask"
	"github.com/yourusername/termask/internal/config"
	"github.com/yourusername/termask/internal/contextfiles"
	"github.com/yourusername/termask/internal/doctor"
	termexport "github.com/yourusername/termask/internal/export"
	"github.com/yourusername/termask/internal/history"
	"github.com/yourusername/termask/internal/markdown"
	"github.com/yourusername/termask/internal/provider"
	"github.com/yourusername/termask/internal/safety"
	prompttpl "github.com/yourusername/termask/internal/template"
	"github.com/yourusername/termask/internal/tui"
	"github.com/yourusername/termask/internal/ui"

	// Register all providers at startup via side-effect imports.
	anthropicprovider "github.com/yourusername/termask/internal/provider/anthropic"
	ollamaprovider "github.com/yourusername/termask/internal/provider/ollama"
	openaiprovider "github.com/yourusername/termask/internal/provider/openai"
)

func init() {
	provider.Register(anthropicprovider.New())
	provider.Register(openaiprovider.New())
	provider.Register(ollamaprovider.New())

	// OpenAI-compatible providers
	provider.Register(openaiprovider.NewCompatible(
		"groq",
		"https://api.groq.com/openai/v1/chat/completions",
		[]provider.Model{
			{ID: "openai/gpt-oss-120b", Description: "GPT-OSS 120B — flagship open-weight"},
			{ID: "openai/gpt-oss-20b", Description: "GPT-OSS 20B — fastest open-weight"},
			{ID: "llama-3.3-70b-versatile", Description: "Llama 3.3 70B — general purpose"},
			{ID: "llama-3.1-8b-instant", Description: "Llama 3.1 8B — low latency"},
			{ID: "groq/compound", Description: "Compound — model system with tools"},
			{ID: "groq/compound-mini", Description: "Compound Mini — faster model system"},
		},
	))
	provider.Register(openaiprovider.NewCompatible(
		"together",
		"https://api.together.xyz/v1/chat/completions",
		[]provider.Model{
			{ID: "moonshotai/Kimi-K2.5", Description: "Kimi K2.5 — recommended chat/reasoning"},
			{ID: "zai-org/GLM-5.1", Description: "GLM-5.1 — recommended coding/function calling"},
			{ID: "openai/gpt-oss-120b", Description: "GPT-OSS 120B — medium general purpose"},
			{ID: "openai/gpt-oss-20b", Description: "GPT-OSS 20B — small & fast"},
			{ID: "deepseek-ai/DeepSeek-V3.1", Description: "DeepSeek V3.1 — hybrid reasoning"},
			{ID: "Qwen/Qwen3-Coder-480B-A35B-Instruct-FP8", Description: "Qwen3 Coder — coding agents"},
			{ID: "meta-llama/Llama-3.3-70B-Instruct-Turbo", Description: "Llama 3.3 70B — general purpose"},
		},
	))
}

func main() {
	root := &cobra.Command{
		Use:   "termask",
		Short: "AI-powered terminal assistant — BYOK",
	}

	root.AddCommand(
		askCmd(),
		chatCmd(),
		tuiCmd(),
		initCmd(),
		switchCmd(),
		providersCmd(),
		modelsCmd(),
		historyCmd(),
		templatesCmd(),
		configCmd(),
		doctorCmd(),
		shellCmd(),
	)

	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}

// ── ask ───────────────────────────────────────────────────────────────────────

func askCmd() *cobra.Command {
	var providerName string
	var plainOutput bool
	var continueLast bool
	var templateName string
	var files []string
	var savePath string
	var safetyMode bool
	var renderMarkdown bool

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
				var lines []string
				for scanner.Scan() {
					lines = append(lines, scanner.Text())
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
			if _, err := provider.Get(pName); err != nil {
				return err
			}

			if !plainOutput {
				ui.PrintHeader(query, pName, pcfg.Model)
			}

			ctx := context.Background()
			var prior []provider.Message
			var session history.Session
			var store history.Store
			if cfg.HistoryEnabled {
				historyPath, err := history.DefaultPath()
				if err != nil {
					return err
				}
				store = history.NewStore(historyPath)
				if continueLast {
					session, err = store.Latest()
					if err != nil {
						return err
					}
					if providerName == "" {
						providerName = session.Provider
						pName = session.Provider
					}
					for _, msg := range session.Messages {
						prior = append(prior, provider.Message{Role: msg.Role, Content: msg.Content})
					}
				} else {
					session = history.NewSession(pName, pcfg.Model)
				}
			}

			out := io.Writer(os.Stdout)
			if renderMarkdown || cfg.RenderMarkdown {
				out = nil
			}
			resp, err := ask.Run(ctx, cfg, ask.Request{
				ProviderName: providerName,
				Query:        query,
				History:      prior,
				Out:          out,
			})
			if err != nil {
				return fmt.Errorf("%s: %w", pName, err)
			}
			if renderMarkdown || cfg.RenderMarkdown {
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
			}
			return nil
		},
	}

	cmd.Flags().StringVarP(&providerName, "provider", "p", "", "Provider to use (overrides default)")
	cmd.Flags().BoolVar(&plainOutput, "plain", false, "Plain output — no borders (for scripting)")
	cmd.Flags().BoolVar(&continueLast, "continue", false, "Continue the latest saved conversation")
	cmd.Flags().StringVar(&templateName, "template", "", "Prompt template to apply")
	cmd.Flags().StringArrayVar(&files, "file", nil, "Attach a text file as explicit context")
	cmd.Flags().StringVar(&savePath, "save", "", "Save the answer text to a file")
	cmd.Flags().BoolVar(&safetyMode, "safety", false, "Analyze shell commands in the answer for risky patterns")
	cmd.Flags().BoolVar(&renderMarkdown, "render", false, "Render Markdown when supported by the terminal")
	return cmd
}

// ── chat ─────────────────────────────────────────────────────────────────────

func chatCmd() *cobra.Command {
	var providerName string

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
			session := history.NewSession(pName, pcfg.Model)
			storePath, err := history.DefaultPath()
			if err != nil {
				return err
			}
			store := history.NewStore(storePath)
			scanner := bufio.NewScanner(os.Stdin)
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
				resp, err := ask.Run(context.Background(), cfg, ask.Request{
					ProviderName: providerName,
					Query:        query,
					History:      prior,
					Out:          os.Stdout,
				})
				if err != nil {
					return err
				}
				fmt.Println()
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
			return tui.Run(context.Background(), cfg)
		},
	}
}

// ── init ──────────────────────────────────────────────────────────────────────

func initCmd() *cobra.Command {
	var providerName string
	var apiKey string
	var model string

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Add or update a provider's API key",
		Example: `  termask init                          # interaktiv (Anthropic)
  termask init --provider openai         # OpenAI API Key setzen
  termask init --provider groq --key sk-groq-...`,
		RunE: func(cmd *cobra.Command, args []string) error {
			scanner := bufio.NewScanner(os.Stdin)
			cfg, _ := config.Load()
			registerConfiguredProviders(cfg)

			if providerName == "" {
				fmt.Printf("Provider [anthropic/openai/groq/together/ollama] (default: anthropic): ")
				scanner.Scan()
				providerName = strings.TrimSpace(scanner.Text())
				if providerName == "" {
					providerName = "anthropic"
				}
			}

			if providerName != "ollama" && apiKey == "" {
				fmt.Printf("API Key für %s: ", providerName)
				scanner.Scan()
				apiKey = strings.TrimSpace(scanner.Text())
				if apiKey == "" {
					return fmt.Errorf("API Key darf nicht leer sein")
				}
			}

			if model == "" {
				p, err := provider.Get(providerName)
				if err == nil {
					models, _ := p.Models(context.Background(), provider.ProviderConfig{})
					if len(models) > 0 {
						fmt.Printf("Model (default: %s): ", models[0].ID)
						scanner.Scan()
						m := strings.TrimSpace(scanner.Text())
						if m != "" {
							model = m
						}
					}
				}
			}

			if err := config.SetProviderKey(providerName, apiKey, model); err != nil {
				return err
			}

			path, _ := config.ConfigPath()
			ui.Success.Printf("\n✓ %s konfiguriert — gespeichert in %s\n", providerName, path)
			ui.Dim.Printf("  Zum Standard machen: termask switch %s\n", providerName)
			return nil
		},
	}

	cmd.Flags().StringVar(&providerName, "provider", "", "Provider-Name")
	cmd.Flags().StringVar(&apiKey, "key", "", "API Key (alternativ: interaktive Eingabe)")
	cmd.Flags().StringVar(&model, "model", "", "Standard-Modell für diesen Provider")
	return cmd
}

// ── switch ────────────────────────────────────────────────────────────────────

func switchCmd() *cobra.Command {
	var interactive bool
	cmd := &cobra.Command{
		Use:     "switch <provider>",
		Short:   "Set the default provider",
		Example: "  termask switch openai",
		Args: func(cmd *cobra.Command, args []string) error {
			if interactive {
				return nil
			}
			return cobra.ExactArgs(1)(cmd, args)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, _ := config.Load()
			registerConfiguredProviders(cfg)
			name := ""
			if interactive {
				selected, err := chooseProvider()
				if err != nil {
					return err
				}
				name = selected
			} else {
				name = args[0]
			}
			if _, err := provider.Get(name); err != nil {
				return err
			}
			if err := config.SetDefault(name); err != nil {
				return err
			}
			ui.Success.Printf("✓ Standard-Provider: %s\n", name)
			return nil
		},
	}
	cmd.Flags().BoolVarP(&interactive, "interactive", "i", false, "Choose provider interactively")
	return cmd
}

// ── providers ─────────────────────────────────────────────────────────────────

func providersCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "providers",
		Short: "List all available providers and their status",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, _ := config.Load()
			registerConfiguredProviders(cfg)

			names := provider.List()
			sort.Strings(names)

			w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
			fmt.Fprintln(w, "PROVIDER\tMODEL\tAPI KEY\tSTATUS")
			fmt.Fprintln(w, "────────\t─────\t───────\t──────")

			for _, name := range names {
				pcfg := cfg.Providers[name]
				model := pcfg.Model
				if model == "" {
					model = "-"
				}
				keyStatus := "✗ not set"
				if pcfg.APIKey != "" {
					keyStatus = "✓ " + maskKey(pcfg.APIKey)
				} else if name == "ollama" {
					keyStatus = "— (local)"
				}
				status := ""
				if name == cfg.DefaultProvider {
					status = "★ default"
				}
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", name, model, keyStatus, status)
			}
			w.Flush()

			fmt.Println()
			ui.Dim.Println("  termask init --provider <name>   API Key hinzufügen")
			ui.Dim.Println("  termask switch <name>            Standard wechseln")
			return nil
		},
	}
}

// ── models ────────────────────────────────────────────────────────────────────

func modelsCmd() *cobra.Command {
	var providerName string
	var selectModel bool

	cmd := &cobra.Command{
		Use:   "models",
		Short: "List available models for a provider",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, _ := config.Load()
			registerConfiguredProviders(cfg)

			if providerName == "" {
				providerName = cfg.DefaultProvider
			}

			p, err := provider.Get(providerName)
			if err != nil {
				return err
			}

			_, pcfg, _ := cfg.ActiveProviderConfig(providerName)
			models, err := p.Models(context.Background(), pcfg)
			if err != nil {
				return fmt.Errorf("models: %w", err)
			}

			ui.Prompt.Printf("Models für %s:\n\n", providerName)
			w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
			for i, m := range models {
				marker := "  "
				if m.ID == pcfg.Model {
					marker = "★ "
				}
				prefix := marker
				if selectModel {
					prefix = fmt.Sprintf("%d) %s", i+1, marker)
				}
				fmt.Fprintf(w, "%s%s\t%s\n", prefix, m.ID, m.Description)
			}
			w.Flush()

			fmt.Println()
			if selectModel {
				selected, err := chooseModel(models)
				if err != nil {
					return err
				}
				if err := config.SetProviderKey(providerName, "", selected); err != nil {
					return err
				}
				ui.Success.Printf("✓ Modell für %s: %s\n", providerName, selected)
			} else {
				ui.Dim.Printf("  Modell wechseln: termask init --provider %s --model <id>\n", providerName)
			}
			return nil
		},
	}

	cmd.Flags().StringVarP(&providerName, "provider", "p", "", "Provider (default: aktiver Default)")
	cmd.Flags().BoolVar(&selectModel, "select", false, "Choose and save a model interactively")
	return cmd
}

// ── history ──────────────────────────────────────────────────────────────────

func historyCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "history",
		Short: "Inspect and export saved conversations",
	}
	cmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List saved sessions",
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := defaultHistoryStore()
			if err != nil {
				return err
			}
			sessions, err := store.List()
			if err != nil {
				return err
			}
			w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
			fmt.Fprintln(w, "ID\tPROVIDER\tMODEL\tMESSAGES\tUPDATED")
			for _, session := range sessions {
				fmt.Fprintf(w, "%s\t%s\t%s\t%d\t%s\n",
					session.ID, session.Provider, session.Model, len(session.Messages), session.UpdatedAt.Format("2006-01-02 15:04"))
			}
			return w.Flush()
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "show <id>",
		Short: "Show a session",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := defaultHistoryStore()
			if err != nil {
				return err
			}
			session, err := store.Get(args[0])
			if err != nil {
				return err
			}
			fmt.Print(termexport.SessionMarkdown(session))
			return nil
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "export <id> <path>",
		Short: "Export a session as Markdown",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := defaultHistoryStore()
			if err != nil {
				return err
			}
			session, err := store.Get(args[0])
			if err != nil {
				return err
			}
			return os.WriteFile(args[1], []byte(termexport.SessionMarkdown(session)), 0600)
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "clear",
		Short: "Delete saved history",
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := defaultHistoryStore()
			if err != nil {
				return err
			}
			if err := store.Clear(); err != nil {
				return err
			}
			ui.Success.Fprintln(os.Stderr, "✓ History gelöscht")
			return nil
		},
	})
	return cmd
}

// ── templates ────────────────────────────────────────────────────────────────

func templatesCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "templates",
		Short: "List and inspect prompt templates",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, _ := config.Load()
			templates := mergedTemplates(cfg)
			names := make([]string, 0, len(templates))
			for name := range templates {
				names = append(names, name)
			}
			sort.Strings(names)
			w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
			fmt.Fprintln(w, "NAME\tDESCRIPTION")
			for _, name := range names {
				fmt.Fprintf(w, "%s\t%s\n", name, templates[name].Description)
			}
			return w.Flush()
		},
	}
	cmd.AddCommand(&cobra.Command{
		Use:   "show <name>",
		Short: "Show a prompt template",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, _ := config.Load()
			tpl, err := resolveTemplate(cfg, args[0])
			if err != nil {
				return err
			}
			fmt.Println(tpl.Prompt)
			return nil
		},
	})
	return cmd
}

// ── config ───────────────────────────────────────────────────────────────────

func configCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Inspect and edit termask configuration",
	}
	cmd.AddCommand(&cobra.Command{
		Use:   "path",
		Short: "Print config path",
		RunE: func(cmd *cobra.Command, args []string) error {
			path, err := config.ConfigPath()
			if err != nil {
				return err
			}
			fmt.Println(path)
			return nil
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "get <key>",
		Short: "Get a supported config value",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			switch args[0] {
			case "default_provider":
				fmt.Println(cfg.DefaultProvider)
			case "system_prompt":
				fmt.Println(cfg.SystemPrompt)
			case "history_enabled":
				fmt.Println(cfg.HistoryEnabled)
			case "render_markdown":
				fmt.Println(cfg.RenderMarkdown)
			default:
				return fmt.Errorf("unsupported config key %q", args[0])
			}
			return nil
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "set <key> <value>",
		Short: "Set a supported config value",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			switch args[0] {
			case "default_provider":
				cfg.DefaultProvider = args[1]
			case "system_prompt":
				cfg.SystemPrompt = args[1]
			case "history_enabled":
				cfg.HistoryEnabled = args[1] == "true"
			case "render_markdown":
				cfg.RenderMarkdown = args[1] == "true"
			default:
				return fmt.Errorf("unsupported config key %q", args[0])
			}
			return config.Save(cfg)
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "edit",
		Short: "Open config in $EDITOR",
		RunE: func(cmd *cobra.Command, args []string) error {
			path, err := config.ConfigPath()
			if err != nil {
				return err
			}
			editor := os.Getenv("EDITOR")
			if editor == "" {
				editor = "vi"
			}
			c := exec.Command(editor, path)
			c.Stdin = os.Stdin
			c.Stdout = os.Stdout
			c.Stderr = os.Stderr
			return c.Run()
		},
	})
	return cmd
}

// ── doctor ───────────────────────────────────────────────────────────────────

func doctorCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Check termask configuration and setup",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				ui.Warn.Fprintf(os.Stderr, "Config load: %v\n", err)
				cfg = config.Default()
			}
			registerConfiguredProviders(cfg)
			path, _ := config.ConfigPath()
			result := doctor.CheckConfig(path, cfg, provider.List())
			for _, item := range result.Items {
				switch item.Level {
				case doctor.LevelOK:
					ui.Success.Fprintf(os.Stdout, "✓ %s\n", item.Message)
				case doctor.LevelWarn:
					ui.Warn.Fprintf(os.Stdout, "! %s\n", item.Message)
				default:
					ui.Err.Fprintf(os.Stdout, "✗ %s\n", item.Message)
				}
			}
			if !result.OK {
				return fmt.Errorf("doctor found issues")
			}
			return nil
		},
	}
}

// ── shell ─────────────────────────────────────────────────────────────────────

func shellCmd() *cobra.Command {
	var shell string
	cmd := &cobra.Command{
		Use:   "shell",
		Short: "Print shell integration snippet",
		RunE: func(cmd *cobra.Command, args []string) error {
			switch shell {
			case "zsh":
				fmt.Print(zshPlugin())
			case "bash":
				fmt.Print(bashPlugin())
			default:
				return fmt.Errorf("unbekannte Shell %q — nutze --shell zsh oder --shell bash", shell)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&shell, "shell", "zsh", "Shell: zsh oder bash")
	return cmd
}

// ── helpers ───────────────────────────────────────────────────────────────────

func maskKey(key string) string {
	if len(key) <= 8 {
		return "****"
	}
	return key[:4] + "..." + key[len(key)-4:]
}

func registerConfiguredProviders(cfg config.Config) {
	for name, pcfg := range cfg.Providers {
		if pcfg.Type == "openai-compatible" && pcfg.BaseURL != "" {
			provider.Register(openaiprovider.NewCompatible(name, pcfg.BaseURL, pcfg.Models))
		}
	}
}

func defaultHistoryStore() (history.Store, error) {
	path, err := history.DefaultPath()
	if err != nil {
		return history.Store{}, err
	}
	return history.NewStore(path), nil
}

func sessionMessages(session history.Session) []provider.Message {
	messages := make([]provider.Message, 0, len(session.Messages))
	for _, msg := range session.Messages {
		messages = append(messages, provider.Message{Role: msg.Role, Content: msg.Content})
	}
	return messages
}

func mergedTemplates(cfg config.Config) map[string]prompttpl.Template {
	templates := prompttpl.Builtins()
	for name, tpl := range cfg.Templates {
		templates[name] = prompttpl.Template{Description: tpl.Description, Prompt: tpl.Prompt}
	}
	return templates
}

func resolveTemplate(cfg config.Config, name string) (prompttpl.Template, error) {
	tpl, ok := mergedTemplates(cfg)[name]
	if !ok {
		return prompttpl.Template{}, fmt.Errorf("unknown template %q — run `termask templates`", name)
	}
	return tpl, nil
}

func chooseProvider() (string, error) {
	names := provider.List()
	sort.Strings(names)
	for i, name := range names {
		fmt.Fprintf(os.Stderr, "%d) %s\n", i+1, name)
	}
	fmt.Fprint(os.Stderr, "Provider: ")
	scanner := bufio.NewScanner(os.Stdin)
	if !scanner.Scan() {
		return "", scanner.Err()
	}
	var idx int
	if _, err := fmt.Sscanf(strings.TrimSpace(scanner.Text()), "%d", &idx); err != nil {
		return "", fmt.Errorf("enter a number")
	}
	if idx < 1 || idx > len(names) {
		return "", fmt.Errorf("provider selection out of range")
	}
	return names[idx-1], nil
}

func chooseModel(models []provider.Model) (string, error) {
	if len(models) == 0 {
		return "", fmt.Errorf("no models available")
	}
	fmt.Fprint(os.Stderr, "Model number: ")
	scanner := bufio.NewScanner(os.Stdin)
	if !scanner.Scan() {
		return "", scanner.Err()
	}
	var idx int
	if _, err := fmt.Sscanf(strings.TrimSpace(scanner.Text()), "%d", &idx); err != nil {
		return "", fmt.Errorf("enter a number")
	}
	if idx < 1 || idx > len(models) {
		return "", fmt.Errorf("model selection out of range")
	}
	return models[idx-1].ID, nil
}

func printSafety(report safety.Report) {
	if len(report.Commands) == 0 {
		ui.Dim.Fprintln(os.Stderr, "No shell commands detected for safety analysis.")
		return
	}
	fmt.Fprintln(os.Stderr)
	ui.Prompt.Fprintln(os.Stderr, "Shell safety:")
	for _, cmd := range report.Commands {
		switch cmd.Risk {
		case safety.RiskHigh:
			ui.Err.Fprintf(os.Stderr, "HIGH   %s\n", cmd.Text)
		case safety.RiskMedium:
			ui.Warn.Fprintf(os.Stderr, "MEDIUM %s\n", cmd.Text)
		default:
			ui.Success.Fprintf(os.Stderr, "LOW    %s\n", cmd.Text)
		}
		for _, reason := range cmd.Reasons {
			ui.Dim.Fprintf(os.Stderr, "       - %s\n", reason)
		}
	}
}

func zshPlugin() string {
	return `# ── termask zsh plugin ─────────────────────────────────────────────────────
# Zu ~/.zshrc hinzufügen (oder in separate Datei auslagern und sourcen)

# Falls ~/.local/bin noch nicht im PATH ist.
case ":$PATH:" in
  *":$HOME/.local/bin:"*) ;;
  *) export PATH="$HOME/.local/bin:$PATH" ;;
esac

_termask_ask() {
  local query provider_flag=""

  # Optionaler Provider-Override: TERMASK_PROVIDER=openai Ctrl+K
  [[ -n "$TERMASK_PROVIDER" ]] && provider_flag="--provider $TERMASK_PROVIDER"

  if [[ -n "$BUFFER" ]]; then
    query="$BUFFER"
    BUFFER=""
    zle reset-prompt
  else
    echo -n "\n  termask › "
    read -r query
  fi

  [[ -z "$query" ]] && zle reset-prompt && return

  eval "termask ask $provider_flag \"$query\"" | less -FIRX
  zle reset-prompt
}

zle -N _termask_ask
bindkey "^K" _termask_ask   # Ctrl+K — nach Belieben ändern
# ────────────────────────────────────────────────────────────────────────────
`
}

func bashPlugin() string {
	return `# ── termask bash plugin ────────────────────────────────────────────────────
# Zu ~/.bashrc hinzufügen

# Falls ~/.local/bin noch nicht im PATH ist.
case ":$PATH:" in
  *":$HOME/.local/bin:"*) ;;
  *) export PATH="$HOME/.local/bin:$PATH" ;;
esac

_termask_ask() {
  local query="$READLINE_LINE"
  local provider_flag=""
  [[ -n "$TERMASK_PROVIDER" ]] && provider_flag="--provider $TERMASK_PROVIDER"

  if [[ -z "$query" ]]; then
    echo -n $'\n  termask › '
    read -r query
  else
    READLINE_LINE=""
    READLINE_POINT=0
  fi

  [[ -z "$query" ]] && return

  echo ""
  eval "termask ask $provider_flag \"$query\"" | less -FIRX
}

bind -x '"\C-k": _termask_ask'
# ────────────────────────────────────────────────────────────────────────────
`
}
