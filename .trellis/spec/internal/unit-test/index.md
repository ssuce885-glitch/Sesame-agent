# Internal Go Test Guidelines

> Use this package when adding or updating tests for the Go runtime.

---

## Scope

- `*_test.go` under `internal/*`
- runtime, engine, API, model, context, task, and tool tests

---

## Checklist

- Prefer focused package tests over full-repo test runs during iteration
- Bug fix -> add a regression test close to the changed package
- Tool contract changes should cover:
  - definition/schema visibility
  - `ExecuteRich` structured output
  - engine/provider propagation when relevant
- Keep tests deterministic; avoid real network calls in package tests

---

## Useful Commands

```bash
go test ./internal/tools -count=1
go test ./internal/engine -count=1
go test ./internal/api/http -count=1
go test ./cmd/agentd -count=1
```

---

## Environment Note

On this project, `go.exe` may fail when the repo is executed directly from a WSL UNC path.
If that happens, mirror the repo to a Windows filesystem path and run focused tests there.
