package app

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"go-agent/internal/config"
	automationpkg "go-agent/internal/v2/automation"
	"go-agent/internal/v2/contextsvc"
	"go-agent/internal/v2/contracts"
	"go-agent/internal/v2/memory"
	"go-agent/internal/v2/observability"
	"go-agent/internal/v2/roles"
	"go-agent/internal/v2/tasks"
	"go-agent/internal/v2/workflows"
)

type routes struct {
	cfg               config.Config
	store             contracts.Store
	sessionMgr        contracts.SessionManager
	taskManager       *tasks.Manager
	memoryService     *memory.Service
	contextService    *contextsvc.Service
	metrics           *observability.Collector
	roleService       *roles.Service
	automationService *automationpkg.Service
	workflowService   workflowTriggerService
	projectStateAuto  projectStateAutoSetter
	defaultSessionID  string
}

type projectStateAutoSetter interface {
	SetProjectStateAutoUpdate(bool)
}

type workflowTriggerService interface {
	Trigger(ctx context.Context, workflow contracts.Workflow, input workflows.TriggerInput) (contracts.WorkflowRun, error)
	Resume(ctx context.Context, runID string) (contracts.WorkflowRun, error)
}

func (r *routes) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v2/sessions", r.handleSessions)
	mux.HandleFunc("GET /v2/sessions/{id}", r.handleGetSession)
	mux.HandleFunc("GET /v2/sessions/{id}/timeline", r.handleTimeline)
	mux.HandleFunc("POST /v2/turn", r.handleTurns)
	mux.HandleFunc("POST /v2/turns", r.handleTurns)
	mux.HandleFunc("POST /v2/turns/{id}/interrupt", r.handleInterruptTurn)
	mux.HandleFunc("POST /v2/tasks", r.handleCreateTask)
	mux.HandleFunc("GET /v2/tasks", r.handleListTasks)
	mux.HandleFunc("GET /v2/tasks/{id}", r.handleGetTask)
	mux.HandleFunc("GET /v2/tasks/{id}/trace", r.handleGetTaskTrace)
	mux.HandleFunc("POST /v2/tasks/{id}/cancel", r.handleCancelTask)
	mux.HandleFunc("POST /v2/memory", r.handleCreateMemory)
	mux.HandleFunc("GET /v2/memory", r.handleSearchMemory)
	mux.HandleFunc("DELETE /v2/memory/{id}", r.handleDeleteMemory)
	mux.HandleFunc("GET /v2/context/preview", r.handleContextPreview)
	mux.HandleFunc("GET /v2/context/blocks", r.handleListContextBlocks)
	mux.HandleFunc("POST /v2/context/blocks", r.handleCreateContextBlock)
	mux.HandleFunc("PUT /v2/context/blocks/{id}", r.handleUpdateContextBlock)
	mux.HandleFunc("DELETE /v2/context/blocks/{id}", r.handleDeleteContextBlock)
	mux.HandleFunc("GET /v2/workflows", r.handleListWorkflows)
	mux.HandleFunc("POST /v2/workflows", r.handleCreateWorkflow)
	mux.HandleFunc("GET /v2/workflows/{id}", r.handleGetWorkflow)
	mux.HandleFunc("PUT /v2/workflows/{id}", r.handleUpdateWorkflow)
	mux.HandleFunc("POST /v2/workflows/{id}/trigger", r.handleTriggerWorkflow)
	mux.HandleFunc("GET /v2/workflow_runs", r.handleListWorkflowRuns)
	mux.HandleFunc("POST /v2/workflow_runs", r.handleCreateWorkflowRun)
	mux.HandleFunc("GET /v2/workflow_runs/{id}", r.handleGetWorkflowRun)
	mux.HandleFunc("PUT /v2/workflow_runs/{id}", r.handleUpdateWorkflowRun)
	mux.HandleFunc("POST /v2/workflow_runs/{id}/resume", r.handleResumeWorkflowRun)
	mux.HandleFunc("GET /v2/approvals", r.handleListApprovals)
	mux.HandleFunc("POST /v2/approvals", r.handleCreateApproval)
	mux.HandleFunc("GET /v2/approvals/{id}", r.handleGetApproval)
	mux.HandleFunc("PUT /v2/approvals/{id}", r.handleUpdateApproval)
	mux.HandleFunc("GET /v2/project_state", r.handleGetProjectState)
	mux.HandleFunc("PUT /v2/project_state", r.handlePutProjectState)
	mux.HandleFunc("GET /v2/settings/{key}", r.handleGetSetting)
	mux.HandleFunc("PUT /v2/settings/{key}", r.handlePutSetting)
	mux.HandleFunc("GET /v2/roles", r.handleListRoles)
	mux.HandleFunc("POST /v2/roles", r.handleCreateRole)
	mux.HandleFunc("GET /v2/roles/{id}", r.handleGetRole)
	mux.HandleFunc("PUT /v2/roles/{id}", r.handleUpdateRole)
	mux.HandleFunc("POST /v2/automations", r.handleCreateAutomation)
	mux.HandleFunc("GET /v2/automations", r.handleListAutomations)
	mux.HandleFunc("GET /v2/automations/{id}/runs", r.handleListAutomationRuns)
	mux.HandleFunc("POST /v2/automations/{id}/pause", r.handlePauseAutomation)
	mux.HandleFunc("POST /v2/automations/{id}/resume", r.handleResumeAutomation)
	mux.HandleFunc("GET /v2/events", r.handleEvents)
	mux.HandleFunc("GET /v2/reports", r.handleReports)
	mux.HandleFunc("GET /v2/metrics", r.handleMetrics)
	mux.HandleFunc("GET /v2/status", r.handleStatus)
	if r.metrics == nil {
		return mux
	}
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		r.metrics.RecordHTTPRequest(req.URL.Path)
		mux.ServeHTTP(w, req)
	})
}

func (r *routes) handleSessions(w http.ResponseWriter, req *http.Request) {
	var body struct {
		WorkspaceRoot string `json:"workspace_root"`
	}
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		writeJSONError(w, http.StatusBadRequest, fmt.Errorf("invalid JSON: %w", err))
		return
	}
	workspaceRoot := firstNonEmpty(body.WorkspaceRoot, r.cfg.Paths.WorkspaceRoot)

	systemPrompt, err := r.cfg.ResolveSystemPrompt()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err)
		return
	}
	session, err := ensureSession(req.Context(), r.store, workspaceRoot, systemPrompt, r.cfg.PermissionProfile)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err)
		return
	}
	r.sessionMgr.Register(session)
	writeJSON(w, http.StatusOK, session)
}

func (r *routes) handleTurns(w http.ResponseWriter, req *http.Request) {
	var body struct {
		SessionID string `json:"session_id"`
		Message   string `json:"message"`
		Kind      string `json:"kind"`
	}
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		writeJSONError(w, http.StatusBadRequest, fmt.Errorf("invalid JSON: %w", err))
		return
	}
	sessionID := firstNonEmpty(body.SessionID, r.defaultSessionID)
	message := strings.TrimSpace(body.Message)
	if message == "" {
		writeJSONError(w, http.StatusBadRequest, fmt.Errorf("message is required"))
		return
	}

	now := time.Now().UTC()
	turn := contracts.Turn{
		ID:          newID("turn"),
		SessionID:   sessionID,
		Kind:        normalizeKind(body.Kind),
		State:       "created",
		UserMessage: message,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := r.store.Turns().Create(req.Context(), turn); err != nil {
		writeJSONError(w, http.StatusInternalServerError, err)
		return
	}
	submittedID, err := r.sessionMgr.SubmitTurn(req.Context(), sessionID, contracts.SubmitTurnInput{Turn: turn})
	if err != nil {
		_ = r.store.Turns().UpdateState(context.WithoutCancel(req.Context()), turn.ID, "failed")
		writeJSONError(w, http.StatusNotFound, err)
		return
	}

	turn.ID = submittedID
	if latest, err := r.store.Turns().Get(req.Context(), submittedID); err == nil {
		turn = latest
	}
	writeJSON(w, http.StatusAccepted, turn)
}

func (r *routes) handleInterruptTurn(w http.ResponseWriter, req *http.Request) {
	turnID := strings.TrimSpace(req.PathValue("id"))
	if turnID == "" {
		writeJSONError(w, http.StatusBadRequest, fmt.Errorf("turn id is required"))
		return
	}

	turn, err := r.store.Turns().Get(req.Context(), turnID)
	if err != nil {
		writeJSONError(w, http.StatusNotFound, fmt.Errorf("turn %q not found", turnID))
		return
	}
	if turn.State != "running" {
		writeJSONError(w, http.StatusConflict, fmt.Errorf("turn %q is not running", turnID))
		return
	}
	if !r.sessionMgr.CancelTurn(turn.SessionID, turn.ID) {
		writeJSONError(w, http.StatusNotFound, fmt.Errorf("turn %q not found", turnID))
		return
	}
	_ = r.store.Turns().UpdateState(req.Context(), turn.ID, "interrupted")
	// CancelTurn may immediately activate the next queued turn. Mirror the
	// manager's runtime state instead of blindly marking the session idle.
	if queue, ok := r.sessionMgr.QueuePayload(turn.SessionID); ok && queue.ActiveTurnID != "" {
		_ = r.store.Sessions().SetActiveTurn(req.Context(), turn.SessionID, queue.ActiveTurnID)
		_ = r.store.Sessions().UpdateState(req.Context(), turn.SessionID, "running")
	} else {
		_ = r.store.Sessions().SetActiveTurn(req.Context(), turn.SessionID, "")
		_ = r.store.Sessions().UpdateState(req.Context(), turn.SessionID, "idle")
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "interrupted"})
}

