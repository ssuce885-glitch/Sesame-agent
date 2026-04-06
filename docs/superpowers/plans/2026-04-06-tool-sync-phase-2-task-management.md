# Tool Sync Phase 2 Task Management Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement Phase 2 task management for `go-agent` with persistent todos, persistent task records, and real `shell`, `agent`, and `remote` task runners.

**Architecture:** Keep long-lived task execution inside `internal/task` as a central manager with pluggable runners, while `internal/tools` stays thin and delegates task lifecycle operations through `ExecContext.TaskManager`. Reuse existing daemon and engine wiring by injecting the manager once at runtime and adapting agent execution behind a narrow interface to avoid package cycles.

**Tech Stack:** Go standard library, existing `internal/task`, `internal/tools`, `internal/config`, `internal/engine`, `cmd/agentd`, Windows `cmd /c` process execution, Go testing package.

---

### Task 1: Extend Config and Task Scaffolding

**Files:**
- Create: `internal/task/types.go`
- Create: `internal/task/store.go`
- Create: `internal/task/runner.go`
- Modify: `internal/config/config.go`
- Modify: `internal/config/config_test.go`

- [x] **Step 1: Write failing config tests for Phase 2 settings**

```go
func TestLoadUsesDefaultsAndRequiresDataDir(t *testing.T) {
	t.Run("uses defaults when data dir is set", func(t *testing.T) {
		t.Setenv("AGENTD_DATA_DIR", t.TempDir())
		t.Setenv("AGENTD_MAX_CONCURRENT_TASKS", "")
		t.Setenv("AGENTD_TASK_OUTPUT_MAX_BYTES", "")
		t.Setenv("AGENTD_REMOTE_EXECUTOR_SHIM_COMMAND", "")
		t.Setenv("AGENTD_REMOTE_EXECUTOR_TIMEOUT_SECONDS", "")

		cfg, err := Load()
		if err != nil {
			t.Fatalf("Load() returned error: %v", err)
		}

		if cfg.MaxConcurrentTasks != 8 {
			t.Fatalf("MaxConcurrentTasks = %d, want %d", cfg.MaxConcurrentTasks, 8)
		}
		if cfg.TaskOutputMaxBytes != 1<<20 {
			t.Fatalf("TaskOutputMaxBytes = %d, want %d", cfg.TaskOutputMaxBytes, 1<<20)
		}
		if cfg.RemoteExecutorShimCommand != "" {
			t.Fatalf("RemoteExecutorShimCommand = %q, want empty", cfg.RemoteExecutorShimCommand)
		}
		if cfg.RemoteExecutorTimeoutSeconds != 300 {
			t.Fatalf("RemoteExecutorTimeoutSeconds = %d, want %d", cfg.RemoteExecutorTimeoutSeconds, 300)
		}
	})
}

func TestLoadReadsTaskExecutionOverrides(t *testing.T) {
	t.Setenv("AGENTD_DATA_DIR", t.TempDir())
	t.Setenv("AGENTD_MAX_CONCURRENT_TASKS", "3")
	t.Setenv("AGENTD_TASK_OUTPUT_MAX_BYTES", "8192")
	t.Setenv("AGENTD_REMOTE_EXECUTOR_SHIM_COMMAND", "ssh deploy@example")
	t.Setenv("AGENTD_REMOTE_EXECUTOR_TIMEOUT_SECONDS", "45")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}

	if cfg.MaxConcurrentTasks != 3 {
		t.Fatalf("MaxConcurrentTasks = %d, want %d", cfg.MaxConcurrentTasks, 3)
	}
	if cfg.TaskOutputMaxBytes != 8192 {
		t.Fatalf("TaskOutputMaxBytes = %d, want %d", cfg.TaskOutputMaxBytes, 8192)
	}
	if cfg.RemoteExecutorShimCommand != "ssh deploy@example" {
		t.Fatalf("RemoteExecutorShimCommand = %q, want %q", cfg.RemoteExecutorShimCommand, "ssh deploy@example")
	}
	if cfg.RemoteExecutorTimeoutSeconds != 45 {
		t.Fatalf("RemoteExecutorTimeoutSeconds = %d, want %d", cfg.RemoteExecutorTimeoutSeconds, 45)
	}
}
```

- [x] **Step 2: Run config tests to verify the new settings are missing**

Run: `go test ./internal/config -run 'TestLoad(UsesDefaultsAndRequiresDataDir|ReadsTaskExecutionOverrides)'`

Expected: FAIL with missing `Config` fields and missing environment parsing for task settings.

- [x] **Step 3: Add Phase 2 config fields and task package scaffolding**

```go
type Config struct {
	Addr                        string
	DataDir                     string
	ModelProvider               string
	Model                       string
	AnthropicAPIKey             string
	AnthropicBaseURL            string
	OpenAIAPIKey                string
	OpenAIBaseURL               string
	ProviderCacheProfile        string
	CacheExpirySeconds          int
	MicrocompactBytesThreshold  int
	LogLevel                    string
	PermissionProfile           string
	MaxToolSteps                int
	MaxShellOutputBytes         int
	ShellTimeoutSeconds         int
	MaxFileWriteBytes           int
	MaxRecentItems              int
	CompactionThreshold         int
	MaxEstimatedTokens          int
	MaxCompactionPasses         int
	SystemPrompt                string
	SystemPromptFile            string
	MaxWorkspacePromptBytes     int
	MaxConcurrentTasks          int
	TaskOutputMaxBytes          int
	RemoteExecutorShimCommand   string
	RemoteExecutorTimeoutSeconds int
}
```

