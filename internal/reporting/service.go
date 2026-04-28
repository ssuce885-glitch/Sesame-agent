package reporting

import (
	"context"
	"strings"
	"time"

	"go-agent/internal/types"
)

type Store interface {
	UpsertReport(context.Context, types.ReportRecord) error
	UpsertReportDelivery(context.Context, types.ReportDelivery) error
	UpsertChildAgentResult(context.Context, types.ChildAgentResult) error
	GetSession(context.Context, string) (types.Session, bool, error)
	ResolveSessionRole(context.Context, string, string) (types.SessionRole, error)
	ResolveSpecialistRoleID(context.Context, string, string) (string, error)
	EnsureRoleSession(context.Context, string, types.SessionRole) (types.Session, types.ContextHead, bool, error)
	GetChildAgentSpec(context.Context, string) (types.ChildAgentSpec, bool, error)
	GetReportGroup(context.Context, string) (types.ReportGroup, bool, error)
	ListReportGroups(context.Context) ([]types.ReportGroup, error)
	ListChildAgentResultsByReportGroup(context.Context, string) ([]types.ChildAgentResult, error)
	ListChildAgentResultsBySessionAndReportGroup(context.Context, string, string) ([]types.ChildAgentResult, error)
	UpsertDigestRecord(context.Context, types.DigestRecord) error
	ListDigestRecordsBySessionAndGroup(context.Context, string, string) ([]types.DigestRecord, error)
}

type coldStore interface {
	InsertColdIndexEntry(context.Context, types.ColdIndexEntry) error
}

type cleanupStore interface {
	CleanupOldReports(context.Context, string, time.Time) (int64, error)
	CleanupOldDigestRecords(context.Context, string, time.Time) (int64, error)
	CleanupOldReportDeliveries(context.Context, string, time.Time) (int64, error)
	CleanupOldChildAgentResults(context.Context, time.Time) (int64, error)
	CleanupDeprecatedMemories(context.Context, time.Time) (int64, error)
	CleanupOldConversationCompactions(context.Context, int) (int64, error)
}

type Service struct {
	store           Store
	coldStore       coldStore
	cleanupStore    cleanupStore
	now             func() time.Time
	pollInterval    time.Duration
	reportReadySink func(context.Context, string, string, types.ReportDeliveryItem) error
	workspaceRoot   string
	lastCleanupAt   time.Time
}

const reportingRunErrorBackoff = 25 * time.Millisecond

func NewService(store Store) *Service {
	return &Service{
		store:        store,
		now:          func() time.Time { return time.Now().UTC() },
		pollInterval: time.Second,
	}
}

func (s *Service) SetColdStore(cs interface {
	InsertColdIndexEntry(context.Context, types.ColdIndexEntry) error
}) {
	if s != nil {
		s.coldStore = cs
	}
}

func (s *Service) SetCleanupStore(cs interface {
	CleanupOldReports(context.Context, string, time.Time) (int64, error)
	CleanupOldDigestRecords(context.Context, string, time.Time) (int64, error)
	CleanupOldReportDeliveries(context.Context, string, time.Time) (int64, error)
	CleanupOldChildAgentResults(context.Context, time.Time) (int64, error)
	CleanupDeprecatedMemories(context.Context, time.Time) (int64, error)
	CleanupOldConversationCompactions(context.Context, int) (int64, error)
}) {
	if s != nil {
		s.cleanupStore = cs
	}
}

func (s *Service) SetWorkspaceRoot(root string) {
	if s != nil {
		s.workspaceRoot = strings.TrimSpace(root)
	}
}

func (s *Service) SetClock(now func() time.Time) {
	if s != nil && now != nil {
		s.now = now
	}
}

func (s *Service) SetPollInterval(interval time.Duration) {
	if s != nil && interval > 0 {
		s.pollInterval = interval
	}
}

func (s *Service) SetReportReadySink(fn func(context.Context, string, string, types.ReportDeliveryItem) error) {
	if s != nil {
		s.reportReadySink = fn
	}
}

func truncateRunes(value string, max int) string {
	if max <= 0 || value == "" {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= max {
		return value
	}
	return string(runes[:max])
}