func (r *routes) handleEvents(w http.ResponseWriter, req *http.Request) {
	sessionID := firstNonEmpty(req.URL.Query().Get("session_id"), r.defaultSessionID)
	afterSeq, _ := strconv.ParseInt(firstNonEmpty(req.URL.Query().Get("after"), req.URL.Query().Get("after_seq")), 10, 64)
	streamEvents(req.Context(), w, r.store, sessionID, afterSeq)
}

func (r *routes) handleGetSession(w http.ResponseWriter, req *http.Request) {
	sessionID := strings.TrimSpace(req.PathValue("id"))
	if sessionID == "" {
		writeJSONError(w, http.StatusBadRequest, fmt.Errorf("session id is required"))
		return
	}
	session, err := r.store.Sessions().Get(req.Context(), sessionID)
	if err != nil {
		writeJSONError(w, http.StatusNotFound, fmt.Errorf("session %q not found", sessionID))
		return
	}
	queue, _ := r.sessionMgr.QueuePayload(sessionID)
	activeTurnID := firstNonEmpty(queue.ActiveTurnID, session.ActiveTurnID)
	writeJSON(w, http.StatusOK, map[string]any{
		"id":             session.ID,
		"workspace_root": session.WorkspaceRoot,
		"state":          session.State,
		"active_turn_id": activeTurnID,
		"created_at":     session.CreatedAt,
		"updated_at":     session.UpdatedAt,
		"queue":          queue,
	})
}

func (r *routes) handleTimeline(w http.ResponseWriter, req *http.Request) {
	sessionID := strings.TrimSpace(req.PathValue("id"))
	if sessionID == "" {
		writeJSONError(w, http.StatusBadRequest, fmt.Errorf("session id is required"))
		return
	}
	if _, err := r.store.Sessions().Get(req.Context(), sessionID); err != nil {
		writeJSONError(w, http.StatusNotFound, fmt.Errorf("session %q not found", sessionID))
		return
	}

	messages, err := r.store.Messages().List(req.Context(), sessionID, contracts.MessageListOptions{})
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err)
		return
	}
	events, err := r.store.Events().List(req.Context(), sessionID, 0, 0)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err)
		return
	}

	blocks := timelineBlocks(messages, events)
	latestSeq := int64(0)
	for _, event := range events {
		if event.Seq > latestSeq {
			latestSeq = event.Seq
		}
	}
	queue, _ := r.sessionMgr.QueuePayload(sessionID)
	reports, _ := r.store.Reports().ListBySession(req.Context(), sessionID)
	queuedReports := 0
	for _, report := range reports {
		if !report.Delivered {
			queuedReports++
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"latest_seq":          latestSeq,
		"blocks":              blocks,
		"queued_report_count": queuedReports,
		"queue":               queue,
	})
}

func (r *routes) handleCreateTask(w http.ResponseWriter, req *http.Request) {
	var body struct {
		ID              string `json:"id"`
		WorkspaceRoot   string `json:"workspace_root"`
		SessionID       string `json:"session_id"`
		RoleID          string `json:"role_id"`
		TurnID          string `json:"turn_id"`
		ParentSessionID string `json:"parent_session_id"`
		ParentTurnID    string `json:"parent_turn_id"`
		ReportSessionID string `json:"report_session_id"`
		Kind            string `json:"kind"`
		Prompt          string `json:"prompt"`
		Start           *bool  `json:"start"`
	}
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		writeJSONError(w, http.StatusBadRequest, fmt.Errorf("invalid JSON: %w", err))
		return
	}
	prompt := strings.TrimSpace(body.Prompt)
	if prompt == "" {
		writeJSONError(w, http.StatusBadRequest, fmt.Errorf("prompt is required"))
		return
	}

	now := time.Now().UTC()
	task := contracts.Task{
		ID:              strings.TrimSpace(body.ID),
		WorkspaceRoot:   firstNonEmpty(body.WorkspaceRoot, r.cfg.Paths.WorkspaceRoot),
		SessionID:       firstNonEmpty(body.SessionID, r.defaultSessionID),
		RoleID:          strings.TrimSpace(body.RoleID),
		TurnID:          strings.TrimSpace(body.TurnID),
		ParentSessionID: firstNonEmpty(body.ParentSessionID, r.defaultSessionID),
		ParentTurnID:    strings.TrimSpace(body.ParentTurnID),
		ReportSessionID: firstNonEmpty(body.ReportSessionID, body.ParentSessionID, r.defaultSessionID),
		Kind:            firstNonEmpty(body.Kind, "shell"),
		State:           "pending",
		Prompt:          prompt,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	if task.ID == "" {
		task.ID = newID("task")
	}
	if err := r.taskManager.Create(req.Context(), task); err != nil {
		writeJSONError(w, http.StatusInternalServerError, err)
		return
	}
	task, _ = r.taskManager.Get(task.ID)

	shouldStart := true
	if body.Start != nil {
		shouldStart = *body.Start
	}
	if shouldStart {
		if err := r.taskManager.Start(req.Context(), task.ID); err != nil {
			_ = r.taskManager.Fail(context.WithoutCancel(req.Context()), task.ID, "Task failed to start: "+err.Error())
			writeJSONError(w, http.StatusInternalServerError, err)
			return
		}
		task, _ = r.taskManager.Get(task.ID)
	}
	writeJSON(w, http.StatusCreated, task)
}

func (r *routes) handleListTasks(w http.ResponseWriter, req *http.Request) {
	query := req.URL.Query()
	workspaceRoot := firstNonEmpty(query.Get("workspace_root"), r.cfg.Paths.WorkspaceRoot)
	if workspaceRoot == "" {
		writeJSONError(w, http.StatusBadRequest, fmt.Errorf("workspace_root is required"))
		return
	}
	items, err := r.store.Tasks().ListByWorkspace(req.Context(), workspaceRoot)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err)
		return
	}
	for i := range items {
		if task, ok := r.taskManager.Get(items[i].ID); ok {
			items[i] = task
		}
	}
	items = filterTasks(items, query.Get("state"), query.Get("role_id"), query.Get("session_id"))
	reverseTasks(items)
	if limit := parsePositiveInt(query.Get("limit")); limit > 0 && len(items) > limit {
		items = items[:limit]
	}
	writeJSON(w, http.StatusOK, items)
}

func (r *routes) handleGetTask(w http.ResponseWriter, req *http.Request) {
	taskID := strings.TrimSpace(req.PathValue("id"))
	if taskID == "" {
		writeJSONError(w, http.StatusBadRequest, fmt.Errorf("task id is required"))
		return
	}
	if task, ok := r.taskManager.Get(taskID); ok {
		writeJSON(w, http.StatusOK, task)
		return
	}
	task, err := r.store.Tasks().Get(req.Context(), taskID)
	if err != nil {
		writeJSONError(w, http.StatusNotFound, err)
		return
	}
	writeJSON(w, http.StatusOK, task)
}

func filterTasks(items []contracts.Task, stateFilter, roleID, sessionID string) []contracts.Task {
	stateSet := splitFilterSet(stateFilter)
	roleID = strings.TrimSpace(roleID)
	sessionID = strings.TrimSpace(sessionID)
	if len(stateSet) == 0 && roleID == "" && sessionID == "" {
		return items
	}
	out := make([]contracts.Task, 0, len(items))
	for _, task := range items {
		if len(stateSet) > 0 && !stateSet[task.State] {
			continue
		}
		if roleID != "" && task.RoleID != roleID {
			continue
		}
		if sessionID != "" && task.SessionID != sessionID {
			continue
		}
		out = append(out, task)
	}
	return out
}

func splitFilterSet(raw string) map[string]bool {
	out := map[string]bool{}
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part != "" {
			out[part] = true
		}
	}
	return out
}

func reverseTasks(items []contracts.Task) {
	for i, j := 0, len(items)-1; i < j; i, j = i+1, j-1 {
		items[i], items[j] = items[j], items[i]
	}
}

func (r *routes) handleGetTaskTrace(w http.ResponseWriter, req *http.Request) {
	taskID := strings.TrimSpace(req.PathValue("id"))
	if taskID == "" {
		writeJSONError(w, http.StatusBadRequest, fmt.Errorf("task id is required"))
		return
	}
	task, ok := r.taskManager.Get(taskID)
	if !ok {
		var err error
		task, err = r.store.Tasks().Get(req.Context(), taskID)
		if err != nil {
			writeJSONError(w, http.StatusNotFound, err)
			return
		}
	}
	trace, err := tasks.BuildTrace(req.Context(), r.store, task, tasks.TraceOptions{})
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err)
		return
	}
	if queue, ok := r.sessionMgr.QueuePayload(task.SessionID); ok {
		trace.State.Queue = &queue
	}
	writeJSON(w, http.StatusOK, trace)
}

func (r *routes) handleCancelTask(w http.ResponseWriter, req *http.Request) {
	taskID := strings.TrimSpace(req.PathValue("id"))
	if taskID == "" {
		writeJSONError(w, http.StatusBadRequest, fmt.Errorf("task id is required"))
		return
	}
	if err := r.taskManager.Cancel(req.Context(), taskID); err != nil {
		writeJSONError(w, http.StatusInternalServerError, err)
		return
	}
	task, ok := r.taskManager.Get(taskID)
	if !ok {
		var err error
		task, err = r.store.Tasks().Get(req.Context(), taskID)
		if err != nil {
			writeJSONError(w, http.StatusNotFound, err)
			return
		}
	}
	writeJSON(w, http.StatusOK, task)
}

