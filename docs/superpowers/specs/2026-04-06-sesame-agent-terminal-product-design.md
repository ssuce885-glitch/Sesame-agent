# 2026-04-06 Sesame-agent Terminal Product Design

## Status

Proposed

## Summary

`Sesame-agent` should become the primary terminal-facing product for this repository.

The first version should feel closer to Claude Code than to a traditional ops CLI:

- users run `sesame-agent`
- the product automatically reuses or launches the local runtime daemon
- the default experience is an interactive, chat-first terminal REPL
- most system operations are exposed as slash commands inside the REPL
- long-running automation remains daemon-owned and continues to run after the terminal client exits

This design deliberately reuses the existing `agentd` HTTP/SSE runtime, session store, runtime graph, and task manager rather than replacing them.

## Background

The repository already has a solid local runtime foundation:

- `agentd` provides the daemon process
- SQLite stores sessions, turns, events, runtime graph objects, and metrics
- HTTP and SSE APIs already back the React console
- runtime graph plan mode exists
- background task execution already exists through `internal/task`

What is missing is the terminal-native product shell.

Today the repository can be driven through HTTP APIs or the web console, but the desired product experience is:

- terminal-first
- directly launchable from a single `sesame-agent` command
- suitable for both interactive human use and long-lived daemon-backed automation
- capable of growing into a deployable operator surface for sessions, tasks, cron jobs, monitoring, and future autonomous workflows

## Goals

- Make `sesame-agent` the main human-facing terminal entrypoint.
- Deliver a Claude Code-like terminal experience without copying its full UI stack.
- Keep the default interaction chat-first and low-friction.
- Preserve long-running daemon behavior so background tasks and cron continue after the REPL exits.
- Reuse the existing local runtime instead of rebuilding execution logic in the CLI.
- Expose task, cron, session, status, and plan controls as slash commands.
- Keep the first version local-first and simple enough to ship incrementally.

## Non-Goals

- A full-screen TUI in v1.
- Remote multi-host control in v1.
- Replacing `agentd` with a single-process front-end runtime.
- Creating a second persistence database for terminal state.
- Unifying runtime-graph `task_records` and execution tasks in v1.
- Building a full notification center, alert router, or dashboard system in v1.

## Product Decisions

The first version locks in the following product decisions:

- Product name: `Sesame-agent`
- Default experience: interactive enhanced REPL
- Top-level interface: thin CLI with a small number of startup flags
- Runtime model: local CLI client + local daemon
- Communication model: localhost HTTP + SSE
- Primary session type: interactive session
- Background automation isolation: worker sessions
- Primary command surface after startup: slash commands
- Terminal client exit behavior: daemon keeps running by default

## User Experience Model

### Core User Journey

The primary user journey is:

1. User runs `sesame-agent`
2. CLI loads config and connects to the local runtime
3. CLI launches the daemon if it is not already available
4. CLI restores or creates the current interactive session
5. User interacts through a chat-first REPL
6. User uses slash commands for management operations
7. Background cron and task activity continues even if the REPL exits

### Interaction Style

The REPL should follow one simple rule:

- plain text means "send this to the agent"
- slash commands mean "control the local product/runtime"

This keeps the mental model easy:

- "ask the agent to do work" -> normal prompt
- "manage the runtime" -> `/...`

## Process Model

### High-Level Architecture

The product consists of two cooperating processes:

- `sesame-agent`
  - CLI entrypoint
  - startup and daemon bootstrap
  - REPL loop
  - local slash command handling
  - output rendering
  - local session selection state
- `agentd`
  - session and turn execution
  - model/tool orchestration
  - SQLite persistence
  - task execution
  - cron scheduling
  - background automation continuity

### Why The Daemon Remains

The daemon should remain the runtime owner because the product is intended to support:

- long-lived deployments
- scheduled automation
- operator-style checks
- background execution that outlives a terminal session

Removing the daemon would make future cron, monitoring, and autonomous workflows harder and would duplicate existing runtime logic.

