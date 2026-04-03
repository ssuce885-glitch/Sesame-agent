# Deployable Responses Runtime Design

Date: 2026-04-04
Branch: `feature/minimal-runtime-loop`
Status: Draft approved in conversation, written for user review

## Goal

Turn `go-agent` from a minimal runtime loop into a locally deployable agent daemon that can:

- run as a long-lived local foreground process first
- connect to a service-provider model endpoint that uses the OpenAI SDK `Responses API` style
- support real local tool calling for:
  - `file_read`
  - `glob`
  - `grep`
  - `file_write`
  - `shell_command`
- handle normal multi-turn conversations
- manage long conversations with working-context selection and context compaction
- persist enough state to survive daemon restarts and continue using prior sessions

This stage prioritizes `Responses API` compatibility. Anthropic support remains a second adapter target, not the first deployment target.

## Deployment Target

First release target:

- local machine
- foreground daemon process
- environment-variable configuration
- local-only listener by default (`127.0.0.1`)
- SQLite-backed durable state

Explicitly out of scope for this stage:

- Windows service wrapper
- `systemd` unit packaging
- container packaging
- remote multi-user deployment
- interactive human approval UI for tool permissions

## Product Boundary

This stage is not trying to fully reproduce Claude Code as a product. It is trying to build a deployable agent runtime core with Claude Code-inspired context management and compaction behavior.

Success means:

- a real provider can stream assistant output
- the provider can request local tools
- local tools execute and return structured results to the model
- the same session can continue across multiple turns
- long sessions compact older context instead of failing due to unbounded history
- the daemon can be restarted without losing session history and summaries

## Architecture

The system will be split into five layers:

### 1. API and Session Layer

Keep the current daemon shell:

- `POST /v1/sessions`
- `POST /v1/sessions/{id}/turns`
- `GET /v1/sessions/{id}/events`

Responsibilities:

- session lifecycle
- turn submission
- SSE replay and live event streaming
- durable event logging

### 2. Agent Core

Introduce a provider-neutral runtime core responsible for:

- conversation items
- working-context construction
- tool schema exposure
- tool-call execution loop
- tool-result re-entry
- context compaction
- memory reference injection

The engine must stop depending on provider-specific payload shapes.

### 3. Provider Adapters

Adapters translate vendor protocol to and from the provider-neutral core.

Stage priority:

1. `Responses API` adapter
2. Anthropic adapter

Adapters should only translate protocol. They should not own conversation policy, compaction policy, or tool execution logic.

### 4. Tool Runtime

Tool execution remains local and provider-independent.

Initial tool set:

- `file_read`
- `glob`
- `grep`
- `file_write`
- `shell_command`

The tool registry becomes the source of truth for tool schema and execution behavior.

### 5. Persistence

SQLite remains the durable store, but persistence expands from only session/turn/event logging to include:

- conversation items
- compaction summaries
- compaction metadata
- memory references
- session runtime snapshots where useful

## Provider-Neutral Core Contract

The core contract is defined around internal objects, not vendor payloads.

### ConversationItem

Minimal item kinds:

- `user_message`
- `assistant_text`
- `tool_call`
- `tool_result`
- `summary`

Each item must carry enough metadata to rebuild provider input and to support compaction.

### ToolSchema

Each local tool exposes:

- `name`
- `description`
- `json_schema`

This schema is produced by the Go tool registry, then translated by adapters to provider-native tool definitions.

### ProviderRequest

Minimal neutral request fields:

- `model`
- `instructions`
- `items`
- `tools`
- `tool_choice`
- `stream`

### ProviderStreamEvent

Minimal neutral stream events:

- `text_delta`
- `tool_call_start`
- `tool_call_delta`
- `tool_call_end`
- `message_end`
- `usage`
- `error`

The engine consumes these events only. It does not inspect vendor-specific response bodies.

## Multi-Turn Context Management

The working context for each turn is layered:

1. system/runtime instructions
2. recent raw conversation items
3. compacted summaries for older history
4. memory references

This replaces the current near-single-turn behavior.

### Working Set Policy

First release policy:

- always keep the most recent `N` conversation items in raw form
- compact older ranges into structured summaries
- inject summaries plus recent raw items into the next provider request

This keeps recent tool interactions fully visible while preventing unbounded growth.

### Summary Shape

Compaction output should be structured, not free-form text. Each summary should capture:

- covered time range
- user goals
- important decisions
- files or paths touched
- tool outcomes
- unresolved threads

This makes future rebuilds and second-order compactions more stable.

### Tool Results in Context

Tool results must become first-class conversation items, not only transient engine events.

The next provider call must be able to see:

- which tool was called
- with which arguments
- what result came back

Without this, multi-turn tool-assisted reasoning will lose continuity.

## Context Compaction

Compaction is for context-window management, not long-term memory.

### Trigger Rules

First release should support both:

- message-count trigger
- rough token-budget trigger

The estimate does not need to be exact in stage one, but it must be deterministic and conservative enough to avoid runaway payload growth.

### Compaction Flow

1. select an older range of conversation items
2. call a compactor model path or compactor prompt
3. create a structured summary
4. mark the compacted range as no longer part of the default working set
5. persist the summary and metadata

