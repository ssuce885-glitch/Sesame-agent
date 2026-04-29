# Sesame Web Console — Architecture & Technical Reference

> Generated 2026-04-29. Covers `web/console/src/`.

## Overview

| Property | Detail |
|----------|--------|
| Framework | React 18 + TypeScript |
| Build tool | Vite |
| Router | React Router 6 (`createBrowserRouter`) |
| State | React Query (`@tanstack/react-query`) + `useReducer` for chat |
| Styling | Tailwind CSS v4 + CSS custom properties (`@theme`) |
| i18n | Custom React Context (zh-CN / en-US) |
| Testing | Vitest + React Testing Library |
| Charts | Recharts |

---

## File Inventory

```
src/
├── main.tsx                          # Entry point (11 lines)
├── App.tsx                           # Router + layout shell (256 lines)
├── index.css                         # Tailwind + design tokens (101 lines)
├── i18n.tsx                          # Locale system (309 lines)
├── vite-env.d.ts                     # Vite types (1 line)
├── api/
│   ├── client.ts                     # All HTTP fetch functions (226 lines)
│   ├── queries.ts                    # React Query hooks (218 lines)
│   ├── types.ts                      # TypeScript interfaces (396 lines)
│   └── events.ts                     # SSE reducer + useSessionEvents (493 lines)
├── components/
│   ├── Composer.tsx                  # Chat input (96 lines)
│   ├── Icon.tsx                      # 11 SVG icons (91 lines)
│   ├── MessageList.tsx               # Chat scroll container (233 lines)
│   ├── Sidebar.tsx                   # Workspace info panel (131 lines)
│   ├── blocks/
│   │   ├── UserMessage.tsx
│   │   ├── AssistantMessage.tsx
│   │   ├── ToolCall.tsx
│   │   ├── ToolCallGroup.tsx
│   │   ├── NoticeBlock.tsx
│   │   └── ErrorBlock.tsx
│   ├── roles/
│   │   ├── RoleList.tsx
│   │   ├── RoleEditor.tsx
│   │   └── RoleDiagnostics.tsx
│   └── usage/
│       ├── SummaryCards.tsx
│       └── UsageChart.tsx
├── pages/
│   ├── ChatPage.tsx                  # Main chat (84 lines)
│   ├── RuntimePage.tsx               # Observability dashboard (646 lines)
│   ├── UsagePage.tsx                 # Token analytics (72 lines)
│   ├── RolesPage.tsx                 # Role management (209 lines)
│   └── runtimePageComponents.tsx     # Shared runtime UI (515 lines)
└── test/
    └── setup.ts                      # Vitest setup (5 lines)
```

---

## Pages & Routes

| Route | Page | Description |
|-------|------|-------------|
| `/` | AppShell | Redirects to `/chat` |
| `/chat` | ChatPage | Real-time chat with SSE streaming |
| `/runtime` | RuntimePage | Full runtime observability dashboard |
| `/usage` | UsagePage | Token usage analytics |
| `/roles` | RolesPage | Role CRUD management |

All pages share the `AppShell` layout: top nav bar + left `Sidebar` + content area.

---

## State Management

### React Query (server state)
- `queryClient` with `retry: 2`, `staleTime: 10_000` default
- 13 query hooks (`useWorkspaceMeta`, `useTimeline`, `useRoles`, etc.)
- 7 mutation hooks (`useSubmitMessage`, `useCreateRole`, etc.)
- Query key convention: `["resource", subKey]`

### useReducer + SSE (chat state)
- `ChatState = { messages, latestSeq, connection, contextHeadSummary }`
- Reducer handles 11 SSE event types via `applyEvent()`
- `useSessionEvents` hook: EventSource lifecycle + auto-reconnect (1s delay)
- 33ms batching window for `assistant.delta` events

### No global store
No Redux, Zustand, or Context for app state. Only React Query cache + local `useReducer` + component `useState`.

---

## Component Hierarchy

```
AppShell
├── header (brand, workspace pill, language switcher, NavTab×4)
├── Sidebar (workspace name/root, session ID, connection dot)
└── main
    ├── [/chat] ChatPage
    │   ├── MessageList
    │   │   ├── (empty) suggestion chips
    │   │   ├── UserMessage (rose left border)
    │   │   ├── AssistantMessage (emerald left border, streaming indicator)
    │   │   ├── ToolCallGroup → ToolCall×N (expandable, status icon)
    │   │   ├── NoticeBlock (amber)
    │   │   └── ErrorBlock (red)
    │   └── Composer (textarea + send button)
    ├── [/runtime] RuntimePage
    │   ├── SummaryCard×4 (context heads, tasks, diagnostics, reports)
    │   ├── CheckpointsPanel (diff + rollback)
    │   └── Panel → Row components (ContextHeadRow, TaskRow, etc.)
    ├── [/usage] UsagePage
    │   ├── SummaryCards (input/output/total/cache hit %)
    │   └── UsageChart (Recharts AreaChart)
    └── [/roles] RolesPage
        ├── RoleList (selection sidebar)
        ├── RoleDiagnostics (error panel)
        └── RoleEditor (CRUD form + version history)
```

