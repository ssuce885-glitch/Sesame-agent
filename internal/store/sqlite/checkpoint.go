package sqlite

import (
	"context"
	"database/sql"
	"time"

	"go-agent/internal/types"
)

func (s *Store) InsertTurnCheckpoint(ctx context.Context, checkpoint types.TurnCheckpoint) error {
	createdAt := checkpoint.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}
	toolCallIDs, err := marshalStringArray(checkpoint.ToolCallIDs)
	if err != nil {
		return err
	}
	toolCallNames, err := marshalStringArray(checkpoint.ToolCallNames)
	if err != nil {
		return err
	}
	completedToolIDs, err := marshalStringArray(checkpoint.CompletedToolIDs)
	if err != nil {
		return err
	}

	_, err = s.db.ExecContext(ctx, `
		insert into turn_checkpoints (
			id, turn_id, session_id, sequence, state, tool_call_ids, tool_call_names,
			next_position, completed_tool_ids, tool_results_json, assistant_items_json, created_at
		) values (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		checkpoint.ID,
		checkpoint.TurnID,
		checkpoint.SessionID,
		checkpoint.Sequence,
		checkpoint.State,
		toolCallIDs,
		toolCallNames,
		checkpoint.NextPosition,
		completedToolIDs,
		checkpoint.ToolResultsJSON,
		checkpoint.AssistantItemsJSON,
		createdAt.UTC().Format(timeLayout),
	)
	return err
}

func (s *Store) GetLatestTurnCheckpoint(ctx context.Context, turnID string) (types.TurnCheckpoint, bool, error) {
	row := s.db.QueryRowContext(ctx, `
		select id, turn_id, session_id, sequence, state, tool_call_ids, tool_call_names,
			next_position, completed_tool_ids, tool_results_json, assistant_items_json, created_at
		from turn_checkpoints
		where turn_id = ?
		order by sequence desc, created_at desc, id desc
		limit 1
	`, turnID)
	checkpoint, err := scanTurnCheckpoint(row)
	if err == sql.ErrNoRows {
		return types.TurnCheckpoint{}, false, nil
	}
	if err != nil {
		return types.TurnCheckpoint{}, false, err
	}
	return checkpoint, true, nil
}

func (s *Store) DeleteTurnCheckpoints(ctx context.Context, turnID string) (int64, error) {
	result, err := s.db.ExecContext(ctx, `
		delete from turn_checkpoints
		where turn_id = ?
	`, turnID)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func (s *Store) ListInterruptedTurnsWithCheckpoints(ctx context.Context) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `
		select distinct t.id
		from turns t
		join turn_checkpoints c on c.turn_id = t.id
		where t.state in (?, ?, ?, ?, ?)
		order by t.created_at asc, t.id asc
	`,
		types.TurnStateBuildingContext,
		types.TurnStateModelStreaming,
		types.TurnStateToolDispatching,
		types.TurnStateToolRunning,
		types.TurnStateLoopContinue,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []string
	for rows.Next() {
		var turnID string
		if err := rows.Scan(&turnID); err != nil {
			return nil, err
		}
		out = append(out, turnID)
	}
	return out, rows.Err()
}

type turnCheckpointScanner interface {
	Scan(dest ...any) error
}

func scanTurnCheckpoint(scanner turnCheckpointScanner) (types.TurnCheckpoint, error) {
	var checkpoint types.TurnCheckpoint
	var toolCallIDs string
	var toolCallNames string
	var completedToolIDs string
	var createdAt string
	if err := scanner.Scan(
		&checkpoint.ID,
		&checkpoint.TurnID,
		&checkpoint.SessionID,
		&checkpoint.Sequence,
		&checkpoint.State,
		&toolCallIDs,
		&toolCallNames,
		&checkpoint.NextPosition,
		&completedToolIDs,
		&checkpoint.ToolResultsJSON,
		&checkpoint.AssistantItemsJSON,
		&createdAt,
	); err != nil {
		return types.TurnCheckpoint{}, err
	}

	var err error
	checkpoint.ToolCallIDs, err = unmarshalStringArray(toolCallIDs)
	if err != nil {
		return types.TurnCheckpoint{}, err
	}
	checkpoint.ToolCallNames, err = unmarshalStringArray(toolCallNames)
	if err != nil {
		return types.TurnCheckpoint{}, err
	}
	checkpoint.CompletedToolIDs, err = unmarshalStringArray(completedToolIDs)
	if err != nil {
		return types.TurnCheckpoint{}, err
	}
	checkpoint.CreatedAt, err = time.Parse(timeLayout, createdAt)
	if err != nil {
		return types.TurnCheckpoint{}, err
	}
	return checkpoint, nil
}
