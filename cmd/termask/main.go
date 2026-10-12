package main

import (
	"context"
	"os"
	"os/signal"
	"sync"

	"github.com/mmeister86/termask/internal/provider"
	"github.com/spf13/cobra"

	// Register all providers at startup via side-effect imports.
	anthropicprovider "github.com/mmeister86/termask/internal/provider/anthropic"
	ollamaprovider "github.com/mmeister86/termask/internal/provider/ollama"
	openaiprovider "github.com/mmeister86/termask/internal/provider/openai"
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

// rootCtx is the shared process-lifetime context, created lazily so the
// SIGINT handler is registered only when first needed. The stop func is
// released once the context is done, so Ctrl+C at idle prompts terminates.
var rootCtx = sync.OnceValue(func() context.Context {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	go func() { <-ctx.Done(); stop() }()
	return ctx
})

// turnCtx returns a fresh per-turn cancellable context for long-lived loops
// (chat, agent session, TUI requests). Never reuse rootCtx() per turn: it is
// cancelled permanently by the first Ctrl+C, poisoning every later request.
// Call stop when the turn finishes to release the signal handler.
func turnCtx() (context.Context, context.CancelFunc) {
	return signal.NotifyContext(context.Background(), os.Interrupt)
}

func main() {
	root := &cobra.Command{
		Use:   "termask",
		Short: "AI-powered terminal assistant — BYOK",
	}

	root.AddCommand(
		askCmd(),
		agentCmd(),
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
