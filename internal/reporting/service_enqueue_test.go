package reporting

import (
	"strings"
	"testing"
	"time"

	"go-agent/internal/task"
)

func TestReportFromTaskOutcomeClampsReportBody(t *testing.T) {
	readyAt := time.Date(2026, 4, 28, 1, 2, 3, 0, time.UTC)
	report, ok := ReportFromTaskOutcome("/tmp/workspace", task.Task{
		ID:                 "task_1",
		Status:             task.TaskStatusCompleted,
		Description:        "Long report",
		ParentSessionID:    "session_parent",
		FinalResultKind:    task.FinalResultKindAssistantText,
		FinalResultText:    strings.Repeat("界", reportBodyRuneLimit+10),
		FinalResultReadyAt: &readyAt,
	}, readyAt)
	if !ok {
		t.Fatal("expected report")
	}
	if len(report.Envelope.Sections) != 1 {
		t.Fatalf("sections = %d, want 1", len(report.Envelope.Sections))
	}
	body := report.Envelope.Sections[0].Text
	if len([]rune(body)) != reportBodyRuneLimit {
		t.Fatalf("body rune length = %d, want %d", len([]rune(body)), reportBodyRuneLimit)
	}
	if !strings.HasSuffix(body, reportBodyTruncatedSuffix) {
		t.Fatalf("body missing truncation suffix: %q", body)
	}
}