```go
cfg := Config{
	Addr:                        envOrDefaultWithFallback("AGENTD_ADDR", uc.Listen.Addr, "127.0.0.1:4317"),
	DataDir:                     envOrDefault("AGENTD_DATA_DIR", ""),
	ModelProvider:               envOrDefaultWithFallback("AGENTD_MODEL_PROVIDER", uc.Provider, "anthropic"),
	Model:                       model,
	AnthropicAPIKey:             envOrDefaultWithFallback("ANTHROPIC_API_KEY", uc.Anthropic.APIKey, ""),
	AnthropicBaseURL:            envOrDefaultWithFallback("ANTHROPIC_BASE_URL", uc.Anthropic.BaseURL, "https://api.anthropic.com"),
	OpenAIAPIKey:                envOrDefaultWithFallback("OPENAI_API_KEY", uc.OpenAI.APIKey, ""),
	OpenAIBaseURL:               envOrDefaultWithFallback("OPENAI_BASE_URL", uc.OpenAI.BaseURL, ""),
	ProviderCacheProfile:        envOrDefault("AGENTD_PROVIDER_CACHE_PROFILE", "none"),
	CacheExpirySeconds:          intEnvOrDefault("AGENTD_CACHE_EXPIRY_SECONDS", 86400),
	MicrocompactBytesThreshold:  intEnvOrDefault("AGENTD_MICROCOMPACT_BYTES_THRESHOLD", 4096),
	LogLevel:                    envOrDefault("AGENTD_LOG_LEVEL", "info"),
	PermissionProfile:           envOrDefault("AGENTD_PERMISSION_PROFILE", "read_only"),
	MaxToolSteps:                intEnvOrDefaultWithFallback("AGENTD_MAX_TOOL_STEPS", uc.MaxToolSteps, 8),
	MaxShellOutputBytes:         intEnvOrDefault("AGENTD_MAX_SHELL_OUTPUT_BYTES", 4096),
	ShellTimeoutSeconds:         intEnvOrDefault("AGENTD_SHELL_TIMEOUT_SECONDS", 30),
	MaxFileWriteBytes:           intEnvOrDefault("AGENTD_MAX_FILE_WRITE_BYTES", 1<<20),
	MaxRecentItems:              intEnvOrDefault("AGENTD_MAX_RECENT_ITEMS", 8),
	CompactionThreshold:         intEnvOrDefault("AGENTD_COMPACTION_THRESHOLD", 16),
	MaxEstimatedTokens:          intEnvOrDefault("AGENTD_MAX_ESTIMATED_TOKENS", 6000),
	MaxCompactionPasses:         intEnvOrDefault("AGENTD_MAX_COMPACTION_PASSES", 1),
	SystemPrompt:                envOrDefault("AGENTD_SYSTEM_PROMPT", ""),
	SystemPromptFile:            envOrDefault("AGENTD_SYSTEM_PROMPT_FILE", ""),
	MaxWorkspacePromptBytes:     intEnvOrDefault("AGENTD_MAX_WORKSPACE_PROMPT_BYTES", 32768),
	MaxConcurrentTasks:           intEnvOrDefault("AGENTD_MAX_CONCURRENT_TASKS", 8),
	TaskOutputMaxBytes:           intEnvOrDefault("AGENTD_TASK_OUTPUT_MAX_BYTES", 1<<20),
	RemoteExecutorShimCommand:    envOrDefault("AGENTD_REMOTE_EXECUTOR_SHIM_COMMAND", ""),
	RemoteExecutorTimeoutSeconds: intEnvOrDefault("AGENTD_REMOTE_EXECUTOR_TIMEOUT_SECONDS", 300),
}
```

```go
type TaskType string

const (
	TaskTypeShell  TaskType = "shell"
	TaskTypeAgent  TaskType = "agent"
	TaskTypeRemote TaskType = "remote"
)

type TaskStatus string

const (
	TaskStatusPending   TaskStatus = "pending"
	TaskStatusRunning   TaskStatus = "running"
	TaskStatusCompleted TaskStatus = "completed"
	TaskStatusFailed    TaskStatus = "failed"
	TaskStatusStopped   TaskStatus = "stopped"
)

type Task struct {
	ID            string     `json:"id"`
	Type          TaskType   `json:"type"`
	Status        TaskStatus `json:"status"`
	Command       string     `json:"command"`
	Description   string     `json:"description,omitempty"`
	WorkspaceRoot string     `json:"workspace_root"`
	Output        string     `json:"output,omitempty"`
	OutputPath    string     `json:"output_path,omitempty"`
	Error         string     `json:"error,omitempty"`
	StartTime     time.Time  `json:"start_time"`
	EndTime       *time.Time `json:"end_time,omitempty"`
}

type TodoItem struct {
	Content    string `json:"content"`
	Status     string `json:"status"`
	ActiveForm string `json:"activeForm,omitempty"`
}

type CreateTaskInput struct {
	Type          TaskType
	Command       string
	Description   string
	WorkspaceRoot string
	Start         bool
}

type UpdateTaskInput struct {
	Status      TaskStatus
	Description string
}

func isTerminalStatus(status TaskStatus) bool {
	switch status {
	case TaskStatusCompleted, TaskStatusFailed, TaskStatusStopped:
		return true
	default:
		return false
	}
}

func validateStatusTransition(from, to TaskStatus) error {
	if from == to {
		return nil
	}
	allowed := map[TaskStatus]map[TaskStatus]struct{}{
		TaskStatusPending: {
			TaskStatusRunning: {},
			TaskStatusStopped: {},
		},
		TaskStatusRunning: {
			TaskStatusCompleted: {},
			TaskStatusFailed:    {},
			TaskStatusStopped:   {},
		},
	}
	if _, ok := allowed[from][to]; ok {
		return nil
	}
	return fmt.Errorf("invalid status transition from %q to %q", from, to)
}
```

```go
type OutputSink interface {
	Append(taskID string, chunk []byte) error
}

type Runner interface {
	Run(ctx context.Context, task *Task, sink OutputSink) error
}

type AgentExecutor interface {
	RunTask(ctx context.Context, workspaceRoot string, prompt string, sink io.Writer) error
}

type RemoteExecutorConfig struct {
	ShimCommand    string
	TimeoutSeconds int
}
```

```go
type tasksFilePayload struct {
	Tasks []Task `json:"tasks"`
}

func loadTasksFile(path string) ([]Task, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	var payload tasksFilePayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, err
	}
	return payload.Tasks, nil
}

func writeTasksFile(path string, tasks []Task) error {
	payload := tasksFilePayload{Tasks: tasks}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

func writeTodosFile(path string, todos []TodoItem) error {
	data, err := json.MarshalIndent(todos, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}
```

