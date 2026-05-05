package tui

import (
	"context"
	"encoding/json"
	"testing"
)

func TestReplaceTimelineRefreshesEntriesAndLastSeq(t *testing.T) {
	model := NewModelWithOptions(ModelOptions{
		Timeline: SessionTimelineResponse{
			LatestSeq: 4,
			Blocks: []TimelineBlock{
				{Kind: "user_message", Text: "old"},
				{Kind: "assistant_message", Content: []ContentBlock{{Type: "text", Text: "old reply"}}},
			},
		},
	})

	model.appendUserEntry("optimistic")
	model.replaceTimeline(SessionTimelineResponse{
		LatestSeq: 7,
		Blocks: []TimelineBlock{
			{Kind: "user_message", Text: "hello"},
			{Kind: "assistant_message", Content: []ContentBlock{{Type: "text", Text: "hello reply"}}},
		},
	})

	if model.lastSeq != 7 {
		t.Fatalf("lastSeq = %d, want 7", model.lastSeq)
	}
	if len(model.entries) != 2 {
		t.Fatalf("entries len = %d, want 2", len(model.entries))
	}
	if got := model.entries[0].Body; got != "hello" {
		t.Fatalf("first entry body = %q, want hello", got)
	}
	if got := model.entries[1].Body; got != "hello reply" {
		t.Fatalf("second entry body = %q, want hello reply", got)
	}
}

func TestLoadTimelineCmdReturnsTimelineMessage(t *testing.T) {
	client := fakeRuntimeClient{
		timeline: SessionTimelineResponse{
			LatestSeq: 3,
			Blocks: []TimelineBlock{
				{Kind: "assistant_message", Content: []ContentBlock{{Type: "text", Text: "ok"}}},
			},
		},
	}
	model := NewModelWithOptions(ModelOptions{
		Context:   context.Background(),
		Client:    client,
		SessionID: "session-1",
	})

	cmd := model.loadTimelineCmd()
	if cmd == nil {
		t.Fatal("loadTimelineCmd returned nil")
	}
	raw := cmd()
	msg, ok := raw.(tuiTimelineMsg)
	if !ok {
		t.Fatalf("message type = %T, want tuiTimelineMsg", raw)
	}
	if msg.Err != nil {
		t.Fatalf("timeline error: %v", msg.Err)
	}
	if msg.Timeline.LatestSeq != 3 {
		t.Fatalf("latest seq = %d, want 3", msg.Timeline.LatestSeq)
	}
}

func TestStartSessionStreamCmdDoesNotCancelBeforeReady(t *testing.T) {
	model := NewModelWithOptions(ModelOptions{
		Context:   context.Background(),
		Client:    fakeRuntimeClient{},
		SessionID: "session-1",
	})

	cmd := model.startSessionStreamCmd("session-1", 0)
	if cmd == nil {
		t.Fatal("startSessionStreamCmd returned nil")
	}
	if model.streamCancel != nil {
		t.Fatal("streamCancel was set before stream ready")
	}
}

func TestAutomationResponseJSONTagsDecodeWorkflowID(t *testing.T) {
	var response AutomationResponse
	if err := json.Unmarshal([]byte(`{
		"id":"automation_1",
		"workspace_root":"/workspace",
		"title":"Watch docs",
		"goal":"Keep docs fresh",
		"state":"active",
		"owner":"role:reviewer",
		"workflow_id":"workflow_docs",
		"watcher_path":"roles/reviewer/automations/watch.sh",
		"watcher_cron":"@every 5m",
		"created_at":"2026-05-03T00:00:00Z",
		"updated_at":"2026-05-03T00:00:01Z"
	}`), &response); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if response.WorkspaceRoot != "/workspace" || response.WorkflowID != "workflow_docs" || response.WatcherPath == "" {
		t.Fatalf("unexpected decoded automation response: %+v", response)
	}
}

func TestFormatAutomationListIncludesWorkflowID(t *testing.T) {
	lines := formatAutomationList([]AutomationResponse{{
		ID:          "automation_1",
		Title:       "Watch docs",
		Goal:        "Keep docs fresh",
		State:       "active",
		Owner:       "role:reviewer",
		WorkflowID:  "workflow_docs",
		WatcherCron: "@every 5m",
	}})
	if len(lines) != 1 {
		t.Fatalf("lines len = %d, want 1", len(lines))
	}
	if got := lines[0]; got != "Watch docs [active | role:reviewer | workflow:workflow_docs | @every 5m] - Keep docs fresh" {
		t.Fatalf("line = %q", got)
	}
}

type fakeRuntimeClient struct {
	timeline SessionTimelineResponse
}

func (f fakeRuntimeClient) Status(context.Context) (StatusResponse, error) {
	return StatusResponse{}, nil
}

func (f fakeRuntimeClient) SubmitTurn(context.Context, SubmitTurnRequest) (Turn, error) {
	return Turn{}, nil
}

func (f fakeRuntimeClient) InterruptTurn(context.Context, string) error {
	return nil
}

func (f fakeRuntimeClient) StreamEvents(context.Context, string, int64) (<-chan Event, <-chan error, error) {
	return nil, nil, nil
}

func (f fakeRuntimeClient) GetTimeline(context.Context, string) (SessionTimelineResponse, error) {
	return f.timeline, nil
}

func (f fakeRuntimeClient) GetSession(context.Context, string) (SessionInfo, error) {
	return SessionInfo{}, nil
}

func (f fakeRuntimeClient) GetWorkspaceReports(context.Context, string) (ReportsResponse, error) {
	return ReportsResponse{}, nil
}

func (f fakeRuntimeClient) GetAutomations(context.Context, string) ([]AutomationResponse, error) {
	return nil, nil
}

func (f fakeRuntimeClient) GetProjectState(context.Context, string) (ProjectStateResponse, error) {
	return ProjectStateResponse{}, nil
}

func (f fakeRuntimeClient) UpdateProjectState(context.Context, string, string) (ProjectStateResponse, error) {
	return ProjectStateResponse{}, nil
}

func (f fakeRuntimeClient) GetSetting(context.Context, string) (SettingResponse, error) {
	return SettingResponse{}, nil
}

func (f fakeRuntimeClient) SetSetting(context.Context, string, string) (SettingResponse, error) {
	return SettingResponse{}, nil
}

func (f fakeRuntimeClient) EnsureSession(context.Context, string) (SessionInfo, error) {
	return SessionInfo{}, nil
}
