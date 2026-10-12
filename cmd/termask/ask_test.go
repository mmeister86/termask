package main

import (
	"testing"

	"github.com/yourusername/termask/internal/config"
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
