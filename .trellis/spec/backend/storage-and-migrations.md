# Storage And Migration Code-Spec

## Scenario: SQLite Repository Or Schema Change

### 1. Scope / Trigger

Use this spec when changing:

- `internal/v2/schema`
- `internal/v2/store`
- repository interfaces in `internal/v2/contracts/store.go`
- persisted contracts under `internal/v2/contracts`

Trigger: Sesame keeps long-running runtime state in SQLite. Schema drift or incompatible migrations can break existing development databases and automation state.

### 2. Signatures

Migrations:

```go
var MigrationNNN = Migration{
    Version: NNN,
    Name:    "short_name",
    Up:      `...`,
}
```

Store accessors:

```go
type Store interface {
    WithTx(ctx context.Context, fn func(tx Store) error) error
    ProjectStates() ProjectStateRepository
    Memories() MemoryRepository
}
```

Repository pattern:

```go
type exampleRepo struct {
    db *sql.DB
    tx *sql.Tx
}

func (r *exampleRepo) execer() execer { return repoExec(r.db, r.tx) }
```

### 3. Contracts

Migration contract:

- Add one numbered migration file per schema change.
- Register it in `schema.List`.
- Migrations must be idempotent enough for tests that construct partial legacy schemas.
- If adding columns to a table introduced by older migrations, ensure compatibility with partial legacy tests.
- Keep default values explicit for non-null columns.

Repository contract:

- Trim identity fields before querying/upserting.
- Missing keyed state should return `(zero, false, nil)` for `Get` methods that model optional documents.
- Repositories must work inside `WithTx`; add the repo to both normal `Store` initialization and tx-store initialization.
- `Create` methods that use upsert must not silently drop new persisted fields on update.

Time contract:

- Store times using existing `timeString`, `parseTime`, and `sqlNow` helpers.
- Preserve `CreatedAt` on upsert unless the table contract explicitly replaces it.
- Always update `UpdatedAt` on upsert/update.

### 4. Validation & Error Matrix

| Case | Behavior |
| --- | --- |
| Empty workspace root for optional state get | return not found without SQL error |
| Empty workspace root for optional state upsert | no-op |
| Missing row | return `false, nil` for optional state repos; return `sql.ErrNoRows` only for required entity repos |
| Partial legacy schema | migration must succeed or create missing prerequisite table/index |
| Tx repository path | same behavior as non-tx path |

### 5. Good/Base/Bad Cases

Good:

- `OpenInMemory` creates all v2 tables and records schema versions.
- A migration that adds memory metadata also preserves existing memories with default owner/visibility.
- Role runtime state repository round-trips `(workspace_root, role_id)` and source metadata.

Base:

- Empty optional runtime state is omitted from prompt assembly.

Bad:

- Adding a repository accessor to `Store` but forgetting `WithTx`.
- Adding a column to `INSERT` but not to `ON CONFLICT DO UPDATE`.
- SQL scanning fields in a different order than the `SELECT`.

### 6. Tests Required

Required assertions:

- `OpenInMemory` includes new tables.
- Repository CRUD/upsert round-trips all fields.
- Migration compatibility test covers legacy partial DB when a migration depends on an older table.
- `WithTx` exposes the new repository if the repository is part of runtime writes.
- Full `go test ./...` passes.

### 7. Wrong vs Correct

#### Wrong

```go
ALTER TABLE v2_some_table ADD COLUMN value TEXT NOT NULL;
```

This fails on existing rows.

#### Correct

```go
ALTER TABLE v2_some_table ADD COLUMN value TEXT NOT NULL DEFAULT '';
```

#### Wrong

```go
txStore.project = &projectStateRepo{db: s.db}
```

This escapes the transaction.

#### Correct

```go
txStore.project = &projectStateRepo{db: s.db, tx: tx}
```
