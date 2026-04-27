package tools

import (
	"context"
	"strings"
	"testing"

	"go-agent/internal/types"
)

type automationQueryServiceStub struct {
	automation         types.AutomationSpec
	automations        []types.AutomationSpec
	watcher            types.AutomationWatcherRuntime
	heartbeats         []types.AutomationHeartbeat
	gotAutomationID    string
	gotHeartbeatFilter types.AutomationHeartbeatFilter
}

type automationControlServiceStub struct {
	automationQueryServiceStub
	controlCalls int
}

func (s *automationControlServiceStub) Control(context.Context, string, types.AutomationControlAction) (types.AutomationSpec, bool, error) {
	s.controlCalls++
	return s.automation, true, nil
}

func (s *automationQueryServiceStub) ApplyRequest(context.Context, types.ApplyAutomationRequest) (types.AutomationSpec, error) {
	return types.AutomationSpec{}, nil
}

func (s *automationQueryServiceStub) Apply(context.Context, types.AutomationSpec) (types.AutomationSpec, error) {
	return types.AutomationSpec{}, nil
}

func (s *automationQueryServiceStub) Get(_ context.Context, id string) (types.AutomationSpec, bool, error) {
	s.gotAutomationID = id
	return s.automation, true, nil
}

func (s *automationQueryServiceStub) List(context.Context, types.AutomationListFilter) ([]types.AutomationSpec, error) {
	return s.automations, nil
}

func (s *automationQueryServiceStub) Control(context.Context, string, types.AutomationControlAction) (types.AutomationSpec, bool, error) {
	return types.AutomationSpec{}, false, nil
}

func (s *automationQueryServiceStub) Delete(context.Context, string) (bool, error) {
	return false, nil
}

func (s *automationQueryServiceStub) EmitTrigger(context.Context, types.AutomationTriggerRequest) (types.TriggerEvent, error) {
	return types.TriggerEvent{}, nil
}

func (s *automationQueryServiceStub) RecordHeartbeat(context.Context, types.AutomationHeartbeatRequest) (types.AutomationHeartbeat, error) {
	return types.AutomationHeartbeat{}, nil
}

func (s *automationQueryServiceStub) GetWatcher(context.Context, string) (types.AutomationWatcherRuntime, bool, error) {
	return s.watcher, true, nil
}

func (s *automationQueryServiceStub) ListHeartbeats(_ context.Context, filter types.AutomationHeartbeatFilter) ([]types.AutomationHeartbeat, error) {
	s.gotHeartbeatFilter = filter
	return s.heartbeats, nil
}

func TestAutomationControlRequiresAutomationStandardBehaviorSkill(t *testing.T) {
	service := &automationControlServiceStub{
		automationQueryServiceStub: automationQueryServiceStub{
			automation: types.AutomationSpec{ID: "auto_1", State: types.AutomationStateActive},
		},
	}
	tool := automationControlTool{}

	_, err := tool.ExecuteDecoded(context.Background(), DecodedCall{
		Input: AutomationControlInput{
			AutomationID: "auto_1",
			Action:       types.AutomationControlActionPause,
		},
	}, ExecContext{AutomationService: service})
	if err == nil {
		t.Fatal("expected error when automation-standard-behavior is not active")
	}
	if !strings.Contains(err.Error(), "automation-standard-behavior") {
		t.Fatalf("error = %v, want automation-standard-behavior guidance", err)
	}
}

func TestAutomationControlAllowsActiveAutomationStandardBehaviorSkill(t *testing.T) {
	service := &automationControlServiceStub{
		automationQueryServiceStub: automationQueryServiceStub{
			automation: types.AutomationSpec{ID: "auto_1", State: types.AutomationStateActive},
		},
	}
	tool := automationControlTool{}

	output, err := tool.ExecuteDecoded(context.Background(), DecodedCall{
		Input: AutomationControlInput{
			AutomationID: "auto_1",
			Action:       types.AutomationControlActionPause,
		},
	}, ExecContext{
		AutomationService: service,
		ActiveSkillNames:  []string{"automation-standard-behavior"},
	})
	if err != nil {
		t.Fatalf("ExecuteDecoded() error = %v", err)
	}
	if output.Data == nil {
		t.Fatal("expected output data")
	}
	if service.controlCalls != 1 {
		t.Fatalf("controlCalls = %d, want 1", service.controlCalls)
	}
}

func TestAutomationQueryToolGetIncludesWatcherAndHeartbeats(t *testing.T) {
	service := &automationQueryServiceStub{
		automation: types.AutomationSpec{ID: "auto_1", Title: "Detector", WorkspaceRoot: "/workspace", Goal: "watch", State: types.AutomationStateActive},
		watcher:    types.AutomationWatcherRuntime{AutomationID: "auto_1", WatcherID: "watcher:auto_1", State: types.AutomationWatcherStateRunning},
		heartbeats: []types.AutomationHeartbeat{{AutomationID: "auto_1", WatcherID: "watcher:auto_1", Status: "healthy"}},
	}

	tool := automationQueryTool{}
	result, err := tool.ExecuteDecoded(context.Background(), DecodedCall{
		Input: AutomationQueryInput{
			Mode:              automationQueryModeGet,
			AutomationID:      "auto_1",
			IncludeWatcher:    true,
			IncludeHeartbeats: true,
			HeartbeatLimit:    3,
		},
	}, ExecContext{AutomationService: service})
	if err != nil {
		t.Fatalf("ExecuteDecoded() error = %v", err)
	}

	output, ok := result.Data.(AutomationQueryOutput)
	if !ok {
		t.Fatalf("Data type = %T, want AutomationQueryOutput", result.Data)
	}
	if output.Automation == nil || output.Automation.ID != "auto_1" {
		t.Fatalf("Automation = %#v, want auto_1", output.Automation)
	}
	if output.Watcher == nil || output.Watcher.WatcherID != "watcher:auto_1" {
		t.Fatalf("Watcher = %#v, want watcher:auto_1", output.Watcher)
	}
	if len(output.Heartbeats) != 1 || output.Heartbeats[0].Status != "healthy" {
		t.Fatalf("Heartbeats = %#v, want one healthy heartbeat", output.Heartbeats)
	}
	if service.gotHeartbeatFilter.AutomationID != "auto_1" || service.gotHeartbeatFilter.Limit != 3 {
		t.Fatalf("Heartbeat filter = %#v, want automation_id auto_1 limit 3", service.gotHeartbeatFilter)
	}
}
