package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"

	"go-agent/internal/types"
)

func appendAutomationWorkspaceRootCondition(conditions *[]string, args *[]any, column, workspaceRoot string) {
	workspaceRoot = strings.TrimSpace(workspaceRoot)
	if workspaceRoot == "" {
		return
	}
	escaped := escapeSQLiteLikePattern(workspaceRoot)
	*conditions = append(*conditions, "("+column+" = ? or "+column+" like ? escape '\\' or "+column+" like ? escape '\\')")
	*args = append(*args,
		workspaceRoot,
		escaped+"/automations/%",
		escaped+"\\\\automations\\\\%",
	)
}

func escapeSQLiteLikePattern(value string) string {
	replacer := strings.NewReplacer(
		`\`, `\\`,
		`%`, `\%`,
		`_`, `\_`,
	)
	return replacer.Replace(value)
}

func (s *Store) UpsertAutomation(ctx context.Context, spec types.AutomationSpec) error {
	return upsertAutomationWithExec(ctx, s.db, spec)
}

func (t runtimeTx) UpsertAutomation(ctx context.Context, spec types.AutomationSpec) error {
	return upsertAutomationWithExec(ctx, t.tx, spec)
}

func upsertAutomationWithExec(ctx context.Context, execer execContexter, spec types.AutomationSpec) error {
	spec = normalizeAutomationSpecForStore(spec)
	payload, err := json.Marshal(spec)
	if err != nil {
		return err
	}

	_, err = execer.ExecContext(ctx, `
		insert into automations (
			id, workspace_root, state, payload, created_at, updated_at
		)
		values (?, ?, ?, ?, ?, ?)
		on conflict(id) do update set
			workspace_root = excluded.workspace_root,
			state = excluded.state,
			payload = excluded.payload,
			updated_at = excluded.updated_at
	`,
		spec.ID,
		spec.WorkspaceRoot,
		spec.State,
		string(payload),
		spec.CreatedAt.UTC().Format(timeLayout),
		spec.UpdatedAt.UTC().Format(timeLayout),
	)
	return err
}

func (s *Store) GetAutomation(ctx context.Context, id string) (types.AutomationSpec, bool, error) {
	return getAutomationWithQueryer(ctx, s.db, id)
}

func (t runtimeTx) GetAutomation(ctx context.Context, id string) (types.AutomationSpec, bool, error) {
	return getAutomationWithQueryer(ctx, t.tx, id)
}

func getAutomationWithQueryer(ctx context.Context, queryer queryContexter, id string) (types.AutomationSpec, bool, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return types.AutomationSpec{}, false, nil
	}
	rows, err := queryer.QueryContext(ctx, `
		select payload, created_at, updated_at
		from automations
		where id = ?
	`, id)
	if err != nil {
		return types.AutomationSpec{}, false, err
	}
	defer rows.Close()

	items, err := scanAutomationSpecs(rows)
	if err != nil {
		return types.AutomationSpec{}, false, err
	}
	if len(items) == 0 {
		return types.AutomationSpec{}, false, nil
	}
	return items[0], true, nil
}

func (s *Store) ListAutomations(ctx context.Context, filter types.AutomationListFilter) ([]types.AutomationSpec, error) {
	return listAutomationsWithQueryer(ctx, s.db, filter)
}

func (t runtimeTx) ListAutomations(ctx context.Context, filter types.AutomationListFilter) ([]types.AutomationSpec, error) {
	return listAutomationsWithQueryer(ctx, t.tx, filter)
}

func listAutomationsWithQueryer(ctx context.Context, queryer queryContexter, filter types.AutomationListFilter) ([]types.AutomationSpec, error) {
	filter = normalizeAutomationListFilterForStore(filter)
	query := `
		select payload, created_at, updated_at
		from automations
	`
	args := make([]any, 0, 3)
	conditions := make([]string, 0, 2)
	if filter.WorkspaceRoot != "" {
		appendAutomationWorkspaceRootCondition(&conditions, &args, "workspace_root", filter.WorkspaceRoot)
	}
	if filter.State != "" {
		conditions = append(conditions, "state = ?")
		args = append(args, filter.State)
	}
	if len(conditions) > 0 {
		query += " where " + strings.Join(conditions, " and ")
	}
	query += " order by updated_at desc, created_at desc, id asc"
	if filter.Limit > 0 {
		query += " limit ?"
		args = append(args, filter.Limit)
	}

	rows, err := queryer.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanAutomationSpecs(rows)
}

func (s *Store) DeleteAutomation(ctx context.Context, id string) (bool, error) {
	return deleteAutomationWithExec(ctx, s.db, id)
}

func (t runtimeTx) DeleteAutomation(ctx context.Context, id string) (bool, error) {
	return deleteAutomationWithExec(ctx, t.tx, id)
}

func deleteAutomationWithExec(ctx context.Context, execer execContexter, id string) (bool, error) {
	result, err := execer.ExecContext(ctx, `
		delete from automations
		where id = ?
	`, strings.TrimSpace(id))
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return affected > 0, nil
}

func scanAutomationSpecs(rows *sql.Rows) ([]types.AutomationSpec, error) {
	out := make([]types.AutomationSpec, 0)
	for rows.Next() {
		var (
			payload   string
			createdAt string
			updatedAt string
		)
		if err := rows.Scan(&payload, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		var spec types.AutomationSpec
		if err := json.Unmarshal([]byte(payload), &spec); err != nil {
			return nil, err
		}
		if parsed, err := parsePendingOptionalTime(createdAt); err == nil && !parsed.IsZero() {
			spec.CreatedAt = parsed
		}
		if parsed, err := parsePendingOptionalTime(updatedAt); err == nil && !parsed.IsZero() {
			spec.UpdatedAt = parsed
		}
		out = append(out, normalizeAutomationSpecForStore(spec))
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}
