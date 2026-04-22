# Sesame

English | [简体中文](README.zh-CN.md)

Sesame is a local-first workspace agent runtime for terminal-driven software operations.

It keeps chat, tool execution, approvals, background tasks, automation, reports, and runtime state in one workspace-scoped loop so you can ask for work, execute it, inspect what happened, and continue from the same context.

## Why Sesame

- Local-first by default. Workspace state lives on your machine instead of in a remote SaaS control plane.
- One runtime spine. Interactive turns, background execution, approvals, reports, and automations share the same runtime model.
- Workspace-scoped state. Context history, tasks, incidents, reports, roles, and runtime data stay attached to a workspace.
- Terminal-native workflow. Use the CLI/TUI for day-to-day work and the web console when you need broader runtime visibility.
- File-backed roles. Specialist roles live under `roles/<role_id>/` and can be managed as part of the workspace.

## Features

- CLI and TUI entrypoints for interactive agent workflows
- Local daemon that automatically manages runtime state for a workspace
- Context history with load, reopen, and branch-style workflows
- Built-in tools for shell, file work, patching, search, task delegation, and approvals
- Task-backed specialist delegation with child reports returned to the main conversation
- Workspace automations, incidents, mailbox reports, and runtime inspection
- Web console for chat, runtime state, roles, and usage

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
go run ./cmd/sesame --workspace-root /path/to/workspace
```

If this is your first run, complete setup first:

```bash
go run ./cmd/sesame setup --workspace-root /path/to/workspace
```

To reopen provider configuration later:

```bash
go run ./cmd/sesame configure --workspace-root /path/to/workspace
```

`configure` opens a shared configuration home page with two entries:
- `Model Setup` (required)
- `Third-Party Integrations` (optional)

Discord setup is under `Third-Party Integrations`. Startup only requires completing `Model Setup`; Discord can be configured later.

When configuration is missing, normal `sesame` startup automatically enters setup.

Or check daemon/runtime status:

```bash
go run ./cmd/sesame --workspace-root /path/to/workspace --status
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
  -> child report / approval / result
  -> main parent responds to the user
```

Specialist roles may use internal sessions or context handles as implementation details, but the intended public model is workspace runtime orchestration rather than multi-agent chat rooms.

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
  Built-in tools, tool runtime, approvals, and execution control
- `internal/automation`
  Watchers, dispatch, incidents, and automation lifecycle
- `internal/reporting`
  Mailbox/report delivery
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
- runtime diagnostics, reports, approvals, and automations exposed in the console

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

Build the console:

```bash
cd web/console
npm run build
```

The public repository currently does not ship test source files. The published tree is trimmed to runtime, CLI, daemon, connector, and console code needed to build and run Sesame.

## License

License metadata has not been finalized yet.
