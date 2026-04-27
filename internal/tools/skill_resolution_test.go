package tools

import "testing"

func TestResolveChildTaskSkillNamesDoesNotAutoActivateFromPrompt(t *testing.T) {
	got, err := resolveChildTaskSkillNames(ExecContext{
		ActiveSkillNames: []string{"automation-standard-behavior"},
	}, "please use some browser skill automatically")
	if err != nil {
		t.Fatalf("resolveChildTaskSkillNames() error = %v", err)
	}
	if len(got) != 1 || got[0] != "automation-standard-behavior" {
		t.Fatalf("resolveChildTaskSkillNames() = %v, want inherited active skills only", got)
	}
}
