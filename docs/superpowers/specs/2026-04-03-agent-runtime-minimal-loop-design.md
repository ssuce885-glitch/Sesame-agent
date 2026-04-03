# go-agent Minimal Runtime Loop Design

## Summary

This document defines the first durable implementation target for `go-agent`: a minimal but extensible agent runtime that supports session management, persistent event streams, streaming model output, and provider-backed tool loops.

The initial delivery is intentionally staged:

1. Session/event spine: `create session -> submit turn -> persist events -> SSE replay/live stream`
2. Unified model streaming contract inside `internal/model`
3. Real provider adapters for `Anthropic` and `OpenAI-compatible`
4. Streaming tool-call loop on top of the unified model contract

The design explicitly uses `E:\project\claude-code-2.1.88` as a reference implementation source for runtime patterns, but does not attempt to clone Claude Code's full product surface. We are extracting the runtime kernel and reimplementing it as a Go daemon with HTTP/SSE APIs.

## Goals

- Provide a durable daemon-native runtime instead of a terminal-first CLI runtime.
- Establish a stable internal event model that can back both persistence and SSE.
- Support streaming output from multiple model providers behind one internal interface.
- Preserve clean boundaries so future work can add providers, tools, permissions, memory behavior, and task orchestration without rewriting the runtime core.

## Non-Goals

- Rebuild Claude Code's REPL, Ink UI, slash commands, or command ecosystem.
- Implement every Claude Code permission rule, fallback, or transcript repair mechanism in the first pass.
- Support every provider-specific feature up front.
- Implement parallel tool execution in the first streaming loop.

## Reference Scope from Claude Code

The implementation should keep consulting Claude Code source during execution, especially these areas:

- Provider selection and client construction
  - `recovered/src/utils/model/providers.ts`
  - `recovered/src/services/api/client.ts`
- Streaming query loop and tool-call handling
  - `recovered/src/query.ts`
  - `recovered/src/QueryEngine.ts`
- Tool execution streaming and progress/result pairing
  - `recovered/src/services/tools/toolExecution.ts`

We are borrowing design ideas, not byte-for-byte behavior. The Go implementation should stay smaller, stricter, and daemon-oriented.

## Architecture

The runtime is split into five core layers:

### 1. API Layer

The HTTP layer owns request validation, route wiring, and translating durable events into SSE.

Target routes:

- `POST /v1/sessions`
- `POST /v1/sessions/{id}/turns`
- `GET /v1/sessions/{id}/events?after=<seq>`

The route surface should align with the README examples and present a session-scoped API.

### 2. Session Layer

The session manager owns in-memory runtime state for active sessions and active turns, including interruption/cancellation behavior.

Responsibilities:

- register a newly created session
- start a turn run for a session
- cancel any active turn before starting a replacement turn
- expose lightweight runtime state for observability

### 3. Engine Layer

The engine owns a single turn lifecycle:

- emit turn start events
- build model request input
- consume provider stream events
- translate provider events into durable runtime events
- execute tools when a complete tool call is emitted
- append tool results to the next model request cycle
- emit terminal completion/failure/interruption events

The engine should not contain provider-specific protocol code.

### 4. Model Layer

The model layer exposes one internal streaming interface regardless of provider.

This replaces the current "single `Next()` response" shape with a streaming contract.

### 5. Persistence and Streaming Layer

SQLite remains the durable source of truth for sessions, turns, and events.

The event bus remains the in-process fanout path for live subscribers.

SSE should be built from both:

- historical replay from SQLite after `after=<seq>`
- live events from the in-process bus

This makes reconnect/resume behavior reliable.

## Unified Model Contract

The most important design choice is introducing one internal provider-neutral stream protocol.

### Request Shape

The request supplied to a model client should include:

- session-scoped metadata needed for logging or provider headers
- user message / conversation input for the turn
- prior tool results for the current loop
- exposed tool schema definitions
- model/provider selection

The first version can stay small and only include fields actually consumed by the first two providers.

### Stream Event Shape

The internal stream should normalize all providers into events in this family:

- `text_delta`
- `tool_call_start`
- `tool_call_delta`
- `tool_call_end`
- `message_end`
- `usage`
- `error`

This event vocabulary is intentionally smaller than Claude Code's internal message/update system. It is sufficient for:

- engine-driven event persistence
- SSE forwarding
- deterministic tool execution boundaries
- future provider expansion

### Why Streaming First

Streaming is required in the first design, not added later, because:

- SSE is a first-class API surface
- tool-call boundaries should emerge from the provider stream
- future progress events and cancellations become simpler if the engine is already stream-native

This follows Claude Code's architecture direction in `query.ts`, but our implementation should avoid carrying over unrelated UI concerns.

## Provider Strategy

The first implementation supports:

- `Anthropic`
- `OpenAI-compatible`

### Provider Selection

Provider selection should be explicit in configuration and request construction, rather than buried in engine logic.

Suggested direction:

