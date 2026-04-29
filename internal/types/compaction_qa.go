package types

import "time"

type CompactionQAStatus string

const (
	CompactionQAStatusPending  CompactionQAStatus = "pending"
	CompactionQAStatusPassed   CompactionQAStatus = "passed"
	CompactionQAStatusDegraded CompactionQAStatus = "degraded"
	CompactionQAStatusFailed   CompactionQAStatus = "failed"
)

type CompactionQA struct {
	ID                  string             `json:"id"`
	CompactionID        string             `json:"compaction_id"`
	SessionID           string             `json:"session_id"`
	CompactionKind      string             `json:"compaction_kind"`
	SourceItemCount     int                `json:"source_item_count"`
	SummaryText         string             `json:"summary_text"`
	SourceItemsPreview  string             `json:"source_items_preview"`
	RetainedConstraints []string           `json:"retained_constraints"`
	LostConstraints     []string           `json:"lost_constraints"`
	HallucinationCheck  string             `json:"hallucination_check"`
	Confidence          float64            `json:"confidence"`
	ReviewModel         string             `json:"review_model"`
	QAStatus            CompactionQAStatus `json:"qa_status"`
	CreatedAt           time.Time          `json:"created_at"`
}
