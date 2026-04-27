# Sesame

English | [简体中文](README.zh-CN.md)

Sesame is a local-first personal assistant runtime for workspace-scoped tools, roles, automations, reports, and memory.

It keeps chat, tool execution, background tasks, automation, reports, and runtime state in one workspace-scoped loop so you can ask for work, execute it, inspect what happened, and continue from the same context.

## Why Sesame

- Local-first by default. Workspace state lives on your machine instead of in a remote SaaS control plane.
- One runtime spine. Interactive turns, background execution, reports, and automations share the same runtime model.
- Workspace-scoped state. Context history, tasks, reports, roles, and runtime data stay attached to a workspace.
- Terminal-native workflow. Use the CLI/TUI for day-to-day work and the web console when you need broader runtime visibility.
- File-backed roles. Specialist roles live under `roles/<role_id>/` and can be managed as part of the workspace.

## Features

- CLI and TUI entrypoints for interactive agent workflows
- Local daemon that automatically manages runtime state for a workspace
- Context history with load, reopen, and branch-style workflows
- Built-in tools for shell, file work, patching, search, and task delegation
- Task-backed specialist delegation with reports returned to the main conversation
- Skill-gated role automations with a watcher asset -> owner role task -> main agent report -> policy loop
- Role-owned automation source files, watcher contract validation, and runtime inspection
- Web console for chat, runtime state, roles, and usage
- Optional Discord ingress for remote workspace interaction

## Work Scenarios

Sesame is useful when work is centered on a local workspace and the assistant needs to keep state across tools, roles, tasks, and automation runs.

- **Personal workspace operations**: Ask the main session to inspect files, run commands, update local assets, summarize runtime state, or continue from previous context.
- **Role-based recurring work**: Create specialist roles for durable responsibilities such as research intake, log triage, release checks, or workspace maintenance. Delegated work runs in that role's persistent session and reports back to the main conversation.
- **Role-owned automation**: Let a role own watcher scripts under `roles/<role_id>/automations/<automation_id>/`. When a watcher detects a signal, Sesame dispatches one owner-role task and delivers the result to the main agent.
- **Remote follow-up through Discord**: Use Discord as an ingress path into the same workspace runtime when you are away from the terminal, while keeping execution and state local.
- **Runtime inspection and recovery**: Use the web console to review chats, reports, tasks, roles, automation runs, and usage when a workflow needs diagnosis or cleanup.

Example requests:

```text
Ask the research role to scan these sources every weekday and report important changes.
When this watcher detects a failed job, have the owning role inspect the workspace and report the cause.
Summarize what happened in this workspace today and show which background tasks are still active.
```

## Quick Start

### Requirements

- Go `1.24+`
- A model provider configured in `~/.sesame/config.json`
- Linux or WSL is the primary tested environment today

### 1. Clone and enter a workspace

```bash
git clone <your-fork-or-repo-url>
cd Sesame-agent
mkdir -p /path/to/workspace
```

### 2. Start Sesame

Run Sesame from the repository root and point it at the workspace you want to use:

```bash
go run ./cmd/sesame --workspace /path/to/workspace
```

If this is your first run, complete setup first:

```bash
go run ./cmd/sesame setup
```

To reopen provider configuration later:

```bash
go run ./cmd/sesame configure
```

`configure` opens a shared configuration home page with two entries:
- `Model Setup` (required)
- `Third-Party Integrations` (optional)

Discord setup is under `Third-Party Integrations`. Startup only requires completing `Model Setup`; Discord can be configured later.

Discord `Allowed User IDs` is required when Discord is enabled. Leaving it empty is rejected in the setup flow so a bot cannot accidentally accept messages from everyone or silently reject all users.

When configuration is missing, normal `sesame` startup automatically enters setup.

Or check daemon/runtime status:

```bash
go run ./cmd/sesame --workspace /path/to/workspace --status
```

### 3. Open the console

When the local daemon is running, open the web console in your browser:

```text
http://127.0.0.1:4317/
```

### 4. Start working

