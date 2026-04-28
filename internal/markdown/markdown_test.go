package markdown

import (
	"strings"
	"testing"
)

func TestRenderHighlightsCodeFences(t *testing.T) {
	out := Render("before\n```bash\necho hi\n```\nafter")
	if !strings.Contains(out, "code: bash") {
		t.Fatalf("rendered output missing code label:\n%s", out)
	}
	if !strings.Contains(out, "echo hi") {
		t.Fatalf("rendered output missing code:\n%s", out)
	}
}
