package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"github.com/yourusername/termask/internal/config"
	"github.com/yourusername/termask/internal/provider"
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
		initCmd(),
		switchCmd(),
		providersCmd(),
		modelsCmd(),
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

	cmd := &cobra.Command{
		Use:   "ask [question]",
		Short: "Ask a question (streams the answer)",
		Example: `  termask ask "ffmpeg: alle mkv zu mp4 konvertieren"
  termask ask --provider openai "erkläre goroutines"
  echo "was ist ein inode?" | termask ask`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				ui.Err.Fprintf(os.Stderr, "Config: %v\n", err)
				ui.Warn.Fprintln(os.Stderr, "→ Führe `termask init` aus.")
				os.Exit(1)
			}

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

			// Resolve provider
			pName, pcfg, err := cfg.ActiveProviderConfig(providerName)
			if err != nil {
				return err
			}
			p, err := provider.Get(pName)
			if err != nil {
				return err
			}

			if !plainOutput {
				ui.PrintHeader(query, pName, pcfg.Model)
			}

			ctx := context.Background()
			if err := p.Ask(ctx, pcfg, cfg.SystemPrompt, query, os.Stdout); err != nil {
				return fmt.Errorf("%s: %w", pName, err)
			}

			if !plainOutput {
				ui.PrintFooter()
			} else {
				fmt.Println()
			}
			return nil
		},
	}

	cmd.Flags().StringVarP(&providerName, "provider", "p", "", "Provider to use (overrides default)")
	cmd.Flags().BoolVar(&plainOutput, "plain", false, "Plain output — no borders (for scripting)")
	return cmd
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
	return &cobra.Command{
		Use:     "switch <provider>",
		Short:   "Set the default provider",
		Example: "  termask switch openai",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
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
}

// ── providers ─────────────────────────────────────────────────────────────────

func providersCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "providers",
		Short: "List all available providers and their status",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, _ := config.Load()

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

	cmd := &cobra.Command{
		Use:   "models",
		Short: "List available models for a provider",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, _ := config.Load()

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
			for _, m := range models {
				marker := "  "
				if m.ID == pcfg.Model {
					marker = "★ "
				}
				fmt.Fprintf(w, "%s%s\t%s\n", marker, m.ID, m.Description)
			}
			w.Flush()

			fmt.Println()
			ui.Dim.Printf("  Modell wechseln: termask init --provider %s --model <id>\n", providerName)
			return nil
		},
	}

	cmd.Flags().StringVarP(&providerName, "provider", "p", "", "Provider (default: aktiver Default)")
	return cmd
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

func zshPlugin() string {
	return `# ── termask zsh plugin ─────────────────────────────────────────────────────
# Zu ~/.zshrc hinzufügen (oder in separate Datei auslagern und sourcen)

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
