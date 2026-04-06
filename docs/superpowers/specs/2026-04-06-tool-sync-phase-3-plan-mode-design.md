# Tool Sync Phase 3 Plan Mode Design

**Date**: 2026-04-06  
**Status**: Draft  
**Author**: Codex

## Overview

Phase 3 begins by adding Claude Code style `plan mode` to `go-agent`.

This subphase adds:

- `enter_plan_mode`
- `exit_plan_mode`

The design intentionally treats `plan mode` as the first consumer of the runtime graph foundation that already exists in `agentd.db`. It does not introduce a second session database, a second persistence model, or a plan-specific state silo.

## Goals

- Add persistent `plan mode` lifecycle tools on top of the existing runtime graph.
- Reuse the existing SQLite store and existing `runs` and `plans` tables.
- Keep tools thin and move plan lifecycle logic into a reusable service layer.
- Support lazy creation of a session-scoped `run` when a plan tool needs one.
- Guarantee that each session has at most one active plan at a time.
- Make same-turn follow-up tools reuse the same lazily created `run`.

## Non-Goals

- Implementing `enter_worktree`, `exit_worktree`, or `schedule_cron` in this subphase.
- Adding a new workspace-local database such as `.claude/sessions.db`.
- Adding new plan terminal states such as `cancelled`.
- Designing UI-specific plan mode surfaces beyond tool inputs and outputs.
- Reworking the existing runtime graph schema beyond the minimum needed for plan mode.

## Selected Approach

Phase 3 plan mode uses a **thin tool layer plus a reusable `runtimegraph.Service`**:

1. `internal/tools/builtin_plan.go` validates inputs and delegates.
2. `internal/runtimegraph` owns plan-mode business logic and runtime graph coordination.
3. `internal/store/sqlite` remains the persistence layer and gains focused query and transaction helpers.

This approach is preferred over putting runtime graph logic directly in tools because:

- later phases will add many more runtime-graph-aware tools
- lazy run creation, active-plan lookup, and state archiving should not be duplicated
- service-level tests are simpler and more durable than testing all business behavior through tools
- the existing repository already separates orchestration concerns from tool definitions

## Architecture

### Package Layout

Planned files for this subphase:

```text
internal/runtimegraph/
├── service.go          # shared service wiring and interfaces
├── plan.go             # enter/exit/get active plan behavior
└── runtimegraph_test.go

internal/tools/
├── builtin_plan.go     # enter_plan_mode / exit_plan_mode
└── tools_test.go       # schemas, validation, wiring coverage

internal/store/sqlite/
├── runtime_objects.go  # plan query helper additions
├── tx.go               # transaction helper for runtime graph writes
└── store_test.go       # focused store-level coverage
```

No new persistence package is introduced.

### Service Boundary

`runtimegraph.Service` is the business layer for runtime graph lifecycle operations that span multiple rows or multiple runtime object types.

For Phase 3 plan mode it owns:

- lazy run creation
- lookup of active plans for a session
- archiving previous active plans
- creating a new active plan
- finalizing the active plan on exit
- updating shared per-turn runtime context after lazy run creation

Tools do not mutate runtime graph objects directly.

### Persistence Strategy

Phase 3 reuses the existing `agentd.db` store. The relevant tables already exist:

- `runs`
- `plans`
- `sessions`

This design explicitly rejects introducing `.claude/sessions.db` because it would duplicate session and runtime graph state that already lives in SQLite and is already recovered by daemon startup logic.

## Core Data Model

### Existing Types Reused

The design continues to use:

- `types.Run`
- `types.Plan`
- `types.PlanState`

### Minimal Type Change

`types.Plan` gains one new field so tool input can be persisted without overloading unrelated fields:

```go
type Plan struct {
	ID           string    `json:"id"`
	RunID        string    `json:"run_id"`
	State        PlanState `json:"state"`
	Title        string    `json:"title,omitempty"`
	Summary      string    `json:"summary,omitempty"`
	ParentPlanID string    `json:"parent_plan_id,omitempty"`
	PlanFile     string    `json:"plan_file,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}
```

No new `PlanState` values are added in this subphase.

Valid terminal states remain:

- `completed`
- `approved`
- `failed`

### Shared Turn Runtime Context

`ExecContext` currently passes scalar fields by value. That is not sufficient for lazy run creation because a tool-created `RunID` would not automatically be visible to later tools in the same turn.

Phase 3 introduces a shared turn-scoped pointer owned by `internal/runtimegraph`:

```go
type TurnContext struct {
	CurrentSessionID string
	CurrentTurnID    string
	CurrentRunID     string
	CurrentTaskID    string
}

