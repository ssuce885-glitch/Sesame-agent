# Provider-Native Context Compaction Design

Date: 2026-04-04
Branch: `feature/minimal-runtime-loop`
Status: Draft approved in conversation, written for user review

## Goal

Extend the deployable responses runtime with Claude Code-inspired automatic context compaction that:

- works for long-running multi-turn sessions with tool calling
- preserves recent working context quality
- reduces repeated prompt cost through provider-native caching when the provider supports it
- degrades cleanly to local-only compaction when the provider does not support native caching
- keeps provider-specific cache semantics out of the engine core

This design does not replace the earlier deployable runtime design. It refines the context-management layer so the runtime can support cached microcompact, rolling compaction, and future provider adaptation.

## Current Gaps

The current branch already has a neutral conversation contract, SQLite-backed conversation items and summaries, and a working context manager. It does not yet have Claude Code-grade rolling compaction behavior.

Known gaps in the current branch:

- compaction is only attempted when no summaries exist yet
- `cmd/agentd/main.go` does not yet wire a production compactor into the runtime path
- `MaxEstimatedTokens` is configured but not actually used to drive working-set selection
- the existing compaction flow behaves like a one-time summary bootstrap, not a rolling policy
- no provider-native cache state is stored in SQLite
- `openai_compatible` only maps basic Responses-style request fields and does not carry cache references such as `previous_response_id`

The result is that the runtime can loop, call tools, and persist history, but it cannot yet do provider-aware rolling compression.

## Design Principles

### 1. One compaction core, many provider adapters

We should not build a separate compaction system for Ark, Anthropic, MiniMax, and every future provider. The runtime owns one generic compaction policy. Providers only translate that policy into provider-native cache operations.

### 2. Native cache is an optimization layer, not the source of truth

SQLite remains the authoritative local record for:

- conversation history
- compaction boundaries
- summaries
- active cache generation
- recovery after restart

Provider caches may expire, rotate, or fail. The runtime must still be able to rebuild a usable prompt from local state.

### 3. Cache-aware request shaping matters as much as summary quality

Claude Code-like behavior is not only about summary text. It also depends on keeping the stable prefix stable, moving volatile content later in the prompt, and rotating cache generations deliberately instead of mutating the middle of a cached prefix.

### 4. Capability-based adaptation beats provider-name branching

Future providers should plug in by declaring what cache primitives they support. The engine should ask for capabilities, not hardcode behavior around vendor names.

## Scope

This design covers:

- cached microcompact
- rolling session compaction
- full compaction and boundary rotation
- provider-native cache capability abstraction
- Ark Responses API mapping for `openai_compatible`
- required SQLite state additions
- fallback behavior for providers without native cache support

This design does not yet cover:

- implementation details for a new summary model
- semantic memory extraction changes beyond current lightweight memory refs
- UI for inspecting cache state
- product-level cache analytics dashboards

## Architecture

The compaction system is split into three layers.

### Layer 1: Local Compaction Core

The engine owns the generic logic for:

- estimating context growth
- deciding when to microcompact
- deciding when to do rolling compaction
- generating structured summaries
- defining compaction boundaries
- rebuilding the next working set
- choosing whether a provider cache generation must rotate

This layer never emits provider-specific request fields directly.

### Layer 2: Provider Cache Capability Adapter

Each provider exposes a capability profile and a small set of cache operations.

Required capability questions:

- does the provider support native rolling session cache
- does the provider support native prefix cache
- can tool-call and tool-result content participate in the cache
- can older cache references still be reused after a new turn
- does the cache reference rotate on update or stay stable
- can a cache entry be deleted explicitly
- does expiry use fixed `expire_at`, sliding TTL, or provider-managed eviction

Required adapter operations:

- build initial cache-backed request
- continue from an existing cache reference
- create a new cache generation after compaction
- expose cache usage data such as cached tokens
- report whether a failed request should retry without native cache

### Layer 3: Provider Wire Mapping

This is the existing provider adapter layer, extended with cache-specific request and response fields.

Examples:

- Ark Responses adapter maps local cache decisions to `store`, `caching`, and `previous_response_id`
- Anthropic adapter maps local cache decisions to Anthropic-native cache-aware message shaping
- future MiniMax adapter can implement the same interface if equivalent primitives exist

## Compaction Modes

The runtime will support three compaction modes in increasing order of disruption.

### 1. Cached Microcompact

Purpose:

- reduce prompt size without creating a full durable summary
- preserve cache-friendly stable prefix structure
- trim or replace bulky old tool outputs before a turn crosses a larger threshold

Typical targets:

- large tool outputs that are no longer decision-critical
- repeated shell output blocks
- old file contents that have already informed later decisions

Behavior:

- keep the semantic existence of the old tool step
- replace oversized raw payloads with a smaller local representation
- mark the item as microcompacted in local metadata
- keep recent tool interactions uncompressed

