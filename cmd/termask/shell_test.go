package main

import (
	"strings"
	"testing"
)

func TestAskRendersByDefaultUnlessPlain(t *testing.T) {
	if !shouldRenderAskOutput(false) {
		t.Fatal("ask should render by default")
	}
	if shouldRenderAskOutput(true) {
		t.Fatal("ask --plain should disable rendering")
	}
}

func TestShellPluginsUseDefaultRenderedAskOutput(t *testing.T) {
	if !strings.Contains(zshPlugin(), "termask ask ") {
		t.Fatal("zsh plugin should call termask ask")
	}
	if strings.Contains(zshPlugin(), "--render") {
		t.Fatal("zsh plugin should not need --render anymore")
	}
	if strings.Contains(bashPlugin(), "--render") {
		t.Fatal("bash plugin should not need --render anymore")
	}
}
