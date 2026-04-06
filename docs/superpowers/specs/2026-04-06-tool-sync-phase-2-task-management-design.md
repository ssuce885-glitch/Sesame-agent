# Tool Sync Phase 2 Task Management Design

**Date**: 2026-04-06  
**Status**: Draft  
**Author**: Codex

## Overview

Phase 2 adds Claude Code style task management to `go-agent` by introducing:

- `todo_write`
- `task_create`
- `task_list`
- `task_get`
- `task_output`
- `task_stop`
- `task_update`

This phase establishes the shared task execution foundation that later phases will reuse for cron scheduling, plan mode handoff, agent orchestration, and remote execution workflows.

## Goals

- Add a persistent todo list tool stored under the workspace `.claude/` directory.
- Add a unified task system that supports `shell`, `agent`, and `remote` tasks.
- Execute all three task types for real in the first version.
- Centralize task lifecycle management, persistence, output handling, and stop semantics in `internal/task`.
- Keep tool handlers thin: validate input, delegate to the manager, return structured results.

## Non-Goals

- Rich DAG task dependencies or team collaboration metadata from Claude Code's newer task system.
- Full distributed remote task orchestration, pooling, or multi-target routing.
- UI-specific task presentation work beyond returning text results that the existing runtime can surface.
- Replacing the existing foreground `shell_command` tool. Phase 2 adds background task execution beside it.

## Selected Approach

The design uses a **central `TaskManager` plus three runners**:

1. `ShellRunner` for local background shell tasks
2. `AgentRunner` for local in-process agent tasks
3. `RemoteRunner` for remote execution through a configurable external shim command

This approach fits the current repository better than embedding task behavior inside each tool because:

- the repository already separates orchestration from tools through `executor.go` and `orchestrator.go`
- later phases need a reusable task substrate
- manager-level testing is simpler than end-to-end tool-only testing
- tool implementations stay focused on input/output schemas rather than long-lived process control

## Architecture

### Package Layout

Phase 2 keeps task logic in the existing `internal/task` package rather than creating a new `internal/tasks` package.

Planned files:

```text
internal/task/
├── manager.go          # task registry, lifecycle, persistence, lookup
├── types.go            # Task, TaskType, TaskStatus, TodoItem
├── store.go            # tasks.json / todos.json load-save helpers
├── runner.go           # Runner, AgentExecutor, OutputSink interfaces
├── shell_runner.go     # local shell execution
├── agent_runner.go     # local in-process agent execution
├── remote_runner.go    # external shim execution
└── task_test.go        # manager and persistence tests

internal/tools/
├── builtin_todo.go     # todo_write
├── builtin_task.go     # task_* tools
└── tools_test.go       # tool behavior, registry, permission coverage
```

### Manager-Centered Model

`TaskManager` is the single authority for:

- task IDs
- current task state
- runtime handles for active tasks
- workspace-scoped persistence
- output file paths
- stopping running work
- reloading persisted task state on process restart

The tool layer never mutates task state directly.

### Core Types

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
    ID           string     `json:"id"`
    Type         TaskType   `json:"type"`
    Status       TaskStatus `json:"status"`
    Command      string     `json:"command"`
    Description  string     `json:"description,omitempty"`
    WorkspaceRoot string    `json:"workspace_root"`
    Output       string     `json:"output,omitempty"`
    OutputPath   string     `json:"output_path,omitempty"`
    Error        string     `json:"error,omitempty"`
    StartTime    time.Time  `json:"start_time"`
    EndTime      *time.Time `json:"end_time,omitempty"`
}

type TodoItem struct {
    Content    string `json:"content"`
    Status     string `json:"status"`
    ActiveForm string `json:"activeForm,omitempty"`
}
```

### Persistent State vs Runtime State

Persisted `Task` records must stay JSON-safe. Runtime-only process control data is stored separately inside the manager.

```go
type runningTask struct {
    cancel context.CancelFunc
    done   chan struct{}
}

type Manager struct {
    mu           sync.RWMutex
    tasks        map[string]*Task
    running      map[string]*runningTask
    runners      map[TaskType]Runner
    tasksFile    string
    todosFile    string
    outputsDir   string
}
```

This split avoids leaking non-serializable fields like `context.CancelFunc` into persisted task records.

## Runner Model

### Common Runner Interface

```go
type OutputSink interface {
    Append(taskID string, chunk []byte) error
}

