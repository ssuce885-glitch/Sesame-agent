package app

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"go-agent/internal/config"
	"go-agent/internal/v2/automation"
	"go-agent/internal/v2/contracts"
	"go-agent/internal/v2/memory"
	"go-agent/internal/v2/observability"
	"go-agent/internal/v2/roles"
	"go-agent/internal/v2/tasks"
)

type routes struct {
	cfg               config.Config
	store             contracts.Store
	sessionMgr        contracts.SessionManager
	taskManager       *tasks.Manager
	memoryService     *memory.Service
	metrics           *observability.Collector
	roleService       *roles.Service
	automationService *automation.Service
	projectStateAuto  projectStateAutoSetter
	defaultSessionID  string
}

type projectStateAutoSetter interface {
	SetProjectStateAutoUpdate(bool)
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