- add a provider kind enum/string in config
- allow provider-specific base URL / API key / model fields
- centralize provider construction in `internal/model`

### Anthropic Adapter

The Anthropic adapter should:

- translate internal tool schemas into Anthropic tool definitions
- read streaming text and tool-use deltas from the provider stream
- emit normalized `StreamEvent` values

### OpenAI-compatible Adapter

The OpenAI-compatible adapter should:

- target APIs that expose Chat Completions or Responses-style streaming with tool/function calling
- normalize streaming chunks into the same `StreamEvent` vocabulary

We should not over-generalize the first version for every OpenAI ecosystem quirk. The goal is "works for a standard OpenAI-compatible endpoint with tool calling," not universal compatibility on day one.

## Tool Loop Rules

Claude Code treats tool-use / tool-result pairing as a strict invariant. We should preserve that principle from the beginning.

### First-Version Rules

- Tool execution is serial only.
- The engine executes a tool only after receiving a complete `tool_call_end`.
- A tool result is fed back to the next model loop iteration only after execution fully completes.
- Interrupted or failed turns must not leave an implied completed tool result.

### Why This Matters

This avoids the hardest class of transcript corruption:

- orphaned tool results
- duplicated tool call IDs
- mismatched tool result ordering

We do not need Claude Code's full recovery/tombstone machinery yet, but we do need the invariant that each executed tool call is accounted for cleanly.

## Event Model

`types.Event` remains the durable event envelope. The event catalog should grow to support the turn lifecycle cleanly.

Minimum first-pass event families:

- `turn.started`
- `assistant.started`
- `assistant.delta`
- `assistant.completed`
- `tool.started`
- `tool.completed`
- `turn.completed`
- `turn.failed`
- `turn.interrupted`

Optional but prepared-for-next:

- `tool.progress`
- `permission.requested`
- `context.compacted`
- provider usage/cost events

The model layer emits provider stream events. The engine converts them into `types.Event` records suitable for storage and external subscribers.

## API and Persistence Flow

### Create Session

`POST /v1/sessions`

Flow:

- validate request body
- create `types.Session`
- persist session to SQLite
- register session in `session.Manager`
- return created session payload

### Submit Turn

`POST /v1/sessions/{id}/turns`

Flow:

- validate session existence
- create/persist `types.Turn`
- hand runtime control to `session.Manager`
- session manager starts engine turn execution asynchronously
- return accepted turn metadata immediately

### Stream Events

`GET /v1/sessions/{id}/events?after=<seq>`

Flow:

- read historical events from SQLite using `after`
- write replay events to SSE
- subscribe to the in-process bus
- forward live events until disconnect or context cancellation

This design makes SSE resumable and operationally friendly for external clients.

## Error Handling

First-version behavior should be explicit and conservative:

- provider construction errors fail fast at startup or request preparation time
- provider streaming errors become `turn.failed`
- context cancellation becomes `turn.interrupted`
- unknown tool names become tool execution failure and then `turn.failed`
- permission `ask` can remain unimplemented, but should fail in a clearly labeled way rather than silently degrade

We should avoid hidden retries in the first pass. If we later add retry policies, they belong in the model/provider layer, not in API handlers.

## Testing Strategy

Testing should be built around the staged rollout:

### Session/Event Spine

- creating a session persists it and registers runtime state
- submitting a turn persists a turn record and starts execution
- SSE replays stored events and streams live ones

### Unified Model Stream

- fake streaming client emits text deltas and terminal events
- engine converts stream events into persisted runtime events
- interruption produces `turn.interrupted`

### Provider Adapters

- Anthropic stream chunks normalize into internal `StreamEvent` values
- OpenAI-compatible stream chunks normalize into the same values
- tool-call chunks are assembled correctly into one completed tool call

### Tool Loop

- a completed tool call produces `tool.started` and `tool.completed`
- tool results are supplied to the next model iteration
- incomplete tool calls do not emit synthetic success

## Delivery Order

Implementation order should be:

1. Fix HTTP routes to match the intended session-scoped API.
2. Persist sessions, turns, and events through the real handlers.
3. Make SSE replay history plus live bus events.
4. Replace `model.Next()`-style runtime usage with a streaming model contract.
5. Keep a fake streaming provider for integration tests.
6. Add Anthropic provider adapter.
7. Add OpenAI-compatible provider adapter.
8. Reconnect tool execution on top of the streaming engine loop.

## Design Constraints

- Keep provider-specific protocol code inside `internal/model`.
- Keep engine logic provider-neutral.
- Keep API handlers thin and persistence-focused.
- Prefer stable event semantics over rich but premature features.
- Continue using Claude Code source as a design reference while keeping the Go service implementation minimal.

## Open Decisions Intentionally Deferred

These are deferred on purpose and should not block the first implementation plan:

- advanced provider retry/fallback policies
- streaming tool execution with concurrent tool runs
- full permission approval round-trips
- prompt compaction and transcript repair logic
- rich usage/cost accounting

The first implementation should create seams for these, not implement them all.
