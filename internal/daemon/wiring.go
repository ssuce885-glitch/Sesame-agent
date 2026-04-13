package daemon

import "fmt"

func validateRuntime(runtime *Runtime) error {
	switch {
	case runtime == nil:
		return fmt.Errorf("daemon runtime wiring invalid: runtime is nil")
	case runtime.Engine == nil:
		return fmt.Errorf("daemon runtime wiring invalid: engine missing")
	case runtime.TaskManager == nil:
		return fmt.Errorf("daemon runtime wiring invalid: engine missing task manager")
	case runtime.AutomationService == nil:
		return fmt.Errorf("daemon runtime wiring invalid: engine missing automation service")
	case runtime.RuntimeService == nil:
		return fmt.Errorf("daemon runtime wiring invalid: engine missing runtime service")
	case runtime.SchedulerService == nil:
		return fmt.Errorf("daemon runtime wiring invalid: engine missing scheduler service")
	case runtime.SessionManager == nil:
		return fmt.Errorf("daemon runtime wiring invalid: session manager missing")
	}
	return nil
}
