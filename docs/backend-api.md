# Sesame Backend API — Complete Reference

> Generated 2026-04-29. Covers `internal/api/http/`.

## Overview

| Property | Detail |
|----------|--------|
| Server | Go `net/http` + `http.ServeMux` (Go 1.22+ enhanced patterns) |
| Session binding | `X-Sesame-Context-Binding` header + `?binding=` query param |
| Session role | `X-Sesame-Session-Role` header (default: `main_parent`) |
| SSE | `text/event-stream` with 15s keepalive |
| SPA fallback | `/` serves static console files; unknown paths → `index.html` |

**No traditional auth middleware.** Session resolution uses headers + context enrichment.

---

## Route Tree (41 endpoints)

### Status
```
GET  /v1/status  →  {status, provider, model, permission_profile, provider_cache_profile, pid}
```

### Workspace
```
GET  /v1/workspace  →  {id, name, workspace_root, provider, model, permission_profile, ...}
```

### Session Management

All session routes require `X-Sesame-Context-Binding` header and `X-Sesame-Session-Role` header.

```
POST /v1/session/ensure                      Body: {workspace_root, session_role?}
     → Session (JSON)

POST /v1/session/turns                       Body: {client_turn_id?, message}
     → 202 Turn (JSON)

POST /v1/session/interrupt                   Body: none
     → 202 or 204 if nothing to interrupt

GET  /v1/session/events?after=<int64>       → SSE stream (text/event-stream)
     Replays stored events after `after`, then live bus subscription.
     15s keepalive ticker.

GET  /v1/session/timeline                    → {blocks: TimelineBlock[], latest_seq, queued_report_count, queue}

GET  /v1/session/history                     → {entries: HistoryEntry[], current_head_id?}

POST /v1/session/history/load                Body: {head_id}
     → ContextHead (JSON)

POST /v1/session/reopen                      Body: none
     → ContextHead (JSON). Creates new context head with source=reopen.

GET  /v1/session/checkpoints?limit=<int>    → {checkpoints: FileCheckpoint[]}

GET  /v1/session/checkpoints/:id/diff        → {checkpoint, parent?, diff: string}

POST /v1/session/checkpoints/:id/rollback    → {status: "rolled_back", checkpoint?}
```

### Workspace Reports
```
GET  /v1/reports  →  {workspace_root, items: ReportDeliveryItem[], queued_count}
```

### Runtime Graph
```
GET  /v1/runtime_graph  →  {workspace_root, graph: {runs, plans, tasks, tool_runs, worktrees, diagnostics}}
```
Includes runtime diagnostics for interrupted turns (reasons: `task_session_replay_unsupported`, `unmapped_session`).

### Metrics

All three endpoints support: `?session_id`, `?from`, `?to` (RFC3339), `?page` (default 1), `?page_size` (default 20, max 200).

```
GET  /v1/metrics/overview     →  {input_tokens, output_tokens, cached_tokens, cache_hit_rate}
GET  /v1/metrics/timeseries   →  {bucket, points: [{bucket_start, input_tokens, ...}]}
     Extra: ?bucket=hour|day (default: day)
GET  /v1/metrics/turns        →  {items, page, page_size, total_count}
```
`/v1/metrics/turns` enriches each row with a session title derived from the first user message.

### Memory
```
GET  /v1/memory/candidates  →  {"items":[]}  (hardcoded stub)
```

### Reporting
```
GET  /v1/reporting/overview?session_id=<id>  →  {child_agents, output_contracts, report_groups, child_results, digests}
```
If `session_id` provided, filters child_agents, report_groups, child_results, digests by session.

### Cron
```
GET    /v1/cron?workspace_root=<root>  →  {jobs: ScheduledJob[]}
GET    /v1/cron/:id                    →  ScheduledJob
POST   /v1/cron/:id/pause              →  ScheduledJob
POST   /v1/cron/:id/resume             →  ScheduledJob
DELETE /v1/cron/:id                    →  204
```

### Roles
```
GET    /v1/roles                    →  {roles: RoleSummary[], diagnostics: Diagnostic[]}
POST   /v1/roles                    →  201 RoleResponse  (body: UpsertInput)
GET    /v1/roles/:role_id           →  RoleResponse
PUT    /v1/roles/:role_id           →  RoleResponse  (body: UpsertInput, forces RoleID)
DELETE /v1/roles/:role_id           →  204
GET    /v1/roles/:role_id/versions  →  {versions: RoleResponse[]}
```

Role error mapping: `IsInvalidInput` → 400, `IsNotFound` → 404, `IsConflict` → 409, others → 500.

`RoleSummary`: `{role_id, display_name, description, skills[], version, policy?, budget?}`
`RoleResponse`: RoleSummary + `prompt`

### Automations

Uses Go 1.22 `{automation_id}` path params.

```
GET    /v1/automations?workspace_root=&state=&limit=  →  {automations: AutomationSpec[]}
POST   /v1/automations                                 →  AutomationSpec  (body: ApplyAutomationRequest)
GET    /v1/automations/:automation_id                  →  AutomationSpec
PATCH  /v1/automations/:automation_id                  →  AutomationSpec  (body: ControlAutomationRequest)
DELETE /v1/automations/:automation_id                  →  204
POST   /v1/automations/:automation_id/pause            →  AutomationSpec
POST   /v1/automations/:automation_id/resume           →  AutomationSpec
POST   /v1/automations/:automation_id/install          →  AutomationWatcherRuntime
POST   /v1/automations/:automation_id/reinstall        →  AutomationWatcherRuntime
GET    /v1/automations/:automation_id/watcher          →  AutomationWatcherRuntime
POST   /v1/triggers/emit                               →  TriggerEvent  (body: TriggerEmitRequest)
POST   /v1/triggers/heartbeat                          →  AutomationHeartbeat  (body: TriggerHeartbeatRequest)
```