- [x] **Step 4: Re-run the config tests**

Run: `go test ./internal/config -run 'TestLoad(UsesDefaultsAndRequiresDataDir|ReadsTaskExecutionOverrides)'`

Expected: PASS

- [x] **Step 5: Commit the config and scaffolding changes**

```bash
git add internal/config/config.go internal/config/config_test.go internal/task/types.go internal/task/store.go internal/task/runner.go
git commit -m "feat: add phase 2 task configuration scaffolding"
```

### Task 2: Build Persistent Task Manager Lifecycle

**Files:**
- Modify: `internal/task/manager.go`
- Modify: `internal/task/task_test.go`
- Delete: `internal/task/session.go`
- Delete: `internal/task/events.go`

- [x] **Step 1: Write failing manager tests for persistence, lookup, and restart recovery**

```go
func TestManagerCreateListGetAndUpdateTask(t *testing.T) {
	root := t.TempDir()
	manager := NewManager(Config{MaxConcurrentTasks: 8, TaskOutputMaxBytes: 1 << 20}, nil, nil)

	task, err := manager.Create(context.Background(), CreateTaskInput{
		Type:          TaskTypeShell,
		Command:       "echo hello",
		Description:   "run echo",
		WorkspaceRoot: root,
		Start:         false,
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if task.Status != TaskStatusPending {
		t.Fatalf("Status = %q, want %q", task.Status, TaskStatusPending)
	}

	listed, err := manager.List(root)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(listed) != 1 || listed[0].ID != task.ID {
		t.Fatalf("List() = %#v, want task %q", listed, task.ID)
	}

	got, ok, err := manager.Get(task.ID, root)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if !ok || got.ID != task.ID {
		t.Fatalf("Get() = %#v, %v, want task %q", got, ok, task.ID)
	}

	if err := manager.Update(task.ID, root, UpdateTaskInput{Status: TaskStatusCompleted, Description: "done"}); err != nil {
		t.Fatalf("Update() error = %v", err)
	}
}

func TestManagerReloadMarksRunningTasksFailed(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	raw := `{"tasks":[{"id":"task_1","type":"shell","status":"running","command":"echo hi","workspace_root":"` + filepath.ToSlash(root) + `","start_time":"2026-04-06T10:00:00Z"}]}`
	if err := os.WriteFile(filepath.Join(root, ".claude", "tasks.json"), []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}

	manager := NewManager(Config{MaxConcurrentTasks: 8, TaskOutputMaxBytes: 1 << 20}, nil, nil)

	task, ok, err := manager.Get("task_1", root)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if !ok {
		t.Fatal("Get() ok = false, want true")
	}
	if task.Status != TaskStatusFailed {
		t.Fatalf("Status = %q, want %q", task.Status, TaskStatusFailed)
	}
	if !strings.Contains(task.Error, "process restart") {
		t.Fatalf("Error = %q, want restart marker", task.Error)
	}
}
```

- [x] **Step 2: Run task manager tests and verify the manager API is still missing**

Run: `go test ./internal/task -run 'TestManager(CreateListGetAndUpdateTask|ReloadMarksRunningTasksFailed)'`

Expected: FAIL because the placeholder manager still has the old constructor shape and is missing the lifecycle APIs (`List`, `Get`, `Update`, workspace loading).

- [x] **Step 3: Replace the placeholder manager with a persistent manager**

```go
type runningTask struct {
	workspaceRoot string
	cancel context.CancelFunc
	done   chan struct{}
}

type workspaceState struct {
	tasksFile  string
	todosFile  string
	outputsDir string
	loaded     bool
}

type Config struct {
	MaxConcurrentTasks int
	TaskOutputMaxBytes int
}

type Manager struct {
	mu         sync.RWMutex
	tasks      map[string]*Task
	workspaces map[string]*workspaceState
	running    map[string]*runningTask
	runners    map[TaskType]Runner
	cfg        Config
	remote     RemoteExecutorConfig
}
```

```go
func NewManager(cfg Config, runners map[TaskType]Runner, agentExecutor AgentExecutor) *Manager {
	m := &Manager{
		tasks:      make(map[string]*Task),
		workspaces: make(map[string]*workspaceState),
		running:    make(map[string]*runningTask),
		runners:    make(map[TaskType]Runner),
		cfg:        cfg,
	}
	m.registerDefaultRunners(agentExecutor)
	for taskType, runner := range runners {
		m.runners[taskType] = runner
	}
	return m
}
```

```go
func (m *Manager) ensureWorkspaceLocked(workspaceRoot string) (*workspaceState, error) {
	state, ok := m.workspaces[workspaceRoot]
	if !ok {
		claudeDir := filepath.Join(workspaceRoot, ".claude")
		outputsDir := filepath.Join(claudeDir, "tasks")
		if err := os.MkdirAll(outputsDir, 0o755); err != nil {
			return nil, err
		}
		state = &workspaceState{
			tasksFile:  filepath.Join(claudeDir, "tasks.json"),
			todosFile:  filepath.Join(claudeDir, "todos.json"),
			outputsDir: outputsDir,
		}
		m.workspaces[workspaceRoot] = state
	}
	if state.loaded {
		return state, nil
	}

	persisted, err := loadTasksFile(state.tasksFile)
	if err != nil {
		return nil, err
	}
	needsSave := false
	for _, persistedTask := range persisted {
		taskCopy := persistedTask
		if taskCopy.Status == TaskStatusRunning {
			taskCopy.Status = TaskStatusFailed
			taskCopy.Error = "task interrupted by process restart"
			now := time.Now().UTC()
			taskCopy.EndTime = &now
			needsSave = true
		}
		m.tasks[taskCopy.ID] = &taskCopy
	}
	state.loaded = true
	if needsSave {
		return state, m.saveWorkspaceLocked(workspaceRoot)
	}
	return state, nil
}

func (m *Manager) saveWorkspaceLocked(workspaceRoot string) error {
	state := m.workspaces[workspaceRoot]
	tasks := make([]Task, 0)
	for _, task := range m.tasks {
		if task.WorkspaceRoot == workspaceRoot {
			tasks = append(tasks, *task)
		}
	}
	sort.Slice(tasks, func(i, j int) bool {
		return tasks[i].StartTime.Before(tasks[j].StartTime)
	})
	return writeTasksFile(state.tasksFile, tasks)
}

func (m *Manager) Get(taskID, workspaceRoot string) (Task, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, err := m.ensureWorkspaceLocked(workspaceRoot); err != nil {
		return Task{}, false, err
	}
	task, ok := m.tasks[taskID]
	if !ok || task.WorkspaceRoot != workspaceRoot {
		return Task{}, false, nil
	}
	return *task, true, nil
}
```

