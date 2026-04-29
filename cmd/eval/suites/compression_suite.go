package suites

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"go-agent/cmd/eval/internal/evalcore"
	"go-agent/internal/model"
	"go-agent/internal/store/sqlite"
	"go-agent/internal/types"
)

const (
	compressionCompactionID = "eval_compression_compaction"
	compressionColdIndexID  = "eval_compression_cold_index"
	compressionConstraint   = "All production deletes require dry-run approval"
)

func CompressionSuite(evalcore.SuiteOptions) evalcore.EvalSuite {
	return evalcore.EvalSuite{
		Name:        "compression",
		Description: "Prefills a compacted conversation and verifies cold-index and compaction-QA records preserve hard constraints.",
		Setup:       setupCompressionSuite,
		Verify:      verifyCompressionSuite,
		MinPassRate: 1.0,
	}
}

func setupCompressionSuite(ctx context.Context, env *evalcore.EvalEnv) error {
	store, err := sqlite.Open(env.DBPath)
	if err != nil {
		return err
	}
	defer store.Close()

	headID, err := currentHeadID(ctx, store, env.SessionID)
	if err != nil {
		return err
	}

	items := []model.ConversationItem{
		model.UserMessageItem("For this workspace, hard constraint: All production deletes require dry-run approval."),
		{Kind: model.ConversationItemAssistantText, Text: "Recorded. Production delete workflows must require dry-run approval."},
		model.UserMessageItem("Also remember that schema migrations need rollback notes."),
		{Kind: model.ConversationItemAssistantText, Text: "Understood. Migration work should include rollback notes."},
	}
	for idx, item := range items {
		if err := store.InsertConversationItemWithContextHead(ctx, env.SessionID, headID, "eval_compression_seed", idx+1, item); err != nil {
			return err
		}
	}

	startItemID, _, err := store.GetConversationItemIDByContextHeadAndPosition(ctx, env.SessionID, headID, 1)
	if err != nil {
		return err
	}
	endItemID, _, err := store.GetConversationItemIDByContextHeadAndPosition(ctx, env.SessionID, headID, len(items))
	if err != nil {
		return err
	}

	now := time.Now().UTC()
	if err := store.InsertConversationCompactionWithContextHead(ctx, types.ConversationCompaction{
		ID:              compressionCompactionID,
		SessionID:       env.SessionID,
		ContextHeadID:   headID,
		Kind:            types.ConversationCompactionKindRolling,
		Generation:      1,
		StartItemID:     startItemID,
		EndItemID:       endItemID,
		StartPosition:   1,
		EndPosition:     len(items),
		SummaryPayload:  `{"range_label":"items 1-4","important_choices":["All production deletes require dry-run approval","Schema migrations need rollback notes"],"open_threads":["Keep production delete safeguards visible after compaction"]}`,
		Reason:          "eval prefill",
		ProviderProfile: "eval",
		CreatedAt:       now,
	}); err != nil {
		return err
	}

	if err := store.InsertColdIndexEntry(ctx, types.ColdIndexEntry{
		ID:          compressionColdIndexID,
		WorkspaceID: env.WorkspaceRoot,
		Visibility:  types.MemoryVisibilityShared,
		SourceType:  "compaction",
		SourceID:    compressionCompactionID,
		SearchText:  "production deletes dry-run approval schema migrations rollback notes",
		SummaryLine: "Hard constraint preserved: All production deletes require dry-run approval.",
		OccurredAt:  now,
		CreatedAt:   now,
		ContextRef: types.ColdContextRef{
			SessionID:     env.SessionID,
			ContextHeadID: headID,
			TurnStartPos:  1,
			TurnEndPos:    len(items),
			ItemCount:     len(items),
		},
	}); err != nil {
		return err
	}

	return store.InsertCompactionQA(ctx, types.CompactionQA{
		ID:                  "eval_compression_qa",
		CompactionID:        compressionCompactionID,
		SessionID:           env.SessionID,
		CompactionKind:      string(types.ConversationCompactionKindRolling),
		SourceItemCount:     len(items),
		SummaryText:         "Hard constraint preserved: All production deletes require dry-run approval.",
		SourceItemsPreview:  "hard constraint plus migration rollback notes",
		RetainedConstraints: []string{compressionConstraint},
		LostConstraints:     []string{},
		HallucinationCheck:  "no hallucinations detected",
		Confidence:          0.99,
		ReviewModel:         "eval-prefill",
		QAStatus:            types.CompactionQAStatusPassed,
		CreatedAt:           now,
	})
}

func verifyCompressionSuite(ctx context.Context, dbPath string) ([]evalcore.EvalResult, error) {
	store, err := sqlite.Open(dbPath)
	if err != nil {
		return nil, err
	}
	defer store.Close()

	var results []evalcore.EvalResult
	cold, found, err := store.GetColdIndexEntry(ctx, compressionColdIndexID)
	if err != nil {
		return nil, err
	}
	results = append(results, evalcore.Result(
		"cold index entry exists",
		found,
		compressionColdIndexID,
	))
	if found {
		results = append(results, evalcore.Result(
			"cold index preserves constraint",
			strings.Contains(cold.SummaryLine, compressionConstraint) || strings.Contains(cold.SearchText, "dry-run approval"),
			fmt.Sprintf("summary=%q", cold.SummaryLine),
		))
	}

	qa, found, err := store.GetCompactionQA(ctx, compressionCompactionID)
	if err != nil {
		return nil, err
	}
	results = append(results, evalcore.Result("compaction QA exists", found, compressionCompactionID))
	if found {
		results = append(results,
			evalcore.Result("compaction QA lost_constraints empty", len(qa.LostConstraints) == 0, fmt.Sprintf("lost=%v", qa.LostConstraints)),
			evalcore.Result("compaction QA passed", qa.QAStatus == types.CompactionQAStatusPassed, string(qa.QAStatus)),
			evalcore.Result("compaction QA retained constraint", stringSliceContains(qa.RetainedConstraints, compressionConstraint), fmt.Sprintf("retained=%v", qa.RetainedConstraints)),
		)
	}
	return results, nil
}

func currentHeadID(ctx context.Context, store *sqlite.Store, sessionID string) (string, error) {
	var headID string
	err := store.DB().QueryRowContext(ctx, `
		select id
		from context_heads
		where session_id = ?
		order by updated_at desc, created_at desc, id desc
		limit 1
	`, sessionID).Scan(&headID)
	if err == sql.ErrNoRows {
		return "", fmt.Errorf("session %s has no context head", sessionID)
	}
	if err != nil {
		return "", err
	}
	return headID, nil
}

func stringSliceContains(items []string, needle string) bool {
	for _, item := range items {
		if item == needle {
			return true
		}
	}
	return false
}
