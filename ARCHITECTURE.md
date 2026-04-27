# Sesame Architecture

## Design Goals

**Sesame is a local-first, multi-role, concurrent long-task personal agent runtime:**
Main Agent is the user channel; Role Agents are long-lived executors; Scheduler handles wake-up;
Reporting delivers results back; Memory captures durable knowledge; Context and Compaction
manage the per-invocation model window.

1. **Main Agent is the user channel and orchestrator**
   It understands the user, creates/manages role tasks, reviews reports, and answers the user.
   It should not swallow background task details into its own context. Permission profiles are
   an internal runtime capability switch; the default runtime profile is
   `trusted_local` (full local permissions). There is no user-facing approval/deny workflow.

2. **Role Agents are persistent actors**
   Each role (ops, social, code, research, etc.) owns its identity, memory, permissions,
   schedule, and context boundary. Roles are not one-shot function calls.

3. **Multi-task concurrency with context isolation**
   Unrelated work can run concurrently but must not share prompt history. A Reddit scan log
   must not leak into the Xiaohongshu role's context.

4. **Memory is scoped and governed**
   Memory is not chat summarization. Scopes: head, role, workspace, global. Memory defaults
   to role-private; only explicitly shared or promoted entries cross role boundaries.

5. **Reporting is the role output bus**
   All background role results become standard Reports, delivered to the main agent,
   digest, or another role through a single pipeline.

6. **Scheduler is the background runtime layer**
   Cron, event triggers, dedup, retry, skip_if_running, and timeout belong to the scheduler.
   Main Agent does not need to stay online waiting for results.

7. **Compaction is window management, not memory**
   Compaction keeps the context window runnable. Long-term memory is produced through a
   separate promotion mechanism, not by dumping compaction summaries into memory.

---

## Core Concepts

### Agent

**Agent** is the generic term for any execution participant. There are two concrete kinds:

| Kind | Role | Lifespan |
|------|------|----------|
| **Main Agent** | User-facing channel, orchestrator, report consumer | Persistent per workspace |
| **Role Agent** | Specialist executor, owns a specific domain/function | Persistent, created per role spec |

Main Agent is always `main_parent`. Role Agents are loaded from `roles/<role_id>/`.

**[Current]** Each role gets a persistent session + context head. Delegated role tasks resolve
their `TargetRole` to the canonical main-parent session or to the specialist role session.
Temporary `task_session_*` sessions are only for explicit no-target agent tasks
(`TargetRole == ""`).

### Session

A **Session** owns the runtime state for one agent: active turn, queue, context heads,
permission profile. Each agent has exactly one session. Sessions are persisted in the
`sessions` table.

**Key constraint:** Main Agent session and Role Agent sessions are independent — they do not
share context or queue.

### Session Binding

`sessionbinding` is still active runtime infrastructure, not dead compatibility residue.
It binds request/runtime context identity (`X-Sesame-Context-Binding`) and is used to derive
current-head metadata keys for `runtime_metadata`.

`workspace_session_bindings` remains the canonical mapping for
`workspace + role/specialist role -> persistent session ID`.

**[Current debt]** Responsibilities around binding metadata and session resolution should be
narrowed; do not delete `sessionbinding` without replacing current-head metadata binding.

### Turn

A **Turn** is one model invocation cycle: user input → model reply → tool loop → completion.
Turns are the unit of execution within a session. A turn has states:

```
created → building_context → model_streaming → tool_dispatching → ... → completed
                                                            ↓
                                                  tool_running → loop_continue → ...
```

Two turn kinds exist today: `user_message` (main agent input) and `report_batch` (role
results injected into main agent).

### Context

**Context** is the working set assembled for each turn: system prompt, conversation history,
compacted summaries, context-head summary, workspace prompt. Context is ephemeral —
assembled per-turn, not stored as a single blob.

The **Context Head** is the anchor point for a session's conversation timeline. Browsing
history is modeled as switching between context heads (branch-style). Each turn belongs to
one context head.

### Memory

**Memory** is long-lived, structured information that persists across sessions and turns.
Memory is distinct from context — it is selected into context at prompt-assembly time, not
dumped wholesale.

Memory scopes (in visibility-narrowing order):

| Scope | Visibility | Description |
|-------|-----------|-------------|
| `head` | Per context-head | Ephemeral per-head state |
| `role` | Owner role only | Role-private observations, tool outcomes |
| `workspace` | Workspace-wide | Cross-role workspace knowledge |
| `global` | All workspaces | User preferences, global patterns |

Memory visibility governs cross-role access:

| Visibility | Meaning |
|-----------|---------|
| `private` | Only the owning role sees this entry |
| `shared` | Visible to all roles in the workspace |
| `promoted` | Elevated from private to workspace-visible |

Memory status tracks lifecycle:

| Status | Meaning |
|--------|---------|
| `active` | Currently in use |
| `deprecated` | No longer relevant but preserved for history |
| `superseded` | Replaced by a newer entry |

Memory kinds: `workspace_overview`, `workspace_choice`, `file_focus`, `open_thread`,
`tool_outcome`, `global_preference`.

**[Current]** Memory entries include `owner_role_id`, `visibility`, `status`, `last_used_at`,
and `usage_count` fields. Role-scoped recall filters by visibility via the unified
`ListVisibleMemoryEntries(workspaceID, roleID)` API:
- Main Agent (`roleID=""`): sees unowned + shared + promoted only
- Role Agent (`roleID="<id>"`): sees own (any visibility) + unowned + shared + promoted

**[Current]** `last_used_at` and `usage_count` are updated when a memory entry is selected
into a prompt. Context-head summaries are stored separately from memory entries.

### Compaction

**Compaction** is window management, not memory. When a conversation grows beyond constraints,
compaction:
1. Selects an item range to compact
2. Produces a structured summary
3. Replaces the source range with the summary
4. Records the boundary as a `conversation_compaction` event

The original items are not deleted — they are excluded from the default prompt assembly but
remain queryable.

Compaction kinds: `micro` (small, frequent), `rolling` (periodic window), `full` (explicit
user request).

**[Current]** Compaction boundary metadata includes `IsPreTurn` and `PreservedUserMessageCount`
for finer-grained prompt assembly control.

### Role

A **Role** is a file-backed agent spec under `roles/<role_id>/`:

```
roles/<role_id>/
├── role.yaml       # metadata: display_name, description, skills, policy
├── prompt.md       # system prompt supplement
```

Roles are loaded into a catalog at startup. Main Agent delegates work to roles via
`delegate_to_role` tool. Roles run as background tasks with their own session.

### Task

A **Task** is a managed work unit. Tasks track state (`pending → running → completed/failed/cancelled`)
and can represent:
- Background agent runs (`TaskTypeAgent`)
- Shell script executors (`TaskTypeCommand`)

Tasks are the execution substrate for role delegation — when Main Agent delegates to a role,
a task is created and the role's session picks it up.

### Report

A **Report** is the output envelope from a role execution. Reports flow through:

```
Role Run → Report Record → Report Delivery → Main Agent Report Turn / Digest / Role Target
```

Current types:
- `task_result` — direct task completion output
- `child_agent_result` — structured child agent result
- `digest` — aggregated report group delivery

**[Current]** Reports carry `TargetRoleID`, `TargetSessionID`, and `Audience` fields for
routing. Delivery states include `archived` and `action_required` in addition to the base
`queued`/`delivered`.

`report_records` + `report_deliveries` are the only source of truth for role/task output.
Older role-output queue storage is dropped by migration and has no runtime model.

### Automation

**Automation** is a watcher-driven trigger chain:

```
Role Asset Detector (watcher script) → Signal → Owner Role Task → Main Agent Report → Policy
```

Automations are role-owned assets under `roles/<role_id>/automations/<automation_id>/`.
The watcher script only detects; a match dispatches a task to the owning role's persistent
session, and the task result is delivered to the main agent through the report pipeline.


## Component Boundaries [Current]