```go
func (m *Manager) Update(taskID, workspaceRoot string, in UpdateTaskInput) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, err := m.ensureWorkspaceLocked(workspaceRoot); err != nil {
		return err
	}
	task, ok := m.tasks[taskID]
	if !ok || task.WorkspaceRoot != workspaceRoot {
		return fmt.Errorf("task %q not found", taskID)
	}
	if err := validateStatusTransition(task.Status, in.Status); err != nil {
		return err
	}
	task.Status = in.Status
	if in.Description != "" {
		task.Description = in.Description
	}
	if isTerminalStatus(task.Status) {
		now := time.Now().UTC()
		task.EndTime = &now
	}
	return m.saveWorkspaceLocked(workspaceRoot)
}
```

- [x] **Step 4: Re-run the manager tests**

Run: `go test ./internal/task -run 'TestManager(CreateListGetAndUpdateTask|ReloadMarksRunningTasksFailed)'`

Expected: PASS

- [x] **Step 5: Commit the manager lifecycle changes**

```bash
git add internal/task/manager.go internal/task/task_test.go internal/task/session.go internal/task/events.go
git commit -m "feat: add persistent task manager lifecycle"
```

### Task 3: Implement Shell Execution and Output Streaming

**Files:**
- Create: `internal/task/shell_runner.go`
- Modify: `internal/task/manager.go`
- Modify: `internal/task/task_test.go`

- [x] **Step 1: Write failing tests for shell task execution, failure, output, and stop**

```go
func TestManagerRunsShellTaskAndCapturesOutput(t *testing.T) {
	root := t.TempDir()
	manager := NewManager(Config{MaxConcurrentTasks: 8, TaskOutputMaxBytes: 1 << 20}, nil, nil)

	task, err := manager.Create(context.Background(), CreateTaskInput{
		Type:          TaskTypeShell,
		Command:       "echo shell-task",
		WorkspaceRoot: root,
		Start:         true,
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	waitForTaskTerminal(t, manager, task.ID, root)
	got, ok, err := manager.Get(task.ID, root)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if !ok {
		t.Fatal("Get() ok = false, want true")
	}
	if got.Status != TaskStatusCompleted {
		t.Fatalf("Status = %q, want %q", got.Status, TaskStatusCompleted)
	}
	output, err := manager.ReadOutput(task.ID, root)
	if err != nil {
		t.Fatalf("ReadOutput() error = %v", err)
	}
	if !strings.Contains(output, "shell-task") {
		t.Fatalf("output = %q, want shell-task", output)
	}
}

func TestManagerStopsRunningShellTask(t *testing.T) {
	root := t.TempDir()
	manager := NewManager(Config{MaxConcurrentTasks: 8, TaskOutputMaxBytes: 1 << 20}, nil, nil)

	task, err := manager.Create(context.Background(), CreateTaskInput{
		Type:          TaskTypeShell,
		Command:       "ping -n 6 127.0.0.1 > nul",
		WorkspaceRoot: root,
		Start:         true,
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if err := manager.Stop(task.ID, root); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	waitForTaskTerminal(t, manager, task.ID, root)

	got, ok, err := manager.Get(task.ID, root)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if !ok {
		t.Fatal("Get() ok = false, want true")
	}
	if got.Status != TaskStatusStopped {
		t.Fatalf("Status = %q, want %q", got.Status, TaskStatusStopped)
	}
}

func waitForTaskTerminal(t *testing.T, manager *Manager, taskID, workspaceRoot string) {
	t.Helper()

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		got, ok, err := manager.Get(taskID, workspaceRoot)
		if err != nil {
			t.Fatalf("Get() error = %v", err)
		}
		if ok && isTerminalStatus(got.Status) {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}

	t.Fatalf("task %q did not reach terminal state", taskID)
}
```

- [x] **Step 2: Run shell-focused task tests**

Run: `go test ./internal/task -run 'TestManager(RunsShellTaskAndCapturesOutput|StopsRunningShellTask)'`

Expected: FAIL because no shell runner is registered and `ReadOutput` / `Stop` are incomplete.

- [x] **Step 3: Add the shell runner and asynchronous manager execution path**

```go
type ShellRunner struct{}

func (ShellRunner) Run(ctx context.Context, task *Task, sink OutputSink) error {
	cmd := exec.CommandContext(ctx, "cmd", "/c", task.Command)
	cmd.Dir = task.WorkspaceRoot

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}

	var wg sync.WaitGroup
	streamPipe := func(r io.Reader) {
		defer wg.Done()
		buf := make([]byte, 4096)
		for {
			n, readErr := r.Read(buf)
			if n > 0 {
				_ = sink.Append(task.ID, buf[:n])
			}
			if readErr != nil {
				return
			}
		}
	}

	wg.Add(2)
	go streamPipe(stdout)
	go streamPipe(stderr)
	err = cmd.Wait()
	wg.Wait()
	return err
}
```

