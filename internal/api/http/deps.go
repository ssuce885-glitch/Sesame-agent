package httpapi

import (
	"context"

	"go-agent/internal/automation"
	"go-agent/internal/model"
	"go-agent/internal/scheduler"
	"go-agent/internal/session"
	"go-agent/internal/types"
)

type Store interface {
	InsertSession(context.Context, types.Session) error
	ListSessions(context.Context) ([]types.Session, error)
	GetSession(context.Context, string) (types.Session, bool, error)
	UpdateSessionSystemPrompt(context.Context, string, string) (types.Session, bool, error)
	GetSelectedSessionID(context.Context) (string, bool, error)
	SetSelectedSessionID(context.Context, string) error
	DeleteSession(context.Context, string) (string, bool, error)
	InsertTurn(context.Context, types.Turn) error
	DeleteTurn(context.Context, string) error
	ListTurnsBySession(context.Context, string) ([]types.Turn, error)
	ListConversationItems(context.Context, string) ([]model.ConversationItem, error)
	ListSessionEvents(context.Context, string, int64) ([]types.Event, error)
	LatestSessionEventSeq(context.Context, string) (int64, error)
}

type Manager interface {
	RegisterSession(types.Session)
	UpdateSession(types.Session) bool
	SubmitTurn(context.Context, string, session.SubmitTurnInput) (string, error)
	ResumeTurn(context.Context, string, session.ResumeTurnInput) (string, error)
}

type Bus interface {
	Subscribe(sessionID string) (<-chan types.Event, func())
}

type CronScheduler interface {
	ListJobs(context.Context, string) ([]types.ScheduledJob, error)
	GetJob(context.Context, string) (types.ScheduledJob, bool, error)
	SetJobEnabled(context.Context, string, bool) (types.ScheduledJob, bool, error)
	DeleteJob(context.Context, string) (bool, error)
}

var _ CronScheduler = (*scheduler.Service)(nil)

type AutomationService interface {
	ApplyRequest(context.Context, types.ApplyAutomationRequest) (types.AutomationSpec, error)
	Apply(context.Context, types.AutomationSpec) (types.AutomationSpec, error)
	Get(context.Context, string) (types.AutomationSpec, bool, error)
	List(context.Context, types.AutomationListFilter) ([]types.AutomationSpec, error)
	Control(context.Context, string, types.AutomationControlAction) (types.AutomationSpec, bool, error)
	InstallWatcher(context.Context, string) (types.AutomationWatcherRuntime, bool, error)
	ReinstallWatcher(context.Context, string) (types.AutomationWatcherRuntime, bool, error)
	GetWatcher(context.Context, string) (types.AutomationWatcherRuntime, bool, error)
	Delete(context.Context, string) (bool, error)
	EmitTrigger(context.Context, types.AutomationTriggerRequest) (types.AutomationIncident, error)
	RecordHeartbeat(context.Context, types.AutomationHeartbeatRequest) (types.AutomationHeartbeat, error)
	ListIncidents(context.Context, types.AutomationIncidentFilter) ([]types.AutomationIncident, error)
	GetIncident(context.Context, string) (types.AutomationIncident, bool, error)
	ControlIncident(context.Context, string, types.IncidentControlAction) (types.AutomationIncident, bool, error)
}

var _ AutomationService = (*automation.Service)(nil)