func (r *routes) handleCreateMemory(w http.ResponseWriter, req *http.Request) {
	var memory contracts.Memory
	if err := json.NewDecoder(req.Body).Decode(&memory); err != nil {
		writeJSONError(w, http.StatusBadRequest, fmt.Errorf("invalid JSON: %w", err))
		return
	}
	if strings.TrimSpace(memory.Content) == "" {
		writeJSONError(w, http.StatusBadRequest, fmt.Errorf("content is required"))
		return
	}
	now := time.Now().UTC()
	if strings.TrimSpace(memory.ID) == "" {
		memory.ID = newID("memory")
	}
	memory.WorkspaceRoot = firstNonEmpty(memory.WorkspaceRoot, r.cfg.Paths.WorkspaceRoot)
	if strings.TrimSpace(memory.Kind) == "" {
		memory.Kind = "note"
	}
	if memory.Confidence == 0 {
		memory.Confidence = 1
	}
	if memory.CreatedAt.IsZero() {
		memory.CreatedAt = now
	}
	memory.UpdatedAt = now
	if err := r.memoryService.Remember(req.Context(), memory); err != nil {
		writeJSONError(w, http.StatusInternalServerError, err)
		return
	}
	got, err := r.store.Memories().Get(req.Context(), memory.ID)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusCreated, got)
}

func (r *routes) handleSearchMemory(w http.ResponseWriter, req *http.Request) {
	workspaceRoot := firstNonEmpty(req.URL.Query().Get("workspace_root"), r.cfg.Paths.WorkspaceRoot)
	limit, _ := strconv.Atoi(req.URL.Query().Get("limit"))
	memories, err := r.memoryService.Recall(req.Context(), workspaceRoot, req.URL.Query().Get("q"), limit)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err)
		return
	}
	if memories == nil {
		memories = []contracts.Memory{}
	}
	writeJSON(w, http.StatusOK, memories)
}

func (r *routes) handleDeleteMemory(w http.ResponseWriter, req *http.Request) {
	id := strings.TrimSpace(req.PathValue("id"))
	if id == "" {
		writeJSONError(w, http.StatusBadRequest, fmt.Errorf("memory id is required"))
		return
	}
	if err := r.memoryService.Forget(req.Context(), id); err != nil {
		writeJSONError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (r *routes) handleContextPreview(w http.ResponseWriter, req *http.Request) {
	sessionID := firstNonEmpty(req.URL.Query().Get("session_id"), r.defaultSessionID)
	if strings.TrimSpace(sessionID) == "" {
		writeJSONError(w, http.StatusBadRequest, fmt.Errorf("session_id is required"))
		return
	}
	systemPrompt := ""
	if resolved, err := r.cfg.ResolveSystemPrompt(); err != nil {
		writeJSONError(w, http.StatusInternalServerError, err)
		return
	} else if strings.TrimSpace(resolved) != "" {
		systemPrompt = resolved
	}
	preview, err := r.contextSvc().Preview(req.Context(), contextsvc.PreviewInput{
		SessionID:        sessionID,
		DefaultSessionID: r.defaultSessionID,
		SystemPrompt:     systemPrompt,
	})
	if err != nil {
		writeContextServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, preview)
}

func (r *routes) handleListContextBlocks(w http.ResponseWriter, req *http.Request) {
	workspaceRoot := firstNonEmpty(req.URL.Query().Get("workspace_root"), r.cfg.Paths.WorkspaceRoot)
	if strings.TrimSpace(workspaceRoot) == "" {
		writeJSONError(w, http.StatusBadRequest, fmt.Errorf("workspace_root is required"))
		return
	}
	limit := parsePositiveInt(req.URL.Query().Get("limit"))
	blocks, err := r.contextSvc().ListBlocks(req.Context(), workspaceRoot, contracts.ContextBlockListOptions{
		Owner:      req.URL.Query().Get("owner"),
		Visibility: req.URL.Query().Get("visibility"),
		Type:       req.URL.Query().Get("type"),
		Limit:      limit,
	})
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, blocks)
}

func (r *routes) handleCreateContextBlock(w http.ResponseWriter, req *http.Request) {
	var input contextsvc.BlockInput
	if err := json.NewDecoder(req.Body).Decode(&input); err != nil {
		writeJSONError(w, http.StatusBadRequest, fmt.Errorf("invalid JSON: %w", err))
		return
	}
	got, err := r.contextSvc().CreateBlock(req.Context(), r.cfg.Paths.WorkspaceRoot, input, newID)
	if err != nil {
		writeContextServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, got)
}

func (r *routes) handleUpdateContextBlock(w http.ResponseWriter, req *http.Request) {
	id := strings.TrimSpace(req.PathValue("id"))
	if id == "" {
		writeJSONError(w, http.StatusBadRequest, fmt.Errorf("context block id is required"))
		return
	}
	var input contextsvc.BlockInput
	if err := json.NewDecoder(req.Body).Decode(&input); err != nil {
		writeJSONError(w, http.StatusBadRequest, fmt.Errorf("invalid JSON: %w", err))
		return
	}
	got, err := r.contextSvc().UpdateBlock(req.Context(), id, input)
	if err != nil {
		writeContextServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, got)
}

func (r *routes) handleDeleteContextBlock(w http.ResponseWriter, req *http.Request) {
	id := strings.TrimSpace(req.PathValue("id"))
	if id == "" {
		writeJSONError(w, http.StatusBadRequest, fmt.Errorf("context block id is required"))
		return
	}
	if err := r.contextSvc().DeleteBlock(req.Context(), id); err != nil {
		writeContextServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (r *routes) handleGetProjectState(w http.ResponseWriter, req *http.Request) {
	workspaceRoot := firstNonEmpty(req.URL.Query().Get("workspace_root"), r.cfg.Paths.WorkspaceRoot)
	state, ok, err := r.store.ProjectStates().Get(req.Context(), workspaceRoot)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err)
		return
	}
	if !ok {
		writeJSON(w, http.StatusOK, contracts.ProjectState{WorkspaceRoot: workspaceRoot})
		return
	}
	writeJSON(w, http.StatusOK, state)
}

func (r *routes) contextSvc() *contextsvc.Service {
	if r.contextService != nil {
		return r.contextService
	}
	return contextsvc.New(r.store, r.memoryService)
}

type workflowInput struct {
	ID             *string `json:"id,omitempty"`
	WorkspaceRoot  *string `json:"workspace_root,omitempty"`
	Name           *string `json:"name,omitempty"`
	Trigger        *string `json:"trigger,omitempty"`
	OwnerRole      *string `json:"owner_role,omitempty"`
	InputSchema    *string `json:"input_schema,omitempty"`
	Steps          *string `json:"steps,omitempty"`
	RequiredTools  *string `json:"required_tools,omitempty"`
	ApprovalPolicy *string `json:"approval_policy,omitempty"`
	ReportPolicy   *string `json:"report_policy,omitempty"`
	FailurePolicy  *string `json:"failure_policy,omitempty"`
	ResumePolicy   *string `json:"resume_policy,omitempty"`
}

type workflowRunInput struct {
	ID            *string `json:"id,omitempty"`
	WorkflowID    *string `json:"workflow_id,omitempty"`
	WorkspaceRoot *string `json:"workspace_root,omitempty"`
	State         *string `json:"state,omitempty"`
	TriggerRef    *string `json:"trigger_ref,omitempty"`
	TaskIDs       *string `json:"task_ids,omitempty"`
	ReportIDs     *string `json:"report_ids,omitempty"`
	ApprovalIDs   *string `json:"approval_ids,omitempty"`
	Trace         *string `json:"trace,omitempty"`
}

type workflowTriggerInput struct {
	TriggerRef *string `json:"trigger_ref,omitempty"`
}

type approvalInput struct {
	ID              *string `json:"id,omitempty"`
	WorkflowRunID   *string `json:"workflow_run_id,omitempty"`
	WorkspaceRoot   *string `json:"workspace_root,omitempty"`
	RequestedAction *string `json:"requested_action,omitempty"`
	RiskLevel       *string `json:"risk_level,omitempty"`
	Summary         *string `json:"summary,omitempty"`
	ProposedPayload *string `json:"proposed_payload,omitempty"`
	State           *string `json:"state,omitempty"`
	DecidedBy       *string `json:"decided_by,omitempty"`
	DecidedAt       *string `json:"decided_at,omitempty"`
}

func (r *routes) workflowWorkspaceRoot(req *http.Request) (string, error) {
	configured := strings.TrimSpace(r.cfg.Paths.WorkspaceRoot)
	requested := strings.TrimSpace(req.URL.Query().Get("workspace_root"))
	if configured == "" {
		if requested == "" {
			return "", fmt.Errorf("workspace_root is required")
		}
		return requested, nil
	}
	if requested != "" && requested != configured {
		return "", fmt.Errorf("workspace_root must match daemon workspace")
	}
	return configured, nil
}

func (r *routes) requireWorkflowWorkspace(w http.ResponseWriter, workspaceRoot, kind, id string) bool {
	configured := strings.TrimSpace(r.cfg.Paths.WorkspaceRoot)
	if configured == "" || strings.TrimSpace(workspaceRoot) == configured {
		return true
	}
	writeJSONError(w, http.StatusNotFound, fmt.Errorf("%s %q not found", kind, id))
	return false
}

func (r *routes) requireWorkflowEntityWorkspace(req *http.Request, w http.ResponseWriter, workspaceRoot, kind, id string) bool {
	configured := strings.TrimSpace(r.cfg.Paths.WorkspaceRoot)
	if configured != "" {
		return r.requireWorkflowWorkspace(w, workspaceRoot, kind, id)
	}
	requested := strings.TrimSpace(req.URL.Query().Get("workspace_root"))
	if requested == "" {
		writeJSONError(w, http.StatusBadRequest, fmt.Errorf("workspace_root is required"))
		return false
	}
	if requested != strings.TrimSpace(workspaceRoot) {
		writeJSONError(w, http.StatusNotFound, fmt.Errorf("%s %q not found", kind, id))
		return false
	}
	return true
}

func (r *routes) handleCreateWorkflow(w http.ResponseWriter, req *http.Request) {
	var input workflowInput
	if err := json.NewDecoder(req.Body).Decode(&input); err != nil {
		writeJSONError(w, http.StatusBadRequest, fmt.Errorf("invalid JSON: %w", err))
		return
	}
	workspaceRoot := strings.TrimSpace(r.cfg.Paths.WorkspaceRoot)
	if workspaceRoot == "" {
		writeJSONError(w, http.StatusBadRequest, fmt.Errorf("workspace_root is required"))
		return
	}
	now := time.Now().UTC()
	id := newID("workflow")
	workflow := contracts.Workflow{
		ID:            id,
		WorkspaceRoot: workspaceRoot,
		Trigger:       "manual",
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	workflow = applyWorkflowInput(workflow, input)
	workflow.ID = id
	workflow.WorkspaceRoot = workspaceRoot
	workflow.CreatedAt = now
	workflow.UpdatedAt = now
	if strings.TrimSpace(workflow.Name) == "" {
		writeJSONError(w, http.StatusBadRequest, fmt.Errorf("name is required"))
		return
	}
	if strings.TrimSpace(workflow.Trigger) == "" {
		workflow.Trigger = "manual"
	}
	if err := r.store.Workflows().Create(req.Context(), workflow); err != nil {
		writeJSONError(w, http.StatusInternalServerError, err)
		return
	}
	got, err := r.store.Workflows().Get(req.Context(), workflow.ID)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusCreated, got)
}

func (r *routes) handleListWorkflows(w http.ResponseWriter, req *http.Request) {
	workspaceRoot, err := r.workflowWorkspaceRoot(req)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err)
		return
	}
	workflows, err := r.store.Workflows().ListByWorkspace(req.Context(), workspaceRoot)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err)
		return
	}
	if workflows == nil {
		workflows = []contracts.Workflow{}
	}
	writeJSON(w, http.StatusOK, workflows)
}

