package safety

import (
	"bufio"
	"regexp"
	"strings"
)

type Risk string

const (
	RiskLow    Risk = "low"
	RiskMedium Risk = "medium"
	RiskHigh   Risk = "high"
)

type Command struct {
	Text    string
	Risk    Risk
	Reasons []string
}

type Report struct {
	Commands []Command
}

var shellFence = regexp.MustCompile("(?s)```(?:bash|sh|zsh|shell)?\\s*\\n(.*?)```")

func AnalyzeShell(answer string) Report {
	var commands []Command
	matches := shellFence.FindAllStringSubmatch(answer, -1)
	if len(matches) > 0 {
		for _, match := range matches {
			commands = append(commands, extractLines(match[1])...)
		}
	} else {
		commands = extractLines(answer)
	}
	return Report{Commands: commands}
}

func extractLines(text string) []Command {
	var commands []Command
	scanner := bufio.NewScanner(strings.NewReader(text))
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if looksLikeCommand(line) {
			commands = append(commands, classify(line))
		}
	}
	return commands
}

func looksLikeCommand(line string) bool {
	prefixes := []string{"$", "sudo ", "rm ", "curl ", "wget ", "find ", "grep ", "sed ", "awk ", "ls ", "cd ", "git ", "docker ", "kubectl ", "ffmpeg ", "chmod ", "chown ", "mv ", "cp ", "mkdir ", "tar "}
	for _, prefix := range prefixes {
		if strings.HasPrefix(line, prefix) {
			return true
		}
	}
	return false
}

func classify(line string) Command {
	line = strings.TrimPrefix(line, "$ ")
	reasons := []string{}
	risk := RiskLow
	mark := func(reason string, r Risk) {
		reasons = append(reasons, reason)
		if r == RiskHigh {
			risk = RiskHigh
		} else if risk != RiskHigh {
			risk = r
		}
	}
	tokens := strings.Fields(line)
	if isRecursiveForcedRm(tokens) {
		mark("recursive forced deletion", RiskHigh)
	}
	if isPipeToShell(tokens) {
		mark("pipes remote or generated content into a shell", RiskHigh)
	}
	for _, check := range []struct {
		pattern string
		reason  string
		risk    Risk
	}{
		{"sudo ", "privileged command", RiskHigh},
		{"curl ", "downloads remote content", RiskMedium},
		{"wget ", "downloads remote content", RiskMedium},
		{"> /etc/", "writes into system configuration", RiskHigh},
		{"chmod 777", "opens broad write/execute permissions", RiskMedium},
		{"chown ", "changes file ownership", RiskMedium},
	} {
		if strings.Contains(line, check.pattern) {
			mark(check.reason, check.risk)
		}
	}
	return Command{Text: line, Risk: risk, Reasons: reasons}
}

// isRecursiveForcedRm reports whether the tokens invoke rm with both a
// recursive and a force flag, in any spelling: -rf, -r -f, --recursive --force.
func isRecursiveForcedRm(tokens []string) bool {
	for i, tok := range tokens {
		if tok != "rm" && !strings.HasSuffix(tok, "/rm") {
			continue
		}
		var recursive, force bool
		for _, flag := range tokens[i+1:] {
			// A new command after a separator has its own flags.
			if flag == ";" || flag == "&&" || flag == "||" || flag == "|" {
				break
			}
			switch flag {
			case "--recursive":
				recursive = true
			case "--force":
				force = true
			default:
				if strings.HasPrefix(flag, "--") {
					continue
				}
				if strings.HasPrefix(flag, "-") && len(flag) > 1 {
					for _, c := range flag[1:] {
						switch c {
						case 'r', 'R':
							recursive = true
						case 'f':
							force = true
						}
					}
				}
			}
		}
		if recursive && force {
			return true
		}
	}
	return false
}

// isPipeToShell reports whether any pipe token feeds sh/bash/zsh,
// with or without a space after the pipe.
func isPipeToShell(tokens []string) bool {
	for i, tok := range tokens {
		name := ""
		if tok == "|" {
			if i+1 >= len(tokens) {
				return false
			}
			name = tokens[i+1]
		} else if strings.HasPrefix(tok, "|") {
			name = strings.TrimPrefix(tok, "|")
		} else {
			continue
		}
		name = strings.Trim(name, "; &")
		if i := strings.LastIndexByte(name, '/'); i >= 0 {
			name = name[i+1:]
		}
		switch name {
		case "sh", "bash", "zsh":
			return true
		}
	}
	return false
}
