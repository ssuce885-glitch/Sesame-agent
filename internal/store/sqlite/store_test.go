package sqlite

import (
	"context"
	"database/sql/driver"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"go-agent/internal/model"
	"go-agent/internal/types"
)

func TestStorePersistsSessionTurnAndEvent(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "agentd.db")

	store, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()

	session := types.Session{
		ID:            "sess_test",
		WorkspaceRoot: "D:/work/demo",
		State:         types.SessionStateIdle,
		CreatedAt:     time.Now().UTC(),
		UpdatedAt:     time.Now().UTC(),
	}
	if err := store.InsertSession(context.Background(), session); err != nil {
		t.Fatalf("InsertSession() error = %v", err)
	}

	turn := types.Turn{
		ID:          "turn_test",
		SessionID:   session.ID,
		State:       types.TurnStateCreated,
		UserMessage: "hello",
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}
	if err := store.InsertTurn(context.Background(), turn); err != nil {
		t.Fatalf("InsertTurn() error = %v", err)
	}

	event, err := types.NewEvent(session.ID, turn.ID, types.EventTurnStarted, types.TurnStartedPayload{
		WorkspaceRoot: session.WorkspaceRoot,
	})
	if err != nil {
		t.Fatalf("NewEvent() error = %v", err)
	}
	if _, err := store.AppendEvent(context.Background(), event); err != nil {
		t.Fatalf("AppendEvent() error = %v", err)
	}

	loaded, err := store.ListSessionEvents(context.Background(), session.ID, 0)
	if err != nil {
		t.Fatalf("ListSessionEvents() error = %v", err)
	}
	if len(loaded) != 1 {
		t.Fatalf("len(events) = %d, want 1", len(loaded))
	}
	if loaded[0].Seq != 1 {
		t.Fatalf("Seq = %d, want 1", loaded[0].Seq)
	}
}

