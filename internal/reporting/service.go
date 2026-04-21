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

type Service struct {
	store           Store
	now             func() time.Time
	pollInterval    time.Duration
	reportReadySink func(context.Context, string, string, types.ReportMailboxItem) error
	workspaceRoot   string
}

const reportingRunErrorBackoff = 25 * time.Millisecond

func NewService(store Store) *Service {
	return &Service{
		store:        store,
		now:          func() time.Time { return time.Now().UTC() },
		pollInterval: time.Second,
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

func (s *Service) SetReportReadySink(fn func(context.Context, string, string, types.ReportMailboxItem) error) {
	if s != nil {
		s.reportReadySink = fn
	}
}