type Runner interface {
    Run(ctx context.Context, task *Task, sink OutputSink) error
}
```

`TaskManager` owns stop semantics by creating cancellable contexts for active tasks. Runners only need to obey context cancellation and write output through the sink.

### ShellRunner

`ShellRunner` runs a background local shell command rooted at the task workspace.

Responsibilities:

- spawn the process with `exec.CommandContext`
- stream stdout and stderr into the output sink
- return a failure when the command exits non-zero
- allow cancellation through context shutdown

It does **not** update task status directly. The manager wraps the runner and performs the final state transition.

### AgentRunner

`AgentRunner` executes a real in-process agent task, but `internal/task` must not import `internal/tools` to avoid package cycles. The runner depends on an injected narrow executor interface:

```go
type AgentExecutor interface {
    RunTask(ctx context.Context, workspaceRoot string, prompt string, sink io.Writer) error
}
```

The initial implementation will adapt existing engine/session capabilities behind this interface. For Phase 2, the task command string is treated as the task prompt.

This keeps dependencies one-directional:

- `internal/tools` imports `internal/task`
- `internal/task` depends on an abstract `AgentExecutor`
- wiring happens from `cmd/agentd` or top-level runtime setup

### RemoteRunner

`RemoteRunner` executes real remote work through a configured **external shim command**.

Configuration:

```go
type RemoteExecutorConfig struct {
    ShimCommand    string
    TimeoutSeconds int
}
```

Example config file:

```json
{
  "remote_executor": {
    "shim_command": "ssh deploy@prod.example.com",
    "timeout_seconds": 300
  }
}
```

Example shim targets:

- `ssh user@host`
- `kubectl exec pod-name --`
- `docker exec container-name`
- `C:\\tools\\remote-runner.cmd`
- `/usr/local/bin/my-executor.sh`

First-version execution model:

- if `shim_command` is empty, remote task creation fails with `remote runner is not configured`
- the runner invokes the configured shim command and passes `task.Command` as the final quoted argument
- stdout/stderr from the shim are captured into the task output log
- timeout is enforced by context

Concrete Phase 2 contract:

- `shim_command = "ssh deploy@prod.example.com"`
- `task.Command = "cd /srv/app && ./deploy.sh"`
- effective invocation: `ssh deploy@prod.example.com "cd /srv/app && ./deploy.sh"`

The shim is responsible for remote authentication, authorization, working directory setup, and protocol details. `go-agent` only launches the shim and captures its output.

Phase 2 intentionally supports only one configured shim. Multi-target routing belongs in a later phase.

## Tool Design

### todo_write

Input:

```json
{
  "todos": [
    {
      "content": "string",
      "status": "pending|in_progress|completed",
      "activeForm": "string"
    }
  ]
}
```

Behavior:

- validates statuses
- writes `.claude/todos.json`
- returns the new todo list
- keeps writes serialized through the manager or dedicated todo file mutex

### task_create

Input:

```json
{
  "type": "shell|agent|remote",
  "command": "string",
  "description": "string (optional)"
}
```

Behavior:

- validates task type
- allocates task ID and output path
- persists an initial `pending` task record
- starts the selected runner in a goroutine
- immediately transitions the task to `running`
- returns task ID and summary fields

For `agent`, `command` is interpreted as the prompt.

### task_list

Returns a summary list of all known tasks for the current workspace, sorted by `start_time` descending.

### task_get

Returns full task details for one task ID:

- type
- status
- command
- description
- output path
- timestamps
- last error

If the task exists but belongs to a different workspace root, the tool returns `task "<id>" not found`.

### task_output

Reads the task log file or inline output cache and returns the current output contents.

First version behavior:

- if `output_path` exists, read from disk
- otherwise return inline `Output`
- if no output exists yet, return an empty string

Like `task_get`, output reads are workspace-scoped. A task from another workspace is treated as not found.

### task_stop

Stops a running task by calling the manager's cancellation path.

Behavior:

- if the task is running, cancel its context and transition to `stopped`
- if it is already terminal, return a no-op success response
- if the task does not exist, return `task not found`

### task_update

Allows manager-mediated updates to a task's status and description metadata without duplicating lifecycle logic in tools.

Phase 2 update scope:

```json
{
  "task_id": "string",
  "status": "pending|running|completed|failed|stopped",
  "description": "string (optional)"
}
```

`task_update` is primarily for lifecycle repair, manual task adjustments, and future external integrations.

## State Transitions

Allowed status flow:

```text
pending -> running
running -> completed
running -> failed
running -> stopped
pending -> stopped
failed/completed/stopped -> terminal
```

`task_update` may set terminal states explicitly, but the manager rejects invalid transitions such as:

- `completed -> running`
- `failed -> pending`

## Persistence Model

### Workspace Layout

```text
.claude/
├── todos.json
├── tasks.json
└── tasks/
    ├── task_001.log
    ├── task_002.log
    └── ...
