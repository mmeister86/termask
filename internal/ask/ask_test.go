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

func stubConfig() config.Config {
	cfg := config.Default()
	cfg.DefaultProvider = "stub-blocking-test"
	cfg.Providers["stub-blocking-test"] = provider.ProviderConfig{Model: "stub-model"}
	return cfg
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