Microcompact is local-first and cache-aware. It does not rewrite old provider cache content in place. If the provider cannot reflect the updated compacted shape through an existing cache reference, the runtime creates a new cache generation from the compacted local state.

### 2. Rolling Session Compaction

Purpose:

- summarize an older stretch of conversation while keeping the current tail raw
- preserve recent tool-call continuity
- keep the session usable for long tool loops

Behavior:

- select an older contiguous range of items
- produce a structured summary
- persist a compaction boundary
- keep recent items after that boundary in raw form
- rebuild the next provider request from:
  - runtime instructions
  - durable summaries
  - recent raw items
  - current user turn

### 3. Full Compaction / Cache Generation Rotation

Purpose:

- establish a new stable prompt head after enough history has accumulated
- reset the provider cache around a clean compacted prefix
- prevent an old server-side cache generation from dragging along stale or bulky context

Behavior:

- write a durable compaction record
- generate a new compact summary package
- build a new cacheable prefix
- create a new provider cache generation
- switch the session head to that new generation
- keep the previous generation for recovery or controlled fallback until expiry or cleanup

## Provider Capability Model

The runtime will treat native cache support as a declared capability, not as an inherent property of `openai_compatible`.

### Capability Levels

#### `none`

Provider has no native cache support that the runtime can rely on.

Runtime behavior:

- use local summaries only
- send full rebuilt prompt every turn
- do not persist provider cache references

#### `native_prefix_only`

Provider can cache a stable prefix but does not offer a rolling session primitive.

Runtime behavior:

- cache system instructions, tool schema, and compact summaries as a prefix
- send recent raw tail on each turn
- rotate prefix generation after full compaction

#### `native_session_and_prefix`

Provider supports both:

- a rolling session-like cache reference
- a stable prefix-like cache reference

Runtime behavior:

- keep a rolling session head for normal turns
- create or rotate a prefix generation after major compaction
- rebuild the session head from the new prefix generation when needed

### First-Class Capability Targets

#### Ark Responses through `openai_compatible`

This is the first deployment target.

Supported primitives from the approved design:

- explicit session cache
- explicit prefix cache
- tool-call information in cached content
- rotating history references through `previous_response_id`
- explicit deletion
- fixed expiry via `expire_at`
- cached token usage visibility

This means Ark can support provider-native cached microcompact and rolling compaction in the first release.

#### Anthropic

Anthropic remains the next provider-native cache profile after Ark, but the exact wire format differs from Ark. The engine will treat Anthropic as another provider-native cache profile rather than special-case it inside the runtime core.

The required product behavior is:

- preserve cache-stable prefixes
- keep recent conversational tail raw
- rotate compacted generations deliberately
- avoid binding the engine to Anthropic-specific message blocks

#### Future providers such as MiniMax

Future providers should be onboarded by implementing the same capability adapter. If they expose equivalent session or prefix primitives, they can get near-native behavior quickly. If not, they will still inherit the local compaction core and degrade cleanly.

## Ark Responses Mapping

For the user's actual deployment target, `openai_compatible` should gain an Ark-specific cache profile while keeping the provider name stable.

### Why keep `openai_compatible`

The runtime already routes many SDK-shaped providers through `openai_compatible`. Changing the provider name to `ark` would create unnecessary churn in the current codebase and configuration.

Instead:

- keep `openai_compatible` as the provider family
- add a cache profile or capability profile under it
- default to `none` unless the backend is explicitly configured as Ark Responses or another supported cache-native backend

### Ark Session Cache Flow

Normal multi-turn loop:

1. first request starts with `store=true` and `caching: {"type": "enabled"}`
2. provider returns a response ID
3. next turn sends `previous_response_id` set to that ID
4. if the turn should continue the rolling cache, send `caching: {"type": "enabled"}` again
5. provider returns a new response ID
6. runtime stores that new ID as the active session head

Important property:

- the active session reference rotates every successful rolling turn

### Ark Prefix Cache Flow

Stable prefix creation:

1. build the compacted stable head
2. send it once with `store=true` and `caching: {"type": "enabled", "prefix": true}`
3. receive the created response ID
4. store that ID as the active prefix generation
5. later turns may reference it through `previous_response_id` without recreating the prefix

Important properties:

- prefix cache creation requires at least 256 input tokens
- prefix cache is appropriate for stable instructions, tool schema, compact summaries, and other repeatable static prompt segments

### Ark Cache Rotation Rule

When microcompact or full compaction changes the stable head materially, the runtime must create a new provider cache generation instead of trying to preserve the old one.

That means:

- recent tail changes alone do not force prefix rotation
- summary package changes do force prefix rotation
- system/tool schema changes do force prefix rotation

## Local Persistence Model

The runtime needs additional durable state beyond `conversation_items` and `conversation_summaries`.

