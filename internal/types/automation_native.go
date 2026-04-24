package types

type SimpleAutomationBuilderInput struct {
	AutomationID     string                 `json:"automation_id"`
	Owner            string                 `json:"owner"`
	WatchScript      string                 `json:"watch_script"`
	IntervalSeconds  int                    `json:"interval_seconds"`
	Title            string                 `json:"title,omitempty"`
	Goal             string                 `json:"goal,omitempty"`
	TimeoutSeconds   int                    `json:"timeout_seconds,omitempty"`
	ReportTarget     string                 `json:"report_target,omitempty"`
	EscalationTarget string                 `json:"escalation_target,omitempty"`
	SimplePolicy     SimpleAutomationPolicy `json:"simple_policy,omitempty"`
}