```go
func (m *Manager) Create(ctx context.Context, in CreateTaskInput) (Task, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	state, err := m.ensureWorkspaceLocked(in.WorkspaceRoot)
	if err != nil {
		return Task{}, err
	}
	if _, ok := m.runners[in.Type]; !ok {
		return Task{}, fmt.Errorf("task type %q is not supported", in.Type)
	}

	task := &Task{
		ID:            types.NewID("task"),
		Type:          in.Type,
		Status:        TaskStatusPending,
		Command:       in.Command,
		Description:   in.Description,
		WorkspaceRoot: in.WorkspaceRoot,
		StartTime:     time.Now().UTC(),
	}
	task.OutputPath = filepath.Join(state.outputsDir, task.ID+".log")
	m.tasks[task.ID] = task
	if err := m.saveWorkspaceLocked(in.WorkspaceRoot); err != nil {
		delete(m.tasks, task.ID)
		return Task{}, err
	}
	if in.Start {
		if err := m.startLocked(*task); err != nil {
			delete(m.tasks, task.ID)
			return Task{}, err
		}
	}
	return *task, nil
}

func (m *Manager) startLocked(task Task) error {
	runner, ok := m.runners[task.Type]
	if !ok {
		return fmt.Errorf("task type %q is not supported", task.Type)
	}
	runCtx, cancel := context.WithCancel(context.Background())
	m.running[task.ID] = &runningTask{
		workspaceRoot: task.WorkspaceRoot,
		cancel:        cancel,
		done:          make(chan struct{}),
	}
	m.tasks[task.ID].Status = TaskStatusRunning
	if err := m.saveWorkspaceLocked(task.WorkspaceRoot); err != nil {
		delete(m.running, task.ID)
		cancel()
		return err
	}

	go func(snapshot Task, run Runner) {
		err := run.Run(runCtx, &snapshot, m)
		m.finishRun(snapshot.ID, snapshot.WorkspaceRoot, err, runCtx.Err())
	}(task, runner)
	return nil
}
```

```go
func (m *Manager) Append(taskID string, chunk []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	task, ok := m.tasks[taskID]
	if !ok {
		return fmt.Errorf("task %q not found", taskID)
	}
	next := task.Output + string(chunk)
	if m.cfg.TaskOutputMaxBytes > 0 && len(next) > m.cfg.TaskOutputMaxBytes {
		next = next[:m.cfg.TaskOutputMaxBytes]
	}
	task.Output = next
	return os.WriteFile(task.OutputPath, []byte(task.Output), 0o644)
}

func (m *Manager) finishRun(taskID, workspaceRoot string, runErr error, ctxErr error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	task, ok := m.tasks[taskID]
	if !ok {
		return
	}
	delete(m.running, taskID)

	now := time.Now().UTC()
	task.EndTime = &now
	switch {
	case errors.Is(ctxErr, context.Canceled):
		task.Status = TaskStatusStopped
	case runErr != nil:
		task.Status = TaskStatusFailed
		task.Error = runErr.Error()
	default:
		task.Status = TaskStatusCompleted
	}
	_ = m.saveWorkspaceLocked(workspaceRoot)
}
```

- [x] **Step 4: Re-run shell-focused task tests**

Run: `go test ./internal/task -run 'TestManager(RunsShellTaskAndCapturesOutput|StopsRunningShellTask)'`

Expected: PASS

- [x] **Step 5: Commit the shell runner changes**

```bash
git add internal/task/shell_runner.go internal/task/manager.go internal/task/task_test.go
git commit -m "feat: add shell task runner"
```

### Task 4: Implement Agent and Remote Runners

**Files:**
- Create: `internal/task/agent_runner.go`
- Create: `internal/task/remote_runner.go`
- Modify: `internal/task/manager.go`
- Modify: `internal/task/task_test.go`

- [x] **Step 1: Write failing tests for agent execution and remote shim execution**

```go
type fakeAgentExecutor struct {
	prompts []string
}

func (f *fakeAgentExecutor) RunTask(ctx context.Context, workspaceRoot string, prompt string, sink io.Writer) error {
	f.prompts = append(f.prompts, prompt)
	_, _ = io.WriteString(sink, "agent:"+prompt)
	return nil
}

func TestManagerRunsAgentTaskThroughExecutor(t *testing.T) {
	root := t.TempDir()
	exec := &fakeAgentExecutor{}
	manager := NewManager(Config{MaxConcurrentTasks: 8, TaskOutputMaxBytes: 1 << 20}, nil, exec)

	task, err := manager.Create(context.Background(), CreateTaskInput{
		Type:          TaskTypeAgent,
		Command:       "summarize the workspace",
		WorkspaceRoot: root,
		Start:         true,
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	waitForTaskTerminal(t, manager, task.ID, root)
	output, err := manager.ReadOutput(task.ID, root)
	if err != nil {
		t.Fatalf("ReadOutput() error = %v", err)
	}
	if output != "agent:summarize the workspace" {
		t.Fatalf("output = %q, want %q", output, "agent:summarize the workspace")
	}
}

func TestManagerRunsRemoteTaskThroughShim(t *testing.T) {
	root := t.TempDir()
	shim := filepath.Join(root, "remote-shim.cmd")
	script := "@echo off\r\necho remote:%~1\r\n"
	if err := os.WriteFile(shim, []byte(script), 0o644); err != nil {
		t.Fatal(err)
	}

	manager := NewManager(Config{MaxConcurrentTasks: 8, TaskOutputMaxBytes: 1 << 20}, nil, nil)
	manager.SetRemoteConfig(RemoteExecutorConfig{ShimCommand: shim, TimeoutSeconds: 30})

	task, err := manager.Create(context.Background(), CreateTaskInput{
		Type:          TaskTypeRemote,
		Command:       "deploy now",
		WorkspaceRoot: root,
		Start:         true,
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	waitForTaskTerminal(t, manager, task.ID, root)
	output, err := manager.ReadOutput(task.ID, root)
	if err != nil {
		t.Fatalf("ReadOutput() error = %v", err)
	}
	if !strings.Contains(output, "remote:deploy now") {
		t.Fatalf("output = %q, want remote command echo", output)
	}
}

func TestManagerRejectsRemoteTaskWhenShimMissing(t *testing.T) {
	root := t.TempDir()
	manager := NewManager(Config{MaxConcurrentTasks: 8, TaskOutputMaxBytes: 1 << 20}, nil, nil)

	_, err := manager.Create(context.Background(), CreateTaskInput{
		Type:          TaskTypeRemote,
		Command:       "deploy now",
		WorkspaceRoot: root,
		Start:         true,
	})
	if err == nil || !strings.Contains(err.Error(), "not configured") {
		t.Fatalf("Create() error = %v, want not configured", err)
	}
}
```

