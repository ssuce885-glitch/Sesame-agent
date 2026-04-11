package tools

import (
	"context"
	"reflect"
	"testing"

	"go-agent/internal/runtimegraph"
	"go-agent/internal/task"
)

func TestTaskCreateCarriesExplicitActiveSkillNames(t *testing.T) {
	workspaceRoot := t.TempDir()
	manager := task.NewManager(task.Config{}, nil, nil)

	tool := taskCreateTool{}
	decoded, err := tool.Decode(Call{
		Name: "task_create",
		Input: map[string]any{
			"type":    "agent",
			"command": "child prompt mentions $brainstorming but should not be scanned",
			"start":   false,
		},
	})
	if err != nil {
		t.Fatalf("Decode() error = %v", err)
	}

	output, err := tool.ExecuteDecoded(context.Background(), decoded, ExecContext{
		WorkspaceRoot: workspaceRoot,
		TaskManager:   manager,
		ActiveSkillNames: []string{
			"brainstorming",
			"",
			"brainstorming",
			"writing-plans",
		},
		TurnContext: &runtimegraph.TurnContext{
			CurrentSessionID: "session-task-create",
			CurrentTurnID:    "turn-task-create",
		},
	})
	if err != nil {
		t.Fatalf("ExecuteDecoded() error = %v", err)
	}

	typed, ok := output.Data.(TaskCreateOutput)
	if !ok {
		t.Fatalf("output.Data type = %T, want TaskCreateOutput", output.Data)
	}

	created, ok, err := manager.Get(typed.TaskID, workspaceRoot)
	if err != nil {
		t.Fatalf("manager.Get() error = %v", err)
	}
	if !ok {
		t.Fatalf("manager.Get() did not return task %q", typed.TaskID)
	}

	want := []string{"brainstorming", "writing-plans"}
	if !reflect.DeepEqual(created.ActivatedSkillNames, want) {
		t.Fatalf("created.ActivatedSkillNames = %v, want %v", created.ActivatedSkillNames, want)
	}
}