func (r *routes) handleGetWorkflow(w http.ResponseWriter, req *http.Request) {
	id := strings.TrimSpace(req.PathValue("id"))
	if id == "" {
		writeJSONError(w, http.StatusBadRequest, fmt.Errorf("workflow id is required"))
		return
	}
	workflow, err := r.store.Workflows().Get(req.Context(), id)
	if errors.Is(err, sql.ErrNoRows) {
		writeJSONError(w, http.StatusNotFound, fmt.Errorf("workflow %q not found", id))
		return
	}
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err)
		return
	}
	if !r.requireWorkflowEntityWorkspace(req, w, workflow.WorkspaceRoot, "workflow", id) {
		return
	}
	writeJSON(w, http.StatusOK, workflow)
}

func (r *routes) handleUpdateWorkflow(w http.ResponseWriter, req *http.Request) {
	id := strings.TrimSpace(req.PathValue("id"))
	if id == "" {
		writeJSONError(w, http.StatusBadRequest, fmt.Errorf("workflow id is required"))
		return
	}
	existing, err := r.store.Workflows().Get(req.Context(), id)
	if errors.Is(err, sql.ErrNoRows) {
		writeJSONError(w, http.StatusNotFound, fmt.Errorf("workflow %q not found", id))
		return
	}
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err)
		return
	}
	if !r.requireWorkflowEntityWorkspace(req, w, existing.WorkspaceRoot, "workflow", id) {
		return
	}
	var input workflowInput
	if err := json.NewDecoder(req.Body).Decode(&input); err != nil {
		writeJSONError(w, http.StatusBadRequest, fmt.Errorf("invalid JSON: %w", err))
		return
	}
	workflow := applyWorkflowInput(existing, input)
	workflow.ID = existing.ID
	workflow.WorkspaceRoot = existing.WorkspaceRoot
	workflow.CreatedAt = existing.CreatedAt
	workflow.UpdatedAt = time.Now().UTC()
	if strings.TrimSpace(workflow.Name) == "" {
		writeJSONError(w, http.StatusBadRequest, fmt.Errorf("name is required"))
		return
	}
	if strings.TrimSpace(workflow.Trigger) == "" {
		workflow.Trigger = "manual"
	}
	if err := r.store.Workflows().Update(req.Context(), workflow); err != nil {
		writeJSONError(w, http.StatusInternalServerError, err)
		return
	}
	got, err := r.store.Workflows().Get(req.Context(), id)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, got)
}

func (r *routes) handleCreateWorkflowRun(w http.ResponseWriter, req *http.Request) {
	var input workflowRunInput
	if err := json.NewDecoder(req.Body).Decode(&input); err != nil {
		writeJSONError(w, http.StatusBadRequest, fmt.Errorf("invalid JSON: %w", err))
		return
	}
	workflowID := stringPtrValue(input.WorkflowID)
	if strings.TrimSpace(workflowID) == "" {
		writeJSONError(w, http.StatusBadRequest, fmt.Errorf("workflow_id is required"))
		return
	}
	workflow, err := r.store.Workflows().Get(req.Context(), workflowID)
	if errors.Is(err, sql.ErrNoRows) {
		writeJSONError(w, http.StatusNotFound, fmt.Errorf("workflow %q not found", workflowID))
		return
	}
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err)
		return
	}
	if !r.requireWorkflowEntityWorkspace(req, w, workflow.WorkspaceRoot, "workflow", workflowID) {
		return
	}
	now := time.Now().UTC()
	id := newID("wfrun")
	run := contracts.WorkflowRun{
		ID:            id,
		WorkflowID:    workflow.ID,
		WorkspaceRoot: workflow.WorkspaceRoot,
		State:         "queued",
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	run = applyWorkflowRunInput(run, input)
	state, err := workflowRunStateFromInput(input.State, "queued")
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err)
		return
	}
	run.ID = id
	run.WorkflowID = workflow.ID
	run.WorkspaceRoot = workflow.WorkspaceRoot
	run.State = state
	run.CreatedAt = now
	run.UpdatedAt = now
	if err := r.store.Workflows().CreateRun(req.Context(), run); err != nil {
		writeJSONError(w, http.StatusInternalServerError, err)
		return
	}
	got, err := r.store.Workflows().GetRun(req.Context(), run.ID)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusCreated, got)
}

func (r *routes) handleTriggerWorkflow(w http.ResponseWriter, req *http.Request) {
	id := strings.TrimSpace(req.PathValue("id"))
	if id == "" {
		writeJSONError(w, http.StatusBadRequest, fmt.Errorf("workflow id is required"))
		return
	}
	workflow, err := r.store.Workflows().Get(req.Context(), id)
	if errors.Is(err, sql.ErrNoRows) {
		writeJSONError(w, http.StatusNotFound, fmt.Errorf("workflow %q not found", id))
		return
	}
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err)
		return
	}
	if !r.requireWorkflowEntityWorkspace(req, w, workflow.WorkspaceRoot, "workflow", id) {
		return
	}
	if r.workflowService == nil {
		writeJSONError(w, http.StatusInternalServerError, workflows.ErrUnavailable)
		return
	}

	var input workflowTriggerInput
	if err := json.NewDecoder(req.Body).Decode(&input); err != nil && !errors.Is(err, io.EOF) {
		writeJSONError(w, http.StatusBadRequest, fmt.Errorf("invalid JSON: %w", err))
		return
	}

	run, err := r.workflowService.Trigger(context.WithoutCancel(req.Context()), workflow, workflows.TriggerInput{
		TriggerRef: stringPtrValue(input.TriggerRef),
	})
	switch {
	case errors.Is(err, workflows.ErrInvalidWorkflow):
		writeJSONError(w, http.StatusBadRequest, err)
		return
	case errors.Is(err, workflows.ErrUnavailable):
		writeJSONError(w, http.StatusInternalServerError, err)
		return
	case err != nil:
		writeJSONError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, workflowTriggerStatusCode(run.State), run)
}

func (r *routes) handleListWorkflowRuns(w http.ResponseWriter, req *http.Request) {
	workspaceRoot, err := r.workflowWorkspaceRoot(req)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err)
		return
	}
	runs, err := r.store.Workflows().ListRunsByWorkspace(req.Context(), workspaceRoot, contracts.WorkflowRunListOptions{
		WorkflowID: req.URL.Query().Get("workflow_id"),
		State:      req.URL.Query().Get("state"),
		Limit:      parsePositiveInt(req.URL.Query().Get("limit")),
	})
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err)
		return
	}
	if runs == nil {
		runs = []contracts.WorkflowRun{}
	}
	writeJSON(w, http.StatusOK, runs)
}

func (r *routes) handleGetWorkflowRun(w http.ResponseWriter, req *http.Request) {
	id := strings.TrimSpace(req.PathValue("id"))
	if id == "" {
		writeJSONError(w, http.StatusBadRequest, fmt.Errorf("workflow run id is required"))
		return
	}
	run, err := r.store.Workflows().GetRun(req.Context(), id)
	if errors.Is(err, sql.ErrNoRows) {
		writeJSONError(w, http.StatusNotFound, fmt.Errorf("workflow run %q not found", id))
		return
	}
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err)
		return
	}
	if !r.requireWorkflowEntityWorkspace(req, w, run.WorkspaceRoot, "workflow run", id) {
		return
	}
	writeJSON(w, http.StatusOK, run)
}

func (r *routes) handleResumeWorkflowRun(w http.ResponseWriter, req *http.Request) {
	id := strings.TrimSpace(req.PathValue("id"))
	if id == "" {
		writeJSONError(w, http.StatusBadRequest, fmt.Errorf("workflow run id is required"))
		return
	}
	existing, err := r.store.Workflows().GetRun(req.Context(), id)
	if errors.Is(err, sql.ErrNoRows) {
		writeJSONError(w, http.StatusNotFound, fmt.Errorf("workflow run %q not found", id))
		return
	}
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err)
		return
	}
	if !r.requireWorkflowEntityWorkspace(req, w, existing.WorkspaceRoot, "workflow run", id) {
		return
	}
	if r.workflowService == nil {
		writeJSONError(w, http.StatusInternalServerError, workflows.ErrUnavailable)
		return
	}

	run, err := r.workflowService.Resume(context.WithoutCancel(req.Context()), id)
	switch {
	case errors.Is(err, workflows.ErrInvalidWorkflow):
		writeJSONError(w, http.StatusBadRequest, err)
		return
	case errors.Is(err, sql.ErrNoRows):
		writeJSONError(w, http.StatusNotFound, fmt.Errorf("workflow run %q not found", id))
		return
	case errors.Is(err, workflows.ErrUnavailable):
		writeJSONError(w, http.StatusInternalServerError, err)
		return
	case err != nil:
		writeJSONError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, run)
}

