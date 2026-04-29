package daemon

import (
	"go-agent/internal/automation"
	"go-agent/internal/engine"
	"go-agent/internal/reporting"
	"go-agent/internal/runtimegraph"
	"go-agent/internal/scheduler"
	"go-agent/internal/session"
	"go-agent/internal/store/sqlite"
	"go-agent/internal/stream"
	"go-agent/internal/task"
)

// Runtime holds the daemon-owned runtime services after bootstrap/build completes.
type Runtime struct {
	Store             *sqlite.Store
	Bus               *stream.Bus
	Engine            *engine.Engine
	FileCheckpoints   *engine.FileCheckpointService
	SessionManager    *session.Manager
	TaskManager       *task.Manager
	RuntimeService    *runtimegraph.Service
	AutomationService *automation.Service
	WatcherService    *automation.WatcherService
	SchedulerService  *scheduler.Service
	ReportingService  *reporting.Service
	TaskNotifier      *taskTerminalNotifier
}
