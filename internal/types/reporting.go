package types

import (
	"encoding/json"
	"time"
)

type ChildAgentMode string

const (
	ChildAgentModeBackgroundWorker ChildAgentMode = "background_worker"
	ChildAgentModeSubagent         ChildAgentMode = "subagent"
)

type ScheduleKind string

const (
	ScheduleKindEvery ScheduleKind = "every"
	ScheduleKindCron  ScheduleKind = "cron"
	ScheduleKindAt    ScheduleKind = "at"
)

type ContractSection struct {
	ID       string `json:"id"`
	Title    string `json:"title,omitempty"`
	Required bool   `json:"required,omitempty"`
	MaxItems int    `json:"max_items,omitempty"`
}

type OutputContractRules struct {
	IncludeSeverity                         bool `json:"include_severity,omitempty"`
	IncludeRawLogs                          bool `json:"include_raw_logs,omitempty"`
	MustBeConcise                           bool `json:"must_be_concise,omitempty"`
	MustIncludeActionItemsOnWarningOrHigher bool `json:"must_include_action_items_on_warning_or_higher,omitempty"`
}

type OutputContractUIHints struct {
	RenderAs   string `json:"render_as,omitempty"`
	Expandable bool   `json:"expandable,omitempty"`
}

type OutputContract struct {
	ContractID string                `json:"contract_id"`
	Intent     string                `json:"intent,omitempty"`
	Sections   []ContractSection     `json:"sections,omitempty"`
	Rules      OutputContractRules   `json:"rules,omitempty"`
	Tone       string                `json:"tone,omitempty"`
	UIHints    OutputContractUIHints `json:"ui_hints,omitempty"`
	CreatedAt  time.Time             `json:"created_at,omitempty"`
	UpdatedAt  time.Time             `json:"updated_at,omitempty"`
}

type ScheduleSpec struct {
	Kind         ScheduleKind `json:"kind,omitempty"`
	EveryMinutes int          `json:"every_minutes,omitempty"`
	Expr         string       `json:"expr,omitempty"`
	Timezone     string       `json:"timezone,omitempty"`
	At           string       `json:"at,omitempty"`
}

type ChildAgentSpec struct {
	AgentID             string         `json:"agent_id"`
	SessionID           string         `json:"session_id,omitempty"`
	Purpose             string         `json:"purpose,omitempty"`
	Mode                ChildAgentMode `json:"mode,omitempty"`
	ActivatedSkillNames []string       `json:"activated_skill_names,omitempty"`
	OutputContractRef   string         `json:"output_contract_ref,omitempty"`
	ReportGroups        []string       `json:"report_groups,omitempty"`
	Schedule            ScheduleSpec   `json:"schedule,omitempty"`
	CreatedAt           time.Time      `json:"created_at,omitempty"`
	UpdatedAt           time.Time      `json:"updated_at,omitempty"`
}

type AggregationContract struct {
	DedupeWindowMinutes int      `json:"dedupe_window_minutes,omitempty"`
	PriorityOrder       []string `json:"priority_order,omitempty"`
	Sections            []string `json:"sections,omitempty"`
}

type DeliveryProfile struct {
	Channels []string `json:"channels,omitempty"`
}

type ReportGroup struct {
	GroupID     string              `json:"group_id"`
	SessionID   string              `json:"session_id,omitempty"`
	Title       string              `json:"title,omitempty"`
	Sources     []string            `json:"sources,omitempty"`
	Schedule    ScheduleSpec        `json:"schedule,omitempty"`
	Aggregation AggregationContract `json:"aggregation,omitempty"`
	Delivery    DeliveryProfile     `json:"delivery,omitempty"`
	CreatedAt   time.Time           `json:"created_at,omitempty"`
	UpdatedAt   time.Time           `json:"updated_at,omitempty"`
}

type ReportSectionContent struct {
	ID    string   `json:"id"`
	Title string   `json:"title,omitempty"`
	Text  string   `json:"text,omitempty"`
	Items []string `json:"items,omitempty"`
}

type ReportEnvelope struct {
	Source   string                 `json:"source,omitempty"`
	Status   string                 `json:"status,omitempty"`
	Severity string                 `json:"severity,omitempty"`
	Title    string                 `json:"title,omitempty"`
	Summary  string                 `json:"summary,omitempty"`
	Sections []ReportSectionContent `json:"sections,omitempty"`
	Payload  json.RawMessage        `json:"payload,omitempty"`
}

type ChildAgentResult struct {
	ResultID        string         `json:"result_id"`
	SessionID       string         `json:"session_id,omitempty"`
	AgentID         string         `json:"agent_id"`
	ContractID      string         `json:"contract_id,omitempty"`
	RunID           string         `json:"run_id,omitempty"`
	TaskID          string         `json:"task_id,omitempty"`
	ReportGroupRefs []string       `json:"report_group_refs,omitempty"`
	ObservedAt      time.Time      `json:"observed_at,omitempty"`
	Envelope        ReportEnvelope `json:"envelope"`
	CreatedAt       time.Time      `json:"created_at,omitempty"`
	UpdatedAt       time.Time      `json:"updated_at,omitempty"`
}

type DigestRecord struct {
	DigestID        string          `json:"digest_id"`
	SessionID       string          `json:"session_id,omitempty"`
	GroupID         string          `json:"group_id"`
	RunID           string          `json:"run_id,omitempty"`
	TaskID          string          `json:"task_id,omitempty"`
	SourceResultIDs []string        `json:"source_result_ids,omitempty"`
	WindowStart     time.Time       `json:"window_start,omitempty"`
	WindowEnd       time.Time       `json:"window_end,omitempty"`
	Delivery        DeliveryProfile `json:"delivery,omitempty"`
	Envelope        ReportEnvelope  `json:"envelope"`
	CreatedAt       time.Time       `json:"created_at,omitempty"`
	UpdatedAt       time.Time       `json:"updated_at,omitempty"`
}

type ReportingOverview struct {
	ChildAgents     []ChildAgentSpec   `json:"child_agents"`
	OutputContracts []OutputContract   `json:"output_contracts"`
	ReportGroups    []ReportGroup      `json:"report_groups"`
	ChildResults    []ChildAgentResult `json:"child_results"`
	Digests         []DigestRecord     `json:"digests"`
}
