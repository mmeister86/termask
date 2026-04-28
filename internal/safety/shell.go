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
	checks := []struct {
		pattern string
		reason  string
		risk    Risk
	}{
		{"rm -rf", "recursive forced deletion", RiskHigh},
		{"sudo ", "privileged command", RiskHigh},
		{"curl ", "downloads remote content", RiskMedium},
		{"wget ", "downloads remote content", RiskMedium},
		{"| sh", "pipes remote or generated content into a shell", RiskHigh},
		{"| bash", "pipes remote or generated content into a shell", RiskHigh},
		{"> /etc/", "writes into system configuration", RiskHigh},
		{"chmod 777", "opens broad write/execute permissions", RiskMedium},
		{"chown ", "changes file ownership", RiskMedium},
	}
	for _, check := range checks {
		if strings.Contains(line, check.pattern) {
			reasons = append(reasons, check.reason)
			if check.risk == RiskHigh {
				risk = RiskHigh
			} else if risk != RiskHigh {
				risk = check.risk
			}
		}
	}
	return Command{Text: line, Risk: risk, Reasons: reasons}
}
