package permissions

type Decision string

const (
	DecisionAllow Decision = "allow"
	DecisionAsk   Decision = "ask"
	DecisionDeny  Decision = "deny"
)

type Engine struct{}

func NewEngine() *Engine {
	return &Engine{}
}

func (e *Engine) Decide(toolName string) Decision {
	switch toolName {
	case "file_read", "glob", "grep":
		return DecisionAllow
	case "file_write", "shell_command":
		return DecisionAsk
	default:
		return DecisionDeny
	}
}