```
┌──────────────────────────────────────────────────────────────┐
│                       Main Agent Session                      │
│                                                              │
│  ┌──────────┐   ┌──────────┐   ┌─────────────────────────┐   │
│  │   User    │   │  Engine  │   │     Tool Registry       │   │
│  │ Channel   │──→│ (RunTurn)│──→│ - delegate_to_role      │   │
│  │ (CLI/TUI) │   │          │   │ - task_create           │   │
│  └──────────┘   │          │   │ - schedule_report       │   │
│       ↑         │          │   │ - automation_query      │   │
│       │         │          │   │ - 30+ other tools       │   │
│  ┌────┴─────┐   │          │   └─────────────────────────┘   │
│  │  Reports  │   │          │                                │
│  │  Panel    │   └────┬─────┘                                │
│  └──────────┘        │                                       │
│                      │ context assembly                      │
│                      ↓                                       │
│  ┌──────────────────────────────────────────────────────┐    │
│  │  Context Layer                                        │    │
│  │  ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌────────┐ │    │
│  │  │  Recent   │ │ Compacted│ │ Visible  │ │Worksp. │ │    │
│  │  │  Items    │ │Summary   │ │ Memory   │ │Prompt  │ │    │
│  │  └──────────┘ └──────────┘ └──────────┘ └────────┘ │    │
│  └──────────────────────────────────────────────────────┘    │
└──────────────────────────────────────────────────────────────┘
                              │
                   ┌──────────┼──────────┐
                   │          │          │
                   ↓          ↓          ↓
        ┌─────────────────────────────────────────────┐
        │              Daemon / Runtime                 │
        │                                               │
        │  ┌──────────┐ ┌──────────┐ ┌──────────────┐  │
        │  │ Session  │ │  Task    │ │  Scheduler    │  │
        │  │ Manager  │ │  Manager │ │  (cron)       │  │
        │  └──────────┘ └──────────┘ └──────────────┘  │
        │  ┌──────────┐ ┌──────────┐ ┌──────────────┐  │
        │  │ Reporting│ │Automation│ │  Discord      │  │
        │  │ Service  │ │ Service  │ │  Connector    │  │
        │  └──────────┘ └──────────┘ └──────────────┘  │
        │  ┌────────────────────────────────────────┐   │
        │  │           SQLite Store                  │   │
        │  └────────────────────────────────────────┘   │
        └─────────────────────────────────────────────────┘
                              │
                   ┌──────────┘
                   ↓
        ┌──────────────────────┐
        │   Role Agent Session  │
        │   (per active role)   │
        │                       │
        │  ┌──────────────────┐ │
        │  │  Engine (RunTurn)│ │
        │  │  Role system     │ │
        │  │  prompt          │ │
        │  │  Restricted tool │ │
        │  │  set             │ │
        │  └──────────────────┘ │
        └──────────────────────┘
```


## Data Flow: Role Delegation [Current]

```
Main Agent Turn
  │
  ├─ user: "check disk usage on prod"
  │
  ├─ Engine calls delegate_to_role(target_role="ops", ...)
  │     │
  │     ├─ TaskManager.Create(task_type=agent, owner="ops")
  │     │
  │     └─ Returns task_id + CompleteTurn to Main Agent
  │
  ├─ Engine finishes the current assistant tool batch
  │
  ├─ Main Agent turn completes (no in-turn task_wait polling)
  │
  └─ Later (async):
       │
       ├─ Scheduler / TaskNotifier picks up pending task
       │   for the ops role
       │
       ├─ Persistent Role Agent Session starts turn
       │     │
       │     ├─ Context: role system prompt + task message
       │     ├─ Engine runs tool loop (restricted scope)
       │     └─ Turn completes → produces ReportRecord
       │
       ├─ ReportDelivery created
       │     │
       │     ├─ Agent report delivery: queued
       │     ├─ If Main Agent session is idle:
       │     │   inject ReportBatch turn
       │     └─ Main Agent sees report in its turn
       │
       └─ Main Agent presents result to user
```

Role sessions are persistent — one session per role, reused across tasks with a stable context
head. Delegated role turns enter `session.Manager` so work for the same role is serialized
through that role's queue. Temporary `task_session_*` execution is reserved for explicit
no-target agent tasks (`TargetRole == ""`).


## Data Ownership

| Concept | Owned By | Persisted In | Accessed By |
|---------|----------|-------------|-------------|
| Session | Session Manager | `sessions`, `turns` | Engine, HTTP API |
| Context Head | Session | `context_heads`, `conversation_items` | Engine (prompt assembly) |
| Compaction | Engine | `conversation_compactions` | Engine (prompt assembly) |
| Context Head Summary | Engine / Context layer | `context_head_summaries` | Engine (prompt assembly) |
| Memory | Memory Layer | `memory_entries` | Engine (prompt assembly) |
| Task | Task Manager | `task_records`, `runs` | Engine, Scheduler, Delegation |
| Report | Reporting Service | `report_records`, `report_deliveries` | Report batch, Engine |
| Session Binding | Runtime binding metadata | `runtime_metadata`, `workspace_session_bindings` | API, CLI, Daemon, Store |
| Automation | Automation Service | `automations`, `watchers`, `triggers` | Scheduler, Engine |
| Role | Role Service | Filesystem (`roles/<id>/`) | Engine, HTTP API |
| Workspace | Workspace Manager | `workspace_state` | Daemon |


## Schema Map

### sessions [Current]
```
id, workspace_root, system_prompt, permission_profile, state, active_turn_id,
created_at, updated_at
```

### turns [Current]
```
id, session_id, context_head_id, client_turn_id, kind, state, execution_mode,
user_message, ...
```

### context_heads [Current]
```
id, session_id, parent_head_id, source_kind, title, preview, created_at, updated_at
```

### conversation_items [Current]
```
(context_head_id, position) → role, content, type, metadata
```

