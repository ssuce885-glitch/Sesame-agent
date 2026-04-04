# Session Focus Recovery Design

Date: 2026-04-04
Branch: `feature/minimal-runtime-loop`
Status: Draft approved in conversation, written for user review

## Goal

Make the runtime reopen on the previously selected task after daemon restart, instead of automatically switching to a newly created task. Starting a new task should require an explicit user-facing switch action.

## Background

The current runtime persists sessions, turns, events, and conversation history in SQLite, but it does not persist which session the user interface was focused on. `session.Manager` only knows about sessions registered during the current process lifetime, and the existing HTTP API does not expose a list endpoint or an explicit session-selection action.

This repository continues to follow runtime patterns inspired by the Claude Code reference project at `E:\project\claude-code-2.1.88`, but the implementation stays scoped to the smaller Go daemon and its HTTP/SSE surface.

## Product Requirement

After restart:

- the interface should default to the same task/session that was selected before shutdown
- creating a new task must not steal focus from the previously selected task
- switching to a different task must be explicit

The selected task is defined as the session the user last chose to view, not merely the most recently updated session.

## Architecture

The selected session should be treated as global runtime metadata, not as intrinsic session data.

### Why Not Store It On `sessions`

The focused task is an application-wide UI/runtime concept:

- it belongs to the current daemon state
- only one session can be selected at a time
- it can change without mutating the session itself

For that reason, it should live in a small metadata table instead of on each `sessions` row.

## Persistence Model

Add a SQLite table for runtime metadata with a minimal key/value shape:

- `key`
- `value`
- `updated_at`

Use the key `last_selected_session_id` to store the current focused session.

Behavior rules:

- if the selected session exists, return it as the default task after restart
- if the key is missing, select the most recently updated session as a compatibility fallback and persist that value
- if the key points to a missing session, fall back the same way and rewrite the metadata
- if no sessions exist, expose no selected session

## Runtime Recovery

Daemon startup should recover more than interrupted turns. It should also rebuild the in-memory session registry from persisted sessions.

Startup sequence:

1. open SQLite
2. load persisted sessions ordered by update time
3. register each session back into `session.Manager`
4. recover interrupted turns as already designed
5. resolve the selected session from runtime metadata
6. if needed, compute and persist the compatibility fallback selection

This keeps `session.Manager` consistent with SQLite and makes post-restart turn submission work for existing sessions.

## HTTP API Changes

The API needs an explicit way for the interface to discover and update focus.

### `GET /v1/sessions`

Return:

- list of persisted sessions
- `selected_session_id`

Each session entry may also include `is_selected` to simplify client rendering.

### `POST /v1/sessions`

Keep current behavior for creating a new session, but do not change the selected session automatically.

### `POST /v1/sessions/{id}/select`

Set the selected session explicitly:

- validate the session exists
- persist `last_selected_session_id`
- return success plus the selected session id

This endpoint is the only write path that changes focus during normal usage.

## Session Manager Boundary

`session.Manager` should remain responsible for in-memory execution behavior:

- registered sessions
- runtime cancellation handles
- active turn ids

It should not become the source of truth for durable selected-session state. Durable selection belongs in SQLite; the manager simply needs recovered sessions registered at startup so existing sessions can continue to accept turns.

## Compatibility

Existing databases will not have runtime metadata yet.

Compatibility behavior:

- migration creates the metadata table if missing
- old data remains valid
- first startup after upgrade picks the most recently updated session if any exist
- that fallback is persisted so later restarts are stable

This avoids breaking old local state while still moving to explicit selection semantics.

## Error Handling

Failure expectations:

- if metadata lookup fails, startup should fail loudly rather than continue with ambiguous focus behavior
- if selected-session fallback computation fails, startup should fail
- selecting a nonexistent session over HTTP should return `404`
- creating a session while another session is selected must not modify the stored selection

## Testing

Required coverage:

- SQLite tests for runtime metadata read/write and fallback behavior
- startup recovery tests proving persisted sessions are re-registered into `session.Manager`
- startup tests proving old databases without metadata choose the newest session once and persist it
- HTTP tests for:
  - `GET /v1/sessions` returning `selected_session_id`
  - `POST /v1/sessions` not stealing focus
  - `POST /v1/sessions/{id}/select` changing focus

## Acceptance Criteria

This feature is complete when:

- restarting the daemon keeps the UI focused on the previously selected task
- creating a new task does not change the default selected task
- explicitly selecting another task persists across restart
- existing persisted sessions can still accept turns after restart
- older SQLite data upgrades without manual repair

## Non-Goals

- multi-user focus state
- per-browser or per-client selected-session state
- frontend-local-only focus persistence
- automatic task switching based on recency or activity after explicit selection exists

## Recommendation

Implement durable selected-session metadata in SQLite, restore all persisted sessions into `session.Manager` at startup, and expose explicit list/select HTTP routes. This is the smallest design that matches the intended Claude Code-like behavior: reopen on the same task, and switch tasks only when the user asks to.