type ExecContext struct {
	WorkspaceRoot    string
	PermissionEngine *permissions.Engine
	TaskManager      *task.Manager
	RuntimeService   *runtimegraph.Service
	TurnContext      *runtimegraph.TurnContext
}
```

The runtime service may update `TurnContext.CurrentRunID` after lazily creating a run, allowing later tools in the same turn to reuse the same runtime graph root.

## Store Helpers

### Transaction Style

This design adopts a callback-style transaction helper rather than exposing raw transaction lifecycle to service callers.

```go
type RuntimeTx interface {
	InsertRun(context.Context, types.Run) error
	UpsertPlan(context.Context, types.Plan) error
	ListActivePlansForSession(context.Context, string) ([]types.Plan, error)
}

func (s *Store) WithTx(ctx context.Context, fn func(tx RuntimeTx) error) error
```

This is preferred because:

- it automatically centralizes rollback behavior
- it keeps service code focused on business transitions
- later runtime graph tools will also need grouped read/write transactions

### Session-Scoped Active Plan Lookup

The store gains a focused query helper:

```go
func (s *Store) ListActivePlansForSession(ctx context.Context, sessionID string) ([]types.Plan, error)
```

Expected SQL shape:

```sql
select p.payload, p.created_at, p.updated_at
from plans p
join runs r on p.run_id = r.id
where r.session_id = ? and p.state = 'active'
order by p.created_at desc, p.id desc
```

This avoids repeatedly loading the full runtime graph when a tool only needs active plans for one session.

## Tool Design

### enter_plan_mode

Input:

```json
{
  "plan_file": "string"
}
```

Behavior:

- requires `plan_file`
- requires a configured runtime service
- requires a configured shared turn runtime context
- requires `TurnContext.CurrentSessionID`
- lazily creates a `run` if `TurnContext.CurrentRunID` is empty
- archives all active plans for the current session to `completed`
- creates a new active plan attached to the active or newly created run
- updates `TurnContext.CurrentRunID` when lazy run creation occurs
- returns structured JSON text

Response payload:

```json
{
  "plan_id": "plan_xxx",
  "run_id": "run_xxx",
  "state": "active",
  "plan_file": "docs/superpowers/plans/example.md"
}
```

Schema:

```json
{
  "type": "object",
  "properties": {
    "plan_file": {
      "type": "string",
      "description": "Path to the plan file for the new active plan."
    }
  },
  "required": ["plan_file"],
  "additionalProperties": false
}
```

### exit_plan_mode

Input:

```json
{
  "state": "completed|approved|failed"
}
```

Behavior:

- defaults to `completed` when no state is provided
- validates that state is one of `completed`, `approved`, `failed`
- requires a configured runtime service
- requires a configured shared turn runtime context
- requires `TurnContext.CurrentSessionID`
- finds active plans for the current session
- returns `no active plan found` if none exist
- updates all active plans for that session to the same terminal state inside one transaction
  - only one should normally exist, but the implementation defensively repairs inconsistent state
- returns structured JSON text

Response payload:

```json
{
  "plan_id": "plan_xxx",
  "state": "completed"
}
```

Schema:

```json
{
  "type": "object",
  "properties": {
    "state": {
      "type": "string",
      "enum": ["completed", "approved", "failed"],
      "description": "Final state for the current active plan."
    }
  },
  "additionalProperties": false
}
```

## Service Semantics

### EnterPlanMode

Service contract:

```go
type EnterPlanModeInput struct {
	SessionID string
	TurnID    string
	RunID     string
	PlanFile  string
}

type EnterPlanModeOutput struct {
	PlanID   string
	RunID    string
	State    types.PlanState
	PlanFile string
}

func (s *Service) EnterPlanMode(ctx context.Context, turnCtx *TurnContext, in EnterPlanModeInput) (EnterPlanModeOutput, error)
```

Transaction flow:

1. validate required session and plan file
2. if `RunID` is empty, create a new `types.Run`
3. query all active plans for the session
4. update each active plan to `completed`
5. create a new active plan
6. commit transaction
7. after success, update `turnCtx.CurrentRunID`

Lazy run creation uses existing `types.Run` fields:

- `SessionID = SessionID`
- `TurnID = TurnID`
- `State = types.RunStateRunning`
- `Objective = "Plan mode session"`

### ExitPlanMode

Service contract:

```go
type ExitPlanModeInput struct {
	SessionID  string
	FinalState types.PlanState
}

type ExitPlanModeOutput struct {
	PlanID string
	State  types.PlanState
}

