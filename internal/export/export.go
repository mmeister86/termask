package export

import (
	"fmt"
	"strings"

	"github.com/yourusername/termask/internal/history"
)

func SessionMarkdown(session history.Session) string {
	var out strings.Builder
	fmt.Fprintf(&out, "# termask session %s\n\n", session.ID)
	fmt.Fprintf(&out, "- Provider: `%s`\n", session.Provider)
	fmt.Fprintf(&out, "- Model: `%s`\n\n", session.Model)
	for _, msg := range session.Messages {
		label := "User"
		if msg.Role == "assistant" {
			label = "Assistant"
		}
		fmt.Fprintf(&out, "## **%s**\n\n%s\n\n", label, msg.Content)
	}
	return out.String()
}