## CLI Shape

### Default Invocation

The default invocation is:

```text
sesame-agent [prompt]
```

Behavior:

- no prompt: open interactive REPL
- prompt provided: open interactive REPL and submit it as the first turn

### First-Version Flags

The top-level CLI should stay intentionally thin.

Recommended first-version flags:

- `--resume [session-id]`
- `--model <model>`
- `--permission-mode <mode>`
- `--data-dir <dir>`
- `--print`
- `--version`
- `--status`

Optional later flags can be added, but the outer CLI should not become the primary business command surface.

### Thin External Surface

The CLI should not introduce many subcommands in v1.

Recommended outer-surface rule:

- top-level flags are allowed for startup, resume, printing, and diagnostics
- almost all product operations happen inside the REPL via slash commands

This keeps the experience close to Claude Code while still preserving scriptability where needed.

## REPL Design

### REPL Mode

The REPL is not a full-screen TUI.

It is an enhanced single-column terminal interaction loop with:

- streaming assistant output
- structured tool call rendering
- lightweight status line
- command completion and input history
- slash command routing

### Output Blocks

The REPL should render output as a linear stream of blocks:

- user blocks
- assistant blocks
- tool blocks
- system blocks
- warning/error blocks

This preserves terminal simplicity while still making the output readable.

### Tool Rendering

Tool calls should not be dumped as raw JSON unless requested.

Default rendering should show:

- tool name
- running/completed/failed status
- short args preview
- short result preview

The renderer can expand to detailed output on demand later.

### Status Line

The REPL should keep a compact status line showing:

- current session id or short label
- daemon connectivity
- model
- permission mode
- active/idle turn state

Example:

```text
[sess_main] [connected] [gpt-5.x] [trusted_local] [idle]
```

### Input Features

The first version should include:

- input history via arrow keys
- slash command completion
- clear distinction between local command output and agent output

The first version should not require a full terminal widget framework if a simpler line-editor path is sufficient.

## Slash Command Information Architecture

### Command Families

The first version should group slash commands into a small number of obvious families.

Recommended built-in groups:

- base
  - `/help`
  - `/clear`
  - `/exit`
- session
  - `/session list`
  - `/session use <id>`
  - `/session resume [id]`
  - `/session workers`
- status
  - `/status`
  - `/model`
  - `/permissions`
  - `/config`
- task
  - `/task list`
  - `/task show <id>`
  - `/task output <id>`
  - `/task stop <id>`
- plan
  - `/plan enter [file]`
  - `/plan exit [state]`
- cron
  - `/cron add ...`
  - `/cron list`
  - `/cron remove <id>`
  - `/cron pause <id>`
  - `/cron resume <id>`
  - `/cron inspect <id>`

### Slash Commands Are Local

Slash commands are interpreted by the local CLI.

They are not sent to the model as ordinary prompts.

Only plain user text enters the turn submission pipeline.

This is important for correctness, latency, and testability.

## Session, Turn, and Worker Model

### User-Facing Concepts

The terminal product should expose three user-facing runtime concepts:

- session
- turn
- task

`run` remains an internal runtime-graph concept and should not be a primary terminal concept in v1.

### Interactive Session

An interactive session is:

- the current human-facing conversation/work context
- associated with a workspace root
- the session bound to the active REPL

The REPL should have exactly one current interactive session at a time.

### Turn

A turn is one submitted user input plus one execution cycle.

The existing runtime already behaves this way, and the product should preserve that definition.

### Single Active Turn Constraint

The existing `session.Manager.SubmitTurn()` cancels any currently running turn in the same session before starting a new one.

That existing behavior becomes a product design constraint:

- one session can only have one active turn
- the foreground REPL session should therefore remain reserved for foreground interaction

### Worker Sessions

A worker session is a background-only session used by daemon-owned automation.

Worker sessions should be used for:

- cron-triggered executions
- future autonomous monitoring flows
- background operator workflows that must not interrupt the current REPL