---

## Styling System

### Design Tokens (CSS custom properties in `@theme`)

| Token | Value | Usage |
|-------|-------|-------|
| `--color-bg` | `#06070b` | Page background |
| `--color-surface` | `#0d0f16` | Card backgrounds |
| `--color-surface-2` | `#161922` | Elevated surfaces |
| `--color-border` | `#252830` | Borders |
| `--color-text` | `#f5f7fb` | Primary text |
| `--color-text-muted` | `#9aa4b2` | Secondary text |
| `--color-user` | `#fb7185` (rose) | User messages |
| `--color-assistant` | `#34d399` (emerald) | Assistant messages |
| `--color-tool` | `#f59e0b` (amber) | Tool calls |
| `--color-accent` | `#06b6d4` (cyan) | Accents |
| `--color-success` | `#22c55e` | Success states |
| `--color-warning` | `#f59e0b` | Warnings |
| `--color-error` | `#f87171` | Errors |
| `--color-running` | `#f59e0b` | Running indicators |
| `--font-sans` | `Noto Sans SC, PingFang SC, ...` | Body text |
| `--font-mono` | `Fira Code, Consolas, ...` | Code |

Dark mode only (no light theme). Pattern: Tailwind utility classes for layout + inline `style={{color: 'var(--color-xxx)'}}` for theme colors.

---

## i18n

- 2 locales: `en-US`, `zh-CN`
- Dot-path key lookup: `t("nav.chat")` → `"Chat"`
- `{variable}` interpolation in values
- Stored in `localStorage` key `sesame-console.locale`
- Fallback chain: current locale → `en-US` → raw key

---

## API Calls (Frontend)

All via `api/client.ts`. Paths relative to `VITE_API_BASE_URL`.

### Workspace & Session
| Function | Method | Path |
|----------|--------|------|
| `getWorkspace` | GET | `/v1/workspace` |
| `createSession` | POST | `/v1/session/ensure` |
| `ensureCurrentSession` | POST | `/v1/session/ensure` |

### Session-scoped (require `X-Sesame-Context-Binding` header)
| Function | Method | Path |
|----------|--------|------|
| `getTimeline` | GET | `/v1/session/timeline` |
| `getContextHistory` | GET | `/v1/session/history` |
| `reopenContext` | POST | `/v1/session/reopen` |
| `loadContextHistory` | POST | `/v1/session/history/load` |
| `submitMessage` | POST | `/v1/session/turns` |
| `openEventStream` | EventSource | `/v1/session/events?after=N` |
| `listFileCheckpoints` | GET | `/v1/session/checkpoints` |
| `getFileCheckpointDiff` | GET | `/v1/session/checkpoints/:id/diff` |
| `rollbackFileCheckpoint` | POST | `/v1/session/checkpoints/:id/rollback` |

### Metrics & Runtime
| Function | Method | Path |
|----------|--------|------|
| `getMetricsOverview` | GET | `/v1/metrics/overview` |
| `getMetricsTimeseries` | GET | `/v1/metrics/timeseries` |
| `getWorkspaceRuntimeGraph` | GET | `/v1/runtime_graph` |
| `getWorkspaceReports` | GET | `/v1/reports` |

### Roles
| Function | Method | Path |
|----------|--------|------|
| `listRoles` | GET | `/v1/roles` |
| `getRole` | GET | `/v1/roles/:id` |
| `listRoleVersions` | GET | `/v1/roles/:id/versions` |
| `createRole` | POST | `/v1/roles` |
| `updateRole` | PUT | `/v1/roles/:id` |
| `deleteRole` | DELETE | `/v1/roles/:id` |

---

## SSE Event Types (processed by reducer)

| Event | Effect |
|-------|--------|
| `user_message` | Append user entry |
| `turn.started` | Bump `latestSeq` |
| `assistant.delta` | Accumulate streaming text |
| `assistant.completed` | Mark stream finished |
| `tool.started` | Upsert tool card by `tool_call_id` |
| `tool.completed` | Update tool status/result |
| `system.notice` | Append notice block |
| `turn.failed` | Append error block |
| `turn.interrupted` | Append interrupt notice |
| `context_head_summary.*` | Update summary state |
| `context.compacted` | Append compaction notice |
