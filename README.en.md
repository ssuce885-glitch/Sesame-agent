# Sesame

[中文](./README.md) | [English](./README.en.md)

Sesame is a local general-purpose agent with the terminal version as its primary public delivery.

It provides a full-screen TUI, a local daemon, persistent sessions, tool calling, skill loading, and workspace-aware context management. It is suitable for terminal automation, system inspection, scheduled reporting, and multi-agent collaboration.

## Direction

Sesame is evolving toward a local agent runtime that can be invoked by interactive conversation, scheduled jobs, or external programs.

The current version already has the core building blocks in place: terminal chat, daemon, sessions, scheduling, skills, and context management. The full event-driven loop of "external high-frequency detection + on-demand agent execution" is still being completed.

The goal is to leave high-frequency, cheap, deterministic checks to scripts, monitors, or business systems, and let the agent handle the parts that require understanding, judgment, repair, summarization, and skill-driven execution. This reduces long-running token cost and makes permissions, rollback, and reporting easier to control.

## Current Status

The repository is already usable for day-to-day terminal automation workflows:

- Full-screen terminal TUI chat
- Automatic daemon startup or daemon reconnect
- Automatic session selection or creation for the current workspace
- Shell, file, search, patch, task, and permission tools
- Global and workspace skill discovery and installation
- Two-stage skill injection: default turns only receive short implicit hints, while explicitly activated or runtime-selected skills inject their full body
- Capability-based tool routing profiles: `codebase_edit`, `system_inspect`, `web_lookup`, `browser_automation`, `scheduled_report`
- System skills, currently including `skill-installer` and `skill-normalizer`
- Real delayed and recurring reporting jobs
- Mailbox inbox and cron job management
- Permission interruption handling in TUI / REPL, with `/approve` and `/deny` support to resume the current turn
- `Esc` to interrupt the current conversation
- Mouse wheel and `PgUp` / `PgDn` scrolling
- Model and runtime configuration via `~/.sesame/config.json`

## Target Use Cases

The project is moving toward these use cases:

- Server and service health checks: external scripts or monitors perform high-frequency probing, and invoke the daemon for diagnosis, repair attempts, and notifications on failure
- Scheduled inspection and periodic reports: collect status on a fixed schedule, summarize results, and deliver them to mailbox, terminal, or email channels
- Batch-task guarding: automatically collect evidence and produce remediation guidance when jobs fail, queues back up, or processing latency becomes abnormal
- Local workflow automation: use natural language to generate scripts, execute them, validate the results, and summarize the outcome
- Multi-agent collaboration: split complex work into child tasks and let a main agent aggregate the final result
- Skill-driven vertical expansion: install domain-specific skills for email, deployment, databases, or external systems and extend Sesame into a specialized automation runtime

## Recently Completed

This round of work has already completed the following improvements:

- Reworked the skill injection layer to remove full local skill summaries from every turn by default, reducing prompt pollution
- Added a capability-profile routing skeleton so ordinary web lookup and browser automation tasks no longer share the same path
- Added the `skill-normalizer` system skill to normalize third-party or downloaded skills into Sesame format
- Added on-demand `Catalog snapshot` support so the model can answer questions about installed skills / tools based on the actual loaded catalog, instead of only repeating turn-visible tools
- Fixed the permission-request recovery flow so interrupted turns no longer appear stuck
- Fixed copy and rendering so `web_fetch` is no longer mislabeled as `search`
- Fixed session memory refresh under strict providers so it no longer compacts across unresolved tool exchanges and triggers transcript validation failures after interruptions or tool-step exhaustion
- Hardened OpenAI-compatible streamed function call argument handling so early `done`, delta-only completion, or malformed argument sequences no longer break the whole turn

## Requirements

- Go `1.24+`
- A working model configuration, provided either through environment variables or `~/.sesame/config.json`

## Quick Start

Run from the repository root:

```bash
go run ./cmd/sesame
```

If you prefer to build first:

```bash
go build -o sesame ./cmd/sesame
./sesame
```

If `~/.sesame/config.json` does not exist, or required fields are missing, Sesame will guide you through initialization in the terminal.