### Session Classification

To make this model explicit, sessions should gain a lightweight classification field:

- `interactive`
- `worker`

Recommended implementation:

- add `kind` to `types.Session`
- add `kind` to the `sessions` table with default `interactive`

This allows the CLI and APIs to hide worker sessions by default while still exposing them when asked.

## Cron, Task, and Interactive Collaboration Model

### Separation Of Concerns

The product should distinguish:

- cron definition
- execution instance
- session context

In product terms:

- cron job = persistent schedule definition
- task = concrete execution instance from `internal/task`
- worker session = background context used by executions

### Recommended Cron Execution Model

One cron job should own one persistent worker session.

Every cron trigger should:

1. create a new turn in that worker session
2. create or attach a new execution task from `internal/task` for observability

This gives each cron job continuity without polluting the interactive session.

### Default Concurrency Policy

The first version should default to:

- `max_concurrency = 1`
- `skip_if_running = true`

If a job is still running when the next fire time arrives, the default behavior is to skip the new trigger and record the skip.

### Frontend Visibility

Background automation should be visible but not intrusive.

Recommended REPL behavior:

- show a lightweight system notice when a cron run starts
- show a summary notice when it completes or fails
- never inject the full cron transcript into the interactive session by default

The user can inspect details with:

- `/cron inspect <job>`
- `/task output <task>`
- `/session workers`

## Persistence Model

### Reuse Existing Stores

The terminal product should not create a second session database.

It should reuse existing persistence boundaries:

- `agentd.db` for sessions, turns, events, runtime graph objects, and future cron metadata
- `.claude/tasks.json` and `.claude/tasks/*.log` for execution tasks managed by `internal/task`

### Existing Runtime Graph Objects

The repository already persists:

- runs
- plans
- task records
- tool runs
- worktrees

These should remain in `agentd.db`.

The terminal product should reuse them where needed, but should not expose all of them directly in v1.

### Execution Task vs Runtime-Graph Task

The codebase currently has two different task concepts:

- `internal/task.Task`
  - concrete shell/agent/remote execution tasks
- `types.TaskRecord`
  - runtime-graph workflow/planning tasks

The first version should not try to merge them.

Product rule for v1:

- `/task` refers to execution tasks from `internal/task`
- runtime-graph task records remain internal or plan-scoped

This avoids immediate user confusion and avoids a forced storage migration.

### Cron Persistence

Cron definitions should live in `agentd.db`, not in per-workspace JSON files.

Recommended first-version table:

- `scheduled_tasks`

Suggested fields:

- `id`
- `workspace_root`
- `owner_session_id`
- `worker_session_id`
- `cron_expr`
- `prompt`
- `enabled`
- `next_run_at`
- `last_run_at`
- `last_status`
- `last_error`
- `created_at`
- `updated_at`

This is enough for:

- `/cron list`
- `/cron inspect`
- pause/resume
- daemon restart recovery

Per-run historical detail can be added later if needed.

## Daemon Lifecycle

### Default Startup Behavior

`Sesame-agent` should treat daemon management as an internal concern.

Default startup flow:

1. load local config
2. resolve data dir
3. check daemon health endpoint
4. if healthy, reuse it
5. if not healthy, launch daemon
6. wait for health to become ready
7. restore or create current interactive session
8. enter REPL

### Daemon Discovery

The CLI should not trust a PID file alone.

Recommended health strategy:

- daemon listens on localhost
- CLI probes the health/status endpoint
- pid/metadata files are advisory only
- actual readiness is determined by successful health probing

### Exit Behavior

Exiting the REPL should not shut down the daemon by default.

That behavior is required for:

- cron continuity
- background task continuity
- quick future reconnects

If an explicit daemon shutdown path is desired later, it should be opt-in.

### Failure Handling

The product should explicitly handle:

- daemon failed to launch
- daemon became unavailable during REPL use
- SSE stream interruption
- turn submission failure
- invalid local configuration

