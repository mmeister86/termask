package ask

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/mmeister86/termask/internal/config"
	"github.com/mmeister86/termask/internal/provider"
)

type blockingStub struct{}

func (blockingStub) Name() string { return "stub-blocking-test" }

func (blockingStub) Models(context.Context, provider.ProviderConfig) ([]provider.Model, error) {
	return []provider.Model{{ID: "stub-model"}}, nil
}

func (blockingStub) Ask(ctx context.Context, _ provider.ProviderConfig, _, _ string, _ io.Writer) error {
	<-ctx.Done()
	return ctx.Err()
}

func (blockingStub) AskMessages(ctx context.Context, _ provider.ProviderConfig, _ string, _ []provider.Message, _ io.Writer) error {
	<-ctx.Done()
	return ctx.Err()
}

var seenModel string

type modelOverrideStub struct{}

func (modelOverrideStub) Name() string { return "stub-model-override-test" }

func (modelOverrideStub) Models(context.Context, provider.ProviderConfig) ([]provider.Model, error) {
	return []provider.Model{{ID: "stub-model"}}, nil
}

func (modelOverrideStub) Ask(_ context.Context, pcfg provider.ProviderConfig, _, _ string, w io.Writer) error {
	seenModel = pcfg.Model
	_, _ = io.WriteString(w, "ok")
	return nil
}

func (modelOverrideStub) AskMessages(_ context.Context, pcfg provider.ProviderConfig, _ string, _ []provider.Message, w io.Writer) error {
	seenModel = pcfg.Model
	_, _ = io.WriteString(w, "ok")
	return nil
}

func stubConfig() config.Config {
	cfg := config.Default()
	cfg.DefaultProvider = "stub-blocking-test"
	cfg.Providers["stub-blocking-test"] = provider.ProviderConfig{Model: "stub-model"}
	return cfg
}

func TestRunModelOverride(t *testing.T) {
	provider.Register(modelOverrideStub{})
	cfg := stubConfig()
	cfg.DefaultProvider = "stub-model-override-test"
	cfg.Providers["stub-model-override-test"] = provider.ProviderConfig{Model: "stub-model"}

	resp, err := Run(context.Background(), cfg, Request{
		Query:         "hello?",
		ModelOverride: "override-model",
	})
	if err != nil {
		t.Fatalf("Run err = %v", err)
	}
	if resp.Model != "override-model" {
		t.Fatalf("resp.Model = %q, want %q", resp.Model, "override-model")
	}
	if seenModel != "override-model" {
		t.Fatalf("provider saw model = %q, want %q", seenModel, "override-model")
	}
	if cfg.Providers["stub-model-override-test"].Model != "stub-model" {
		t.Fatalf("persisted config mutated: Model = %q", cfg.Providers["stub-model-override-test"].Model)
	}
}

func TestRunRespectsCancelledContext(t *testing.T) {
	provider.Register(blockingStub{})

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // pre-cancelled: a blocking provider must return promptly with a context error

	done := make(chan error, 1)
	go func() {
		_, err := Run(ctx, stubConfig(), Request{
			Query:   "hello?",
			History: []provider.Message{{Role: "user", Content: "prior"}},
		})
		done <- err
	}()

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Run err = %v, want context.Canceled", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return promptly on cancelled context")
	}
}