Automation errors: `AutomationValidationError` → 400 (JSON), others → 500.

### Console Static
```
GET/HEAD  /  (and all sub-paths)  →  Static files from ConsoleRoot or SPA fallback (index.html)
```

---

## Session Binding & Resolution

### Headers
| Header | Purpose | Default |
|--------|---------|---------|
| `X-Sesame-Context-Binding` | Session binding key | `terminal:default` |
| `X-Sesame-Session-Role` | Session role | `main_parent` |

Also accepted as query param: `?binding=...`

### Resolution Flow
```
1. resolveRequestBinding(r) → X-Sesame-Context-Binding header or ?binding=
2. resolveRequestedSessionRole(r, fallback) → X-Sesame-Session-Role header
3. For scoped routes: resolveCurrentSessionID → EnsureRoleSession(workspaceRoot, role)
4. For /v1/session/ensure: requires workspace_root in body, rejects specialist_role_id
```

---

## Dependencies (Dependencies struct)

```go
Bus             // Publish/Subscribe event bus
Store           // DB store (many interfaces)
Manager         // Session lifecycle + turn submission
Scheduler       // Cron job scheduler
Automation      // Automation CRUD + triggers
RoleService     // Role CRUD + version history
FileCheckpoints // Git-based file checkpoints
Status          // Server status info
ConsoleRoot     // Path to console static files
WorkspaceRoot   // Workspace root path
```

### Ad-hoc Store Interfaces

The Store is type-asserted at runtime to various narrow interfaces:

| Interface | Used By | Methods |
|-----------|---------|---------|
| `workspaceReportsStore` | Reports handler | `ListWorkspaceReportDeliveryItems`, `CountQueuedWorkspaceReportDeliveries` |
| `reportingStore` | Reporting handler | `ListChildAgentSpecs`, `ListReportGroups`, `ListChildAgentResults`, `ListDigestRecords` |
| `reportDeliveryStore` | Timeline | `ListReportDeliveryItems`, `CountQueuedReportDeliveries` |
| `contextHeadTimelineStore` | Timeline | `GetCurrentContextHeadID`, `ListConversationTimelineItemsByContextHead` |
| `runtimeGraphStore` | Runtime graph | `ListRuntimeGraph`, `ListRuntimeGraphForWorkspace` |
| `runtimeGraphSessionStore` | Runtime graph | `ListSessions`, `ListSessionEvents` |
| `metricsReader` | Metrics | `GetMetricsOverview`, `ListMetricsTimeseries`, `ListMetricsTurns` |
| `turnInterruptStore` | Turns | `GetSession`, `TryMarkTurnInterrupted` |
| `currentContextHeadStore` | Turns | `GetCurrentContextHeadID` |
| `runtimeTimelineEventStore` | Turns | `AppendEventWithState` |
| `fileCheckpointStore` | Checkpoints | `GetFileCheckpoint`, `ListFileCheckpointsBySession`, `GetLatestFileCheckpoint` |
| `contextHistoryStore` | History | `ListContextHistory`, `CreateReopenContextHead`, `LoadContextHead` |
| `queueSummaryProvider` | Timeline state | `QueuePayload(sessionID)` |

---

## SSE Event Stream Protocol

**Endpoint**: `GET /v1/session/events?after=<seq>&binding=<binding>`

**Content-Type**: `text/event-stream`
**Headers**: `Cache-Control: no-cache`, `Connection: keep-alive`

### Stream Lifecycle
```
1. Send stored events with seq > afterSeq (catch-up)
2. Send SSE keepalive (event: keepalive, data: {session_id, last_seq, time})
3. Enter loop:
   - New event from bus subscription → write SSE frame
   - 15s keepalive ticker
   - Context cancellation → close
```

### SSE Frame Format
```
id: <event_id>
event: <event_type>
data: <json_payload>

```

---

## SSE Event Type Reference

Events are defined in `internal/types/events.go`. Key events consumed by both
the TUI and web console:

| Event Type | Payload | Description |
|-----------|---------|-------------|
| `turn.started` | `{turn_id}` | A new turn has begun |
| `user_message` | `{text, turn_id}` | User message recorded |
| `assistant.started` | `{turn_id}` | Assistant response beginning |
| `assistant.delta` | `{turn_id, delta, index}` | Streaming text chunk |
| `assistant.completed` | `{turn_id, usage?}` | Assistant response done |
| `tool.started` | `{turn_id, tool_call_id, tool_name, input}` | Tool execution started |
| `tool_run.updated` | various | Live tool progress |
| `tool.completed` | `{turn_id, tool_call_id, status, result_preview, is_error}` | Tool execution finished |
| `system.notice` | `{text}` | System notice |
| `turn.failed` | `{turn_id, error}` | Turn execution failed |
| `turn.interrupted` | `{turn_id, reason}` | Turn was interrupted |
| `turn.completed` | `{turn_id}` | Turn finished successfully |
| `context_head_summary.started` | — | Context head summarization started |
| `context_head_summary.completed` | `{context_head_id}` | Summarization completed |
| `context_head_summary.failed` | `{error}` | Summarization failed |
| `context.compacted` | `{context_head_id}` | Context was compacted |
| `child_task_dispatched` | `{task_id, target_role}` | Role task delegated |
| `report_received` | `{task_id, content}` | Delegation report returned |
