# Command Entrypoint Guidelines

> Use this package when changing executable wiring under `cmd/`.

---

## Scope

- `cmd/agentd`
- `cmd/sesame-agent`
- startup/bootstrap code that selects runtime mode

---

## Before You Change Code

1. Read `.trellis/spec/guides/index.md`
2. Read `README.md` sections for startup, provider config, and local console
3. Inspect the matching entrypoint and the internal package it wires:
   - daemon startup → `cmd/agentd` + `internal/api/http` + `internal/engine`
   - terminal/CLI startup → `cmd/sesame-agent` + `internal/cli`

---

## Checklist

- Keep entrypoints thin; push behavior into `internal/*`
- Do not duplicate env parsing logic already present in `internal/config`
- Preserve cross-platform startup behavior where possible
- If flags/env/defaults change, update README and relevant docs in the same task

---

## Verification

```bash
go test ./cmd/agentd -count=1
go test ./internal/cli/... -count=1
```