- [x] **Step 2: Run the new agent/remote tests**

Run: `go test ./internal/task -run 'TestManager(RunsAgentTaskThroughExecutor|RunsRemoteTaskThroughShim|RejectsRemoteTaskWhenShimMissing)'`

Expected: FAIL because the agent and remote runners are not implemented yet.

- [x] **Step 3: Implement the agent and remote runners**

```go
type AgentRunner struct {
	executor AgentExecutor
}

type taskSinkWriter struct {
	taskID string
	sink   OutputSink
}

func (w *taskSinkWriter) Write(p []byte) (int, error) {
	if err := w.sink.Append(w.taskID, p); err != nil {
		return 0, err
	}
	return len(p), nil
}

func (r AgentRunner) Run(ctx context.Context, task *Task, sink OutputSink) error {
	if r.executor == nil {
		return fmt.Errorf("agent runner is not configured")
	}
	writer := &taskSinkWriter{taskID: task.ID, sink: sink}
	return r.executor.RunTask(ctx, task.WorkspaceRoot, task.Command, writer)
}
```

```go
type RemoteRunner struct {
	config RemoteExecutorConfig
}

func (r RemoteRunner) Run(ctx context.Context, task *Task, sink OutputSink) error {
	if strings.TrimSpace(r.config.ShimCommand) == "" {
		return fmt.Errorf("remote runner is not configured")
	}
	command := fmt.Sprintf("%s %q", r.config.ShimCommand, task.Command)
	cmd := exec.CommandContext(ctx, "cmd", "/c", command)
	cmd.Dir = task.WorkspaceRoot
	output, err := cmd.CombinedOutput()
	if len(output) > 0 {
		_ = sink.Append(task.ID, output)
	}
	return err
}
```

```go
func (m *Manager) registerDefaultRunners(agentExecutor AgentExecutor) {
	m.runners[TaskTypeShell] = ShellRunner{}
	m.runners[TaskTypeAgent] = AgentRunner{executor: agentExecutor}
	m.runners[TaskTypeRemote] = RemoteRunner{config: m.remote}
}

func (m *Manager) SetRemoteConfig(cfg RemoteExecutorConfig) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.remote = cfg
	if runner, ok := m.runners[TaskTypeRemote]; ok {
		if remoteRunner, ok := runner.(RemoteRunner); ok {
			remoteRunner.config = cfg
			m.runners[TaskTypeRemote] = remoteRunner
		}
	}
}
```

- [x] **Step 4: Re-run the agent/remote tests**

Run: `go test ./internal/task -run 'TestManager(RunsAgentTaskThroughExecutor|RunsRemoteTaskThroughShim|RejectsRemoteTaskWhenShimMissing)'`

Expected: PASS

- [x] **Step 5: Commit the agent and remote runner changes**

```bash
git add internal/task/agent_runner.go internal/task/remote_runner.go internal/task/manager.go internal/task/task_test.go
git commit -m "feat: add agent and remote task runners"
```

### Task 5: Wire the Manager Into Tools and Runtime

**Files:**
- Create: `internal/tools/builtin_todo.go`
- Create: `internal/tools/builtin_task.go`
- Modify: `internal/tools/types.go`
- Modify: `internal/tools/registry.go`
- Modify: `internal/tools/tools_test.go`
- Modify: `internal/permissions/engine.go`
- Modify: `internal/engine/engine.go`
- Modify: `internal/engine/loop.go`
- Modify: `cmd/agentd/main.go`
- Modify: `cmd/agentd/main_test.go`

- [x] **Step 1: Write failing tests for tool schemas, permission gating, and todo/task lifecycle**