func (r *routes) handleUpdateWorkflowRun(w http.ResponseWriter, req *http.Request) {
	id := strings.TrimSpace(req.PathValue("id"))
	if id == "" {
		writeJSONError(w, http.StatusBadRequest, fmt.Errorf("workflow run id is required"))
		return
	}
	existing, err := r.store.Workflows().GetRun(req.Context(), id)
	if errors.Is(err, sql.ErrNoRows) {
		writeJSONError(w, http.StatusNotFound, fmt.Errorf("workflow run %q not found", id))
		return
	}
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err)
		return
	}
	if !r.requireWorkflowEntityWorkspace(req, w, existing.WorkspaceRoot, "workflow run", id) {
		return
	}
	var input workflowRunInput
	if err := json.NewDecoder(req.Body).Decode(&input); err != nil {
		writeJSONError(w, http.StatusBadRequest, fmt.Errorf("invalid JSON: %w", err))
		return
	}
	run := applyWorkflowRunInput(existing, input)
	run.ID = existing.ID
	run.WorkflowID = existing.WorkflowID
	run.WorkspaceRoot = existing.WorkspaceRoot
	run.CreatedAt = existing.CreatedAt
	run.UpdatedAt = time.Now().UTC()
	if input.State != nil {
		state, err := workflowRunStateFromInput(input.State, existing.State)
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, err)
			return
		}
		run.State = state
	}
	if err := r.store.Workflows().UpdateRun(req.Context(), run); err != nil {
		writeJSONError(w, http.StatusInternalServerError, err)
		return
	}
	got, err := r.store.Workflows().GetRun(req.Context(), id)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, got)
}

func (r *routes) handleListApprovals(w http.ResponseWriter, req *http.Request) {
	workspaceRoot, err := r.workflowWorkspaceRoot(req)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err)
		return
	}
	stateFilter := strings.TrimSpace(req.URL.Query().Get("state"))
	if stateFilter != "" {
		if stateFilter, err = approvalStateFromInput(&stateFilter, ""); err != nil {
			writeJSONError(w, http.StatusBadRequest, err)
			return
		}
	}
	approvals, err := r.store.Workflows().ListApprovalsByWorkspace(req.Context(), workspaceRoot, contracts.ApprovalListOptions{
		WorkflowRunID: req.URL.Query().Get("workflow_run_id"),
		State:         stateFilter,
		Limit:         parsePositiveInt(req.URL.Query().Get("limit")),
	})
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err)
		return
	}
	if approvals == nil {
		approvals = []contracts.Approval{}
	}
	writeJSON(w, http.StatusOK, approvals)
}

func (r *routes) handleCreateApproval(w http.ResponseWriter, req *http.Request) {
	var input approvalInput
	if err := json.NewDecoder(req.Body).Decode(&input); err != nil {
		writeJSONError(w, http.StatusBadRequest, fmt.Errorf("invalid JSON: %w", err))
		return
	}
	workflowRunID := stringPtrValue(input.WorkflowRunID)
	if workflowRunID == "" {
		writeJSONError(w, http.StatusBadRequest, fmt.Errorf("workflow_run_id is required"))
		return
	}
	run, err := r.store.Workflows().GetRun(req.Context(), workflowRunID)
	if errors.Is(err, sql.ErrNoRows) {
		writeJSONError(w, http.StatusNotFound, fmt.Errorf("workflow run %q not found", workflowRunID))
		return
	}
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err)
		return
	}
	if !r.requireWorkflowEntityWorkspace(req, w, run.WorkspaceRoot, "workflow run", workflowRunID) {
		return
	}

	now := time.Now().UTC()
	id := newID("approval")
	state, err := approvalStateFromInput(input.State, "pending")
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err)
		return
	}
	if state != "pending" {
		writeJSONError(w, http.StatusBadRequest, fmt.Errorf("approval state must be pending on create"))
		return
	}
	approval := contracts.Approval{
		ID:            id,
		WorkflowRunID: run.ID,
		WorkspaceRoot: run.WorkspaceRoot,
		State:         state,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	approval = applyApprovalInput(approval, input)
	approval.ID = id
	approval.WorkflowRunID = run.ID
	approval.WorkspaceRoot = run.WorkspaceRoot
	approval.State = state
	approval.DecidedBy = ""
	approval.DecidedAt = time.Time{}
	approval.CreatedAt = now
	approval.UpdatedAt = now
	if strings.TrimSpace(approval.RequestedAction) == "" {
		writeJSONError(w, http.StatusBadRequest, fmt.Errorf("requested_action is required"))
		return
	}
	if input.DecidedAt != nil && strings.TrimSpace(*input.DecidedAt) != "" {
		writeJSONError(w, http.StatusBadRequest, fmt.Errorf("pending approvals cannot include decided_at"))
		return
	}
	if err := r.store.Workflows().CreateApproval(req.Context(), approval); err != nil {
		writeJSONError(w, http.StatusInternalServerError, err)
		return
	}
	got, err := r.store.Workflows().GetApproval(req.Context(), approval.ID)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusCreated, got)
}

func (r *routes) handleGetApproval(w http.ResponseWriter, req *http.Request) {
	id := strings.TrimSpace(req.PathValue("id"))
	if id == "" {
		writeJSONError(w, http.StatusBadRequest, fmt.Errorf("approval id is required"))
		return
	}
	approval, err := r.store.Workflows().GetApproval(req.Context(), id)
	if errors.Is(err, sql.ErrNoRows) {
		writeJSONError(w, http.StatusNotFound, fmt.Errorf("approval %q not found", id))
		return
	}
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err)
		return
	}
	if !r.requireWorkflowEntityWorkspace(req, w, approval.WorkspaceRoot, "approval", id) {
		return
	}
	writeJSON(w, http.StatusOK, approval)
}

func (r *routes) handleUpdateApproval(w http.ResponseWriter, req *http.Request) {
	id := strings.TrimSpace(req.PathValue("id"))
	if id == "" {
		writeJSONError(w, http.StatusBadRequest, fmt.Errorf("approval id is required"))
		return
	}
	existing, err := r.store.Workflows().GetApproval(req.Context(), id)
	if errors.Is(err, sql.ErrNoRows) {
		writeJSONError(w, http.StatusNotFound, fmt.Errorf("approval %q not found", id))
		return
	}
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err)
		return
	}
	if !r.requireWorkflowEntityWorkspace(req, w, existing.WorkspaceRoot, "approval", id) {
		return
	}
	var input approvalInput
	if err := json.NewDecoder(req.Body).Decode(&input); err != nil {
		writeJSONError(w, http.StatusBadRequest, fmt.Errorf("invalid JSON: %w", err))
		return
	}
	if approvalStateIsTerminal(existing.State) {
		mutated, err := approvalTerminalMutationRequested(existing, input)
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, err)
			return
		}
		if mutated {
			writeJSONError(w, http.StatusBadRequest, fmt.Errorf("approval %q is already %s and cannot be modified", id, existing.State))
			return
		}
		writeJSON(w, http.StatusOK, existing)
		return
	}

	approval, err := updatedApprovalFromInput(existing, input, time.Now().UTC())
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err)
		return
	}
	if strings.TrimSpace(approval.RequestedAction) == "" {
		writeJSONError(w, http.StatusBadRequest, fmt.Errorf("requested_action is required"))
		return
	}
	if err := r.store.WithTx(req.Context(), func(tx contracts.Store) error {
		if err := tx.Workflows().UpdateApproval(req.Context(), approval); err != nil {
			return err
		}
		return finalizeWorkflowRunAfterApprovalDecision(req.Context(), tx, approval)
	}); err != nil {
		writeJSONError(w, http.StatusInternalServerError, err)
		return
	}
	got, err := r.store.Workflows().GetApproval(req.Context(), id)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, got)
}

func (r *routes) handlePutProjectState(w http.ResponseWriter, req *http.Request) {
	var body struct {
		WorkspaceRoot   string `json:"workspace_root"`
		Summary         string `json:"summary"`
		SourceSessionID string `json:"source_session_id"`
		SourceTurnID    string `json:"source_turn_id"`
	}
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		writeJSONError(w, http.StatusBadRequest, fmt.Errorf("invalid JSON: %w", err))
		return
	}
	workspaceRoot := firstNonEmpty(body.WorkspaceRoot, r.cfg.Paths.WorkspaceRoot)
	if strings.TrimSpace(workspaceRoot) == "" {
		writeJSONError(w, http.StatusBadRequest, fmt.Errorf("workspace_root is required"))
		return
	}
	now := time.Now().UTC()
	state := contracts.ProjectState{
		WorkspaceRoot:   workspaceRoot,
		Summary:         strings.TrimSpace(body.Summary),
		SourceSessionID: strings.TrimSpace(body.SourceSessionID),
		SourceTurnID:    strings.TrimSpace(body.SourceTurnID),
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	if err := r.store.ProjectStates().Upsert(req.Context(), state); err != nil {
		writeJSONError(w, http.StatusInternalServerError, err)
		return
	}
	got, _, err := r.store.ProjectStates().Get(req.Context(), workspaceRoot)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, got)
}

