package markdown

import (
	"bufio"
	"strings"
)

func Render(input string) string {
	var out strings.Builder
	scanner := bufio.NewScanner(strings.NewReader(input))
	inFence := false
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "```") {
			if inFence {
				out.WriteString(strings.Repeat("-", 72))
				out.WriteByte('\n')
				inFence = false
				continue
			}
			lang := strings.TrimSpace(strings.TrimPrefix(line, "```"))
			if lang == "" {
				lang = "text"
			}
			out.WriteString(strings.Repeat("-", 72))
			out.WriteByte('\n')
			out.WriteString("code: ")
			out.WriteString(lang)
			out.WriteByte('\n')
			out.WriteString(strings.Repeat("-", 72))
			out.WriteByte('\n')
			inFence = true
			continue
		}
		out.WriteString(line)
		out.WriteByte('\n')
	}
	return out.String()
}