### Compaction Limits

To avoid recursive failure modes:

- limit compaction attempts per turn
- limit how much history can be compacted in one pass
- fail cleanly if context still exceeds budget after the configured compaction limit

## Memory

For this stage, memory is a secondary enhancement layer.

Rules:

- memory is not a substitute for compaction
- memory entries are referenced lightly in working context
- memory auto-write should stay conservative
- compaction must work even if memory extraction remains minimal

This keeps the core deployment target focused on reliable multi-turn runtime behavior.

## Responses API Adapter

The first provider target is the OpenAI SDK `Responses API` style used by the user's service provider.

### Outbound Mapping

The adapter maps the neutral request into provider-native request fields:

- `model`
- `input`
- `tools`
- `stream=true`

The adapter is responsible for translating:

- `ConversationItem` -> provider `input`
- `ToolSchema` -> provider `tools`
- internal instructions -> provider instructions or equivalent system content

### Inbound Mapping

The adapter must normalize streamed provider output into neutral stream events:

- assistant text deltas
- tool call start/delta/end
- message end
- usage
- error

### Tool Loop Contract

The full loop must be:

1. provider emits tool call stream events
2. engine waits for a complete tool call
3. engine executes the local tool
4. tool result is persisted
5. tool result is added as a `tool_result` conversation item
6. next provider request continues from that result

The runtime must support:

- multiple serial tool calls in one turn
- assistant text before and after tool calls
- carry-forward of tool history into later turns

### Initial Non-Goal

The first stage does not depend on provider-hosted tools such as built-in web search. Built-in vendor tools may be added later as another tool source, but the deployment target for this stage is model-driven local tool execution.

## Anthropic Follow-Up

Anthropic remains a supported adapter target, but not the first deployment target.

The intended design is:

- `Responses API` and Anthropic both translate into the same neutral core contract
- the engine and context manager stay unchanged
- only adapter translation changes per provider

This is the main reason to invest in the neutral contract instead of encoding the provider protocol directly into the engine.

## Runtime Safety and Permission Boundary

Because this stage targets a trusted local machine without an interactive approval channel, permissions should use explicit runtime profiles rather than relying on `ask`.

### Permission Profiles

- `read_only`
  - allow: `file_read`, `glob`, `grep`
- `workspace_write`
  - allow: read tools + `file_write`
- `trusted_local`
  - allow: read tools + `file_write` + `shell_command`

The stage target needs `trusted_local`, but with hard runtime guardrails.

### Required Guardrails

- file operations must stay inside `workspace_root`
- shell commands run with `workspace_root` as the default working directory
- shell output size is capped
- shell execution time is capped
- file write size is capped
- each turn has a maximum tool-step limit
- context growth has a hard limit even after compaction

## Restart and Recovery Behavior

The daemon should preserve state across restarts, but not pretend to resume in-flight execution.

Rules:

- sessions, turns, events, summaries, and memory refs survive restart
- any turn left running at shutdown is marked `interrupted` or `failed` on startup
- the daemon does not auto-resume interrupted turns
- future turns rebuild working context from persisted state

This produces predictable recovery behavior and avoids hidden partial continuation.

## Observability

Minimum observability for deployability:

- structured stdout logs
- durable SQLite event log as the source of truth
- health/status endpoint with non-sensitive runtime info

Logs should include identifiers such as:

- `session_id`
- `turn_id`
- `provider`
- `tool_name`

## Implementation Order

This spec intentionally stops short of a full task plan, but the expected order is:

1. define the provider-neutral core contract
2. refactor the engine to consume conversation items and provider-neutral tool events
3. promote tool schema generation into the registry
4. implement the `Responses API` adapter for text plus tool calls
5. persist conversation items and summaries
6. build working-context selection and compaction
7. harden permission profiles and runtime guardrails
8. add restart recovery and deployment-level smoke coverage
9. add Anthropic adapter against the same core contract

## Acceptance Criteria

This stage is complete when all of the following are true:

- the daemon runs locally as a long-lived process
- a `Responses API` provider can stream assistant text through the runtime
- the model can call local tools and continue after receiving tool results
- the same session can sustain multiple turns with preserved context
- long conversations compact older history and remain usable
- restarts preserve session history and summaries
- startup works with only documented environment variables and no manual directory repair
- the runtime can be verified through automated tests plus a local smoke run

## Non-Goals For This Stage

- full Claude Code feature parity
- interactive approval UI
- remote multi-user tenancy
- automatic service packaging
- provider-hosted tool integration as a required capability

## Risks and Tradeoffs

- `Responses API` support gives the fastest path to the user's deployment target, but it means Anthropic parity comes later.
- `trusted_local` is powerful enough for real work, but requires strict runtime limits to avoid destructive loops.
- conservative compaction is safer than aggressive compaction, but may use more tokens in early versions.
- local deployability is prioritized over polished service-management features.

## Recommendation

Proceed with a provider-neutral core and a `Responses API` first adapter. This gives the shortest path to a locally deployable, tool-capable, multi-turn runtime without locking the project into one vendor protocol.