func (r *routes) handleGetSetting(w http.ResponseWriter, req *http.Request) {
	key := strings.TrimSpace(req.PathValue("key"))
	if key == "" {
		writeJSONError(w, http.StatusBadRequest, fmt.Errorf("setting key is required"))
		return
	}
	value, err := r.store.Settings().Get(req.Context(), key)
	if errors.Is(err, sql.ErrNoRows) {
		if defaultValue, ok := defaultSettingValue(key); ok {
			writeJSON(w, http.StatusOK, map[string]string{"key": key, "value": defaultValue})
			return
		}
		writeJSONError(w, http.StatusNotFound, fmt.Errorf("setting %q not found", key))
		return
	}
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"key": key, "value": value})
}

func (r *routes) handlePutSetting(w http.ResponseWriter, req *http.Request) {
	key := strings.TrimSpace(req.PathValue("key"))
	if key == "" {
		writeJSONError(w, http.StatusBadRequest, fmt.Errorf("setting key is required"))
		return
	}
	var body struct {
		Value string `json:"value"`
	}
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		writeJSONError(w, http.StatusBadRequest, fmt.Errorf("invalid JSON: %w", err))
		return
	}
	value := strings.TrimSpace(body.Value)
	if key == "project_state_auto" {
		normalized, err := normalizeProjectStateAutoValue(value)
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, err)
			return
		}
		value = normalized
	}
	if err := r.store.Settings().Set(req.Context(), key, value); err != nil {
		writeJSONError(w, http.StatusInternalServerError, err)
		return
	}
	if key == "project_state_auto" && r.projectStateAuto != nil {
		r.projectStateAuto.SetProjectStateAutoUpdate(value == "true")
	}
	writeJSON(w, http.StatusOK, map[string]string{"key": key, "value": value})
}

func (r *routes) handleListRoles(w http.ResponseWriter, req *http.Request) {
	specs, err := r.roleService.List()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err)
		return
	}
	if specs == nil {
		specs = []roles.RoleSpec{}
	}
	writeJSON(w, http.StatusOK, specs)
}

func (r *routes) handleGetRole(w http.ResponseWriter, req *http.Request) {
	id := strings.TrimSpace(req.PathValue("id"))
	if id == "" {
		writeJSONError(w, http.StatusBadRequest, fmt.Errorf("role id is required"))
		return
	}
	spec, ok, err := r.roleService.Get(id)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err)
		return
	}
	if !ok {
		writeJSONError(w, http.StatusNotFound, fmt.Errorf("role %q not found", id))
		return
	}
	writeJSON(w, http.StatusOK, spec)
}

func (r *routes) handleCreateRole(w http.ResponseWriter, req *http.Request) {
	var input roles.SaveInput
	if err := json.NewDecoder(req.Body).Decode(&input); err != nil {
		writeJSONError(w, http.StatusBadRequest, fmt.Errorf("invalid JSON: %w", err))
		return
	}
	spec, err := r.roleService.Create(req.Context(), input)
	if err != nil {
		writeRoleServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, spec)
}

func (r *routes) handleUpdateRole(w http.ResponseWriter, req *http.Request) {
	id := strings.TrimSpace(req.PathValue("id"))
	if id == "" {
		writeJSONError(w, http.StatusBadRequest, fmt.Errorf("role id is required"))
		return
	}
	var input roles.SaveInput
	if err := json.NewDecoder(req.Body).Decode(&input); err != nil {
		writeJSONError(w, http.StatusBadRequest, fmt.Errorf("invalid JSON: %w", err))
		return
	}
	spec, err := r.roleService.Update(req.Context(), id, input)
	if err != nil {
		writeRoleServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, spec)
}

func (r *routes) handleCreateAutomation(w http.ResponseWriter, req *http.Request) {
	var automation contracts.Automation
	if err := json.NewDecoder(req.Body).Decode(&automation); err != nil {
		writeJSONError(w, http.StatusBadRequest, fmt.Errorf("invalid JSON: %w", err))
		return
	}
	if strings.TrimSpace(automation.WatcherPath) == "" {
		writeJSONError(w, http.StatusBadRequest, fmt.Errorf("watcher_path is required"))
		return
	}
	now := time.Now().UTC()
	if strings.TrimSpace(automation.ID) == "" {
		automation.ID = newID("automation")
	}
	automation.WorkspaceRoot = firstNonEmpty(automation.WorkspaceRoot, r.cfg.Paths.WorkspaceRoot)
	if strings.TrimSpace(automation.State) == "" {
		automation.State = "active"
	}
	if automation.CreatedAt.IsZero() {
		automation.CreatedAt = now
	}
	automation.UpdatedAt = now
	if err := r.automationService.Create(req.Context(), automation); err != nil {
		if automationpkg.IsValidationError(err) {
			writeJSONError(w, http.StatusBadRequest, err)
			return
		}
		writeJSONError(w, http.StatusInternalServerError, err)
		return
	}
	got, err := r.store.Automations().Get(req.Context(), automation.ID)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusCreated, got)
}

func (r *routes) handleListAutomations(w http.ResponseWriter, req *http.Request) {
	workspaceRoot := firstNonEmpty(req.URL.Query().Get("workspace_root"), r.cfg.Paths.WorkspaceRoot)
	automations, err := r.store.Automations().ListByWorkspace(req.Context(), workspaceRoot)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err)
		return
	}
	if automations == nil {
		automations = []contracts.Automation{}
	}
	writeJSON(w, http.StatusOK, automations)
}

func (r *routes) handleListAutomationRuns(w http.ResponseWriter, req *http.Request) {
	id := strings.TrimSpace(req.PathValue("id"))
	if id == "" {
		writeJSONError(w, http.StatusBadRequest, fmt.Errorf("automation id is required"))
		return
	}
	if _, err := r.store.Automations().Get(req.Context(), id); err != nil {
		writeJSONError(w, http.StatusNotFound, err)
		return
	}
	limit := parsePositiveInt(req.URL.Query().Get("limit"))
	if limit <= 0 {
		limit = 20
	}
	runs, err := r.store.Automations().ListRunsByAutomation(req.Context(), id, limit)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err)
		return
	}
	if runs == nil {
		runs = []contracts.AutomationRun{}
	}
	writeJSON(w, http.StatusOK, runs)
}

func (r *routes) handlePauseAutomation(w http.ResponseWriter, req *http.Request) {
	id := strings.TrimSpace(req.PathValue("id"))
	if id == "" {
		writeJSONError(w, http.StatusBadRequest, fmt.Errorf("automation id is required"))
		return
	}
	if err := r.automationService.Pause(req.Context(), id); err != nil {
		writeJSONError(w, http.StatusInternalServerError, err)
		return
	}
	automation, err := r.store.Automations().Get(req.Context(), id)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, automation)
}

func (r *routes) handleResumeAutomation(w http.ResponseWriter, req *http.Request) {
	id := strings.TrimSpace(req.PathValue("id"))
	if id == "" {
		writeJSONError(w, http.StatusBadRequest, fmt.Errorf("automation id is required"))
		return
	}
	if err := r.automationService.Resume(req.Context(), id); err != nil {
		if automationpkg.IsValidationError(err) {
			writeJSONError(w, http.StatusBadRequest, err)
			return
		}
		writeJSONError(w, http.StatusInternalServerError, err)
		return
	}
	automation, err := r.store.Automations().Get(req.Context(), id)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, automation)
}

func (r *routes) handleReports(w http.ResponseWriter, req *http.Request) {
	workspaceRoot := firstNonEmpty(req.URL.Query().Get("workspace_root"), r.cfg.Paths.WorkspaceRoot)
	sessions, err := r.store.Sessions().ListByWorkspace(req.Context(), workspaceRoot)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err)
		return
	}

	items := make([]map[string]any, 0)
	queuedCount := 0
	for _, session := range sessions {
		reports, err := r.store.Reports().ListBySession(req.Context(), session.ID)
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, err)
			return
		}
		for _, report := range reports {
			if !report.Delivered {
				queuedCount++
			}
			items = append(items, map[string]any{
				"id":          report.ID,
				"session_id":  report.SessionID,
				"source_kind": report.SourceKind,
				"source_id":   report.SourceID,
				"title":       report.Title,
				"summary":     report.Summary,
				"severity":    report.Severity,
				"status":      report.Status,
				"delivered":   report.Delivered,
				"created_at":  report.CreatedAt,
			})
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items":        items,
		"queued_count": queuedCount,
	})
}

func (r *routes) handleStatus(w http.ResponseWriter, req *http.Request) {
	queue, _ := r.sessionMgr.QueuePayload(r.defaultSessionID)
	writeJSON(w, http.StatusOK, map[string]any{
		"status":             "ok",
		"addr":               r.cfg.Addr,
		"model":              r.cfg.Model,
		"permission_profile": r.cfg.PermissionProfile,
		"default_session_id": r.defaultSessionID,
		"queue":              queue,
	})
}

func (r *routes) handleMetrics(w http.ResponseWriter, req *http.Request) {
	if r.metrics == nil {
		writeJSON(w, http.StatusOK, observability.MetricsSnapshot{})
		return
	}
	writeJSON(w, http.StatusOK, r.metrics.Snapshot())
}

