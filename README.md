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
- Cheap script-based scanning for frequent checks, with model work only when a watcher emits a signal that needs an agent
- Role-owned automation source files, watcher contract validation, and runtime inspection
- Web console for chat, runtime state, roles, and usage
- Optional Discord ingress for remote workspace interaction

## Work Scenarios

Sesame is useful when work is centered on a local workspace and the assistant needs to keep state across tools, roles, tasks, and automation runs.

- **Personal workspace operations**: Ask the main session to inspect files, run commands, update local assets, summarize runtime state, or continue from previous context.
- **Role-based recurring work**: Create specialist roles for durable responsibilities such as research intake, log triage, release checks, or workspace maintenance. Delegated work runs in that role's persistent session and reports back to the main conversation.
- **Cheap signal scanning**: Use plain scripts for frequent deterministic checks, such as polling feeds, inspecting logs, checking local files, or probing external status pages. The script filters routine noise; the model is invoked only when the script reports something worth handling.
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

Do not use the repository root itself as a Sesame workspace. Keep runtime
assets such as `roles/`, `skills/`, and `.sesame/` in a separate workspace
directory, for example `/home/sauce/project/Workspace/sesame-main`.

### 2. Start Sesame

Run Sesame from the repository root and point it at the workspace you want to use:

```bash
go run ./cmd/sesame --workspace /path/to/workspace
```

The command starts or connects to the V2 daemon and opens the TUI.

To run only the daemon:

```bash
go run ./cmd/sesame --daemon --workspace /path/to/workspace
```

### 3. Open the console

When the local daemon is running, open the web console in your browser:

```text
http://127.0.0.1:8421/
```

### 4. Start working

Use the TUI for chat and the web console for broader runtime inspection: reports, tasks, task trace, roles, automations, project state, and memory.

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

This is designed to make scanning cheap. A watcher can run as a small shell, Python, or other local script that performs deterministic checks and exits with structured `script_status` JSON. Normal "nothing changed" runs do not need a model turn. The LLM path is reserved for the moments where the script has found a signal that needs judgment, synthesis, repair, or follow-up work.

Automation creation is intentionally gated:

- Automation-definition work must activate `automation-standard-behavior` and `automation-normalizer` before using the simple automation builder.
- Role-owned automations must be created from the owning specialist role session.
- Owner tasks cannot create, modify, pause, or resume automations; they execute the `automation_goal` and report the result.
- Watcher scripts must emit the supported `script_status` JSON contract. Legacy `{"trigger": ...}` style payloads are rejected.

This keeps creation, runtime execution, and status/report turns separated so a watcher match does not drift into automation reconfiguration or duplicate owner-task dispatch.

## Repository Layout

- `cmd/sesame`
  V2 CLI entrypoint, daemon launcher, and TUI bootstrap
- `internal/v2/app`
  Runtime composition, HTTP server, and daemon loops
- `internal/v2/agent`
  Turn execution, prompt assembly, tool loop, context budget, and project state refresh
- `internal/v2/session`
  Session queueing and turn lifecycle
- `internal/v2/tasks`
  Background task model, runners, output sink, and task trace
- `internal/v2/tools`
  Built-in tools, role policy gates, and execution boundaries
- `internal/v2/automation`
  Watchers, role-owned automation dispatch, and automation lifecycle
- `internal/v2/reports`
  Task report creation
- `internal/v2/roles`
  File-backed role service and role snapshots
- `internal/v2/store`
  Local SQLite persistence
- `internal/skillcatalog`
  Shared skill catalog loading and DTOs
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

### Context System Progress

Recent context work added `AGENTS.md` injection, Workspace Runtime State, Role Runtime State, instruction conflict handling, conversation compaction, tool-result micro-compaction, and owner / visibility / scope filtering for Memory and ContextBlock sources.

The active design work is to move Memory and ContextBlock from searchable / previewable sources into automatic context selection, make Context Preview and real prompt assembly share one Context Package, record dropped-context reasons, and add a stronger final token-budget gate before model requests.

For the concise TODO list, see
[`docs/context-system-todo.zh-CN.md`](docs/context-system-todo.zh-CN.md).

## Roadmap

For the detailed V2 direction, see
[`docs/v2-roadmap-context-workflow.zh-CN.md`](docs/v2-roadmap-context-workflow.zh-CN.md).

### Runtime & Architecture
- Continue simplifying the runtime spine around workspace, task, report, and context-head primitives
- Improve memory and history compaction for longer-running workspaces
- Expand runtime inspection and repair workflows in the console
- Strengthen role versioning, policy boundaries, and diagnostics
- Add more external entrypoints on top of the same local runtime model

### Multi-Modal / Vision Support
- **Vision as a tool**: Introduce an `analyze_image` tool that delegates image understanding to a
  separately configured vision model, independent of the main text model. The tool returns a text
  description that the main model consumes — no changes needed to the engine loop or conversation
  pipeline. Supports Anthropic Messages API and OpenAI-compatible providers.
- **Provider-agnostic vision config**: Support per-provider API keys, base URLs, and model
  selection under a dedicated `vision` config block in `~/.sesame/config.json`, so the vision
  model can be from a different vendor than the main model.
- **Web console image support**: SSE event types for image content, frontend image rendering,
  and user image upload/paste in the chat composer.

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


## License

License metadata has not been finalized yet.
