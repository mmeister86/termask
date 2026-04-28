package template

import (
	"strings"
	texttemplate "text/template"
)

type Template struct {
	Description string `toml:"description"`
	Prompt      string `toml:"prompt"`
}

func Builtins() map[string]Template {
	return map[string]Template{
		"shell": {
			Description: "Generate practical shell commands with brief explanation",
			Prompt:      "Answer with a safe shell command first, then explain briefly. Prefer portable commands and mention risks.\n\nUser request: {{.Input}}",
		},
		"explain": {
			Description: "Explain technical output clearly and concisely",
			Prompt:      "Explain this clearly and concisely. Call out the practical next step when useful.\n\n{{.Input}}",
		},
		"debug": {
			Description: "Debug an error or unexpected behavior",
			Prompt:      "Debug this. Give the most likely causes first, then concrete commands or checks.\n\n{{.Input}}",
		},
		"regex": {
			Description: "Create or explain regular expressions",
			Prompt:      "Help with this regex task. Provide the regex, a short explanation, and one test example.\n\n{{.Input}}",
		},
		"git": {
			Description: "Answer Git workflow questions",
			Prompt:      "Answer as a careful Git assistant. Prefer non-destructive commands and explain any risky step.\n\n{{.Input}}",
		},
		"review": {
			Description: "Review code or command output",
			Prompt:      "Review this for correctness, risks, and simpler alternatives. Lead with concrete findings.\n\n{{.Input}}",
		},
		"translate": {
			Description: "Translate text while preserving formatting",
			Prompt:      "Translate the following text. Preserve formatting and code blocks.\n\n{{.Input}}",
		},
	}
}

func Render(prompt, input string) (string, error) {
	prompt = strings.ReplaceAll(prompt, "{{input}}", "{{.Input}}")
	tpl, err := texttemplate.New("prompt").Parse(prompt)
	if err != nil {
		return "", err
	}
	var out strings.Builder
	if err := tpl.Execute(&out, map[string]string{"Input": input, "input": input}); err != nil {
		return "", err
	}
	return out.String(), nil
}
