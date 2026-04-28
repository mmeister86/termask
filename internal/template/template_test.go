package template

import "testing"

func TestRenderTemplateSubstitutesInput(t *testing.T) {
	out, err := Render("Answer briefly:\n{{input}}", "list files")
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	if out != "Answer briefly:\nlist files" {
		t.Fatalf("out = %q", out)
	}
}

func TestBuiltinsContainShellTemplate(t *testing.T) {
	tpl, ok := Builtins()["shell"]
	if !ok {
		t.Fatal("shell template missing")
	}
	out, err := Render(tpl.Prompt, "remove empty dirs")
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	if out == "remove empty dirs" {
		t.Fatal("shell template did not add guidance")
	}
}