Useful chat commands:

- `/history`
- `/history load <head_id>`
- `/reopen`

## Configuration

Sesame uses two main storage locations:

- Global config and shared local state: `~/.sesame/`
- Workspace runtime state: `<workspace>/.sesame/`

Your model provider configuration lives in:

```text
~/.sesame/config.json
```

Use `sesame configure` any time to return to the shared configuration home page (`Model Setup` and `Third-Party Integrations`).

## How It Works

Sesame is converging on a runtime model with a few explicit primitives:

- `workspace`: the aggregate root for runtime state
- `session`: the main interactive binding to the user
- `context head`: the boundary for history, reload, reopen, and branching
- `task`: the backbone for background execution
- `report`: how child/background work returns to the main line
- `role`: a file-backed execution profile for specialist behavior

In practice, the preferred flow is:

```text
user request
  -> main parent session
  -> tool call or task creation
  -> runtime execution
  -> report delivery / task result
  -> main parent responds to the user
```

Specialist roles may use internal sessions or context handles as implementation details, but the intended public model is workspace runtime orchestration rather than multi-agent chat rooms.

## Automation Model

Simple automations use one explicit runtime chain:

```text
role watcher script
  -> runtime dispatch lock
  -> owner role task
  -> main agent report delivery
  -> policy-driven resume / pause / escalation
```

The watcher is only responsible for detection. When it reports `needs_agent`, Sesame pauses that watcher run, dispatches exactly one task to the owning role, waits for the task result, reports to the main agent, then resumes or pauses according to the automation policy.

Automation creation is intentionally gated:

- Automation-definition work must activate `automation-standard-behavior` and `automation-normalizer` before using the simple automation builder.
- Role-owned automations must be created from the owning specialist role session.
- Owner tasks cannot create, modify, pause, or resume automations; they execute the `automation_goal` and report the result.
- Watcher scripts must emit the supported `script_status` JSON contract. Legacy `{"trigger": ...}` style payloads are rejected.

This keeps creation, runtime execution, and status/report turns separated so a watcher match does not drift into automation reconfiguration or duplicate owner-task dispatch.

## Repository Layout

- `cmd/sesame`
  CLI entrypoint
- `internal/cli`
  TUI, REPL, client calls, and terminal rendering
- `internal/daemon`
  Runtime composition, recovery, HTTP server, and orchestration
- `internal/engine`
  Turn execution, prompt assembly, tool wiring, and context refresh
- `internal/session`
  Session queueing, delegation, and runtime handoff
- `internal/task`
  Background task model and execution
- `internal/tools`
  Built-in tools, tool runtime, capability gates, and execution control
- `internal/automation`
  Watchers, simple owner-task automation, and automation lifecycle
- `internal/reporting`
  Report delivery
- `internal/roles`
  File-backed role catalog and role service
- `internal/store/sqlite`
  Local persistence
- `web/console`
  React-based console UI

## Current Status

Sesame is actively evolving toward a more explicit workspace runtime model:

- workspace as the main runtime boundary
- task as the primary background execution primitive
- role as a file-backed execution profile, not a second public chat line
- runtime diagnostics, reports, tasks, roles, and automations exposed in the console
- automation skills and tool-layer checks working together to enforce mode boundaries
- TUI and Discord flows sharing the same daemon/session runtime

The project is already usable for local operational workflows, but the architecture is still being tightened and simplified.

## Roadmap

- Continue simplifying the runtime spine around workspace, task, report, and context-head primitives
- Improve memory and history compaction for longer-running workspaces
- Expand runtime inspection and repair workflows in the console
- Strengthen role versioning, policy boundaries, and diagnostics
- Add more external entrypoints on top of the same local runtime model

## Development

Build the CLI from source:

```bash
go build ./cmd/sesame
```

Run package checks:

```bash
go test ./...
```

Build the console:

```bash
cd web/console
npm run build
```

If your local checkout includes ignored test files, run the relevant package tests before publishing changes.

## License

License metadata has not been finalized yet.