```go
func TestRegistryDefinitionsExposePhase2ToolSchemas(t *testing.T) {
	registry := NewRegistry()
	defs := registry.Definitions()

	wantNames := []string{
		"file_edit", "file_read", "file_write", "glob", "grep",
		"notebook_edit", "shell_command",
		"task_create", "task_get", "task_list", "task_output", "task_stop", "task_update",
		"todo_write",
	}
	gotNames := make([]string, 0, len(defs))
	for _, def := range defs {
		gotNames = append(gotNames, def.Name)
		if def.Description == "" {
			t.Fatalf("definition %q missing description", def.Name)
		}
		if def.InputSchema == nil {
			t.Fatalf("definition %q missing schema", def.Name)
		}
	}
	if !reflect.DeepEqual(gotNames, wantNames) {
		t.Fatalf("Definitions() names = %v, want %v", gotNames, wantNames)
	}

	requireSchemaFields := func(name string, required []string, props ...string) {
		t.Helper()
		for _, def := range defs {
			if def.Name != name {
				continue
			}
			gotRequired, _ := def.InputSchema["required"].([]string)
			if !reflect.DeepEqual(gotRequired, required) {
				t.Fatalf("definition %q required = %v, want %v", name, gotRequired, required)
			}
			properties, _ := def.InputSchema["properties"].(map[string]any)
			for _, prop := range props {
				if _, ok := properties[prop]; !ok {
					t.Fatalf("definition %q missing property %q", name, prop)
				}
			}
			return
		}
		t.Fatalf("missing definition %q", name)
	}

	requireSchemaFields("todo_write", []string{"todos"}, "todos")
	requireSchemaFields("task_create", []string{"type", "command"}, "type", "command", "description")
	requireSchemaFields("task_get", []string{"task_id"}, "task_id")
	requireSchemaFields("task_list", []string{}, "status")
	requireSchemaFields("task_output", []string{"task_id"}, "task_id")
	requireSchemaFields("task_stop", []string{"task_id"}, "task_id")
	requireSchemaFields("task_update", []string{"task_id"}, "task_id", "status", "description")
}

func TestTodoWriteToolPersistsWorkspaceTodos(t *testing.T) {
	root := t.TempDir()
	manager := task.NewManager(task.Config{MaxConcurrentTasks: 8, TaskOutputMaxBytes: 1 << 20}, nil, nil)

	registry := NewRegistry()
	_, err := registry.Execute(context.Background(), Call{
		Name: "todo_write",
		Input: map[string]any{
			"todos": []any{
				map[string]any{"content": "write tests", "status": "pending", "activeForm": "Writing tests"},
			},
		},
	}, ExecContext{
		WorkspaceRoot:    root,
		PermissionEngine: permissions.NewEngine("trusted_local"),
		TaskManager:      manager,
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	data, err := os.ReadFile(filepath.Join(root, ".claude", "todos.json"))
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if !strings.Contains(string(data), "write tests") {
		t.Fatalf("todos.json = %q, want persisted todo", string(data))
	}
}

func TestTaskToolsDriveTaskLifecycle(t *testing.T) {
	root := t.TempDir()
	manager := task.NewManager(task.Config{MaxConcurrentTasks: 8, TaskOutputMaxBytes: 1 << 20}, nil, nil)
	registry := NewRegistry()
	execCtx := ExecContext{
		WorkspaceRoot:    root,
		PermissionEngine: permissions.NewEngine("trusted_local"),
		TaskManager:      manager,
	}

	created, err := registry.Execute(context.Background(), Call{
		Name: "task_create",
		Input: map[string]any{
			"type":        "shell",
			"command":     "echo tool-task",
			"description": "tool lifecycle",
		},
	}, execCtx)
	if err != nil {
		t.Fatalf("task_create error = %v", err)
	}
	if created.Text == "" {
		t.Fatal("task_create returned empty task id")
	}

	waitForToolTaskTerminal(t, manager, created.Text, root)

	listed, err := registry.Execute(context.Background(), Call{Name: "task_list", Input: map[string]any{}}, execCtx)
	if err != nil {
		t.Fatalf("task_list error = %v", err)
	}
	if !strings.Contains(listed.Text, created.Text) {
		t.Fatalf("task_list text = %q, want task id %q", listed.Text, created.Text)
	}

	got, err := registry.Execute(context.Background(), Call{
		Name:  "task_get",
		Input: map[string]any{"task_id": created.Text},
	}, execCtx)
	if err != nil {
		t.Fatalf("task_get error = %v", err)
	}
	if !strings.Contains(got.Text, "tool lifecycle") {
		t.Fatalf("task_get text = %q, want description", got.Text)
	}

	output, err := registry.Execute(context.Background(), Call{
		Name:  "task_output",
		Input: map[string]any{"task_id": created.Text},
	}, execCtx)
	if err != nil {
		t.Fatalf("task_output error = %v", err)
	}
	if !strings.Contains(output.Text, "tool-task") {
		t.Fatalf("task_output text = %q, want shell output", output.Text)
	}
}

func TestTaskToolsRequireTrustedLocalAndManager(t *testing.T) {
	root := t.TempDir()
	registry := NewRegistry()

	_, err := registry.Execute(context.Background(), Call{
		Name:  "task_list",
		Input: map[string]any{},
	}, ExecContext{
		WorkspaceRoot:    root,
		PermissionEngine: permissions.NewEngine("workspace_write"),
	})
	if err == nil || !strings.Contains(err.Error(), "denied") {
		t.Fatalf("task_list error = %v, want denied", err)
	}
}

func waitForToolTaskTerminal(t *testing.T, manager *task.Manager, taskID, workspaceRoot string) {
	t.Helper()

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		got, ok, err := manager.Get(taskID, workspaceRoot)
		if err != nil {
			t.Fatalf("Get() error = %v", err)
		}
		if ok && (got.Status == task.TaskStatusCompleted || got.Status == task.TaskStatusFailed || got.Status == task.TaskStatusStopped) {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}

	t.Fatalf("task %q did not reach terminal state", taskID)
}
```

- [x] **Step 2: Run the focused tool tests and verify they fail**

Run: `go test ./internal/tools -run 'Test(RegistryDefinitionsExposePhase2ToolSchemas|TodoWriteToolPersistsWorkspaceTodos|TaskToolsDriveTaskLifecycle|TaskToolsRequireTrustedLocalAndManager)'`

Expected: FAIL because Phase 2 tools are not registered and `ExecContext` has no task manager yet.

- [x] **Step 3: Add Phase 2 tools, extend ExecContext, and wire the manager through runtime**

```go
type ExecContext struct {
	WorkspaceRoot    string
	PermissionEngine *permissions.Engine
	TaskManager      *task.Manager
}
```

```go
type Engine struct {
	model                   model.StreamingClient
	registry                *tools.Registry
	permission              *permissions.Engine
	store                   ConversationStore
	ctxManager              *contextstate.Manager
	compactor               contextstate.Compactor
	runtime                 *contextstate.Runtime
	meta                    RuntimeMetadata
	basePrompt              string
	maxWorkspacePromptBytes int
	maxToolSteps            int
	taskManager             *task.Manager
}

func (e *Engine) SetTaskManager(manager *task.Manager) {
	if e == nil {
		return
	}
	e.taskManager = manager
}
```

```go
result, execErr := e.registry.Execute(ctx, tools.Call{
	Name:  call.Name,
	Input: call.Input,
}, tools.ExecContext{
	WorkspaceRoot:    in.Session.WorkspaceRoot,
	PermissionEngine: e.permission,
	TaskManager:      e.taskManager,
})
```

```go
taskManager := task.NewManager(task.Config{
	MaxConcurrentTasks: cfg.MaxConcurrentTasks,
	TaskOutputMaxBytes: cfg.TaskOutputMaxBytes,
}, nil, buildAgentTaskExecutor(runner))
taskManager.SetRemoteConfig(task.RemoteExecutorConfig{
	ShimCommand:    cfg.RemoteExecutorShimCommand,
	TimeoutSeconds: cfg.RemoteExecutorTimeoutSeconds,
})
runner.SetTaskManager(taskManager)
```

