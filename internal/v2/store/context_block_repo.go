package store

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"go-agent/internal/v2/contracts"
)

type contextBlockRepo struct {
	db *sql.DB
	tx *sql.Tx
}

var _ contracts.ContextBlockRepository = (*contextBlockRepo)(nil)

func (r *contextBlockRepo) execer() execer { return repoExec(r.db, r.tx) }

func (r *contextBlockRepo) Create(ctx context.Context, block contracts.ContextBlock) error {
	block = normalizeContextBlock(block)
	_, err := r.execer().Exec(`
INSERT INTO v2_context_blocks (
	id, workspace_root, type, owner, visibility, source_ref, title, summary, evidence,
	confidence, importance_score, expiry_policy, expires_at, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		block.ID,
		block.WorkspaceRoot,
		block.Type,
		block.Owner,
		block.Visibility,
		block.SourceRef,
		block.Title,
		block.Summary,
		block.Evidence,
		block.Confidence,
		block.ImportanceScore,
		block.ExpiryPolicy,
		contextBlockExpiresAtString(block.ExpiresAt),
		timeString(block.CreatedAt),
		timeString(block.UpdatedAt),
	)
	return err
}

func (r *contextBlockRepo) Get(ctx context.Context, id string) (contracts.ContextBlock, error) {
	return scanContextBlock(r.execer().QueryRow(`
SELECT id, workspace_root, type, owner, visibility, source_ref, title, summary, evidence,
	confidence, importance_score, expiry_policy, expires_at, created_at, updated_at
FROM v2_context_blocks
WHERE id = ?`, strings.TrimSpace(id)))
}

func (r *contextBlockRepo) Update(ctx context.Context, block contracts.ContextBlock) error {
	block = normalizeContextBlock(block)
	_, err := r.execer().Exec(`
UPDATE v2_context_blocks
SET workspace_root = ?,
	type = ?,
	owner = ?,
	visibility = ?,
	source_ref = ?,
	title = ?,
	summary = ?,
	evidence = ?,
	confidence = ?,
	importance_score = ?,
	expiry_policy = ?,
	expires_at = ?,
	updated_at = ?
WHERE id = ?`,
		block.WorkspaceRoot,
		block.Type,
		block.Owner,
		block.Visibility,
		block.SourceRef,
		block.Title,
		block.Summary,
		block.Evidence,
		block.Confidence,
		block.ImportanceScore,
		block.ExpiryPolicy,
		contextBlockExpiresAtString(block.ExpiresAt),
		timeString(block.UpdatedAt),
		block.ID,
	)
	return err
}

func (r *contextBlockRepo) Delete(ctx context.Context, id string) error {
	_, err := r.execer().Exec(`DELETE FROM v2_context_blocks WHERE id = ?`, strings.TrimSpace(id))
	return err
}

func (r *contextBlockRepo) ListByWorkspace(ctx context.Context, workspaceRoot string, opts contracts.ContextBlockListOptions) ([]contracts.ContextBlock, error) {
	query := `
SELECT id, workspace_root, type, owner, visibility, source_ref, title, summary, evidence,
	confidence, importance_score, expiry_policy, expires_at, created_at, updated_at
FROM v2_context_blocks
WHERE workspace_root = ?`
	args := []any{strings.TrimSpace(workspaceRoot)}
	if strings.TrimSpace(opts.Owner) != "" {
		query += ` AND owner = ?`
		args = append(args, strings.TrimSpace(opts.Owner))
	}
	if strings.TrimSpace(opts.Visibility) != "" {
		query += ` AND visibility = ?`
		args = append(args, strings.TrimSpace(opts.Visibility))
	}
	if strings.TrimSpace(opts.Type) != "" {
		query += ` AND type = ?`
		args = append(args, strings.TrimSpace(opts.Type))
	}
	query += ` ORDER BY importance_score DESC, updated_at DESC`
	if opts.Limit > 0 {
		query += ` LIMIT ?`
		args = append(args, opts.Limit)
	}
	rows, err := r.execer().Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var blocks []contracts.ContextBlock
	for rows.Next() {
		block, err := scanContextBlock(rows)
		if err != nil {
			return nil, err
		}
		blocks = append(blocks, block)
	}
	return blocks, rows.Err()
}

func normalizeContextBlock(block contracts.ContextBlock) contracts.ContextBlock {
	block.ID = strings.TrimSpace(block.ID)
	block.WorkspaceRoot = strings.TrimSpace(block.WorkspaceRoot)
	block.Type = firstNonEmptyStore(block.Type, "fact")
	block.Owner = firstNonEmptyStore(block.Owner, "workspace")
	block.Visibility = firstNonEmptyStore(block.Visibility, "global")
	block.SourceRef = strings.TrimSpace(block.SourceRef)
	block.Title = strings.TrimSpace(block.Title)
	block.Summary = strings.TrimSpace(block.Summary)
	block.Evidence = strings.TrimSpace(block.Evidence)
	block.ExpiryPolicy = strings.TrimSpace(block.ExpiryPolicy)
	if block.CreatedAt.IsZero() {
		block.CreatedAt = sqlNow()
	}
	if block.UpdatedAt.IsZero() {
		block.UpdatedAt = sqlNow()
	}
	return block
}

func scanContextBlock(row interface {
	Scan(dest ...any) error
}) (contracts.ContextBlock, error) {
	var block contracts.ContextBlock
	var expiresAt, createdAt, updatedAt string
	err := row.Scan(
		&block.ID,
		&block.WorkspaceRoot,
		&block.Type,
		&block.Owner,
		&block.Visibility,
		&block.SourceRef,
		&block.Title,
		&block.Summary,
		&block.Evidence,
		&block.Confidence,
		&block.ImportanceScore,
		&block.ExpiryPolicy,
		&expiresAt,
		&createdAt,
		&updatedAt,
	)
	if err != nil {
		return contracts.ContextBlock{}, err
	}
	if strings.TrimSpace(expiresAt) != "" {
		parsed, err := parseTime(expiresAt)
		if err != nil {
			return contracts.ContextBlock{}, err
		}
		block.ExpiresAt = &parsed
	}
	block.CreatedAt, err = parseTime(createdAt)
	if err != nil {
		return contracts.ContextBlock{}, err
	}
	block.UpdatedAt, err = parseTime(updatedAt)
	if err != nil {
		return contracts.ContextBlock{}, err
	}
	return block, nil
}

func contextBlockExpiresAtString(expiresAt *time.Time) string {
	if expiresAt == nil || expiresAt.IsZero() {
		return ""
	}
	return timeString(*expiresAt)
}