```

### tasks.json Format

```json
{
  "tasks": [
    {
      "id": "task_001",
      "type": "shell",
      "status": "completed",
      "command": "go test ./...",
      "description": "Run the Go test suite",
      "workspace_root": "E:/project/go-agent",
      "output_path": ".claude/tasks/task_001.log",
      "start_time": "2026-04-06T10:00:00Z",
      "end_time": "2026-04-06T10:00:02Z"
    }
  ]
}
```

### Startup Reload

On initialization, the manager loads `tasks.json` if it exists.

Reload rules:

- terminal tasks remain as-is
- `running` tasks from a previous process are downgraded to `failed`
- downgraded tasks receive an error such as `task interrupted by process restart`

This avoids lying about work that is no longer executing.

## Engine and Tool Wiring

`tools.ExecContext` will gain an optional task manager dependency:

```go
type ExecContext struct {
    WorkspaceRoot    string
    PermissionEngine *permissions.Engine
    TaskManager      *task.Manager
}
```

The manager is created once by runtime wiring and injected into tool execution. This keeps task state shared across all tool calls in the daemon process.

To avoid package cycles, `internal/task` must not depend on `internal/tools.Registry` directly. `AgentRunner` uses the abstract `AgentExecutor` interface instead.

## Configuration Changes

Phase 2 requires extending runtime config with task execution settings:

```go
type Config struct {
    // existing fields...
    MaxConcurrentTasks         int
    TaskOutputMaxBytes         int
    RemoteExecutorShimCommand  string
    RemoteExecutorTimeoutSeconds int
}
```

Defaults:

- `MaxConcurrentTasks = 8`
- `TaskOutputMaxBytes = 1 << 20`
- `RemoteExecutorShimCommand = ""`
- `RemoteExecutorTimeoutSeconds = 300`

## Permission Model

Phase 2 adds write/execute-sensitive tools and therefore must extend permission handling.

Recommended profile behavior:

- `read_only`
  - allow: none of the new Phase 2 tools
- `workspace_write`
  - allow: `todo_write`, `task_list`, `task_get`, `task_output`
  - deny: `task_create`, `task_stop`, `task_update`
- `trusted_local`
  - allow all Phase 2 tools

Rationale:

- `todo_write` mutates workspace state
- `task_create` and `task_stop` can launch or interrupt local and remote execution
- `task_output` can expose command output and should stay aligned with profiles that already allow active execution context

If the existing profile model proves too coarse during implementation, the fallback is to allow all Phase 2 tools only under `trusted_local`.

## Error Handling

Phase 2 standardizes the following errors:

- `task manager is not configured`
- `task type "<type>" is not supported`
- `task "<id>" not found`
- `task "<id>" is not running`
- `remote runner is not configured`
- `invalid status transition from "<from>" to "<to>"`
- `task output exceeds configured limit`

Errors must be tool-facing and user-readable. They do not need a custom numeric code in Phase 2.

## Testing Strategy

### internal/task

Manager and runner tests should cover:

- create/list/get persistence round-trip
- startup reload of `running` tasks into `failed`
- shell task success path
- shell task failure path
- shell task stop path
- agent runner through a fake `AgentExecutor`
- remote runner through a mock shim script
- missing remote shim configuration
- concurrent output appends

### internal/tools

Tool tests should cover:

- registry definitions for all Phase 2 tools
- permission gating by profile
- todo file persistence
- task create/list/get/output/stop/update lifecycle
- manager injection failure path

### Full Verification

Before calling Phase 2 complete:

- `go test ./internal/task`
- `go test ./internal/tools`
- isolated-home `go test ./...`

## Risks and Mitigations

| Risk | Impact | Mitigation |
|------|--------|------------|
| `internal/task` imports `internal/tools` and creates a package cycle | blocks compilation | use `AgentExecutor` interface instead of direct registry dependency |
| shell and remote output handling races | corrupted logs or flaky tests | centralize writes through manager sink and guard with mutex |
| process restart leaves zombie running tasks in metadata | inaccurate state | downgrade persisted `running` tasks to `failed` on reload |
| remote shim command quoting is shell-sensitive | execution bugs | keep Phase 2 to a single shim string, document quoting expectations, and test with simple mock scripts |
| agent runner scope creeps into full subagent platform | delayed delivery | keep Phase 2 agent runner narrow: execute one prompt through injected executor and capture output |

## Open Questions Resolved

- **Task architecture**: use a central `TaskManager` plus three runners.
- **Remote execution**: use a configured external shim command rather than hardcoding SSH/HTTP.
- **Package placement**: keep the implementation inside `internal/task`.
- **Execution scope**: Phase 2 executes `shell`, `agent`, and `remote` tasks for real in v1.