func TestStorePersistsRuntimeGraph(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "agentd.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()

	now := time.Date(2026, 4, 5, 10, 30, 0, 0, time.UTC)
	startedAt := now.Add(2 * time.Minute)
	completedAt := now.Add(5 * time.Minute)

	run := types.Run{
		ID:        "run_1",
		SessionID: "sess_runtime",
		TurnID:    "turn_runtime",
		State:     types.RunStateRunning,
		Objective: "ship runtime graph storage",
		Result:    "runtime graph stored",
		Error:     "sample run warning",
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := store.InsertRun(context.Background(), run); err != nil {
		t.Fatalf("InsertRun() error = %v", err)
	}

	plan := types.Plan{
		ID:           "plan_1",
		RunID:        run.ID,
		State:        types.PlanStateActive,
		Title:        "Minimal runtime graph",
		Summary:      "Persist runs, plans, tasks, tool runs, and worktrees.",
		ParentPlanID: "plan_root",
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if err := store.UpsertPlan(context.Background(), plan); err != nil {
		t.Fatalf("UpsertPlan() error = %v", err)
	}

	task := types.TaskRecord{
		ID:          "task_1",
		RunID:       run.ID,
		PlanID:      plan.ID,
		State:       types.TaskStateRunning,
		Title:       "Write runtime graph tests",
		Description: "Verify SQLite round-trip behavior.",
		Owner:       "codex",
		WorktreeID:  "worktree_1",
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := store.UpsertTaskRecord(context.Background(), task); err != nil {
		t.Fatalf("UpsertTaskRecord() error = %v", err)
	}

	toolRun := types.ToolRun{
		ID:          "tool_run_1",
		RunID:       run.ID,
		TaskID:      task.ID,
		State:       types.ToolRunStateCompleted,
		ToolName:    "Bash",
		InputJSON:   `{"command":"go test ./internal/store/sqlite"}`,
		OutputJSON:  `{"exit_code":0}`,
		Error:       "stderr captured for verification",
		StartedAt:   startedAt,
		CompletedAt: completedAt,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := store.UpsertToolRun(context.Background(), toolRun); err != nil {
		t.Fatalf("UpsertToolRun() error = %v", err)
	}

	worktree := types.Worktree{
		ID:             "worktree_1",
		RunID:          run.ID,
		TaskID:         task.ID,
		State:          types.WorktreeStateActive,
		WorktreePath:   "E:/project/go-agent/.worktrees/minimal-runtime-loop",
		WorktreeBranch: "feature/minimal-runtime-loop",
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if err := store.UpsertWorktree(context.Background(), worktree); err != nil {
		t.Fatalf("UpsertWorktree() error = %v", err)
	}

	graph, err := store.ListRuntimeGraph(context.Background())
	if err != nil {
		t.Fatalf("ListRuntimeGraph() error = %v", err)
	}

	if len(graph.Runs) != 1 {
		t.Fatalf("len(graph.Runs) = %d, want 1", len(graph.Runs))
	}
	if got := graph.Runs[0]; got.ID != run.ID || got.SessionID != run.SessionID || got.TurnID != run.TurnID || got.State != run.State || got.Objective != run.Objective || got.Result != run.Result || got.Error != run.Error || !got.CreatedAt.Equal(run.CreatedAt.UTC()) || !got.UpdatedAt.Equal(run.UpdatedAt.UTC()) {
		t.Fatalf("graph.Runs[0] = %#v, want %#v", got, run)
	}

	if len(graph.Plans) != 1 {
		t.Fatalf("len(graph.Plans) = %d, want 1", len(graph.Plans))
	}
	if got := graph.Plans[0]; got.ID != plan.ID || got.RunID != plan.RunID || got.State != plan.State || got.Title != plan.Title || got.Summary != plan.Summary || got.ParentPlanID != plan.ParentPlanID || !got.CreatedAt.Equal(plan.CreatedAt.UTC()) || !got.UpdatedAt.Equal(plan.UpdatedAt.UTC()) {
		t.Fatalf("graph.Plans[0] = %#v, want %#v", got, plan)
	}

	if len(graph.Tasks) != 1 {
		t.Fatalf("len(graph.Tasks) = %d, want 1", len(graph.Tasks))
	}
	if got := graph.Tasks[0]; got.ID != task.ID || got.RunID != task.RunID || got.PlanID != task.PlanID || got.State != task.State || got.Title != task.Title || got.Description != task.Description || got.Owner != task.Owner || got.WorktreeID != task.WorktreeID || !got.CreatedAt.Equal(task.CreatedAt.UTC()) || !got.UpdatedAt.Equal(task.UpdatedAt.UTC()) {
		t.Fatalf("graph.Tasks[0] = %#v, want %#v", got, task)
	}

	if len(graph.ToolRuns) != 1 {
		t.Fatalf("len(graph.ToolRuns) = %d, want 1", len(graph.ToolRuns))
	}
	if got := graph.ToolRuns[0]; got.ID != toolRun.ID || got.RunID != toolRun.RunID || got.TaskID != toolRun.TaskID || got.ToolName != toolRun.ToolName || got.State != toolRun.State || got.InputJSON != toolRun.InputJSON || got.OutputJSON != toolRun.OutputJSON || got.Error != toolRun.Error || !got.StartedAt.Equal(toolRun.StartedAt.UTC()) || !got.CompletedAt.Equal(toolRun.CompletedAt.UTC()) || !got.CreatedAt.Equal(toolRun.CreatedAt.UTC()) || !got.UpdatedAt.Equal(toolRun.UpdatedAt.UTC()) {
		t.Fatalf("graph.ToolRuns[0] = %#v, want %#v", got, toolRun)
	}

	if len(graph.Worktrees) != 1 {
		t.Fatalf("len(graph.Worktrees) = %d, want 1", len(graph.Worktrees))
	}
	if got := graph.Worktrees[0]; got.ID != worktree.ID || got.RunID != worktree.RunID || got.TaskID != worktree.TaskID || got.State != worktree.State || got.WorktreePath != worktree.WorktreePath || got.WorktreeBranch != worktree.WorktreeBranch || !got.CreatedAt.Equal(worktree.CreatedAt.UTC()) || !got.UpdatedAt.Equal(worktree.UpdatedAt.UTC()) {
		t.Fatalf("graph.Worktrees[0] = %#v, want %#v", got, worktree)
	}
}

func TestStoreListActivePlansForSession(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "agentd.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()

	now := time.Date(2026, 4, 6, 9, 0, 0, 0, time.UTC)
	runA := types.Run{
		ID:        "run_session_a",
		SessionID: "sess_a",
		State:     types.RunStateRunning,
		CreatedAt: now,
		UpdatedAt: now,
	}
	runB := types.Run{
		ID:        "run_session_b",
		SessionID: "sess_b",
		State:     types.RunStateRunning,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := store.InsertRun(context.Background(), runA); err != nil {
		t.Fatalf("InsertRun(runA) error = %v", err)
	}
	if err := store.InsertRun(context.Background(), runB); err != nil {
		t.Fatalf("InsertRun(runB) error = %v", err)
	}

	for _, plan := range []types.Plan{
		{
			ID:        "plan_active_a",
			RunID:     runA.ID,
			State:     types.PlanStateActive,
			PlanFile:  "docs/plans/a.md",
			CreatedAt: now,
			UpdatedAt: now,
		},
		{
			ID:        "plan_completed_a",
			RunID:     runA.ID,
			State:     types.PlanStateCompleted,
			PlanFile:  "docs/plans/old.md",
			CreatedAt: now.Add(time.Minute),
			UpdatedAt: now.Add(time.Minute),
		},
		{
			ID:        "plan_active_b",
			RunID:     runB.ID,
			State:     types.PlanStateActive,
			PlanFile:  "docs/plans/b.md",
			CreatedAt: now.Add(2 * time.Minute),
			UpdatedAt: now.Add(2 * time.Minute),
		},
	} {
		if err := store.UpsertPlan(context.Background(), plan); err != nil {
			t.Fatalf("UpsertPlan(%s) error = %v", plan.ID, err)
		}
	}

	got, err := store.ListActivePlansForSession(context.Background(), "sess_a")
	if err != nil {
		t.Fatalf("ListActivePlansForSession() error = %v", err)
	}
	if len(got) != 1 || got[0].ID != "plan_active_a" || got[0].PlanFile != "docs/plans/a.md" {
		t.Fatalf("ListActivePlansForSession() = %#v, want only active sess_a plan", got)
	}
}

func TestStorePersistsReportingObjects(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "agentd.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	now := time.Date(2026, 4, 10, 12, 0, 0, 0, time.UTC)

	spec := types.ChildAgentSpec{
		AgentID:           "docker-check",
		SessionID:         "sess_reporting",
		Purpose:           "Check Docker health every hour.",
		Mode:              types.ChildAgentModeBackgroundWorker,
		OutputContractRef: "ops-monitor-v1",
		ReportGroups:      []string{"ops-daily"},
		Schedule: types.ScheduleSpec{
			Kind:         types.ScheduleKindEvery,
			EveryMinutes: 60,
		},
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := store.UpsertChildAgentSpec(ctx, spec); err != nil {
		t.Fatalf("UpsertChildAgentSpec() error = %v", err)
	}

	contract := types.OutputContract{
		ContractID: "ops-monitor-v1",
		Intent:     "Operator-facing monitoring summary",
		Sections: []types.ContractSection{
			{ID: "overall_status", Title: "Overall Status", Required: true, MaxItems: 1},
			{ID: "key_findings", Title: "Key Findings", Required: true, MaxItems: 3},
		},
		Rules: types.OutputContractRules{
			IncludeSeverity: true,
			MustBeConcise:   true,
		},
		Tone: "concise_operator",
		UIHints: types.OutputContractUIHints{
			RenderAs:   "task_card",
			Expandable: true,
		},
		CreatedAt: now.Add(time.Minute),
		UpdatedAt: now.Add(time.Minute),
	}
	if err := store.UpsertOutputContract(ctx, contract); err != nil {
		t.Fatalf("UpsertOutputContract() error = %v", err)
	}

	group := types.ReportGroup{
		GroupID:   "ops-daily",
		SessionID: "sess_reporting",
		Title:     "Daily Ops Digest",
		Sources:   []string{"docker-check", "server-health"},
		Schedule: types.ScheduleSpec{
			Kind:     types.ScheduleKindCron,
			Expr:     "0 9 * * *",
			Timezone: "Asia/Shanghai",
		},
		Aggregation: types.AggregationContract{
			DedupeWindowMinutes: 60,
			PriorityOrder:       []string{"critical", "warning", "ok", "recovered"},
			Sections:            []string{"executive_summary", "critical_issues", "action_items"},
		},
		Delivery: types.DeliveryProfile{
			Channels: []string{"tui", "email"},
		},
		CreatedAt: now.Add(2 * time.Minute),
		UpdatedAt: now.Add(2 * time.Minute),
	}
	if err := store.UpsertReportGroup(ctx, group); err != nil {
		t.Fatalf("UpsertReportGroup() error = %v", err)
	}

	loadedSpec, ok, err := store.GetChildAgentSpec(ctx, spec.AgentID)
	if err != nil {
		t.Fatalf("GetChildAgentSpec() error = %v", err)
	}
	if !ok {
		t.Fatal("GetChildAgentSpec() ok = false, want true")
	}
	if loadedSpec.AgentID != spec.AgentID || loadedSpec.OutputContractRef != spec.OutputContractRef || len(loadedSpec.ReportGroups) != 1 || loadedSpec.ReportGroups[0] != "ops-daily" || loadedSpec.Schedule.Kind != types.ScheduleKindEvery || loadedSpec.Schedule.EveryMinutes != 60 {
		t.Fatalf("loaded spec = %#v, want %#v", loadedSpec, spec)
	}
	if loadedSpec.SessionID != spec.SessionID {
		t.Fatalf("loaded spec session = %q, want %q", loadedSpec.SessionID, spec.SessionID)
	}

	loadedContract, ok, err := store.GetOutputContract(ctx, contract.ContractID)
	if err != nil {
		t.Fatalf("GetOutputContract() error = %v", err)
	}
	if !ok {
		t.Fatal("GetOutputContract() ok = false, want true")
	}
	if loadedContract.ContractID != contract.ContractID || loadedContract.Intent != contract.Intent || loadedContract.Tone != contract.Tone || len(loadedContract.Sections) != 2 || !loadedContract.Rules.IncludeSeverity || !loadedContract.UIHints.Expandable {
		t.Fatalf("loaded contract = %#v, want %#v", loadedContract, contract)
	}

	loadedGroup, ok, err := store.GetReportGroup(ctx, group.GroupID)
	if err != nil {
		t.Fatalf("GetReportGroup() error = %v", err)
	}
	if !ok {
		t.Fatal("GetReportGroup() ok = false, want true")
	}
	if loadedGroup.GroupID != group.GroupID || loadedGroup.Title != group.Title || len(loadedGroup.Sources) != 2 || loadedGroup.Schedule.Expr != "0 9 * * *" || len(loadedGroup.Delivery.Channels) != 2 {
		t.Fatalf("loaded group = %#v, want %#v", loadedGroup, group)
	}
	if loadedGroup.SessionID != group.SessionID {
		t.Fatalf("loaded group session = %q, want %q", loadedGroup.SessionID, group.SessionID)
	}

	specs, err := store.ListChildAgentSpecs(ctx)
	if err != nil {
		t.Fatalf("ListChildAgentSpecs() error = %v", err)
	}
	if len(specs) != 1 || specs[0].AgentID != spec.AgentID {
		t.Fatalf("ListChildAgentSpecs() = %#v, want single %q", specs, spec.AgentID)
	}

	contracts, err := store.ListOutputContracts(ctx)
	if err != nil {
		t.Fatalf("ListOutputContracts() error = %v", err)
	}
	if len(contracts) != 1 || contracts[0].ContractID != contract.ContractID {
		t.Fatalf("ListOutputContracts() = %#v, want single %q", contracts, contract.ContractID)
	}

	groups, err := store.ListReportGroups(ctx)
	if err != nil {
		t.Fatalf("ListReportGroups() error = %v", err)
	}
	if len(groups) != 1 || groups[0].GroupID != group.GroupID {
		t.Fatalf("ListReportGroups() = %#v, want single %q", groups, group.GroupID)
	}
	sessionSpecs, err := store.ListChildAgentSpecsBySession(ctx, "sess_reporting")
	if err != nil {
		t.Fatalf("ListChildAgentSpecsBySession() error = %v", err)
	}
	if len(sessionSpecs) != 1 || sessionSpecs[0].AgentID != spec.AgentID {
		t.Fatalf("ListChildAgentSpecsBySession() = %#v, want single %q", sessionSpecs, spec.AgentID)
	}
	sessionGroups, err := store.ListReportGroupsBySession(ctx, "sess_reporting")
	if err != nil {
		t.Fatalf("ListReportGroupsBySession() error = %v", err)
	}
	if len(sessionGroups) != 1 || sessionGroups[0].GroupID != group.GroupID {
		t.Fatalf("ListReportGroupsBySession() = %#v, want single %q", sessionGroups, group.GroupID)
	}
}

func TestStorePersistsChildAgentResultsAndDigests(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "agentd.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	now := time.Date(2026, 4, 10, 13, 0, 0, 0, time.UTC)

	result := types.ChildAgentResult{
		ResultID:        "result_docker_1",
		SessionID:       "sess_reporting",
		AgentID:         "docker-check",
		ContractID:      "ops-monitor-v1",
		RunID:           "run_ops_1",
		TaskID:          "task_ops_1",
		ReportGroupRefs: []string{"ops-daily"},
		ObservedAt:      now,
		Envelope: types.ReportEnvelope{
			Source:   "docker",
			Status:   "warning",
			Severity: "warning",
			Title:    "1 unhealthy container",
			Summary:  "api container restarted 3 times in the last hour.",
			Sections: []types.ReportSectionContent{
				{ID: "overall_status", Title: "Overall Status", Text: "1 container unhealthy"},
				{ID: "action_items", Title: "Action Items", Items: []string{"Inspect api container logs"}},
			},
			Payload: []byte(`{"containers":[{"name":"api","status":"restarting"}]}`),
		},
		CreatedAt: now.Add(time.Minute),
		UpdatedAt: now.Add(2 * time.Minute),
	}
	if err := store.UpsertChildAgentResult(ctx, result); err != nil {
		t.Fatalf("UpsertChildAgentResult() error = %v", err)
	}

	digest := types.DigestRecord{
		DigestID:        "digest_ops_daily_1",
		SessionID:       "sess_reporting",
		GroupID:         "ops-daily",
		RunID:           "run_digest_1",
		TaskID:          "task_digest_1",
		SourceResultIDs: []string{result.ResultID},
		WindowStart:     now.Add(-time.Hour),
		WindowEnd:       now,
		Delivery: types.DeliveryProfile{
			Channels: []string{"tui"},
		},
		Envelope: types.ReportEnvelope{
			Source:   "ops-daily",
			Status:   "warning",
			Severity: "warning",
			Title:    "Ops daily digest",
			Summary:  "1 warning requires follow-up.",
			Sections: []types.ReportSectionContent{
				{ID: "executive_summary", Title: "Executive Summary", Text: "Docker api container is unstable."},
			},
			Payload: []byte(`{"warning_count":1}`),
		},
		CreatedAt: now.Add(3 * time.Minute),
		UpdatedAt: now.Add(4 * time.Minute),
	}
	if err := store.UpsertDigestRecord(ctx, digest); err != nil {
		t.Fatalf("UpsertDigestRecord() error = %v", err)
	}

	loadedResult, ok, err := store.GetChildAgentResult(ctx, result.ResultID)
	if err != nil {
		t.Fatalf("GetChildAgentResult() error = %v", err)
	}
	if !ok {
		t.Fatal("GetChildAgentResult() ok = false, want true")
	}
	if loadedResult.AgentID != result.AgentID || loadedResult.ContractID != result.ContractID || loadedResult.Envelope.Status != "warning" || len(loadedResult.ReportGroupRefs) != 1 || string(loadedResult.Envelope.Payload) != string(result.Envelope.Payload) {
		t.Fatalf("loaded result = %#v, want %#v", loadedResult, result)
	}
	if loadedResult.SessionID != result.SessionID {
		t.Fatalf("loaded result session = %q, want %q", loadedResult.SessionID, result.SessionID)
	}

	loadedDigest, ok, err := store.GetDigestRecord(ctx, digest.DigestID)
	if err != nil {
		t.Fatalf("GetDigestRecord() error = %v", err)
	}
	if !ok {
		t.Fatal("GetDigestRecord() ok = false, want true")
	}
	if loadedDigest.GroupID != digest.GroupID || loadedDigest.Envelope.Title != digest.Envelope.Title || len(loadedDigest.SourceResultIDs) != 1 || len(loadedDigest.Delivery.Channels) != 1 || string(loadedDigest.Envelope.Payload) != string(digest.Envelope.Payload) {
		t.Fatalf("loaded digest = %#v, want %#v", loadedDigest, digest)
	}
	if loadedDigest.SessionID != digest.SessionID {
		t.Fatalf("loaded digest session = %q, want %q", loadedDigest.SessionID, digest.SessionID)
	}

	results, err := store.ListChildAgentResultsByAgent(ctx, result.AgentID)
	if err != nil {
		t.Fatalf("ListChildAgentResultsByAgent() error = %v", err)
	}
	if len(results) != 1 || results[0].ResultID != result.ResultID {
		t.Fatalf("ListChildAgentResultsByAgent() = %#v, want single %q", results, result.ResultID)
	}

	groupResults, err := store.ListChildAgentResultsByReportGroup(ctx, "ops-daily")
	if err != nil {
		t.Fatalf("ListChildAgentResultsByReportGroup() error = %v", err)
	}
	if len(groupResults) != 1 || groupResults[0].ResultID != result.ResultID {
		t.Fatalf("ListChildAgentResultsByReportGroup() = %#v, want single %q", groupResults, result.ResultID)
	}

	digests, err := store.ListDigestRecordsByGroup(ctx, digest.GroupID)
	if err != nil {
		t.Fatalf("ListDigestRecordsByGroup() error = %v", err)
	}
	if len(digests) != 1 || digests[0].DigestID != digest.DigestID {
		t.Fatalf("ListDigestRecordsByGroup() = %#v, want single %q", digests, digest.DigestID)
	}
	sessionResults, err := store.ListChildAgentResultsBySession(ctx, "sess_reporting")
	if err != nil {
		t.Fatalf("ListChildAgentResultsBySession() error = %v", err)
	}
	if len(sessionResults) != 1 || sessionResults[0].ResultID != result.ResultID {
		t.Fatalf("ListChildAgentResultsBySession() = %#v, want single %q", sessionResults, result.ResultID)
	}
	sessionDigests, err := store.ListDigestRecordsBySession(ctx, "sess_reporting")
	if err != nil {
		t.Fatalf("ListDigestRecordsBySession() error = %v", err)
	}
	if len(sessionDigests) != 1 || sessionDigests[0].DigestID != digest.DigestID {
		t.Fatalf("ListDigestRecordsBySession() = %#v, want single %q", sessionDigests, digest.DigestID)
	}
}

func TestStoreWithTxCommitsAndRollsBack(t *testing.T) {
	t.Run("commits when callback returns nil", func(t *testing.T) {
		store, err := Open(filepath.Join(t.TempDir(), "agentd.db"))
		if err != nil {
			t.Fatalf("Open() error = %v", err)
		}
		defer store.Close()

		now := time.Date(2026, 4, 6, 10, 0, 0, 0, time.UTC)
		err = store.WithTx(context.Background(), func(tx RuntimeTx) error {
			return tx.InsertRun(context.Background(), types.Run{
				ID:        "run_commit",
				SessionID: "sess_commit",
				State:     types.RunStateRunning,
				CreatedAt: now,
				UpdatedAt: now,
			})
		})
		if err != nil {
			t.Fatalf("WithTx(commit) error = %v", err)
		}

		graph, err := store.ListRuntimeGraph(context.Background())
		if err != nil {
			t.Fatalf("ListRuntimeGraph() error = %v", err)
		}
		if len(graph.Runs) != 1 || graph.Runs[0].ID != "run_commit" {
			t.Fatalf("graph.Runs = %#v, want committed run", graph.Runs)
		}
	})

	t.Run("rolls back when callback returns error", func(t *testing.T) {
		store, err := Open(filepath.Join(t.TempDir(), "agentd.db"))
		if err != nil {
			t.Fatalf("Open() error = %v", err)
		}
		defer store.Close()

		now := time.Date(2026, 4, 6, 10, 30, 0, 0, time.UTC)
		wantErr := errors.New("force rollback")
		err = store.WithTx(context.Background(), func(tx RuntimeTx) error {
			if err := tx.InsertRun(context.Background(), types.Run{
				ID:        "run_rollback",
				SessionID: "sess_rollback",
				State:     types.RunStateRunning,
				CreatedAt: now,
				UpdatedAt: now,
			}); err != nil {
				return err
			}
			return wantErr
		})
		if !errors.Is(err, wantErr) {
			t.Fatalf("WithTx(rollback) error = %v, want %v", err, wantErr)
		}

		graph, err := store.ListRuntimeGraph(context.Background())
		if err != nil {
			t.Fatalf("ListRuntimeGraph() error = %v", err)
		}
		if len(graph.Runs) != 0 {
			t.Fatalf("graph.Runs = %#v, want rollback to leave no rows", graph.Runs)
		}
	})
}

func TestStorePreservesCreatedAtOnRuntimeObjectUpdate(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "agentd.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()

	createdAt := time.Date(2026, 4, 5, 11, 0, 0, 0, time.UTC)
	initialUpdatedAt := createdAt
	nextUpdatedAt := createdAt.Add(10 * time.Minute)

	plan := types.Plan{
		ID:        "plan_created_at",
		RunID:     "run_created_at",
		State:     types.PlanStateDraft,
		Title:     "Keep created_at stable",
		Summary:   "initial summary",
		CreatedAt: createdAt,
		UpdatedAt: initialUpdatedAt,
	}
	if err := store.UpsertPlan(context.Background(), plan); err != nil {
		t.Fatalf("UpsertPlan(initial) error = %v", err)
	}

	plan.Summary = "updated summary"
	plan.UpdatedAt = nextUpdatedAt
	if err := store.UpsertPlan(context.Background(), plan); err != nil {
		t.Fatalf("UpsertPlan(update) error = %v", err)
	}

	var createdAtRaw, updatedAtRaw string
	if err := store.db.QueryRowContext(context.Background(), `
		select created_at, updated_at
		from plans
		where id = ?
	`, plan.ID).Scan(&createdAtRaw, &updatedAtRaw); err != nil {
		t.Fatalf("plan timestamp query error = %v", err)
	}

	if got, err := time.Parse(timeLayout, createdAtRaw); err != nil {
		t.Fatalf("Parse(created_at) error = %v", err)
	} else if !got.Equal(createdAt) {
		t.Fatalf("plan created_at = %s, want %s", got, createdAt)
	}
	if got, err := time.Parse(timeLayout, updatedAtRaw); err != nil {
		t.Fatalf("Parse(updated_at) error = %v", err)
	} else if !got.Equal(nextUpdatedAt) {
		t.Fatalf("plan updated_at = %s, want %s", got, nextUpdatedAt)
	}

	graph, err := store.ListRuntimeGraph(context.Background())
	if err != nil {
		t.Fatalf("ListRuntimeGraph() error = %v", err)
	}
	if len(graph.Plans) != 1 {
		t.Fatalf("len(graph.Plans) = %d, want 1", len(graph.Plans))
	}
	got := graph.Plans[0]
	if got.Summary != "updated summary" {
		t.Fatalf("plan summary = %q, want %q", got.Summary, "updated summary")
	}
	if !got.CreatedAt.Equal(createdAt) {
		t.Fatalf("plan created_at = %s, want %s", got.CreatedAt, createdAt)
	}
	if !got.UpdatedAt.Equal(nextUpdatedAt) {
		t.Fatalf("plan updated_at = %s, want %s", got.UpdatedAt, nextUpdatedAt)
	}
}

func TestInsertRunRejectsDuplicateID(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "agentd.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()

	first := types.Run{
		ID:        "run_duplicate",
		SessionID: "sess_duplicate",
		TurnID:    "turn_duplicate",
		State:     types.RunStateRunning,
		Objective: "first insert wins",
		CreatedAt: time.Date(2026, 4, 5, 12, 0, 0, 0, time.UTC),
		UpdatedAt: time.Date(2026, 4, 5, 12, 0, 0, 0, time.UTC),
	}
	if err := store.InsertRun(context.Background(), first); err != nil {
		t.Fatalf("InsertRun(first) error = %v", err)
	}

	second := first
	second.SessionID = "sess_other"
	second.TurnID = "turn_other"
	second.Objective = "duplicate should fail"
	second.CreatedAt = first.CreatedAt.Add(5 * time.Minute)
	second.UpdatedAt = first.UpdatedAt.Add(5 * time.Minute)
	if err := store.InsertRun(context.Background(), second); err == nil {
		t.Fatal("InsertRun(second) error = nil, want duplicate ID failure")
	}

	var count int
	if err := store.db.QueryRowContext(context.Background(), `select count(*) from runs where id = ?`, first.ID).Scan(&count); err != nil {
		t.Fatalf("count query error = %v", err)
	}
	if count != 1 {
		t.Fatalf("count = %d, want 1", count)
	}

	graph, err := store.ListRuntimeGraph(context.Background())
	if err != nil {
		t.Fatalf("ListRuntimeGraph() error = %v", err)
	}
	if len(graph.Runs) != 1 {
		t.Fatalf("len(graph.Runs) = %d, want 1", len(graph.Runs))
	}
	got := graph.Runs[0]
	if got.SessionID != first.SessionID || got.TurnID != first.TurnID || got.Objective != first.Objective {
		t.Fatalf("run = %#v, want original first insert", got)
	}
	if !got.CreatedAt.Equal(first.CreatedAt) || !got.UpdatedAt.Equal(first.UpdatedAt) {
		t.Fatalf("run timestamps = %s/%s, want %s/%s", got.CreatedAt, got.UpdatedAt, first.CreatedAt, first.UpdatedAt)
	}
}

func TestListRuntimeGraphReadsConsistentSnapshot(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "agentd.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()

	baseAt := time.Date(2026, 4, 5, 12, 30, 0, 0, time.UTC)
	if _, err := store.db.ExecContext(context.Background(), `pragma journal_mode = wal`); err != nil {
		t.Fatalf("set WAL mode error = %v", err)
	}
	insertBaseGraph := func() {
		if err := store.InsertRun(context.Background(), types.Run{
			ID:        "run_snapshot_base",
			SessionID: "sess_snapshot",
			TurnID:    "turn_snapshot",
			State:     types.RunStateRunning,
			Objective: "baseline snapshot",
			CreatedAt: baseAt,
			UpdatedAt: baseAt,
		}); err != nil {
			t.Fatalf("InsertRun(base) error = %v", err)
		}
		if err := store.UpsertPlan(context.Background(), types.Plan{
			ID:           "plan_snapshot_base",
			RunID:        "run_snapshot_base",
			State:        types.PlanStateActive,
			Title:        "baseline plan",
			Summary:      "base graph",
			ParentPlanID: "plan_snapshot_parent",
			CreatedAt:    baseAt,
			UpdatedAt:    baseAt,
		}); err != nil {
			t.Fatalf("UpsertPlan(base) error = %v", err)
		}
		if err := store.UpsertTaskRecord(context.Background(), types.TaskRecord{
			ID:          "task_snapshot_base",
			RunID:       "run_snapshot_base",
			PlanID:      "plan_snapshot_base",
			State:       types.TaskStateRunning,
			Title:       "baseline task",
			Description: "base task",
			Owner:       "owner_base",
			WorktreeID:  "worktree_snapshot_base",
			CreatedAt:   baseAt,
			UpdatedAt:   baseAt,
		}); err != nil {
			t.Fatalf("UpsertTaskRecord(base) error = %v", err)
		}
		if err := store.UpsertToolRun(context.Background(), types.ToolRun{
			ID:          "tool_run_snapshot_base",
			RunID:       "run_snapshot_base",
			TaskID:      "task_snapshot_base",
			State:       types.ToolRunStateCompleted,
			ToolName:    "Bash",
			InputJSON:   `{"command":"echo base"}`,
			OutputJSON:  `{"exit_code":0}`,
			Error:       "base stderr",
			StartedAt:   baseAt.Add(1 * time.Minute),
			CompletedAt: baseAt.Add(2 * time.Minute),
			CreatedAt:   baseAt,
			UpdatedAt:   baseAt,
		}); err != nil {
			t.Fatalf("UpsertToolRun(base) error = %v", err)
		}
		if err := store.UpsertWorktree(context.Background(), types.Worktree{
			ID:             "worktree_snapshot_base",
			RunID:          "run_snapshot_base",
			TaskID:         "task_snapshot_base",
			State:          types.WorktreeStateActive,
			WorktreePath:   "E:/project/go-agent/.worktrees/base",
			WorktreeBranch: "feature/base",
			CreatedAt:      baseAt,
			UpdatedAt:      baseAt,
		}); err != nil {
			t.Fatalf("UpsertWorktree(base) error = %v", err)
		}
	}
	insertBaseGraph()

	hookStarted := make(chan struct{})
	releaseHook := make(chan struct{})
	originalHook := runtimeGraphReadHook
	runtimeGraphReadHook = func(stage string) {
		if stage != "after_runs" {
			return
		}
		close(hookStarted)
		<-releaseHook
	}
	t.Cleanup(func() {
		runtimeGraphReadHook = originalHook
	})

	type result struct {
		graph types.RuntimeGraph
		err   error
	}
	done := make(chan result, 1)
	go func() {
		graph, err := store.ListRuntimeGraph(context.Background())
		done <- result{graph: graph, err: err}
	}()

	select {
	case <-hookStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("ListRuntimeGraph() did not reach the snapshot hook")
	}

	newAt := baseAt.Add(1 * time.Hour)
	if err := store.UpsertPlan(context.Background(), types.Plan{
		ID:        "plan_snapshot_new",
		RunID:     "run_snapshot_base",
		State:     types.PlanStateActive,
		Title:     "new plan",
		Summary:   "new graph row",
		CreatedAt: newAt,
		UpdatedAt: newAt,
	}); err != nil {
		t.Fatalf("UpsertPlan(new) error = %v", err)
	}
	if err := store.UpsertTaskRecord(context.Background(), types.TaskRecord{
		ID:         "task_snapshot_new",
		RunID:      "run_snapshot_base",
		PlanID:     "plan_snapshot_new",
		State:      types.TaskStateRunning,
		Title:      "new task",
		Owner:      "owner_new",
		WorktreeID: "worktree_snapshot_new",
		CreatedAt:  newAt,
		UpdatedAt:  newAt,
	}); err != nil {
		t.Fatalf("UpsertTaskRecord(new) error = %v", err)
	}
	if err := store.UpsertToolRun(context.Background(), types.ToolRun{
		ID:         "tool_run_snapshot_new",
		RunID:      "run_snapshot_base",
		TaskID:     "task_snapshot_new",
		State:      types.ToolRunStateCompleted,
		ToolName:   "Bash",
		InputJSON:  `{"command":"echo new"}`,
		OutputJSON: `{"exit_code":0}`,
		Error:      "new stderr",
		CreatedAt:  newAt,
		UpdatedAt:  newAt,
	}); err != nil {
		t.Fatalf("UpsertToolRun(new) error = %v", err)
	}
	if err := store.UpsertWorktree(context.Background(), types.Worktree{
		ID:             "worktree_snapshot_new",
		RunID:          "run_snapshot_base",
		TaskID:         "task_snapshot_new",
		State:          types.WorktreeStateActive,
		WorktreePath:   "E:/project/go-agent/.worktrees/new",
		WorktreeBranch: "feature/new",
		CreatedAt:      newAt,
		UpdatedAt:      newAt,
	}); err != nil {
		t.Fatalf("UpsertWorktree(new) error = %v", err)
	}

	close(releaseHook)

	var got result
	select {
	case got = <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("ListRuntimeGraph() did not finish after releasing the hook")
	}
	if got.err != nil {
		t.Fatalf("ListRuntimeGraph() error = %v", got.err)
	}

	if len(got.graph.Runs) != 1 || len(got.graph.Plans) != 1 || len(got.graph.Tasks) != 1 || len(got.graph.ToolRuns) != 1 || len(got.graph.Worktrees) != 1 {
		t.Fatalf("snapshot graph = %#v, want only baseline rows", got.graph)
	}
	if got.graph.Plans[0].ID != "plan_snapshot_base" || got.graph.Tasks[0].ID != "task_snapshot_base" || got.graph.ToolRuns[0].ID != "tool_run_snapshot_base" || got.graph.Worktrees[0].ID != "worktree_snapshot_base" {
		t.Fatalf("snapshot graph IDs = %#v, want baseline rows only", got.graph)
	}
}

func TestOpenConfiguresSQLiteForConcurrentRuntimeUse(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "agentd.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()

	var journalMode string
	if err := store.db.QueryRowContext(context.Background(), `pragma journal_mode`).Scan(&journalMode); err != nil {
		t.Fatalf("journal_mode query error = %v", err)
	}
	if journalMode != "wal" {
		t.Fatalf("journal_mode = %q, want %q", journalMode, "wal")
	}

	var busyTimeout int
	if err := store.db.QueryRowContext(context.Background(), `pragma busy_timeout`).Scan(&busyTimeout); err != nil {
		t.Fatalf("busy_timeout query error = %v", err)
	}
	if busyTimeout < 5000 {
		t.Fatalf("busy_timeout = %d, want at least %d", busyTimeout, 5000)
	}
}

func TestOpenKeepsSQLitePragmasAfterReplacingConnection(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "agentd.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()

	assertPragmas := func() {
		t.Helper()

		var journalMode string
		if err := store.db.QueryRowContext(context.Background(), `pragma journal_mode`).Scan(&journalMode); err != nil {
			t.Fatalf("journal_mode query error = %v", err)
		}
		if journalMode != "wal" {
			t.Fatalf("journal_mode = %q, want %q", journalMode, "wal")
		}

		var busyTimeout int
		if err := store.db.QueryRowContext(context.Background(), `pragma busy_timeout`).Scan(&busyTimeout); err != nil {
			t.Fatalf("busy_timeout query error = %v", err)
		}
		if busyTimeout < sqliteBusyTimeoutMillis {
			t.Fatalf("busy_timeout = %d, want at least %d", busyTimeout, sqliteBusyTimeoutMillis)
		}
	}

	assertPragmas()

	conn, err := store.db.Conn(context.Background())
	if err != nil {
		t.Fatalf("Conn() error = %v", err)
	}
	_ = conn.Raw(func(any) error { return driver.ErrBadConn })
	_ = conn.Close()

	assertPragmas()
}

func TestStoreConcurrentWriteWaitsForReleasedConnection(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "agentd.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()

	conn, err := store.db.Conn(context.Background())
	if err != nil {
		t.Fatalf("Conn() error = %v", err)
	}

	tx, err := conn.BeginTx(context.Background(), nil)
	if err != nil {
		_ = conn.Close()
		t.Fatalf("BeginTx() error = %v", err)
	}

	now := time.Now().UTC().Format(timeLayout)
	if _, err := tx.ExecContext(context.Background(), `
		insert into runtime_metadata (key, value, updated_at)
		values (?, ?, ?)
	`, "write_lock", "held", now); err != nil {
		_ = tx.Rollback()
		_ = conn.Close()
		t.Fatalf("ExecContext(lock row) error = %v", err)
	}

	done := make(chan error, 1)
	go func() {
		event, err := types.NewEvent("sess_lock", "turn_lock", types.EventTurnStarted, types.TurnStartedPayload{
			WorkspaceRoot: "/tmp/lock-test",
		})
		if err != nil {
			done <- err
			return
		}
		_, err = store.AppendEvent(context.Background(), event)
		done <- err
	}()

	select {
	case err := <-done:
		t.Fatalf("AppendEvent() returned before releasing the connection: %v", err)
	case <-time.After(100 * time.Millisecond):
	}

	if err := tx.Commit(); err != nil {
		_ = conn.Close()
		t.Fatalf("Commit() error = %v", err)
	}
	if err := conn.Close(); err != nil {
		t.Fatalf("Conn.Close() error = %v", err)
	}

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("AppendEvent() error = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("AppendEvent() did not finish after the connection was released")
	}
}

func TestSeparateStoresWaitForWriteLockRelease(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "agentd.db")

	storeA, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open(storeA) error = %v", err)
	}
	defer storeA.Close()

	storeB, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open(storeB) error = %v", err)
	}
	defer storeB.Close()

	conn, err := storeA.db.Conn(context.Background())
	if err != nil {
		t.Fatalf("storeA.Conn() error = %v", err)
	}

	tx, err := conn.BeginTx(context.Background(), nil)
	if err != nil {
		_ = conn.Close()
		t.Fatalf("storeA.BeginTx() error = %v", err)
	}

	now := time.Now().UTC().Format(timeLayout)
	if _, err := tx.ExecContext(context.Background(), `
		insert into runtime_metadata (key, value, updated_at)
		values (?, ?, ?)
	`, "cross_store_lock", "held", now); err != nil {
		_ = tx.Rollback()
		_ = conn.Close()
		t.Fatalf("storeA.ExecContext(lock row) error = %v", err)
	}

	done := make(chan error, 1)
	go func() {
		event, err := types.NewEvent("sess_cross_lock", "turn_cross_lock", types.EventTurnStarted, types.TurnStartedPayload{
			WorkspaceRoot: "/tmp/cross-lock-test",
		})
		if err != nil {
			done <- err
			return
		}
		_, err = storeB.AppendEvent(context.Background(), event)
		done <- err
	}()

	select {
	case err := <-done:
		t.Fatalf("storeB.AppendEvent() returned before releasing storeA lock: %v", err)
	case <-time.After(100 * time.Millisecond):
	}

	if err := tx.Commit(); err != nil {
		_ = conn.Close()
		t.Fatalf("storeA.Commit() error = %v", err)
	}
	if err := conn.Close(); err != nil {
		t.Fatalf("storeA.Conn.Close() error = %v", err)
	}

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("storeB.AppendEvent() error = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("storeB.AppendEvent() did not finish after the lock was released")
	}
}

func TestStorePreservesCreatedAtOnTaskRecordUpdate(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "agentd.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()

	createdAt := time.Date(2026, 4, 5, 13, 0, 0, 0, time.UTC)
	nextUpdatedAt := createdAt.Add(15 * time.Minute)

	task := types.TaskRecord{
		ID:          "task_created_at",
		RunID:       "run_created_at",
		PlanID:      "plan_created_at",
		State:       types.TaskStateRunning,
		Title:       "Keep task created_at stable",
		Description: "initial task payload",
		Owner:       "owner_one",
		WorktreeID:  "worktree_one",
		CreatedAt:   createdAt,
		UpdatedAt:   createdAt,
	}
	if err := store.UpsertTaskRecord(context.Background(), task); err != nil {
		t.Fatalf("UpsertTaskRecord(initial) error = %v", err)
	}

	task.Description = "updated task payload"
	task.Owner = "owner_two"
	task.WorktreeID = "worktree_two"
	task.UpdatedAt = nextUpdatedAt
	if err := store.UpsertTaskRecord(context.Background(), task); err != nil {
		t.Fatalf("UpsertTaskRecord(update) error = %v", err)
	}

	var createdAtRaw, updatedAtRaw string
	if err := store.db.QueryRowContext(context.Background(), `
		select created_at, updated_at
		from task_records
		where id = ?
	`, task.ID).Scan(&createdAtRaw, &updatedAtRaw); err != nil {
		t.Fatalf("task timestamp query error = %v", err)
	}

	if got, err := time.Parse(timeLayout, createdAtRaw); err != nil {
		t.Fatalf("Parse(created_at) error = %v", err)
	} else if !got.Equal(createdAt) {
		t.Fatalf("task created_at = %s, want %s", got, createdAt)
	}
	if got, err := time.Parse(timeLayout, updatedAtRaw); err != nil {
		t.Fatalf("Parse(updated_at) error = %v", err)
	} else if !got.Equal(nextUpdatedAt) {
		t.Fatalf("task updated_at = %s, want %s", got, nextUpdatedAt)
	}

	graph, err := store.ListRuntimeGraph(context.Background())
	if err != nil {
		t.Fatalf("ListRuntimeGraph() error = %v", err)
	}
	if len(graph.Tasks) != 1 {
		t.Fatalf("len(graph.Tasks) = %d, want 1", len(graph.Tasks))
	}
	got := graph.Tasks[0]
	if got.Description != "updated task payload" || got.Owner != "owner_two" || got.WorktreeID != "worktree_two" {
		t.Fatalf("task payload = %#v, want updated values", got)
	}
	if !got.CreatedAt.Equal(createdAt) {
		t.Fatalf("task created_at = %s, want %s", got.CreatedAt, createdAt)
	}
	if !got.UpdatedAt.Equal(nextUpdatedAt) {
		t.Fatalf("task updated_at = %s, want %s", got.UpdatedAt, nextUpdatedAt)
	}
}

func TestStorePreservesCreatedAtOnToolRunUpdate(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "agentd.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()

	createdAt := time.Date(2026, 4, 5, 14, 0, 0, 0, time.UTC)
	nextUpdatedAt := createdAt.Add(20 * time.Minute)

	toolRun := types.ToolRun{
		ID:          "tool_run_created_at",
		RunID:       "run_created_at",
		TaskID:      "task_created_at",
		State:       types.ToolRunStateRunning,
		ToolName:    "Bash",
		InputJSON:   `{"command":"echo first"}`,
		OutputJSON:  `{"exit_code":0}`,
		Error:       "initial stderr",
		StartedAt:   createdAt.Add(1 * time.Minute),
		CompletedAt: createdAt.Add(2 * time.Minute),
		CreatedAt:   createdAt,
		UpdatedAt:   createdAt,
	}
	if err := store.UpsertToolRun(context.Background(), toolRun); err != nil {
		t.Fatalf("UpsertToolRun(initial) error = %v", err)
	}

	toolRun.OutputJSON = `{"exit_code":1}`
	toolRun.Error = "updated stderr"
	toolRun.UpdatedAt = nextUpdatedAt
	if err := store.UpsertToolRun(context.Background(), toolRun); err != nil {
		t.Fatalf("UpsertToolRun(update) error = %v", err)
	}

	var createdAtRaw, updatedAtRaw string
	if err := store.db.QueryRowContext(context.Background(), `
		select created_at, updated_at
		from tool_runs
		where id = ?
	`, toolRun.ID).Scan(&createdAtRaw, &updatedAtRaw); err != nil {
		t.Fatalf("tool run timestamp query error = %v", err)
	}

	if got, err := time.Parse(timeLayout, createdAtRaw); err != nil {
		t.Fatalf("Parse(created_at) error = %v", err)
	} else if !got.Equal(createdAt) {
		t.Fatalf("tool run created_at = %s, want %s", got, createdAt)
	}
	if got, err := time.Parse(timeLayout, updatedAtRaw); err != nil {
		t.Fatalf("Parse(updated_at) error = %v", err)
	} else if !got.Equal(nextUpdatedAt) {
		t.Fatalf("tool run updated_at = %s, want %s", got, nextUpdatedAt)
	}

	graph, err := store.ListRuntimeGraph(context.Background())
	if err != nil {
		t.Fatalf("ListRuntimeGraph() error = %v", err)
	}
	if len(graph.ToolRuns) != 1 {
		t.Fatalf("len(graph.ToolRuns) = %d, want 1", len(graph.ToolRuns))
	}
	got := graph.ToolRuns[0]
	if got.OutputJSON != `{"exit_code":1}` || got.Error != "updated stderr" {
		t.Fatalf("tool run payload = %#v, want updated values", got)
	}
	if !got.CreatedAt.Equal(createdAt) {
		t.Fatalf("tool run created_at = %s, want %s", got.CreatedAt, createdAt)
	}
	if !got.UpdatedAt.Equal(nextUpdatedAt) {
		t.Fatalf("tool run updated_at = %s, want %s", got.UpdatedAt, nextUpdatedAt)
	}
}

func TestStorePreservesCreatedAtOnWorktreeUpdate(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "agentd.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()

	createdAt := time.Date(2026, 4, 5, 15, 0, 0, 0, time.UTC)
	nextUpdatedAt := createdAt.Add(25 * time.Minute)

	worktree := types.Worktree{
		ID:             "worktree_created_at",
		RunID:          "run_created_at",
		TaskID:         "task_created_at",
		State:          types.WorktreeStateActive,
		WorktreePath:   "E:/project/go-agent/.worktrees/initial",
		WorktreeBranch: "feature/initial",
		CreatedAt:      createdAt,
		UpdatedAt:      createdAt,
	}
	if err := store.UpsertWorktree(context.Background(), worktree); err != nil {
		t.Fatalf("UpsertWorktree(initial) error = %v", err)
	}

	worktree.WorktreePath = "E:/project/go-agent/.worktrees/updated"
	worktree.WorktreeBranch = "feature/updated"
	worktree.UpdatedAt = nextUpdatedAt
	if err := store.UpsertWorktree(context.Background(), worktree); err != nil {
		t.Fatalf("UpsertWorktree(update) error = %v", err)
	}

	var createdAtRaw, updatedAtRaw string
	if err := store.db.QueryRowContext(context.Background(), `
		select created_at, updated_at
		from worktrees
		where id = ?
	`, worktree.ID).Scan(&createdAtRaw, &updatedAtRaw); err != nil {
		t.Fatalf("worktree timestamp query error = %v", err)
	}

	if got, err := time.Parse(timeLayout, createdAtRaw); err != nil {
		t.Fatalf("Parse(created_at) error = %v", err)
	} else if !got.Equal(createdAt) {
		t.Fatalf("worktree created_at = %s, want %s", got, createdAt)
	}
	if got, err := time.Parse(timeLayout, updatedAtRaw); err != nil {
		t.Fatalf("Parse(updated_at) error = %v", err)
	} else if !got.Equal(nextUpdatedAt) {
		t.Fatalf("worktree updated_at = %s, want %s", got, nextUpdatedAt)
	}

	graph, err := store.ListRuntimeGraph(context.Background())
	if err != nil {
		t.Fatalf("ListRuntimeGraph() error = %v", err)
	}
	if len(graph.Worktrees) != 1 {
		t.Fatalf("len(graph.Worktrees) = %d, want 1", len(graph.Worktrees))
	}
	got := graph.Worktrees[0]
	if got.WorktreePath != "E:/project/go-agent/.worktrees/updated" || got.WorktreeBranch != "feature/updated" {
		t.Fatalf("worktree payload = %#v, want updated values", got)
	}
	if !got.CreatedAt.Equal(createdAt) {
		t.Fatalf("worktree created_at = %s, want %s", got.CreatedAt, createdAt)
	}
	if !got.UpdatedAt.Equal(nextUpdatedAt) {
		t.Fatalf("worktree updated_at = %s, want %s", got.UpdatedAt, nextUpdatedAt)
	}
}

func TestStoreDeleteTurnRemovesRow(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "agentd.db")

	store, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()

	turn := types.Turn{
		ID:          "turn_delete",
		SessionID:   "sess_test",
		State:       types.TurnStateCreated,
		UserMessage: "hello",
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}
	if err := store.InsertTurn(context.Background(), turn); err != nil {
		t.Fatalf("InsertTurn() error = %v", err)
	}
	if err := store.DeleteTurn(context.Background(), turn.ID); err != nil {
		t.Fatalf("DeleteTurn() error = %v", err)
	}

	var count int
	if err := store.db.QueryRowContext(context.Background(), `select count(*) from turns where id = ?`, turn.ID).Scan(&count); err != nil {
		t.Fatalf("count query error = %v", err)
	}
	if count != 0 {
		t.Fatalf("count = %d, want 0", count)
	}
}

func TestStorePersistsConversationItemsAndSummaries(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "agentd.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()

	firstItem := model.ConversationItem{
		Kind: model.ConversationItemUserMessage,
		Text: "inspect repository",
	}
	if err := store.InsertConversationItem(context.Background(), "sess_1", "turn_1", 2, firstItem); err != nil {
		t.Fatalf("InsertConversationItem() error = %v", err)
	}

	secondItem := model.ConversationItem{
		Kind: model.ConversationItemAssistantText,
		Text: "use glob first",
	}
	if err := store.InsertConversationItem(context.Background(), "sess_1", "turn_1", 1, secondItem); err != nil {
		t.Fatalf("InsertConversationItem() error = %v", err)
	}

	summary := model.Summary{
		RangeLabel:       "turns 1-4",
		UserGoals:        []string{"inspect repository"},
		ImportantChoices: []string{"use glob first"},
	}
	if err := store.InsertConversationSummary(context.Background(), "sess_1", 4, summary); err != nil {
		t.Fatalf("InsertConversationSummary() error = %v", err)
	}

	items, err := store.ListConversationItems(context.Background(), "sess_1")
	if err != nil {
		t.Fatalf("ListConversationItems() error = %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("len(items) = %d, want 2", len(items))
	}
	if items[0].Kind != model.ConversationItemAssistantText || items[0].Text != "use glob first" {
		t.Fatalf("first item = %#v, want assistant text round-trip", items[0])
	}
	if items[1].Kind != model.ConversationItemUserMessage || items[1].Text != "inspect repository" {
		t.Fatalf("second item = %#v, want user message round-trip", items[1])
	}

	summaries, err := store.ListConversationSummaries(context.Background(), "sess_1")
	if err != nil {
		t.Fatalf("ListConversationSummaries() error = %v", err)
	}
	if len(summaries) != 1 {
		t.Fatalf("len(summaries) = %d, want 1", len(summaries))
	}
	if summaries[0].RangeLabel != "turns 1-4" {
		t.Fatalf("RangeLabel = %q, want %q", summaries[0].RangeLabel, "turns 1-4")
	}
	if len(summaries[0].ImportantChoices) != 1 || summaries[0].ImportantChoices[0] != "use glob first" {
		t.Fatalf("ImportantChoices = %#v, want [%q]", summaries[0].ImportantChoices, "use glob first")
	}

	entry := types.MemoryEntry{
		ID:          "mem_1",
		Scope:       types.MemoryScopeWorkspace,
		WorkspaceID: "ws_1",
		Content:     "workspace prefers rg before grep fallback",
		SourceRefs:  []string{"turn_1"},
		Confidence:  0.9,
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}
	if err := store.InsertMemoryEntry(context.Background(), entry); err != nil {
		t.Fatalf("InsertMemoryEntry() error = %v", err)
	}

	entries, err := store.ListMemoryEntriesByWorkspace(context.Background(), "ws_1")
	if err != nil {
		t.Fatalf("ListMemoryEntriesByWorkspace() error = %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("len(entries) = %d, want 1", len(entries))
	}
}

func TestStoreListConversationTimelineItemsPreservesTurnID(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "agentd.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()

	if err := store.InsertConversationItem(context.Background(), "sess_turns", "turn_alpha", 1, model.UserMessageItem("hello")); err != nil {
		t.Fatalf("InsertConversationItem() error = %v", err)
	}
	items, err := store.ListConversationTimelineItems(context.Background(), "sess_turns")
	if err != nil {
		t.Fatalf("ListConversationTimelineItems() error = %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("len(items) = %d, want 1", len(items))
	}
	if items[0].TurnID != "turn_alpha" {
		t.Fatalf("timeline item turn_id = %q, want %q", items[0].TurnID, "turn_alpha")
	}
	if items[0].Item.Kind != model.ConversationItemUserMessage || items[0].Item.Text != "hello" {
		t.Fatalf("timeline item = %#v, want user message payload", items[0])
	}
}

func TestStoreListsGlobalMemoryEntriesAlongsideWorkspaceScope(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "agentd.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()

	global := types.MemoryEntry{
		ID:         "mem_global",
		Scope:      types.MemoryScopeGlobal,
		Content:    "[Global durable memory] I prefer answers in Chinese",
		SourceRefs: []string{"turn_global"},
		Confidence: 0.9,
		CreatedAt:  time.Now().UTC().Add(-time.Minute),
		UpdatedAt:  time.Now().UTC().Add(-time.Minute),
	}
	if err := store.InsertMemoryEntry(context.Background(), global); err != nil {
		t.Fatalf("InsertMemoryEntry(global) error = %v", err)
	}

	workspace := types.MemoryEntry{
		ID:          "mem_workspace",
		Scope:       types.MemoryScopeWorkspace,
		WorkspaceID: "ws_global_mix",
		Content:     "[Workspace durable memory]\nChoices: prefer rg",
		SourceRefs:  []string{"turn_workspace"},
		Confidence:  0.8,
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}
	if err := store.InsertMemoryEntry(context.Background(), workspace); err != nil {
		t.Fatalf("InsertMemoryEntry(workspace) error = %v", err)
	}

	entries, err := store.ListMemoryEntriesByWorkspace(context.Background(), "ws_global_mix")
	if err != nil {
		t.Fatalf("ListMemoryEntriesByWorkspace() error = %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("len(entries) = %d, want 2", len(entries))
	}
	if entries[0].ID != global.ID || entries[0].Scope != types.MemoryScopeGlobal {
		t.Fatalf("entries[0] = %#v, want global memory first", entries[0])
	}
	if entries[1].ID != workspace.ID {
		t.Fatalf("entries[1] = %#v, want workspace memory second", entries[1])
	}
}

func TestStoreDeleteMemoryEntriesRemovesTargetedRows(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "agentd.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()

	first := types.MemoryEntry{
		ID:          "mem_delete_1",
		Scope:       types.MemoryScopeWorkspace,
		WorkspaceID: "ws_delete",
		Content:     "first memory",
		SourceRefs:  []string{"turn_1"},
		Confidence:  0.7,
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}
	second := types.MemoryEntry{
		ID:          "mem_delete_2",
		Scope:       types.MemoryScopeWorkspace,
		WorkspaceID: "ws_delete",
		Content:     "second memory",
		SourceRefs:  []string{"turn_2"},
		Confidence:  0.8,
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}
	if err := store.InsertMemoryEntry(context.Background(), first); err != nil {
		t.Fatalf("InsertMemoryEntry(first) error = %v", err)
	}
	if err := store.InsertMemoryEntry(context.Background(), second); err != nil {
		t.Fatalf("InsertMemoryEntry(second) error = %v", err)
	}

	if err := store.DeleteMemoryEntries(context.Background(), []string{first.ID}); err != nil {
		t.Fatalf("DeleteMemoryEntries() error = %v", err)
	}

	entries, err := store.ListMemoryEntriesByWorkspace(context.Background(), "ws_delete")
	if err != nil {
		t.Fatalf("ListMemoryEntriesByWorkspace() error = %v", err)
	}
	if len(entries) != 1 || entries[0].ID != second.ID {
		t.Fatalf("entries = %#v, want only second memory after targeted delete", entries)
	}
}

func TestStoreDeleteTurnRemovesConversationItems(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "agentd.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()

	turn := types.Turn{
		ID:          "turn_cleanup",
		SessionID:   "sess_cleanup",
		State:       types.TurnStateCreated,
		UserMessage: "hello",
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}
	if err := store.InsertTurn(context.Background(), turn); err != nil {
		t.Fatalf("InsertTurn() error = %v", err)
	}
	if err := store.InsertConversationItem(context.Background(), turn.SessionID, turn.ID, 1, model.ConversationItem{
		Kind: model.ConversationItemUserMessage,
		Text: "hello",
	}); err != nil {
		t.Fatalf("InsertConversationItem() error = %v", err)
	}

	if err := store.DeleteTurn(context.Background(), turn.ID); err != nil {
		t.Fatalf("DeleteTurn() error = %v", err)
	}

	items, err := store.ListConversationItems(context.Background(), turn.SessionID)
	if err != nil {
		t.Fatalf("ListConversationItems() error = %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("len(items) = %d, want 0", len(items))
	}
}

func TestStoreListsMemoryEntriesByWorkspaceInUpdatedAtOrderWithUTCNormalization(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "agentd.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()

	older := time.Date(2024, 1, 1, 2, 0, 0, 0, time.UTC)
	newer := time.Date(2024, 1, 1, 0, 30, 0, 0, time.FixedZone("EST", -5*60*60))

	if err := store.InsertMemoryEntry(context.Background(), types.MemoryEntry{
		ID:          "mem_old",
		Scope:       types.MemoryScopeWorkspace,
		WorkspaceID: "ws_utc",
		Content:     "older",
		SourceRefs:  []string{"turn_old"},
		Confidence:  0.5,
		CreatedAt:   older,
		UpdatedAt:   older,
	}); err != nil {
		t.Fatalf("InsertMemoryEntry() error = %v", err)
	}

	if err := store.InsertMemoryEntry(context.Background(), types.MemoryEntry{
		ID:          "mem_new",
		Scope:       types.MemoryScopeWorkspace,
		WorkspaceID: "ws_utc",
		Content:     "newer",
		SourceRefs:  []string{"turn_new"},
		Confidence:  0.9,
		CreatedAt:   newer,
		UpdatedAt:   newer,
	}); err != nil {
		t.Fatalf("InsertMemoryEntry() error = %v", err)
	}

	entries, err := store.ListMemoryEntriesByWorkspace(context.Background(), "ws_utc")
	if err != nil {
		t.Fatalf("ListMemoryEntriesByWorkspace() error = %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("len(entries) = %d, want 2", len(entries))
	}
	if entries[0].ID != "mem_new" || entries[0].Scope != types.MemoryScopeWorkspace || len(entries[0].SourceRefs) != 1 || entries[0].SourceRefs[0] != "turn_new" {
		t.Fatalf("entries[0] = %#v, want newest entry round-trip", entries[0])
	}
	if !entries[0].UpdatedAt.Equal(newer.UTC()) || !entries[0].CreatedAt.Equal(newer.UTC()) {
		t.Fatalf("entries[0] times = %s/%s, want %s", entries[0].CreatedAt, entries[0].UpdatedAt, newer.UTC())
	}
	if entries[1].ID != "mem_old" {
		t.Fatalf("entries[1].ID = %q, want %q", entries[1].ID, "mem_old")
	}
}

func TestStoreListsSessionsInUpdatedAtDescOrder(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "agentd.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()

	older := time.Date(2026, 4, 4, 8, 0, 0, 0, time.UTC)
	newer := older.Add(5 * time.Minute)

	if err := store.InsertSession(context.Background(), types.Session{
		ID:            "sess_old",
		WorkspaceRoot: "D:/work/old",
		State:         types.SessionStateIdle,
		CreatedAt:     older,
		UpdatedAt:     older,
	}); err != nil {
		t.Fatalf("InsertSession(old) error = %v", err)
	}
	if err := store.InsertSession(context.Background(), types.Session{
		ID:            "sess_new",
		WorkspaceRoot: "D:/work/new",
		State:         types.SessionStateIdle,
		CreatedAt:     newer,
		UpdatedAt:     newer,
	}); err != nil {
		t.Fatalf("InsertSession(new) error = %v", err)
	}

	sessions, err := store.ListSessions(context.Background())
	if err != nil {
		t.Fatalf("ListSessions() error = %v", err)
	}
	if len(sessions) != 2 {
		t.Fatalf("len(sessions) = %d, want 2", len(sessions))
	}
	if sessions[0].ID != "sess_new" || sessions[1].ID != "sess_old" {
		t.Fatalf("sessions = %#v, want newest first", sessions)
	}
}

func TestStorePersistsAndUpdatesSessionSystemPrompt(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "agentd.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()

	now := time.Now().UTC()
	if err := store.InsertSession(context.Background(), types.Session{
		ID:            "sess_prompt",
		WorkspaceRoot: "D:/work/prompt",
		SystemPrompt:  "focus on internal/model",
		State:         types.SessionStateIdle,
		CreatedAt:     now,
		UpdatedAt:     now,
	}); err != nil {
		t.Fatalf("InsertSession() error = %v", err)
	}

	sessions, err := store.ListSessions(context.Background())
	if err != nil {
		t.Fatalf("ListSessions() error = %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("len(sessions) = %d, want 1", len(sessions))
	}
	if sessions[0].SystemPrompt != "focus on internal/model" {
		t.Fatalf("SystemPrompt = %q, want %q", sessions[0].SystemPrompt, "focus on internal/model")
	}

	updated, ok, err := store.UpdateSessionSystemPrompt(context.Background(), "sess_prompt", "focus on internal/context")
	if err != nil {
		t.Fatalf("UpdateSessionSystemPrompt() error = %v", err)
	}
	if !ok {
		t.Fatal("UpdateSessionSystemPrompt() ok = false, want true")
	}
	if updated.SystemPrompt != "focus on internal/context" {
		t.Fatalf("updated.SystemPrompt = %q, want %q", updated.SystemPrompt, "focus on internal/context")
	}

	got, ok, err := store.GetSession(context.Background(), "sess_prompt")
	if err != nil {
		t.Fatalf("GetSession() error = %v", err)
	}
	if !ok {
		t.Fatal("GetSession() ok = false, want true")
	}
	if got.SystemPrompt != "focus on internal/context" {
		t.Fatalf("GetSession().SystemPrompt = %q, want %q", got.SystemPrompt, "focus on internal/context")
	}
}

func TestStorePersistsSelectedSessionID(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "agentd.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()

	selected, ok, err := store.GetSelectedSessionID(context.Background())
	if err != nil {
		t.Fatalf("GetSelectedSessionID(initial) error = %v", err)
	}
	if ok || selected != "" {
		t.Fatalf("initial selected = %q, %v, want empty false", selected, ok)
	}

	if err := store.SetSelectedSessionID(context.Background(), "sess_focus"); err != nil {
		t.Fatalf("SetSelectedSessionID() error = %v", err)
	}

	selected, ok, err = store.GetSelectedSessionID(context.Background())
	if err != nil {
		t.Fatalf("GetSelectedSessionID(saved) error = %v", err)
	}
	if !ok || selected != "sess_focus" {
		t.Fatalf("selected = %q, %v, want %q true", selected, ok, "sess_focus")
	}
}

func TestStoreDeleteSessionRemovesSessionScopedDataAndFallsBackSelection(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "agentd.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()

	now := time.Date(2026, 4, 5, 10, 0, 0, 0, time.UTC)
	target := types.Session{
		ID:            "sess_delete",
		WorkspaceRoot: "E:/project/go-agent",
		State:         types.SessionStateIdle,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	fallback := types.Session{
		ID:            "sess_keep",
		WorkspaceRoot: "E:/project/go-agent",
		State:         types.SessionStateIdle,
		CreatedAt:     now.Add(time.Minute),
		UpdatedAt:     now.Add(time.Minute),
	}
	if err := store.InsertSession(context.Background(), target); err != nil {
		t.Fatalf("InsertSession(target) error = %v", err)
	}
	if err := store.InsertSession(context.Background(), fallback); err != nil {
		t.Fatalf("InsertSession(fallback) error = %v", err)
	}
	if err := store.SetSelectedSessionID(context.Background(), target.ID); err != nil {
		t.Fatalf("SetSelectedSessionID() error = %v", err)
	}

	turn := types.Turn{
		ID:          "turn_delete",
		SessionID:   target.ID,
		State:       types.TurnStateCompleted,
		UserMessage: "delete this session",
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := store.InsertTurn(context.Background(), turn); err != nil {
		t.Fatalf("InsertTurn() error = %v", err)
	}
	if err := store.InsertConversationItem(context.Background(), target.ID, turn.ID, 1, model.UserMessageItem(turn.UserMessage)); err != nil {
		t.Fatalf("InsertConversationItem() error = %v", err)
	}
	if err := store.InsertConversationSummary(context.Background(), target.ID, 1, model.Summary{RangeLabel: "turns 1-1"}); err != nil {
		t.Fatalf("InsertConversationSummary() error = %v", err)
	}
	event, err := types.NewEvent(target.ID, turn.ID, types.EventAssistantDelta, map[string]any{"text": "hello"})
	if err != nil {
		t.Fatalf("NewEvent() error = %v", err)
	}
	if _, err := store.AppendEvent(context.Background(), event); err != nil {
		t.Fatalf("AppendEvent() error = %v", err)
	}
	if err := store.UpsertTurnUsage(context.Background(), types.TurnUsage{
		TurnID:       turn.ID,
		SessionID:    target.ID,
		Provider:     "openai_compatible",
		Model:        "glm-4-7-251222",
		InputTokens:  12,
		OutputTokens: 3,
		CreatedAt:    now,
		UpdatedAt:    now,
	}); err != nil {
		t.Fatalf("UpsertTurnUsage() error = %v", err)
	}

	run := types.Run{
		ID:        "run_delete",
		SessionID: target.ID,
		TurnID:    turn.ID,
		State:     types.RunStateCompleted,
		Objective: "clean up",
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := store.InsertRun(context.Background(), run); err != nil {
		t.Fatalf("InsertRun() error = %v", err)
	}
	plan := types.Plan{
		ID:        "plan_delete",
		RunID:     run.ID,
		State:     types.PlanStateCompleted,
		Title:     "plan",
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := store.UpsertPlan(context.Background(), plan); err != nil {
		t.Fatalf("UpsertPlan() error = %v", err)
	}
	task := types.TaskRecord{
		ID:          "task_delete",
		RunID:       run.ID,
		PlanID:      plan.ID,
		State:       types.TaskStateCompleted,
		Title:       "task",
		Description: "task",
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := store.UpsertTaskRecord(context.Background(), task); err != nil {
		t.Fatalf("UpsertTaskRecord() error = %v", err)
	}
	if err := store.UpsertToolRun(context.Background(), types.ToolRun{
		ID:        "tool_run_delete",
		RunID:     run.ID,
		TaskID:    task.ID,
		State:     types.ToolRunStateCompleted,
		ToolName:  "shell_command",
		CreatedAt: now,
		UpdatedAt: now,
	}); err != nil {
		t.Fatalf("UpsertToolRun() error = %v", err)
	}
	if err := store.UpsertWorktree(context.Background(), types.Worktree{
		ID:             "worktree_delete",
		RunID:          run.ID,
		TaskID:         task.ID,
		State:          types.WorktreeStateRemoved,
		WorktreePath:   "E:/project/go-agent/.worktrees/tmp",
		WorktreeBranch: "feature/tmp",
		CreatedAt:      now,
		UpdatedAt:      now,
	}); err != nil {
		t.Fatalf("UpsertWorktree() error = %v", err)
	}

	head := types.ProviderCacheHead{
		SessionID:         target.ID,
		Provider:          "openai_compatible",
		CapabilityProfile: "ark_responses",
		ActiveSessionRef:  "resp_active",
		ActivePrefixRef:   "pref_active",
		ActiveGeneration:  1,
		UpdatedAt:         now,
	}
	if err := store.UpsertProviderCacheHead(context.Background(), head); err != nil {
		t.Fatalf("UpsertProviderCacheHead() error = %v", err)
	}
	if err := store.InsertProviderCacheEntry(context.Background(), types.ProviderCacheEntry{
		ID:                "cache_delete",
		SessionID:         target.ID,
		Provider:          head.Provider,
		CapabilityProfile: head.CapabilityProfile,
		CacheKind:         "session",
		ExternalRef:       "resp_active",
		Generation:        1,
		Status:            "active",
		CreatedAt:         now,
		UpdatedAt:         now,
	}); err != nil {
		t.Fatalf("InsertProviderCacheEntry() error = %v", err)
	}
	if err := store.InsertConversationCompaction(context.Background(), types.ConversationCompaction{
		ID:              "compaction_delete",
		SessionID:       target.ID,
		Kind:            "rolling",
		Generation:      1,
		StartPosition:   1,
		EndPosition:     1,
		SummaryPayload:  `{"range_label":"turns 1-1"}`,
		Reason:          "token_budget",
		ProviderProfile: head.CapabilityProfile,
		CreatedAt:       now,
	}); err != nil {
		t.Fatalf("InsertConversationCompaction() error = %v", err)
	}

	nextSelected, deleted, err := store.DeleteSession(context.Background(), target.ID)
	if err != nil {
		t.Fatalf("DeleteSession() error = %v", err)
	}
	if !deleted {
		t.Fatal("DeleteSession() deleted = false, want true")
	}
	if nextSelected != fallback.ID {
		t.Fatalf("nextSelected = %q, want %q", nextSelected, fallback.ID)
	}

	gotSelected, ok, err := store.GetSelectedSessionID(context.Background())
	if err != nil {
		t.Fatalf("GetSelectedSessionID() error = %v", err)
	}
	if !ok || gotSelected != fallback.ID {
		t.Fatalf("selected session = %q, %v, want %q true", gotSelected, ok, fallback.ID)
	}

	if _, ok, err := store.GetSession(context.Background(), target.ID); err != nil {
		t.Fatalf("GetSession(target) error = %v", err)
	} else if ok {
		t.Fatal("GetSession(target) ok = true, want false")
	}

	checks := []struct {
		name  string
		query string
		args  []any
	}{
		{name: "sessions", query: `select count(*) from sessions where id = ?`, args: []any{target.ID}},
		{name: "turns", query: `select count(*) from turns where session_id = ?`, args: []any{target.ID}},
		{name: "events", query: `select count(*) from events where session_id = ?`, args: []any{target.ID}},
		{name: "conversation_items", query: `select count(*) from conversation_items where session_id = ?`, args: []any{target.ID}},
		{name: "conversation_summaries", query: `select count(*) from conversation_summaries where session_id = ?`, args: []any{target.ID}},
		{name: "turn_usage", query: `select count(*) from turn_usage where session_id = ?`, args: []any{target.ID}},
		{name: "conversation_compactions", query: `select count(*) from conversation_compactions where session_id = ?`, args: []any{target.ID}},
		{name: "provider_cache_entries", query: `select count(*) from provider_cache_entries where session_id = ?`, args: []any{target.ID}},
		{name: "provider_cache_heads", query: `select count(*) from provider_cache_heads where session_id = ?`, args: []any{target.ID}},
		{name: "runs", query: `select count(*) from runs where session_id = ?`, args: []any{target.ID}},
		{name: "plans", query: `select count(*) from plans where run_id = ?`, args: []any{run.ID}},
		{name: "task_records", query: `select count(*) from task_records where run_id = ?`, args: []any{run.ID}},
		{name: "tool_runs", query: `select count(*) from tool_runs where run_id = ?`, args: []any{run.ID}},
		{name: "worktrees", query: `select count(*) from worktrees where run_id = ?`, args: []any{run.ID}},
	}
	for _, check := range checks {
		var count int
		if err := store.db.QueryRowContext(context.Background(), check.query, check.args...).Scan(&count); err != nil {
			t.Fatalf("count %s error = %v", check.name, err)
		}
		if count != 0 {
			t.Fatalf("%s count = %d, want 0", check.name, count)
		}
	}
}

func TestStoreDeleteSessionClearsSelectedMetadataWhenLastSessionIsRemoved(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "agentd.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()

	now := time.Date(2026, 4, 5, 10, 0, 0, 0, time.UTC)
	session := types.Session{
		ID:            "sess_last",
		WorkspaceRoot: "E:/project/go-agent",
		State:         types.SessionStateIdle,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := store.InsertSession(context.Background(), session); err != nil {
		t.Fatalf("InsertSession() error = %v", err)
	}
	if err := store.SetSelectedSessionID(context.Background(), session.ID); err != nil {
		t.Fatalf("SetSelectedSessionID() error = %v", err)
	}

	nextSelected, deleted, err := store.DeleteSession(context.Background(), session.ID)
	if err != nil {
		t.Fatalf("DeleteSession() error = %v", err)
	}
	if !deleted {
		t.Fatal("DeleteSession() deleted = false, want true")
	}
	if nextSelected != "" {
		t.Fatalf("nextSelected = %q, want empty", nextSelected)
	}

	selected, ok, err := store.GetSelectedSessionID(context.Background())
	if err != nil {
		t.Fatalf("GetSelectedSessionID() error = %v", err)
	}
	if ok || selected != "" {
		t.Fatalf("selected = %q, %v, want empty false", selected, ok)
	}

	sessions, err := store.ListSessions(context.Background())
	if err != nil {
		t.Fatalf("ListSessions() error = %v", err)
	}
	if len(sessions) != 0 {
		t.Fatalf("len(sessions) = %d, want 0", len(sessions))
	}
}

func TestStoreListsRunningTurnsOldestFirst(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "agentd.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()

	older := time.Date(2026, 4, 4, 8, 0, 0, 0, time.UTC)
	newer := older.Add(5 * time.Minute)

	if err := store.InsertTurn(context.Background(), types.Turn{
		ID:          "turn_created",
		SessionID:   "sess_1",
		State:       types.TurnStateCreated,
		UserMessage: "ignore me",
		CreatedAt:   older.Add(2 * time.Minute),
		UpdatedAt:   older.Add(2 * time.Minute),
	}); err != nil {
		t.Fatalf("InsertTurn(created) error = %v", err)
	}

	if err := store.InsertTurn(context.Background(), types.Turn{
		ID:          "turn_running_old",
		SessionID:   "sess_1",
		State:       types.TurnStateModelStreaming,
		UserMessage: "first running",
		CreatedAt:   older,
		UpdatedAt:   older,
	}); err != nil {
		t.Fatalf("InsertTurn(running old) error = %v", err)
	}

	if err := store.InsertTurn(context.Background(), types.Turn{
		ID:          "turn_running_new",
		SessionID:   "sess_2",
		State:       types.TurnStateToolRunning,
		UserMessage: "second running",
		CreatedAt:   newer,
		UpdatedAt:   newer,
	}); err != nil {
		t.Fatalf("InsertTurn(running new) error = %v", err)
	}

	turns, err := store.ListRunningTurns(context.Background())
	if err != nil {
		t.Fatalf("ListRunningTurns() error = %v", err)
	}
	if len(turns) != 2 {
		t.Fatalf("len(turns) = %d, want 2", len(turns))
	}
	if turns[0].ID != "turn_running_old" || turns[0].State != types.TurnStateModelStreaming {
		t.Fatalf("turns[0] = %#v, want oldest running turn", turns[0])
	}
	if turns[1].ID != "turn_running_new" || turns[1].State != types.TurnStateToolRunning {
		t.Fatalf("turns[1] = %#v, want newest running turn", turns[1])
	}
}

func TestStorePersistsProviderCacheHeadsAndCompactions(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "agentd.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()

	now := time.Date(2026, 4, 4, 8, 0, 0, 0, time.UTC)

	head := types.ProviderCacheHead{
		SessionID:         "sess_cache",
		Provider:          "openai_compatible",
		CapabilityProfile: "ark_responses",
		ActiveSessionRef:  "resp_active",
		ActivePrefixRef:   "resp_prefix",
		ActiveGeneration:  3,
		UpdatedAt:         now,
	}
	if err := store.UpsertProviderCacheHead(context.Background(), head); err != nil {
		t.Fatalf("UpsertProviderCacheHead() error = %v", err)
	}

	entry := types.ProviderCacheEntry{
		ID:                "cache_entry_1",
		SessionID:         head.SessionID,
		Provider:          head.Provider,
		CapabilityProfile: head.CapabilityProfile,
		CacheKind:         "session",
		ExternalRef:       "resp_active",
		Generation:        3,
		Status:            "active",
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	if err := store.InsertProviderCacheEntry(context.Background(), entry); err != nil {
		t.Fatalf("InsertProviderCacheEntry() error = %v", err)
	}

	compaction := types.ConversationCompaction{
		ID:              "compaction_1",
		SessionID:       head.SessionID,
		Kind:            "rolling",
		Generation:      3,
		StartPosition:   1,
		EndPosition:     4,
		SummaryPayload:  `{"range_label":"turns 1-4"}`,
		Reason:          "token_budget",
		ProviderProfile: head.CapabilityProfile,
		CreatedAt:       now,
	}
	if err := store.InsertConversationCompaction(context.Background(), compaction); err != nil {
		t.Fatalf("InsertConversationCompaction() error = %v", err)
	}

	gotHead, ok, err := store.GetProviderCacheHead(context.Background(), head.SessionID, head.Provider, head.CapabilityProfile)
	if err != nil {
		t.Fatalf("GetProviderCacheHead() error = %v", err)
	}
	if !ok {
		t.Fatal("GetProviderCacheHead() ok = false, want true")
	}
	if gotHead.ActiveSessionRef != head.ActiveSessionRef || gotHead.ActivePrefixRef != head.ActivePrefixRef || gotHead.ActiveGeneration != head.ActiveGeneration {
		t.Fatalf("GetProviderCacheHead() = %#v, want %#v", gotHead, head)
	}

	var count int
	if err := store.db.QueryRowContext(context.Background(), `select count(*) from provider_cache_entries where id = ?`, entry.ID).Scan(&count); err != nil {
		t.Fatalf("count provider_cache_entries error = %v", err)
	}
	if count != 1 {
		t.Fatalf("provider_cache_entries count = %d, want 1", count)
	}

	if err := store.db.QueryRowContext(context.Background(), `select count(*) from conversation_compactions where id = ?`, compaction.ID).Scan(&count); err != nil {
		t.Fatalf("count conversation_compactions error = %v", err)
	}
	if count != 1 {
		t.Fatalf("conversation_compactions count = %d, want 1", count)
	}

	compactions, err := store.ListConversationCompactions(context.Background(), head.SessionID)
	if err != nil {
		t.Fatalf("ListConversationCompactions() error = %v", err)
	}
	if len(compactions) != 1 {
		t.Fatalf("len(compactions) = %d, want 1", len(compactions))
	}
	if compactions[0].ID != compaction.ID || compactions[0].SummaryPayload != compaction.SummaryPayload {
		t.Fatalf("compactions[0] = %#v, want %#v", compactions[0], compaction)
	}
}

func TestStoreUpsertsSessionMemory(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "agentd.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()

	createdAt := time.Date(2026, 4, 8, 9, 0, 0, 0, time.UTC)
	first := types.SessionMemory{
		SessionID:      "sess_memory",
		WorkspaceRoot:  "/workspace/demo",
		SourceTurnID:   "turn_1",
		UpToPosition:   12,
		ItemCount:      12,
		SummaryPayload: `{"range_label":"session memory","important_choices":["prefer rg"]}`,
		CreatedAt:      createdAt,
		UpdatedAt:      createdAt.Add(2 * time.Minute),
	}
	if err := store.UpsertSessionMemory(context.Background(), first); err != nil {
		t.Fatalf("UpsertSessionMemory(first) error = %v", err)
	}

	got, ok, err := store.GetSessionMemory(context.Background(), first.SessionID)
	if err != nil {
		t.Fatalf("GetSessionMemory(first) error = %v", err)
	}
	if !ok {
		t.Fatal("GetSessionMemory(first) ok = false, want true")
	}
	if got != first {
		t.Fatalf("GetSessionMemory(first) = %#v, want %#v", got, first)
	}

	second := first
	second.SourceTurnID = "turn_2"
	second.UpToPosition = 18
	second.ItemCount = 18
	second.SummaryPayload = `{"range_label":"session memory","open_threads":["verify prompt compaction"]}`
	second.UpdatedAt = createdAt.Add(5 * time.Minute)
	if err := store.UpsertSessionMemory(context.Background(), second); err != nil {
		t.Fatalf("UpsertSessionMemory(second) error = %v", err)
	}

	got, ok, err = store.GetSessionMemory(context.Background(), first.SessionID)
	if err != nil {
		t.Fatalf("GetSessionMemory(second) error = %v", err)
	}
	if !ok {
		t.Fatal("GetSessionMemory(second) ok = false, want true")
	}
	if got != second {
		t.Fatalf("GetSessionMemory(second) = %#v, want %#v", got, second)
	}

	_, ok, err = store.GetSessionMemory(context.Background(), "sess_memory_missing")
	if err != nil {
		t.Fatalf("GetSessionMemory(missing) error = %v", err)
	}
	if ok {
		t.Fatal("GetSessionMemory(missing) ok = true, want false")
	}
}

func TestStoreProviderCacheHeadsAreScopedByCapabilityProfile(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "agentd.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()

	now := time.Date(2026, 4, 4, 8, 0, 0, 0, time.UTC)

	arkHead := types.ProviderCacheHead{
		SessionID:         "sess_cache",
		Provider:          "openai_compatible",
		CapabilityProfile: "ark_responses",
		ActiveSessionRef:  "resp_ark",
		ActivePrefixRef:   "pref_ark",
		ActiveGeneration:  2,
		UpdatedAt:         now,
	}
	if err := store.UpsertProviderCacheHead(context.Background(), arkHead); err != nil {
		t.Fatalf("UpsertProviderCacheHead(ark) error = %v", err)
	}

	otherHead := types.ProviderCacheHead{
		SessionID:         "sess_cache",
		Provider:          "openai_compatible",
		CapabilityProfile: "anthropic_native",
		ActiveSessionRef:  "resp_other",
		ActivePrefixRef:   "pref_other",
		ActiveGeneration:  5,
		UpdatedAt:         now.Add(time.Minute),
	}
	if err := store.UpsertProviderCacheHead(context.Background(), otherHead); err != nil {
		t.Fatalf("UpsertProviderCacheHead(other) error = %v", err)
	}

	gotArk, ok, err := store.GetProviderCacheHead(context.Background(), arkHead.SessionID, arkHead.Provider, arkHead.CapabilityProfile)
	if err != nil {
		t.Fatalf("GetProviderCacheHead(ark) error = %v", err)
	}
	if !ok {
		t.Fatal("GetProviderCacheHead(ark) ok = false, want true")
	}
	if gotArk.ActiveSessionRef != arkHead.ActiveSessionRef || gotArk.ActivePrefixRef != arkHead.ActivePrefixRef || gotArk.ActiveGeneration != arkHead.ActiveGeneration {
		t.Fatalf("GetProviderCacheHead(ark) = %#v, want %#v", gotArk, arkHead)
	}

	gotOther, ok, err := store.GetProviderCacheHead(context.Background(), otherHead.SessionID, otherHead.Provider, otherHead.CapabilityProfile)
	if err != nil {
		t.Fatalf("GetProviderCacheHead(other) error = %v", err)
	}
	if !ok {
		t.Fatal("GetProviderCacheHead(other) ok = false, want true")
	}
	if gotOther.ActiveSessionRef != otherHead.ActiveSessionRef || gotOther.ActivePrefixRef != otherHead.ActivePrefixRef || gotOther.ActiveGeneration != otherHead.ActiveGeneration {
		t.Fatalf("GetProviderCacheHead(other) = %#v, want %#v", gotOther, otherHead)
	}

	var count int
	if err := store.db.QueryRowContext(context.Background(), `
		select count(*)
		from provider_cache_heads
		where session_id = ? and provider = ?
	`, arkHead.SessionID, arkHead.Provider).Scan(&count); err != nil {
		t.Fatalf("count provider_cache_heads error = %v", err)
	}
	if count != 2 {
		t.Fatalf("provider_cache_heads count = %d, want 2", count)
	}
}

func TestStorePersistsTurnUsage(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "agentd.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()

	first := types.TurnUsage{
		TurnID:       "turn_usage_1",
		SessionID:    "sess_usage_1",
		Model:        "glm-4.5",
		Provider:     "openai_compatible",
		InputTokens:  120,
		OutputTokens: 45,
		CachedTokens: 30,
		CacheHitRate: 0.25,
		CreatedAt:    time.Date(2026, 4, 4, 9, 0, 0, 0, time.UTC),
		UpdatedAt:    time.Date(2026, 4, 4, 9, 0, 0, 0, time.UTC),
	}
	if err := store.UpsertTurnUsage(context.Background(), first); err != nil {
		t.Fatalf("UpsertTurnUsage(first) error = %v", err)
	}

	got, ok, err := store.GetTurnUsage(context.Background(), first.TurnID)
	if err != nil {
		t.Fatalf("GetTurnUsage(first) error = %v", err)
	}
	if !ok {
		t.Fatal("GetTurnUsage(first) ok = false, want true")
	}
	if got.TurnID != first.TurnID || got.SessionID != first.SessionID || got.Provider != first.Provider || got.Model != first.Model {
		t.Fatalf("GetTurnUsage(first) identity = %#v, want %#v", got, first)
	}
	if got.InputTokens != first.InputTokens || got.OutputTokens != first.OutputTokens || got.CachedTokens != first.CachedTokens {
		t.Fatalf("GetTurnUsage(first) tokens = %#v, want %#v", got, first)
	}
	if got.CacheHitRate != first.CacheHitRate {
		t.Fatalf("GetTurnUsage(first) cache_hit_rate = %v, want %v", got.CacheHitRate, first.CacheHitRate)
	}

	second := first
	second.Provider = "anthropic"
	second.Model = "claude-4"
	second.InputTokens = 300
	second.OutputTokens = 80
	second.CachedTokens = 90
	second.CacheHitRate = 0.3
	second.UpdatedAt = first.UpdatedAt.Add(2 * time.Minute)
	if err := store.UpsertTurnUsage(context.Background(), second); err != nil {
		t.Fatalf("UpsertTurnUsage(second) error = %v", err)
	}

	got, ok, err = store.GetTurnUsage(context.Background(), first.TurnID)
	if err != nil {
		t.Fatalf("GetTurnUsage(second) error = %v", err)
	}
	if !ok {
		t.Fatal("GetTurnUsage(second) ok = false, want true")
	}
	if got.Provider != second.Provider || got.Model != second.Model {
		t.Fatalf("GetTurnUsage(second) provider/model = %#v, want %#v", got, second)
	}
	if got.InputTokens != second.InputTokens || got.OutputTokens != second.OutputTokens || got.CachedTokens != second.CachedTokens {
		t.Fatalf("GetTurnUsage(second) tokens = %#v, want %#v", got, second)
	}
	if got.CacheHitRate != second.CacheHitRate {
		t.Fatalf("GetTurnUsage(second) cache_hit_rate = %v, want %v", got.CacheHitRate, second.CacheHitRate)
	}
	if !got.UpdatedAt.Equal(second.UpdatedAt.UTC()) {
		t.Fatalf("GetTurnUsage(second) updated_at = %s, want %s", got.UpdatedAt, second.UpdatedAt.UTC())
	}

	_, ok, err = store.GetTurnUsage(context.Background(), "turn_usage_missing")
	if err != nil {
		t.Fatalf("GetTurnUsage(missing) error = %v", err)
	}
	if ok {
		t.Fatal("GetTurnUsage(missing) ok = true, want false")
	}
}

func TestStoreFinalizeTurnPersistsUsageAndEventsAtomically(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "agentd.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()

	usage := &types.TurnUsage{
		TurnID:       "turn_final_1",
		SessionID:    "sess_final_1",
		Provider:     "openai_compatible",
		Model:        "glm-4.5",
		InputTokens:  150,
		OutputTokens: 40,
		CachedTokens: 30,
		CacheHitRate: 0.2,
		CreatedAt:    time.Date(2026, 4, 4, 12, 0, 0, 0, time.UTC),
		UpdatedAt:    time.Date(2026, 4, 4, 12, 0, 0, 0, time.UTC),
	}

	assistantCompleted, err := types.NewEvent("sess_final_1", "turn_final_1", types.EventAssistantCompleted, struct{}{})
	if err != nil {
		t.Fatalf("NewEvent(assistant.completed) error = %v", err)
	}
	turnUsage, err := types.NewEvent("sess_final_1", "turn_final_1", types.EventTurnUsage, types.TurnUsagePayload{
		Provider:     usage.Provider,
		Model:        usage.Model,
		InputTokens:  usage.InputTokens,
		OutputTokens: usage.OutputTokens,
		CachedTokens: usage.CachedTokens,
		CacheHitRate: usage.CacheHitRate,
	})
	if err != nil {
		t.Fatalf("NewEvent(turn.usage) error = %v", err)
	}
	turnCompleted, err := types.NewEvent("sess_final_1", "turn_final_1", types.EventTurnCompleted, struct{}{})
	if err != nil {
		t.Fatalf("NewEvent(turn.completed) error = %v", err)
	}

	persisted, err := store.FinalizeTurn(context.Background(), usage, []types.Event{
		assistantCompleted,
		turnUsage,
		turnCompleted,
	})
	if err != nil {
		t.Fatalf("FinalizeTurn() error = %v", err)
	}
	if len(persisted) != 3 {
		t.Fatalf("len(persisted events) = %d, want 3", len(persisted))
	}
	for i := range persisted {
		wantSeq := int64(i + 1)
		if persisted[i].Seq != wantSeq {
			t.Fatalf("persisted[%d].Seq = %d, want %d", i, persisted[i].Seq, wantSeq)
		}
	}

	gotUsage, ok, err := store.GetTurnUsage(context.Background(), "turn_final_1")
	if err != nil {
		t.Fatalf("GetTurnUsage() error = %v", err)
	}
	if !ok {
		t.Fatal("GetTurnUsage() ok = false, want true")
	}
	if gotUsage.InputTokens != 150 || gotUsage.OutputTokens != 40 || gotUsage.CachedTokens != 30 {
		t.Fatalf("stored usage = %#v, want 150/40/30", gotUsage)
	}

	events, err := store.ListSessionEvents(context.Background(), "sess_final_1", 0)
	if err != nil {
		t.Fatalf("ListSessionEvents() error = %v", err)
	}
	if len(events) != 3 {
		t.Fatalf("len(stored events) = %d, want 3", len(events))
	}
	wantTypes := []string{types.EventAssistantCompleted, types.EventTurnUsage, types.EventTurnCompleted}
	for i := range wantTypes {
		if events[i].Seq != int64(i+1) {
			t.Fatalf("events[%d].Seq = %d, want %d", i, events[i].Seq, i+1)
		}
		if events[i].Type != wantTypes[i] {
			t.Fatalf("events[%d].Type = %q, want %q", i, events[i].Type, wantTypes[i])
		}
	}
}