## Model Configuration

User config file path:

```text
~/.sesame/config.json
```

OpenAI-compatible example:

```json
{
  "provider": "openai_compatible",
  "model": "glm-4-7-251222",
  "permission_profile": "trusted_local",
  "openai": {
    "api_key": "your-key",
    "base_url": "https://your-provider.example.com/v1",
    "model": "glm-4-7-251222"
  },
  "max_tool_steps": 100,
  "max_recent_items": 12,
  "compaction_threshold": 32,
  "max_estimated_tokens": 16000,
  "microcompact_bytes_threshold": 8192
}
```

Anthropic example:

```json
{
  "provider": "anthropic",
  "model": "claude-sonnet-4-5",
  "permission_profile": "trusted_local",
  "anthropic": {
    "api_key": "your-key",
    "base_url": "https://api.anthropic.com",
    "model": "claude-sonnet-4-5"
  }
}
```

Local fake-model smoke test:

```bash
SESAME_MODEL_PROVIDER=fake SESAME_MODEL=fake-smoke SESAME_PERMISSION_PROFILE=trusted_local go run ./cmd/sesame
```

## Terminal Usage

Common commands:

```bash
go run ./cmd/sesame
go run ./cmd/sesame --status
go run ./cmd/sesame --print "inspect this workspace"
go run ./cmd/sesame --resume sess_123
go run ./cmd/sesame daemon
```

TUI shortcuts:

- `Enter` to send
- `Alt+Enter` for newline
- `Tab` / `Shift+Tab` to switch between `Chat`, `Agents`, `Mailbox`, and `Cron`
- `Esc` to interrupt the current turn
- `Mouse wheel` / `PgUp` / `PgDn` to scroll
- `Ctrl+C` to quit

Common slash commands:

- `/help`
- `/status`
- `/skills`
- `/tools`
- `/approve [<request_id>] [once|run|session]`
- `/deny [<request_id>]`
- `/mailbox`
- `/cron list`
- `/cron inspect <id>`
- `/cron pause <id>`
- `/cron resume <id>`
- `/cron remove <id>`
- `/session list`
- `/session use <id>`
- `/clear`
- `/exit`

TUI view commands:

- `/chat`
- `/agents`

## Skills

Sesame supports system, global, and workspace skills.

Installed skills are activated on demand by default:

- Normal turns only receive short implicit hints when allowed
- Only explicitly named or runtime-selected skills inject their full content

Directory layout:

- System: `~/.sesame/skills/.system`
- Global: `~/.sesame/skills`
- Workspace: `<workspace>/.sesame/skills`

Examples:

```bash
go run ./cmd/sesame skill list
go run ./cmd/sesame skill inspect https://github.com/openai/skills
go run ./cmd/sesame skill install ./path/to/skill
go run ./cmd/sesame skill install openai/skills --path skills/.curated/parallel --scope workspace
go run ./cmd/sesame skill remove parallel
```

## Repository Layout

- `cmd/sesame`: CLI entrypoint
- `internal/`: daemon, runtime, tools, session, storage, config, and other core implementation
- `README.md`: Chinese project overview
- `README.en.md`: English project overview

## Next Steps

To move closer to the direction above, the next phase will focus on:

- Event-driven agent execution paths so external scripts, monitors, and business systems can invoke daemon tasks directly on failures, alerts, or state changes
- Further separation of high-frequency detection from low-frequency intelligent decision-making, reducing long-running model cost and improving execution control
- Reusable automation task templates covering evidence collection, bounded repair, result verification, failure rollback, and report delivery
- Stronger multi-agent collaboration, including skill constraints, child-agent tool boundaries, and report aggregation
- Better task understanding / retrieval so runtime can understand intent before selecting, ranking, and activating skills, rather than relying mainly on name or trigger matching
- Harder runtime budget / arbitration controls instead of relying only on prompt-level soft constraints
- Continued skill normalization so downloaded external skills converge toward Sesame canonical format
- Evaluation of first-class `web_search` / `news_lookup` support instead of relying only on `web_fetch`
- Better observability so TUI / debugging views can show the selected profile, skill activations, and budget usage directly
