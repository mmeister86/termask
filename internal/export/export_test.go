package export

import (
	"strings"
	"testing"

	"github.com/yourusername/termask/internal/history"
)

func TestSessionMarkdown(t *testing.T) {
	session := history.NewSession("anthropic", "claude")
	session.AddUser("hello")
	session.AddAssistant("hi")

	out := SessionMarkdown(session)
	if !strings.Contains(out, "# termask session") {
		t.Fatalf("missing title:\n%s", out)
	}
	if !strings.Contains(out, "**User**") || !strings.Contains(out, "**Assistant**") {
		t.Fatalf("missing message labels:\n%s", out)
	}
}
