package markdown

import (
	"os"
	"strconv"
	"strings"

	"charm.land/glamour/v2"
)

func Render(input string) string {
	renderer, err := glamour.NewTermRenderer(
		glamour.WithStandardStyle("dark"),
		glamour.WithWordWrap(wordWrap()),
	)
	if err != nil {
		return input
	}
	out, err := renderer.Render(normalize(input))
	if err != nil {
		return input
	}
	return strings.TrimRight(out, "\n") + "\n"
}

func normalize(input string) string {
	lines := strings.Split(input, "\n")
	for i, line := range lines {
		trimmed := strings.TrimLeft(line, " ")
		prefixLen := len(line) - len(trimmed)
		if strings.HasPrefix(trimmed, "#") {
			hashes := 0
			for hashes < len(trimmed) && trimmed[hashes] == '#' {
				hashes++
			}
			if hashes > 0 && hashes < len(trimmed) && trimmed[hashes] == ' ' {
				content := strings.TrimSpace(trimmed[hashes+1:])
				lines[i] = line[:prefixLen] + "**" + content + "**"
			}
		}
	}
	return strings.Join(lines, "\n")
}

func wordWrap() int {
	columns, err := strconv.Atoi(os.Getenv("COLUMNS"))
	if err != nil || columns < 40 {
		return 100
	}
	if columns > 120 {
		return 120
	}
	return columns - 4
}