func ensureSession(ctx context.Context, s contracts.Store, workspaceRoot, systemPrompt, permissionProfile string) (contracts.Session, error) {
	sessions, err := s.Sessions().ListByWorkspace(ctx, workspaceRoot)
	if err != nil {
		return contracts.Session{}, err
	}
	if len(sessions) > 0 {
		return sessions[0], nil
	}
	now := time.Now().UTC()
	session := contracts.Session{
		ID:                newID("session"),
		WorkspaceRoot:     workspaceRoot,
		SystemPrompt:      systemPrompt,
		PermissionProfile: permissionProfile,
		State:             "idle",
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	if err := s.Sessions().Create(ctx, session); err != nil {
		return contracts.Session{}, err
	}
	return session, nil
}

func streamEvents(ctx context.Context, w http.ResponseWriter, s contracts.Store, sessionID string, afterSeq int64) {
	setupSSE(w)
	flusher, _ := w.(http.Flusher)
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		nextSeq, _ := writeAvailableEvents(ctx, w, s, sessionID, afterSeq)
		if nextSeq > afterSeq {
			afterSeq = nextSeq
			if flusher != nil {
				flusher.Flush()
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func writeAvailableEvents(ctx context.Context, w http.ResponseWriter, s contracts.Store, sessionID string, afterSeq int64) (int64, error) {
	events, err := s.Events().List(ctx, sessionID, afterSeq, 100)
	if err != nil {
		return afterSeq, err
	}
	for _, event := range events {
		fmt.Fprintf(w, "id: %d\n", event.Seq)
		fmt.Fprintf(w, "event: %s\n", event.Type)
		fmt.Fprintf(w, "data: %s\n\n", event.Payload)
		afterSeq = event.Seq
	}
	return afterSeq, nil
}

func setupSSE(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
}

type timelineBlock struct {
	Kind    string            `json:"kind"`
	Text    string            `json:"text,omitempty"`
	Title   string            `json:"title,omitempty"`
	Status  string            `json:"status,omitempty"`
	Content []timelineContent `json:"content,omitempty"`
}

type timelineContent struct {
	Type          string `json:"type"`
	Text          string `json:"text,omitempty"`
	ToolName      string `json:"tool_name,omitempty"`
	ArgsPreview   string `json:"args_preview,omitempty"`
	ResultPreview string `json:"result_preview,omitempty"`
	ToolCallID    string `json:"tool_call_id,omitempty"`
	Status        string `json:"status,omitempty"`
}

type encodedTimelineToolCall struct {
	Name string         `json:"name"`
	Args map[string]any `json:"args"`
}

func timelineBlocks(messages []contracts.Message, events []contracts.Event) []timelineBlock {
	blocks := make([]timelineBlock, 0, len(messages)+len(events))
	toolNames := make(map[string]string)
	lastAssistant := -1

	for _, msg := range messages {
		switch msg.Role {
		case "user":
			if strings.TrimSpace(msg.Content) != "" {
				blocks = append(blocks, timelineBlock{Kind: "user_message", Text: msg.Content})
			}
			lastAssistant = -1
		case "assistant":
			if strings.HasPrefix(msg.Content, "__thinking_json__:") {
				continue
			}
			if call, ok := decodeTimelineToolCall(msg); ok {
				if lastAssistant < 0 || lastAssistant >= len(blocks) || blocks[lastAssistant].Kind != "assistant_message" {
					blocks = append(blocks, timelineBlock{Kind: "assistant_message"})
					lastAssistant = len(blocks) - 1
				}
				toolNames[msg.ToolCallID] = call.Name
				blocks[lastAssistant].Content = append(blocks[lastAssistant].Content, timelineContent{
					Type:        "tool_call",
					ToolName:    call.Name,
					ArgsPreview: previewJSON(call.Args),
					ToolCallID:  msg.ToolCallID,
					Status:      "running",
				})
				continue
			}
			if strings.TrimSpace(msg.Content) == "" {
				continue
			}
			blocks = append(blocks, timelineBlock{
				Kind: "assistant_message",
				Content: []timelineContent{{
					Type: "text",
					Text: msg.Content,
				}},
			})
			lastAssistant = len(blocks) - 1
		case "tool":
			if lastAssistant < 0 || lastAssistant >= len(blocks) || blocks[lastAssistant].Kind != "assistant_message" {
				blocks = append(blocks, timelineBlock{Kind: "assistant_message"})
				lastAssistant = len(blocks) - 1
			}
			blocks[lastAssistant].Content = append(blocks[lastAssistant].Content, timelineContent{
				Type:          "tool_result",
				ToolName:      toolNames[msg.ToolCallID],
				ResultPreview: previewText(msg.Content),
				ToolCallID:    msg.ToolCallID,
				Status:        "ok",
			})
			markTimelineToolCompleted(&blocks[lastAssistant], msg.ToolCallID)
		}
	}

	for _, event := range events {
		if block, ok := eventNoticeBlock(event); ok {
			blocks = append(blocks, block)
		}
	}
	return blocks
}

func decodeTimelineToolCall(msg contracts.Message) (encodedTimelineToolCall, bool) {
	const prefix = "__tool_call_json__:"
	if strings.TrimSpace(msg.ToolCallID) == "" || !strings.HasPrefix(msg.Content, prefix) {
		return encodedTimelineToolCall{}, false
	}
	var call encodedTimelineToolCall
	if err := json.Unmarshal([]byte(strings.TrimPrefix(msg.Content, prefix)), &call); err != nil {
		return encodedTimelineToolCall{}, false
	}
	if strings.TrimSpace(call.Name) == "" {
		return encodedTimelineToolCall{}, false
	}
	if call.Args == nil {
		call.Args = map[string]any{}
	}
	return call, true
}

func markTimelineToolCompleted(block *timelineBlock, toolCallID string) {
	if block == nil || strings.TrimSpace(toolCallID) == "" {
		return
	}
	for i := range block.Content {
		if block.Content[i].Type == "tool_call" && block.Content[i].ToolCallID == toolCallID {
			block.Content[i].Status = "completed"
			return
		}
	}
}

func eventNoticeBlock(event contracts.Event) (timelineBlock, bool) {
	switch event.Type {
	case "turn_started", "turn.started":
		return timelineBlock{Kind: "notice", Title: "Turn started", Text: "Turn started"}, true
	case "turn_completed", "turn.completed":
		return timelineBlock{Kind: "notice", Title: "Turn completed", Text: "Turn completed"}, true
	case "turn_failed", "turn.failed":
		return timelineBlock{Kind: "notice", Title: "Turn failed", Text: eventText(event, "Turn failed"), Status: "failed"}, true
	case "turn_interrupted", "turn.interrupted":
		return timelineBlock{Kind: "notice", Title: "Turn interrupted", Text: "Turn interrupted", Status: "interrupted"}, true
	case "task_result_ready", "task.result_ready":
		return timelineBlock{Kind: "task_result_ready", Text: eventText(event, "Task result ready")}, true
	case "context_compacted":
		return timelineBlock{Kind: "notice", Title: "Context compacted", Text: eventText(event, "Context compacted")}, true
	case "context_microcompacted":
		return timelineBlock{Kind: "notice", Title: "Context trimmed", Text: eventText(event, "Context trimmed")}, true
	default:
		return timelineBlock{}, false
	}
}

func eventText(event contracts.Event, fallback string) string {
	var payload map[string]any
	if err := json.Unmarshal([]byte(event.Payload), &payload); err != nil {
		return fallback
	}
	for _, key := range []string{"text", "message", "error", "summary"} {
		if value, ok := payload[key].(string); ok && strings.TrimSpace(value) != "" {
			return value
		}
	}
	return fallback
}

func previewJSON(value any) string {
	raw, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	return previewText(string(raw))
}

func previewText(text string) string {
	text = strings.TrimSpace(text)
	const maxRunes = 240
	runes := []rune(text)
	if len(runes) <= maxRunes {
		return text
	}
	return string(runes[:maxRunes]) + "..."
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeJSONError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]string{"error": err.Error()})
}

func writeContextServiceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, contextsvc.ErrNotFound), errors.Is(err, sql.ErrNoRows):
		writeJSONError(w, http.StatusNotFound, err)
	case errors.Is(err, contextsvc.ErrInvalidInput):
		writeJSONError(w, http.StatusBadRequest, err)
	default:
		writeJSONError(w, http.StatusInternalServerError, err)
	}
}

func writeRoleServiceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, roles.ErrInvalidRole):
		writeJSONError(w, http.StatusBadRequest, err)
	case errors.Is(err, roles.ErrRoleExists):
		writeJSONError(w, http.StatusConflict, err)
	case errors.Is(err, roles.ErrRoleNotFound):
		writeJSONError(w, http.StatusNotFound, err)
	default:
		writeJSONError(w, http.StatusInternalServerError, err)
	}
}

func applyWorkflowInput(workflow contracts.Workflow, input workflowInput) contracts.Workflow {
	if input.ID != nil {
		workflow.ID = strings.TrimSpace(*input.ID)
	}
	if input.WorkspaceRoot != nil {
		workflow.WorkspaceRoot = strings.TrimSpace(*input.WorkspaceRoot)
	}
	if input.Name != nil {
		workflow.Name = strings.TrimSpace(*input.Name)
	}
	if input.Trigger != nil {
		workflow.Trigger = strings.TrimSpace(*input.Trigger)
	}
	if input.OwnerRole != nil {
		workflow.OwnerRole = strings.TrimSpace(*input.OwnerRole)
	}
	if input.InputSchema != nil {
		workflow.InputSchema = strings.TrimSpace(*input.InputSchema)
	}
	if input.Steps != nil {
		workflow.Steps = strings.TrimSpace(*input.Steps)
	}
	if input.RequiredTools != nil {
		workflow.RequiredTools = strings.TrimSpace(*input.RequiredTools)
	}
	if input.ApprovalPolicy != nil {
		workflow.ApprovalPolicy = strings.TrimSpace(*input.ApprovalPolicy)
	}
	if input.ReportPolicy != nil {
		workflow.ReportPolicy = strings.TrimSpace(*input.ReportPolicy)
	}
	if input.FailurePolicy != nil {
		workflow.FailurePolicy = strings.TrimSpace(*input.FailurePolicy)
	}
	if input.ResumePolicy != nil {
		workflow.ResumePolicy = strings.TrimSpace(*input.ResumePolicy)
	}
	return workflow
}