func (s *Service) ExitPlanMode(ctx context.Context, in ExitPlanModeInput) (ExitPlanModeOutput, error)
```

Transaction flow:

1. validate required session id
2. validate final state is `completed`, `approved`, or `failed`
3. query active plans for the session
4. fail with `no active plan found` if none exist
5. update all active plans to the requested final state
6. commit transaction
7. return the most recent updated plan id and state

## State Model

### Session-Level Constraint

The single-active-plan rule is scoped to a **session**, not a run.

Rationale:

- plan mode is a session-level concept in user mental models
- lazy creation of a new run would otherwise fail to find previous active plans
- run-scoped lookup would allow multiple active plans to coexist after repeated `enter_plan_mode` calls

### Supported Transitions

Phase 3 plan mode uses the following state transitions:

- implicit creation: `active`
- archive-on-enter: `active -> completed`
- exit default: `active -> completed`
- exit explicit: `active -> approved`
- exit explicit: `active -> failed`

The `draft` state remains in the wider runtime graph model but is not used by the Phase 3 plan mode tools.

## Error Handling

Phase 3 standardizes these tool-facing errors:

- `runtime service is not configured`
- `turn runtime context is not configured`
- `session id is required`
- `plan_file is required`
- `invalid plan state "<value>"`
- `no active plan found`

Database or transaction failures may surface as direct store errors.

Errors remain plain user-readable text in this subphase. No numeric tool error code layer is introduced.

## Permissions

Phase 3 adds new state-mutating tools, so they should follow the same conservative profile strategy used in Phase 2.

Recommended first version:

- `read_only`: deny
- `workspace_write`: deny
- `trusted_local`: allow

This keeps plan mode aligned with other tools that mutate runtime or workspace-adjacent state.

## Runtime Wiring

`agentd` runtime setup will need to:

- construct the SQLite store as it already does
- construct `runtimegraph.Service` from that store
- inject the service into the engine
- create a `runtimegraph.TurnContext` for each turn
- pass both into tool execution through `ExecContext`

The shared turn context should be created in the engine loop and reused across all tool executions for the same turn.

## Testing Strategy

### Service Tests

`internal/runtimegraph/runtimegraph_test.go` should cover:

- `EnterPlanMode` lazily creates a run when no run id exists
- `EnterPlanMode` reuses an existing run id when provided
- `EnterPlanMode` archives prior active plans for the same session
- `EnterPlanMode` does not archive active plans from other sessions
- `EnterPlanMode` updates shared turn context with a newly created run id
- `ExitPlanMode` defaults to `completed`
- `ExitPlanMode` supports `approved` and `failed`
- `ExitPlanMode` returns `no active plan found` when appropriate
- defensive behavior when multiple active plans exist for a session

### Store Tests

`internal/store/sqlite/store_test.go` should cover:

- `ListActivePlansForSession` only returns plans whose runs belong to the target session
- `ListActivePlansForSession` excludes non-active plans
- `WithTx` commits on nil error
- `WithTx` rolls back on callback error

### Tool Tests

`internal/tools/tools_test.go` should cover:

- registry definitions include `enter_plan_mode` and `exit_plan_mode`
- `enter_plan_mode` requires `plan_file`
- `exit_plan_mode` accepts only the allowed state enum
- tools fail when runtime service is missing
- tools fail when turn context is missing
- tools return JSON payloads from service outputs

### Wiring Tests

`internal/engine` or `cmd/agentd` tests should cover:

- same-turn tool executions share one `runtimegraph.TurnContext`
- lazy run creation in one tool becomes visible to later tools in the same turn

## Risks and Mitigations

| Risk | Impact | Mitigation |
|------|--------|------------|
| same-turn tools do not share the lazily created run id | broken runtime graph continuity | use shared `runtimegraph.TurnContext` pointer rather than value fields |
| partial enter-plan updates leave two active plans | inconsistent session state | require transaction-wrapped archive-and-create flow |
| plan mode logic leaks into tools | hard-to-maintain future tools | centralize business logic in `runtimegraph.Service` |
| later phases need richer plan metadata | rework pressure | keep service boundary stable and extend types incrementally |

## Implementation Readiness

This design is intentionally scoped so the next implementation plan can focus on one subphase:

- add `runtimegraph.Service`
- add shared turn runtime context
- add store transaction/query helpers
- add `enter_plan_mode` and `exit_plan_mode`
- wire service/context through engine and tool execution

`enter_worktree`, `exit_worktree`, and `schedule_cron` should be designed and implemented in later Phase 3 subprojects on top of the same service layer.
