package markdown

import (
	"regexp"
	"strings"
	"testing"
)

var ansiPattern = regexp.MustCompile(`\x1b\[[0-9;?]*[ -/]*[@-~]`)

func TestRenderHighlightsCodeFences(t *testing.T) {
	out := Render("before\n```bash\necho hi\n```\nafter")
	plain := stripANSI(out)
	if !strings.Contains(plain, "echo hi") {
		t.Fatalf("rendered output missing code:\n%s", out)
	}
	if strings.Contains(plain, "```") {
		t.Fatalf("rendered output still contains raw fences:\n%s", out)
	}
}

func TestRenderRemovesCommonMarkdownMarkers(t *testing.T) {
	out := Render("## **Assistant**\n\nDas macht **kein Re-Encoding**.")
	plain := stripANSI(out)
	if strings.Contains(plain, "##") || strings.Contains(plain, "**") {
		t.Fatalf("rendered output still contains markdown markers:\n%s", out)
	}
	if !strings.Contains(plain, "Assistant") || !strings.Contains(plain, "kein Re-Encoding") {
		t.Fatalf("rendered output missing text:\n%s", out)
	}
}

func stripANSI(input string) string {
	return ansiPattern.ReplaceAllString(input, "")
}