func applyWorkflowRunInput(run contracts.WorkflowRun, input workflowRunInput) contracts.WorkflowRun {
	if input.ID != nil {
		run.ID = strings.TrimSpace(*input.ID)
	}
	if input.WorkflowID != nil {
		run.WorkflowID = strings.TrimSpace(*input.WorkflowID)
	}
	if input.WorkspaceRoot != nil {
		run.WorkspaceRoot = strings.TrimSpace(*input.WorkspaceRoot)
	}
	if input.State != nil {
		run.State = strings.TrimSpace(*input.State)
	}
	if input.TriggerRef != nil {
		run.TriggerRef = strings.TrimSpace(*input.TriggerRef)
	}
	if input.TaskIDs != nil {
		run.TaskIDs = strings.TrimSpace(*input.TaskIDs)
	}
	if input.ReportIDs != nil {
		run.ReportIDs = strings.TrimSpace(*input.ReportIDs)
	}
	if input.ApprovalIDs != nil {
		run.ApprovalIDs = strings.TrimSpace(*input.ApprovalIDs)
	}
	if input.Trace != nil {
		run.Trace = strings.TrimSpace(*input.Trace)
	}
	return run
}

func applyApprovalInput(approval contracts.Approval, input approvalInput) contracts.Approval {
	if input.ID != nil {
		approval.ID = strings.TrimSpace(*input.ID)
	}
	if input.WorkflowRunID != nil {
		approval.WorkflowRunID = strings.TrimSpace(*input.WorkflowRunID)
	}
	if input.WorkspaceRoot != nil {
		approval.WorkspaceRoot = strings.TrimSpace(*input.WorkspaceRoot)
	}
	if input.RequestedAction != nil {
		approval.RequestedAction = strings.TrimSpace(*input.RequestedAction)
	}
	if input.RiskLevel != nil {
		approval.RiskLevel = strings.TrimSpace(*input.RiskLevel)
	}
	if input.Summary != nil {
		approval.Summary = strings.TrimSpace(*input.Summary)
	}
	if input.ProposedPayload != nil {
		approval.ProposedPayload = strings.TrimSpace(*input.ProposedPayload)
	}
	if input.State != nil {
		approval.State = strings.TrimSpace(*input.State)
	}
	if input.DecidedBy != nil {
		approval.DecidedBy = strings.TrimSpace(*input.DecidedBy)
	}
	return approval
}

func updatedApprovalFromInput(existing contracts.Approval, input approvalInput, now time.Time) (contracts.Approval, error) {
	approval := applyApprovalInput(existing, input)
	approval.ID = existing.ID
	approval.WorkflowRunID = existing.WorkflowRunID
	approval.WorkspaceRoot = existing.WorkspaceRoot
	approval.CreatedAt = existing.CreatedAt
	approval.UpdatedAt = now

	state, err := approvalStateFromInput(input.State, existing.State)
	if err != nil {
		return contracts.Approval{}, err
	}
	if existing.State != "pending" {
		return contracts.Approval{}, fmt.Errorf("approval %q cannot transition from %s", existing.ID, existing.State)
	}
	switch state {
	case "pending":
		if input.DecidedAt != nil && strings.TrimSpace(*input.DecidedAt) != "" {
			return contracts.Approval{}, fmt.Errorf("pending approvals cannot include decided_at")
		}
		approval.State = "pending"
		approval.DecidedBy = ""
		approval.DecidedAt = time.Time{}
	case "approved", "rejected", "expired":
		approval.State = state
		approval.DecidedBy = strings.TrimSpace(approval.DecidedBy)
		if approval.DecidedBy == "" {
			return contracts.Approval{}, fmt.Errorf("decided_by is required for terminal approvals")
		}
		decidedAt, err := approvalTimeFromInput(input.DecidedAt, existing.DecidedAt)
		if err != nil {
			return contracts.Approval{}, err
		}
		if decidedAt.IsZero() {
			decidedAt = now
		}
		approval.DecidedAt = decidedAt
	default:
		return contracts.Approval{}, fmt.Errorf("approval %q cannot transition from %s to %s", existing.ID, existing.State, state)
	}
	return approval, nil
}

func approvalTerminalMutationRequested(existing contracts.Approval, input approvalInput) (bool, error) {
	if input.State != nil {
		state, err := approvalStateFromInput(input.State, existing.State)
		if err != nil {
			return false, err
		}
		if state != existing.State {
			return true, nil
		}
	}
	if input.RequestedAction != nil && strings.TrimSpace(*input.RequestedAction) != existing.RequestedAction {
		return true, nil
	}
	if input.RiskLevel != nil && strings.TrimSpace(*input.RiskLevel) != existing.RiskLevel {
		return true, nil
	}
	if input.Summary != nil && strings.TrimSpace(*input.Summary) != existing.Summary {
		return true, nil
	}
	if input.ProposedPayload != nil && strings.TrimSpace(*input.ProposedPayload) != existing.ProposedPayload {
		return true, nil
	}
	if input.DecidedBy != nil && strings.TrimSpace(*input.DecidedBy) != existing.DecidedBy {
		return true, nil
	}
	if input.DecidedAt != nil {
		decidedAt, err := approvalTimeFromInput(input.DecidedAt, existing.DecidedAt)
		if err != nil {
			return false, err
		}
		if !approvalTimesEqual(decidedAt, existing.DecidedAt) {
			return true, nil
		}
	}
	return false, nil
}

func approvalStateIsTerminal(state string) bool {
	switch strings.TrimSpace(state) {
	case "approved", "rejected", "expired":
		return true
	default:
		return false
	}
}

func approvalTimesEqual(left, right time.Time) bool {
	if left.IsZero() || right.IsZero() {
		return left.IsZero() && right.IsZero()
	}
	return left.Equal(right)
}

func finalizeWorkflowRunAfterApprovalDecision(ctx context.Context, store contracts.Store, approval contracts.Approval) error {
	if store == nil || !approvalStateIsTerminal(approval.State) || strings.TrimSpace(approval.WorkflowRunID) == "" {
		return nil
	}

	run, err := store.Workflows().GetRun(ctx, approval.WorkflowRunID)
	if err != nil {
		return err
	}
	if strings.TrimSpace(run.State) != "waiting_approval" {
		return nil
	}

	run.Trace = appendWorkflowRunTraceEvent(run.Trace, newWorkflowRunTraceEvent("approval_resolved", map[string]any{
		"approval_id": approval.ID,
		"state":       approval.State,
		"message":     approvalResolutionMessage(approval.State),
	}))
	if strings.TrimSpace(approval.State) == "approved" {
		run.UpdatedAt = time.Now().UTC()
		return store.Workflows().UpdateRun(ctx, run)
	}

	run.State = "failed"
	run.Trace = appendWorkflowRunTraceEvent(run.Trace, newWorkflowRunTraceEvent("run_failed", map[string]any{
		"approval_id": approval.ID,
		"state":       run.State,
		"message":     workflowRunApprovalClosureMessage(approval.State, run.State),
	}))
	run.UpdatedAt = time.Now().UTC()
	return store.Workflows().UpdateRun(ctx, run)
}

func approvalResolutionMessage(state string) string {
	switch strings.TrimSpace(state) {
	case "approved":
		return "Approval approved."
	case "rejected":
		return "Approval rejected."
	case "expired":
		return "Approval expired."
	default:
		return "Approval resolved."
	}
}

func workflowRunApprovalClosureMessage(approvalState, runState string) string {
	switch strings.TrimSpace(approvalState) {
	case "approved":
		return "Approval approved; workflow run closed without automatic post-approval resume."
	case "rejected":
		return "Approval rejected; workflow run closed at the approval gate."
	case "expired":
		return "Approval expired; workflow run closed at the approval gate."
	default:
		return fmt.Sprintf("Approval resolved; workflow run marked %s.", strings.TrimSpace(runState))
	}
}

func workflowRunStateFromInput(value *string, fallback string) (string, error) {
	if value == nil {
		return fallback, nil
	}
	state := strings.TrimSpace(*value)
	if state == "" {
		return "", fmt.Errorf("state is required")
	}
	switch state {
	case "queued", "running", "waiting_approval", "completed", "failed", "interrupted":
		return state, nil
	default:
		return "", fmt.Errorf("unsupported workflow run state %q", state)
	}
}

func approvalStateFromInput(value *string, fallback string) (string, error) {
	if value == nil {
		return fallback, nil
	}
	state := strings.TrimSpace(*value)
	if state == "" {
		return "", fmt.Errorf("state is required")
	}
	switch state {
	case "pending", "approved", "rejected", "expired":
		return state, nil
	default:
		return "", fmt.Errorf("unsupported approval state %q", state)
	}
}

func approvalTimeFromInput(value *string, fallback time.Time) (time.Time, error) {
	if value == nil {
		return fallback, nil
	}
	raw := strings.TrimSpace(*value)
	if raw == "" {
		return time.Time{}, nil
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02 15:04:05"} {
		if parsed, err := time.Parse(layout, raw); err == nil {
			return parsed.UTC(), nil
		}
	}
	return time.Time{}, fmt.Errorf("decided_at must be RFC3339 timestamp")
}

func workflowTriggerStatusCode(state string) int {
	switch strings.TrimSpace(state) {
	case "completed":
		return http.StatusCreated
	case "failed", "interrupted":
		return http.StatusOK
	case "queued", "running", "waiting_approval":
		return http.StatusAccepted
	default:
		return http.StatusAccepted
	}
}

func stringPtrValue(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}

func normalizeKind(kind string) string {
	if kind == "report_batch" {
		return "report_batch"
	}
	return "user_message"
}

func defaultSettingValue(key string) (string, bool) {
	switch key {
	case "project_state_auto":
		return "true", true
	default:
		return "", false
	}
}

func normalizeProjectStateAutoValue(value string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "true", "on", "1", "enabled", "enable":
		return "true", nil
	case "false", "off", "0", "disabled", "disable":
		return "false", nil
	default:
		return "", fmt.Errorf("project_state_auto must be true or false")
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func parsePositiveInt(raw string) int {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return 0
	}
	return value
}

func newID(prefix string) string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())
	}
	return prefix + "-" + hex.EncodeToString(b[:])
}