### conversation_compactions [Current]
```
id, session_id, context_head_id, kind, generation, start_item_id, end_item_id,
summary_payload, metadata_json, reason, provider_profile, created_at
```
Metadata now includes `preserved_user_message_count` and `is_pre_turn` fields.

### memory_entries [Current]
```
id, scope(head|role|workspace|global), workspace_id, kind,
source_session_id, source_context_head_id, owner_role_id,
visibility(private|shared|promoted), status(active|deprecated|superseded),
content, source_refs, confidence, last_used_at, usage_count,
created_at, updated_at
```

**[Current]** `last_used_at` and `usage_count` are bumped when recalled entries are selected
for prompt injection.

### context_head_summaries [Current]
```
session_id, context_head_id, workspace_root, source_turn_id, up_to_item_id,
item_count, summary_payload, created_at, updated_at
```

Context-head summaries are rolling prompt summaries for the active context head. They are
not durable memory entries and are not part of memory recall/ranking.

### report_records [Current source]
```
id, workspace_root, session_id, source_session_id, source_role_id,
source_kind(task_result|child_agent_result|digest), source_id,
target_role_id, target_session_id, audience(user|main_agent|role|workspace),
envelope(JSON), observed_at, created_at, updated_at
```

### report_deliveries [Current source]
```
id, workspace_root, session_id, report_id, target_role_id, target_session_id,
audience(user|main_agent|role|workspace),
channel(agent_report), state(queued|delivered|archived|action_required),
observed_at, injected_turn_id, injected_at, created_at, updated_at
```

### task_records [Current]
```
id, run_id, plan_id, parent_task_id, state, payload(JSON), ...
```


## Known Concept Overlaps

| Concept A | Concept B | Problem | Status |
|-----------|-----------|---------|--------|
| Removed report queues | `report_records` / `report_deliveries` | Older role-output queues duplicated report ownership. | **Resolved.** Removed from runtime; migration drops old tables. |
| Report UI queue | ReportBatch injection | Reports views and report-batch turns now read/claim the same `report_deliveries` rows. | **Resolved.** One persisted report pipeline. |
| `memory_entries` scope | role ownership | **Resolved.** Memory now has `owner_role_id`, `visibility`, and `status` fields. Role-scoped recall filters via `ListVisibleMemoryEntries`. | Current |
| Context-head summaries | `memory_entries` | Prompt summaries and durable memory have different lifetimes and ranking rules. | **Resolved.** Runtime uses `context_head_summaries`, separate from memory entries. |
| Compaction summary | Memory | Both produce condensed representations but differ in lifetime and purpose. Should stay separate. | Current |
| `sessionbinding` | session ownership metadata | Still required for context binding and current-head metadata keys; responsibilities should narrow to metadata/binding concerns. | Current + narrowing debt |
| `automation_simple` | `delegate_to_role` | Both create background role work and deliver through `report_records` + `report_deliveries`. | **Resolved.** Simple automation dispatches owner role tasks and reports to `main_agent`. |


## Phased Implementation Plan

### Phase 1 — Foundation (in progress)

**Goal:** Multi-role memory isolation, unified reporting, stable compaction.

| Step | What | Status |
|------|------|--------|
| 1.1 | **Memory schema: add role ownership** — `owner_role_id`, `visibility`, `status`, `last_used_at`, `usage_count`. | Done |
| 1.2 | **Memory recall: filter by role** — `ListVisibleMemoryEntries(workspaceID, roleID)`. Main agent sees unowned+shared+promoted; role sees own+unowned+shared+promoted. | Done |
| 1.3 | **Compaction → Compact Boundary** — Persist boundaries with `IsPreTurn`, `PreservedUserMessageCount`. | Done |
| 1.4 | **Reporting: single report pipeline** — `report_records` + `report_deliveries` own report storage, delivery, and report-batch injection. | Done |

### Phase 2 — Runtime (planned)

| Step | What |
|------|------|
| 2.1 | Scheduler extension: `role_runs`, dedupe, skip_if_running, retry. |
| 2.2 | Role session/context lifecycle: each role gets persistent session + context head. **Done in runtime; keep docs/specs synchronized.** |
| 2.3 | Report → Memory promotion rules. |
| 2.4 | Remove `child_reports` / `pending_task_completions` storage after migration to `report_records` + `report_deliveries`. **Done.** |
| 2.5 | `last_used_at` / `usage_count` bump on memory recall. **Done.** |
| 2.6 | Context-head summary rename and memory-layer separation. **Done.** |
| 2.7 | Narrow `sessionbinding` responsibilities to context binding + current-head metadata contracts. |
