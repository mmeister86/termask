package main

import (
	"testing"
)

// rootCtx() must return the same non-cancelled context on repeated calls
// (lazy singleton): the SIGINT handler is registered once, and no stray
// Ctrl+C from an earlier run poisons later commands.
func TestRootCtxIsStableSingleton(t *testing.T) {
	a := rootCtx()
	b := rootCtx()
	if a != b {
		t.Fatal("rootCtx() returned different contexts on repeated calls")
	}
	select {
	case <-a.Done():
		t.Fatal("rootCtx() already cancelled")
	default:
	}
}

// Each turn/request in a long-lived loop must get a fresh cancellable
// context: cancelling one turn's context must not poison later turns.
func TestTurnCtxIsIndependentPerCall(t *testing.T) {
	ctx1, stop1 := turnCtx()
	ctx2, stop2 := turnCtx()
	defer stop2()
	stop1() // simulate Ctrl+C ending the first turn

	select {
	case <-ctx1.Done():
	default:
		t.Fatal("first turn ctx not cancelled after stop")
	}
	select {
	case <-ctx2.Done():
		t.Fatal("second turn ctx cancelled by first turn's stop")
	default:
	}
}