### 1. `conversation_compactions`

Purpose:

- record every microcompact, rolling compaction, and full compaction boundary

Recommended fields:

- `id`
- `session_id`
- `kind` (`micro`, `rolling`, `full`)
- `generation`
- `start_position`
- `end_position`
- `summary_payload`
- `reason`
- `provider_profile`
- `created_at`

### 2. `provider_cache_entries`

Purpose:

- track all provider-native cache references created by the runtime

Recommended fields:

- `id`
- `session_id`
- `provider`
- `capability_profile`
- `cache_kind` (`session`, `prefix`)
- `external_ref`
- `parent_external_ref`
- `generation`
- `status` (`active`, `superseded`, `expired`, `failed`, `deleted`)
- `expires_at`
- `last_used_at`
- `metadata_json`
- `created_at`
- `updated_at`

### 3. `provider_cache_heads`

Purpose:

- point the session to the currently active provider-native references

Recommended fields:

- `session_id`
- `provider`
- `capability_profile`
- `active_session_ref`
- `active_prefix_ref`
- `active_generation`
- `updated_at`

This separate head record avoids expensive scans when rebuilding the next request.

## Working-Set Construction

The next provider request should be built from four logical regions.

### Region 1: Stable Prefix

Stable content that should move as little as possible:

- runtime instructions
- permission/tool policy
- tool schemas
- durable compact summaries
- other long-lived session framing

This region is the best candidate for prefix caching.

### Region 2: Rolling Tail

Recent raw items that still need full fidelity:

- most recent assistant reasoning output exposed through user-visible text
- most recent tool calls
- most recent tool results
- current unresolved thread context

This region stays outside durable summary until older.

### Region 3: Current Turn Input

- latest user message
- any user-supplied attachments or current turn inputs

### Region 4: Memory References

Short recalled memory remains light and late in the prompt. Memory is still not the same thing as compaction.

## Trigger Policy

The runtime should use deterministic local thresholds rather than waiting for provider errors.

### Required triggers

- estimated token budget threshold
- item-count threshold
- oversized-tool-result threshold for microcompact
- maximum number of rolling compactions since last full rotation

### Trigger order

1. try cached microcompact for obviously oversized stale payloads
2. if still over threshold, perform rolling session compaction
3. if the stable head has drifted too far or summary structure changed enough, rotate to a new full cache generation

This ordering gives the cheapest and least disruptive reduction first.

## Failure and Fallback Rules

### Provider cache failure

If a cache-native request fails due to expired or invalid provider references:

- mark the affected cache entry as failed or expired
- rebuild the prompt from local state
- retry without the broken reference
- if needed, create a fresh provider-native generation

### Provider without native cache support

If capability is `none`:

- keep all local compaction behavior
- skip provider cache creation entirely

### Over-budget after compaction

If the prompt is still too large after the maximum allowed compaction passes:

- fail the turn cleanly
- emit a clear event explaining that compaction budget was exhausted
- do not enter an unbounded retry loop

## Observability

Each turn should expose enough metadata for debugging cache behavior.

Desired runtime metadata:

- whether microcompact ran
- whether rolling compaction ran
- whether a new provider cache generation was created
- active capability profile
- active generation number
- provider cached-token usage when available

This should flow into:

- structured logs
- SQLite metadata
- non-sensitive status or debug endpoints when useful

## Testing Strategy

### Unit tests

- trigger selection for microcompact vs rolling compaction vs full rotation
- capability-driven request construction
- cache-head updates after successful turns
- fallback when a provider cache reference expires

### Provider adapter tests

Ark Responses:

- initial session cache creation
- rolling session update with new `previous_response_id`
- prefix cache creation and reuse
- full compaction rotation to a new generation
- cached-token usage extraction

### Persistence tests

- compaction records are inserted correctly
- provider cache heads and entries survive restart
- superseded generations remain queryable until cleanup

### Recovery tests

- restart with active cache heads
- rebuild after expired provider-native refs
- continue a session from durable local state only

## Acceptance Criteria

This design is implemented successfully when:

- long sessions no longer rely on one-time summary bootstrap behavior
- the runtime can perform rolling compaction multiple times per session
- Ark-backed `openai_compatible` can use provider-native session and prefix caching
- cached microcompact works without forcing a separate compaction engine per provider
- local SQLite state is sufficient to recover from provider cache expiry
- providers without native cache support still benefit from the same local compaction core
- future providers can onboard by adding a capability adapter instead of rewriting compaction policy

## Recommendation

Implement one shared local compaction core and one provider cache capability interface. Ship Ark Responses as the first fully native cache profile under `openai_compatible`, keep Anthropic on the same interface as the next native profile, and make all future providers opt into the same interface.

This gives the project the shortest path to Claude Code-style automatic compression on the user's real deployment target without locking the runtime to a single vendor's cache model.
