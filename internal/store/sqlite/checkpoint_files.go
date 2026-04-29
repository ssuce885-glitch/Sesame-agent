package sqlite

import (
	"context"
	"database/sql"
	"time"

	"go-agent/internal/types"
)

func (s *Store) InsertFileCheckpoint(ctx context.Context, checkpoint types.FileCheckpoint) error {
	createdAt := checkpoint.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}
	filesChanged, err := marshalStringArray(checkpoint.FilesChanged)
	if err != nil {
		return err
	}

	_, err = s.db.ExecContext(ctx, `
		insert into file_checkpoints (
			id, session_id, turn_id, tool_call_id, tool_name, reason, git_commit_hash,
			files_changed, diff_summary, parent_checkpoint_id, created_at
		) values (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		checkpoint.ID,
		checkpoint.SessionID,
		checkpoint.TurnID,
		checkpoint.ToolCallID,
		checkpoint.ToolName,
		checkpoint.Reason,
		checkpoint.GitCommitHash,
		filesChanged,
		checkpoint.DiffSummary,
		checkpoint.ParentCheckpointID,
		createdAt.UTC().Format(timeLayout),
	)
	return err
}

func (s *Store) GetFileCheckpoint(ctx context.Context, id string) (types.FileCheckpoint, bool, error) {
	row := s.db.QueryRowContext(ctx, `
		select id, session_id, turn_id, tool_call_id, tool_name, reason, git_commit_hash,
			files_changed, diff_summary, parent_checkpoint_id, created_at
		from file_checkpoints
		where id = ?
	`, id)
	checkpoint, err := scanFileCheckpoint(row)
	if err == sql.ErrNoRows {
		return types.FileCheckpoint{}, false, nil
	}
	if err != nil {
		return types.FileCheckpoint{}, false, err
	}
	return checkpoint, true, nil
}

func (s *Store) ListFileCheckpointsBySession(ctx context.Context, sessionID string, limit int) ([]types.FileCheckpoint, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx, `
		select id, session_id, turn_id, tool_call_id, tool_name, reason, git_commit_hash,
			files_changed, diff_summary, parent_checkpoint_id, created_at
		from file_checkpoints
		where session_id = ?
		order by created_at desc, id desc
		limit ?
	`, sessionID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []types.FileCheckpoint
	for rows.Next() {
		checkpoint, err := scanFileCheckpoint(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, checkpoint)
	}
	return out, rows.Err()
}

func (s *Store) GetLatestFileCheckpoint(ctx context.Context, sessionID string) (types.FileCheckpoint, bool, error) {
	row := s.db.QueryRowContext(ctx, `
		select id, session_id, turn_id, tool_call_id, tool_name, reason, git_commit_hash,
			files_changed, diff_summary, parent_checkpoint_id, created_at
		from file_checkpoints
		where session_id = ?
		order by created_at desc, id desc
		limit 1
	`, sessionID)
	checkpoint, err := scanFileCheckpoint(row)
	if err == sql.ErrNoRows {
		return types.FileCheckpoint{}, false, nil
	}
	if err != nil {
		return types.FileCheckpoint{}, false, err
	}
	return checkpoint, true, nil
}

type fileCheckpointScanner interface {
	Scan(dest ...any) error
}

func scanFileCheckpoint(scanner fileCheckpointScanner) (types.FileCheckpoint, error) {
	var checkpoint types.FileCheckpoint
	var filesChanged string
	var createdAt string
	if err := scanner.Scan(
		&checkpoint.ID,
		&checkpoint.SessionID,
		&checkpoint.TurnID,
		&checkpoint.ToolCallID,
		&checkpoint.ToolName,
		&checkpoint.Reason,
		&checkpoint.GitCommitHash,
		&filesChanged,
		&checkpoint.DiffSummary,
		&checkpoint.ParentCheckpointID,
		&createdAt,
	); err != nil {
		return types.FileCheckpoint{}, err
	}

	var err error
	checkpoint.FilesChanged, err = unmarshalStringArray(filesChanged)
	if err != nil {
		return types.FileCheckpoint{}, err
	}
	checkpoint.CreatedAt, err = time.Parse(timeLayout, createdAt)
	if err != nil {
		return types.FileCheckpoint{}, err
	}
	return checkpoint, nil
}
