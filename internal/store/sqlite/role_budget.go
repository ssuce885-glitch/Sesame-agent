package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"go-agent/internal/types"
)

func (s *Store) UpsertTurnCost(ctx context.Context, cost types.TurnCost) error {
	return upsertTurnCostWithExec(ctx, s.db, cost)
}

func upsertTurnCostWithExec(ctx context.Context, execer execContexter, cost types.TurnCost) error {
	cost.TurnID = strings.TrimSpace(cost.TurnID)
	cost.SessionID = strings.TrimSpace(cost.SessionID)
	if cost.ID == "" && cost.TurnID != "" {
		cost.ID = "turn_cost_" + cost.TurnID
	}
	if cost.ID == "" {
		cost.ID = types.NewID("turn_cost")
	}
	if cost.CreatedAt.IsZero() {
		cost.CreatedAt = time.Now().UTC()
	}
	_, err := execer.ExecContext(ctx, `
		insert into turn_costs (
			id, turn_id, session_id, owner_role_id, input_tokens, output_tokens, created_at
		)
		values (?, ?, ?, ?, ?, ?, ?)
		on conflict(id) do update set
			turn_id = excluded.turn_id,
			session_id = excluded.session_id,
			owner_role_id = excluded.owner_role_id,
			input_tokens = excluded.input_tokens,
			output_tokens = excluded.output_tokens,
			created_at = excluded.created_at
	`,
		cost.ID,
		cost.TurnID,
		cost.SessionID,
		strings.TrimSpace(cost.OwnerRoleID),
		cost.InputTokens,
		cost.OutputTokens,
		cost.CreatedAt.UTC().Format(timeLayout),
	)
	return err
}

func (s *Store) GetTurnCost(ctx context.Context, turnID string) (types.TurnCost, bool, error) {
	row := s.db.QueryRowContext(ctx, `
		select id, turn_id, session_id, owner_role_id, input_tokens, output_tokens, created_at
		from turn_costs
		where turn_id = ?
		order by created_at desc, id asc
		limit 1
	`, strings.TrimSpace(turnID))
	cost, err := scanTurnCost(row)
	if errors.Is(err, sql.ErrNoRows) {
		return types.TurnCost{}, false, nil
	}
	if err != nil {
		return types.TurnCost{}, false, err
	}
	return cost, true, nil
}

func (s *Store) ListTurnCostsBySession(ctx context.Context, sessionID string) ([]types.TurnCost, error) {
	rows, err := s.db.QueryContext(ctx, `
		select id, turn_id, session_id, owner_role_id, input_tokens, output_tokens, created_at
		from turn_costs
		where session_id = ?
		order by created_at desc, id asc
	`, strings.TrimSpace(sessionID))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []types.TurnCost
	for rows.Next() {
		cost, err := scanTurnCost(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, cost)
	}
	return out, rows.Err()
}

type turnCostScanner interface {
	Scan(dest ...any) error
}

func scanTurnCost(scanner turnCostScanner) (types.TurnCost, error) {
	var cost types.TurnCost
	var createdAt string
	if err := scanner.Scan(
		&cost.ID,
		&cost.TurnID,
		&cost.SessionID,
		&cost.OwnerRoleID,
		&cost.InputTokens,
		&cost.OutputTokens,
		&createdAt,
	); err != nil {
		return types.TurnCost{}, err
	}
	parsed, err := time.Parse(timeLayout, createdAt)
	if err != nil {
		return types.TurnCost{}, err
	}
	cost.CreatedAt = parsed
	return cost, nil
}