Recommended behavior:

- startup failures exit loudly and early
- runtime connection failures move the REPL into degraded mode
- SSE reconnect should resume from the last seen event sequence where possible

## API Implications

The terminal client should continue to talk to the daemon via local HTTP/SSE.

Existing APIs already cover much of the session/turn/timeline path.

Recommended additions for terminal support:

- session APIs
  - support `kind`
  - optionally filter by `interactive` vs `worker`
- task APIs
  - list/get/output/stop execution tasks
- cron APIs
  - add/list/inspect/pause/resume/remove jobs
- focus helpers
  - optionally add an API to find or create the current interactive session for a workspace

The CLI should not link directly to runtime internals across process boundaries.

## Internal Implementation Structure

Recommended new packages for the terminal product:

- `cmd/sesame-agent`
  - main entrypoint
- `internal/cli`
  - top-level app wiring
- `internal/cli/daemon`
  - local daemon discovery, launch, health wait
- `internal/cli/client`
  - HTTP and SSE client wrapper for local runtime APIs
- `internal/cli/repl`
  - input loop, slash command routing, session focus
- `internal/cli/render`
  - block rendering, status line, tool block formatting
- `internal/cli/commands`
  - slash command implementations

This structure keeps the terminal product isolated from core runtime code while still reusing the daemon as intended.

## Configuration Model

The product should reuse existing daemon configuration where possible.

`sesame-agent` should support:

- reading the same user config file used by `agentd`
- passing temporary startup overrides from CLI flags to the launched daemon
- showing effective local runtime config via `/status` or `/config`

The first version should avoid introducing a second configuration universe.

## Testing Strategy

### Unit Tests

Add unit coverage for:

- CLI flag parsing
- daemon discovery and startup decision logic
- slash command parsing and routing
- REPL block rendering
- session selection logic

### Integration Tests

Add integration coverage for:

- auto-launching the daemon when absent
- reconnecting to an existing daemon
- submitting prompts through the CLI client
- receiving streaming SSE updates into the renderer
- slash commands hitting local task and cron endpoints
- worker sessions remaining isolated from the interactive session

### Regression Tests

Add regression coverage for:

- a background cron run not cancelling the active interactive turn
- daemon restart marking running turns interrupted and recovering cleanly
- `/task` only surfacing execution tasks
- worker sessions being hidden from normal session list views

## First-Version Delivery Scope

The first version should ship the following:

- `sesame-agent` terminal entrypoint
- automatic local daemon reuse/launch
- chat-first enhanced REPL
- session restore/create flow
- streaming turn rendering
- basic slash commands
  - `/help`
  - `/clear`
  - `/exit`
  - `/status`
  - `/session`
  - `/task`
  - `/plan`
  - `/cron`
- execution-task inspection
- cron management basics
- worker-session isolation for background automation

## Explicitly Deferred

The first version should defer:

- full-screen TUI
- remote daemon control
- cross-host control plane
- task model unification
- worktree UX
- complex notification routing
- advanced cron overlap policies
- multi-pane dashboards

## Incremental Delivery Order

Recommended implementation order:

1. CLI bootstrap and daemon auto-launch/reuse
2. interactive REPL shell with status line and streaming output
3. session restore/create and basic plain-text turn submission
4. basic slash commands
5. execution task APIs and `/task`
6. cron persistence, scheduler APIs, and `/cron`
7. worker-session classification and visibility rules
8. polishing, reconnect handling, and UX refinement

This sequence delivers user-visible value early while keeping risk localized.

## Rationale Summary

This design is intentionally conservative in runtime architecture and ambitious in product experience.

It avoids rewriting the daemon, avoids building a second state system, and avoids prematurely over-designing the UI. At the same time, it establishes the correct long-term product shape:

- terminal-first
- daemon-backed
- chat-first
- slash-command managed
- safe for long-running automation

That combination best matches the repository's existing strengths and the intended direction of `Sesame-agent`.
