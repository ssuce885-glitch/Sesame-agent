package tools

import (
	"reflect"
	"testing"
)

func TestExplicitActiveSkillNamesIgnoresChildPromptText(t *testing.T) {
	childPrompt := "please use $brainstorming in the child prompt"
	_ = childPrompt

	execCtx := ExecContext{
		ActiveSkillNames: []string{
			"brainstorming",
			"",
			"brainstorming",
			" writing-plans ",
			"",
			" writing-plans ",
		},
	}

	got := explicitActiveSkillNames(execCtx)
	want := []string{"brainstorming", " writing-plans "}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("explicitActiveSkillNames() = %v, want %v", got, want)
	}
}
