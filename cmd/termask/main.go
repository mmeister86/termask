package main

import (
	"context"
	"os"
	"os/signal"

	"github.com/spf13/cobra"
	"github.com/mmeister86/termask/internal/provider"

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

// appCtx is cancelled on Ctrl+C (SIGINT) so in-flight provider calls abort
// instead of hanging until the server responds. Process-lifetime: no stop call.
var appCtx, _ = signal.NotifyContext(context.Background(), os.Interrupt)

// rootCtx returns the shared cancellable context for all commands.
func rootCtx() context.Context { return appCtx }

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
