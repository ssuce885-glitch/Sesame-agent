# Backend Code-Spec Index

Scope: Go backend under `internal/v2`, including agent prompt assembly, context preview, memory, store repositories, schema migrations, tools, roles, tasks, workflows, and automations.

## Pre-Development Checklist

Before editing backend runtime code, read:

- `context-system.md` for prompt authority, runtime state, memory visibility, and context preview rules.
- `storage-and-migrations.md` for SQLite schema/repository contracts.

For cross-layer context work, also read:

- `../guides/context-review-checklist.md`

## Current Architecture Map

- `internal/v2/agent`: turn loop, prompt assembly, compaction, Workspace Runtime State injection, role execution context.
- `internal/v2/contextasm`: pure context assembly primitives: scope, visibility, prompt package metadata, runtime state Markdown builders.
- `internal/v2/contextsvc`: context preview and ContextBlock CRUD.
- `internal/v2/memory`: memory scoring and cleanup service.
- `internal/v2/tools`: tool implementations and runtime visibility gates.
- `internal/v2/store`: SQLite repositories.
- `internal/v2/schema`: ordered migrations.
- `internal/v2/contracts`: cross-layer data and repository interfaces.

## Required Validation

Run focused tests for touched packages, then run:

```bash
go test ./...
```

If `.trellis/spec` changes, no separate generator is required; keep Markdown committed with the code/spec decision.
