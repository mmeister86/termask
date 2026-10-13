package main

import (
	"bufio"
	"strings"
	"testing"
)

// Piped init must not lose the API key: the key is read from initCmd's
// existing scanner, not a second scanner on an already-buffered stdin.
func TestReadAPIKeyUsesPassedScanner(t *testing.T) {
	scanner := bufio.NewScanner(strings.NewReader("test-key-123\n"))
	key, err := readAPIKey(scanner)
	if err != nil {
		t.Fatalf("readAPIKey err = %v", err)
	}
	if key != "test-key-123" {
		t.Fatalf("key = %q, want %q", key, "test-key-123")
	}
}
