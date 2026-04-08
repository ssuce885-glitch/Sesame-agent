# Internal Runtime Backend Guidelines

> Use this package for the Go runtime core under `internal/`.

---

## Scope

Primary subsystems:

- `internal/engine` — turn loop / tool execution orchestration
- `internal/model` — model providers and request shaping
- `internal/tools` — tool registry, runtime, built-ins
- `internal/api/http` — daemon HTTP/SSE API
- `internal/context`, `internal/runtimegraph`, `internal/session` — turn/session state
- `internal/store/sqlite`, `internal/stream`, `internal/task` — persistence / events / background tasks
- `internal/types`, `internal/permissions`, `internal/runtime` — shared contracts and helpers

---

## Before You Change Code

1. Read `.trellis/spec/guides/index.md`
2. If the change touches tool execution or structured results, read:
   - `docs/superpowers/specs/2026-04-06-tool-sync-phase-3-plan-mode-design.md`
   - `docs/superpowers/plans/2026-04-06-tool-sync-phase-2-task-management.md`
3. If the change touches streaming / UI event flow, read:
   - `docs/superpowers/specs/2026-04-05-console-event-stream-reliability-design.md`

---

## Checklist

- Prefer extending existing contracts over adding parallel ad-hoc payloads
- Keep `internal/types` and provider-facing payloads consistent
- When changing tool I/O, check:
  - tool definition schema
  - runtime normalization/persistence
  - engine loop mapping
  - provider request serialization
- When changing event or API payloads, check:
  - `internal/api/http`
  - `internal/types`
  - `web/console/src`
- Search before adding helpers/constants; reuse first

---

## Verification

Run focused tests first:

```bash
go test ./internal/tools -count=1
go test ./internal/engine -count=1
go test ./internal/api/http -count=1
```

If `go.exe` on a WSL UNC path fails, mirror to a Windows path before running focused tests.
