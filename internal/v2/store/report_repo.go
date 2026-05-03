package store

import (
	"context"
	"database/sql"
	"go-agent/internal/v2/contracts"
)

type reportRepo struct {
	db *sql.DB
	tx *sql.Tx
}

var _ contracts.ReportRepository = (*reportRepo)(nil)

func (r *reportRepo) execer() execer { return repoExec(r.db, r.tx) }

func (r *reportRepo) Create(ctx context.Context, report contracts.Report) error {
	_, err := r.execer().Exec(`
INSERT INTO v2_reports (id, session_id, source_kind, source_id, status, severity, title, summary, delivered, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		report.ID, report.SessionID, report.SourceKind, report.SourceID, report.Status, report.Severity, report.Title, report.Summary, boolInt(report.Delivered), timeString(report.CreatedAt))
	return err
}

func (r *reportRepo) Get(ctx context.Context, id string) (contracts.Report, error) {
	return scanReport(r.execer().QueryRow(`
SELECT id, session_id, source_kind, source_id, status, severity, title, summary, delivered, created_at
FROM v2_reports WHERE id = ?`, id))
}

func (r *reportRepo) ListBySession(ctx context.Context, sessionID string) ([]contracts.Report, error) {
	rows, err := r.execer().Query(`
SELECT id, session_id, source_kind, source_id, status, severity, title, summary, delivered, created_at
FROM v2_reports WHERE session_id = ? ORDER BY created_at ASC`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var reports []contracts.Report
	for rows.Next() {
		report, err := scanReport(rows)
		if err != nil {
			return nil, err
		}
		reports = append(reports, report)
	}
	return reports, rows.Err()
}

func (r *reportRepo) MarkDelivered(ctx context.Context, id string) error {
	_, err := r.execer().Exec(`UPDATE v2_reports SET delivered = 1 WHERE id = ?`, id)
	return err
}

func scanReport(row interface {
	Scan(dest ...any) error
}) (contracts.Report, error) {
	var report contracts.Report
	var delivered int
	var createdAt string
	err := row.Scan(&report.ID, &report.SessionID, &report.SourceKind, &report.SourceID, &report.Status, &report.Severity, &report.Title, &report.Summary, &delivered, &createdAt)
	if err != nil {
		return contracts.Report{}, err
	}
	report.Delivered = intBool(delivered)
	report.CreatedAt, err = parseTime(createdAt)
	if err != nil {
		return contracts.Report{}, err
	}
	return report, nil
}
