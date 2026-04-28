package contextfiles

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildIncludesNamedFiles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "main.go")
	if err := os.WriteFile(path, []byte("package main\n"), 0644); err != nil {
		t.Fatal(err)
	}
	out, err := Build([]string{path}, 1024)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if !strings.Contains(out, "File: "+path) || !strings.Contains(out, "package main") {
		t.Fatalf("context missing file header/content:\n%s", out)
	}
}

func TestBuildRejectsOversizedFiles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "large.txt")
	if err := os.WriteFile(path, []byte("abcdef"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := Build([]string{path}, 3); err == nil {
		t.Fatal("Build() error = nil, want oversized error")
	}
}