```go
type taskEventSink struct {
	writer io.Writer
}

func (s taskEventSink) Emit(_ context.Context, event types.Event) error {
	switch event.Type {
	case types.EventAssistantDelta:
		var payload types.AssistantDeltaPayload
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			return err
		}
		_, err := io.WriteString(s.writer, payload.Text)
		return err
	case types.EventTurnFailed:
		var payload types.TurnFailedPayload
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			return err
		}
		return errors.New(payload.Message)
	default:
		return nil
	}
}

type agentTaskExecutor struct {
	runner *engine.Engine
}

func buildAgentTaskExecutor(runner *engine.Engine) task.AgentExecutor {
	if runner == nil {
		return nil
	}
	return agentTaskExecutor{runner: runner}
}

func (a agentTaskExecutor) RunTask(ctx context.Context, workspaceRoot string, prompt string, sink io.Writer) error {
	sessionID := types.NewID("task_session")
	turnID := types.NewID("task_turn")
	return a.runner.RunTurn(ctx, engine.Input{
		Session: types.Session{ID: sessionID, WorkspaceRoot: workspaceRoot},
		Turn: types.Turn{ID: turnID, SessionID: sessionID, UserMessage: prompt},
		Sink: taskEventSink{writer: sink},
	})
}
```

```go
func (todoWriteTool) Execute(_ context.Context, call Call, execCtx ExecContext) (Result, error) {
	if execCtx.TaskManager == nil {
		return Result{}, fmt.Errorf("task manager is not configured")
	}
	todos, err := decodeTodoItems(call.Input["todos"])
	if err != nil {
		return Result{}, err
	}
	if err := execCtx.TaskManager.WriteTodos(execCtx.WorkspaceRoot, todos); err != nil {
		return Result{}, err
	}
	return Result{Text: filepath.Join(execCtx.WorkspaceRoot, ".claude", "todos.json")}, nil
}

func decodeTodoItems(raw any) ([]task.TodoItem, error) {
	items, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("todos must be an array")
	}

	todos := make([]task.TodoItem, 0, len(items))
	for _, item := range items {
		mapped, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("todo items must be objects")
		}
		todo := task.TodoItem{
			Content:    strings.TrimSpace(asString(mapped["content"])),
			Status:     strings.TrimSpace(asString(mapped["status"])),
			ActiveForm: strings.TrimSpace(asString(mapped["activeForm"])),
		}
		if todo.Content == "" {
			return nil, fmt.Errorf("todo content is required")
		}
		switch todo.Status {
		case "pending", "in_progress", "completed":
		default:
			return nil, fmt.Errorf("invalid todo status %q", todo.Status)
		}
		todos = append(todos, todo)
	}
	return todos, nil
}

func asString(v any) string {
	s, _ := v.(string)
	return s
}
```

```go
func (taskCreateTool) Execute(ctx context.Context, call Call, execCtx ExecContext) (Result, error) {
	if execCtx.TaskManager == nil {
		return Result{}, fmt.Errorf("task manager is not configured")
	}
	created, err := execCtx.TaskManager.Create(ctx, task.CreateTaskInput{
		Type:          task.TaskType(call.StringInput("type")),
		Command:       call.StringInput("command"),
		Description:   call.StringInput("description"),
		WorkspaceRoot: execCtx.WorkspaceRoot,
		Start:         true,
	})
	if err != nil {
		return Result{}, err
	}
	return Result{Text: created.ID}, nil
}
```

- [x] **Step 4: Re-run the focused tool and daemon wiring tests**

Run: `go test ./internal/tools ./cmd/agentd -run 'Test(RegistryDefinitionsExposePhase2ToolSchemas|TodoWriteToolPersistsWorkspaceTodos|TaskToolsDriveTaskLifecycle|TaskToolsRequireTrustedLocalAndManager|BuildRuntimeWiringUsesConfig|ConfigureRuntimeGuardrailsAffectsTools)'`

Expected: PASS

- [x] **Step 5: Commit the tool and wiring changes**

```bash
git add internal/tools/builtin_todo.go internal/tools/builtin_task.go internal/tools/types.go internal/tools/registry.go internal/tools/tools_test.go internal/permissions/engine.go internal/engine/engine.go internal/engine/loop.go cmd/agentd/main.go cmd/agentd/main_test.go
git commit -m "feat: add phase 2 task management tools"
```

### Task 6: Final Verification and Plan Sync

**Files:**
- Modify: `docs/superpowers/plans/2026-04-06-tool-sync-phase-2-task-management.md`

- [x] **Step 1: Run focused package tests**

Run: `go test ./internal/task ./internal/tools ./internal/config ./cmd/agentd`

Expected: PASS

- [x] **Step 2: Run the full test suite with isolated home directories**

Run:

```powershell
$originalHome = $env:HOME
$originalUserProfile = $env:USERPROFILE
$tempHome = Join-Path $env:TEMP ('go-agent-test-home-' + [guid]::NewGuid().ToString())
New-Item -ItemType Directory -Path $tempHome | Out-Null
try {
  $env:HOME = $tempHome
  $env:USERPROFILE = $tempHome
  go test ./...
}
finally {
  $env:HOME = $originalHome
  $env:USERPROFILE = $originalUserProfile
  Remove-Item -LiteralPath $tempHome -Recurse -Force -ErrorAction SilentlyContinue
}
```

Expected: PASS

- [x] **Step 3: Review the diff to confirm the scope stayed inside Phase 2**

Run: `git diff --stat master...HEAD`

Expected: only task/config/runtime wiring files, tool files, and this plan file changed.

- [x] **Step 4: Mark completed checkboxes in this plan**

```markdown
- [x] **Step N: ...**
```

- [x] **Step 5: Commit the verified Phase 2 slice**

```bash
git add internal/task internal/tools internal/config cmd/agentd docs/superpowers/plans/2026-04-06-tool-sync-phase-2-task-management.md
git commit -m "feat: implement tool sync phase 2 task management"
```
