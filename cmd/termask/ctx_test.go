package main

import (
	"testing"
)

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
