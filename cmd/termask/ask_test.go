package main

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/mmeister86/termask/internal/config"
)

func TestAskContinueRequiresHistory(t *testing.T) {
	cfg := config.Default()
	cfg.HistoryEnabled = false
	if err := continueError(cfg, true); err == nil {
		t.Fatal("continueError = nil, want --continue needs history_enabled=true")
	}
	if err := continueError(cfg, false); err != nil {
		t.Fatalf("continueError (no continue) = %v, want nil", err)
	}
	cfg.HistoryEnabled = true
	if err := continueError(cfg, true); err != nil {
		t.Fatalf("continueError (history on) = %v, want nil", err)
	}
}

func TestIsCanceled(t *testing.T) {
	if !isCanceled(fmt.Errorf("turn: %w", context.Canceled)) {
		t.Fatal("isCanceled(wrapped canceled) = false, want true")
	}
	if isCanceled(errors.New("boom")) {
		t.Fatal("isCanceled(other) = true, want false")
	}
	if isCanceled(nil) {
		t.Fatal("isCanceled(nil) = true, want false")
	}
}
