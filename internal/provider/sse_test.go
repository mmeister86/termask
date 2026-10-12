package provider_test

import (
	"strings"
	"testing"

	"github.com/mmeister86/termask/internal/provider"
)

func TestScanSSE_TwoEventsAndDone(t *testing.T) {
	body := ": comment\n\ndata: {\"a\":1}\n\ndata: {\"b\":2}\n\ndata: [DONE]\n"
	var got []string
	err := provider.ScanSSE(strings.NewReader(body), func(data string) error {
		got = append(got, data)
		return nil
	})
	if err != nil {
		t.Fatalf("ScanSSE: %v", err)
	}
	if len(got) != 2 || got[0] != `{"a":1}` || got[1] != `{"b":2}` {
		t.Fatalf("got %q", got)
	}
}

func TestScanSSE_LargeLine(t *testing.T) {
	big := strings.Repeat("x", 100*1024) // >64 KiB default scanner limit
	body := "data: " + big + "\ndata: [DONE]\n"
	var got string
	err := provider.ScanSSE(strings.NewReader(body), func(data string) error {
		got = data
		return nil
	})
	if err != nil {
		t.Fatalf("ScanSSE large line: %v", err)
	}
	if got != big {
		t.Fatalf("large line mangled: len %d", len(got))
	}
}
