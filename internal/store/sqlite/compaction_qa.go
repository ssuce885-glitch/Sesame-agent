package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"go-agent/internal/types"
)

func (s *Store) InsertCompactionQA(ctx context.Context, qa types.CompactionQA) error {
	createdAt := qa.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}
	retained, err := marshalStringArray(qa.RetainedConstraints)
	if err != nil {
		return err
	}
	lost, err := marshalStringArray(qa.LostConstraints)
	if err != nil {
		return err
	}

	_, err = s.db.ExecContext(ctx, `
		insert into compaction_qa (
			id, compaction_id, session_id, compaction_kind, source_item_count,
			summary_text, source_items_preview, retained_constraints, lost_constraints,
			hallucination_check, confidence, review_model, qa_status, created_at
		) values (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		qa.ID,
		qa.CompactionID,
		qa.SessionID,
		qa.CompactionKind,
		qa.SourceItemCount,
		qa.SummaryText,
		qa.SourceItemsPreview,
		retained,
		lost,
		qa.HallucinationCheck,
		qa.Confidence,
		qa.ReviewModel,
		qa.QAStatus,
		createdAt.UTC().Format(timeLayout),
	)
	return err
}

func (s *Store) GetCompactionQA(ctx context.Context, compactionID string) (types.CompactionQA, bool, error) {
	row := s.db.QueryRowContext(ctx, `
		select id, compaction_id, session_id, compaction_kind, source_item_count,
			summary_text, source_items_preview, retained_constraints, lost_constraints,
			hallucination_check, confidence, review_model, qa_status, created_at
		from compaction_qa
		where compaction_id = ?
		order by created_at desc, id asc
		limit 1
	`, compactionID)
	qa, err := scanCompactionQA(row)
	if err == sql.ErrNoRows {
		return types.CompactionQA{}, false, nil
	}
	if err != nil {
		return types.CompactionQA{}, false, err
	}
	return qa, true, nil
}

func (s *Store) ListCompactionQABySession(ctx context.Context, sessionID string, limit int) ([]types.CompactionQA, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.QueryContext(ctx, `
		select id, compaction_id, session_id, compaction_kind, source_item_count,
			summary_text, source_items_preview, retained_constraints, lost_constraints,
			hallucination_check, confidence, review_model, qa_status, created_at
		from compaction_qa
		where session_id = ?
		order by created_at desc, id asc
		limit ?
	`, sessionID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []types.CompactionQA
	for rows.Next() {
		qa, err := scanCompactionQA(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, qa)
	}
	return out, rows.Err()
}

type compactionQAScanner interface {
	Scan(dest ...any) error
}

func scanCompactionQA(scanner compactionQAScanner) (types.CompactionQA, error) {
	var qa types.CompactionQA
	var retained string
	var lost string
	var status string
	var createdAt string
	if err := scanner.Scan(
		&qa.ID,
		&qa.CompactionID,
		&qa.SessionID,
		&qa.CompactionKind,
		&qa.SourceItemCount,
		&qa.SummaryText,
		&qa.SourceItemsPreview,
		&retained,
		&lost,
		&qa.HallucinationCheck,
		&qa.Confidence,
		&qa.ReviewModel,
		&status,
		&createdAt,
	); err != nil {
		return types.CompactionQA{}, err
	}

	var err error
	qa.RetainedConstraints, err = unmarshalStringArray(retained)
	if err != nil {
		return types.CompactionQA{}, err
	}
	qa.LostConstraints, err = unmarshalStringArray(lost)
	if err != nil {
		return types.CompactionQA{}, err
	}
	qa.QAStatus = types.CompactionQAStatus(status)
	qa.CreatedAt, err = time.Parse(timeLayout, createdAt)
	if err != nil {
		return types.CompactionQA{}, err
	}
	return qa, nil
}

func marshalStringArray(values []string) (string, error) {
	if values == nil {
		values = []string{}
	}
	raw, err := json.Marshal(values)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func unmarshalStringArray(raw string) ([]string, error) {
	if raw == "" {
		return nil, nil
	}
	var out []string
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil, err
	}
	return out, nil
}
