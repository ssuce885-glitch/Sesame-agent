package automation

import (
	"encoding/json"
	"testing"

	"go-agent/internal/types"
)

func TestCompileWatcherSignalsSimpleContinueDefaultsCooldown(t *testing.T) {
	signals, lifecycle, err := compileWatcherSignals(types.AutomationSpec{
		ID:               "cleanup_docs_a",
		Mode:             types.AutomationModeSimple,
		WatcherLifecycle: json.RawMessage(`{"after_dispatch":"continue"}`),
		Signals: []types.AutomationSignal{{
			Kind:     "poll",
			Selector: "echo match",
			Payload:  json.RawMessage(`{"trigger_on":"stdout_contains","match":"match"}`),
		}},
	})
	if err != nil {
		t.Fatalf("compileWatcherSignals() error = %v", err)
	}
	if lifecycle.AfterDispatch != "continue" {
		t.Fatalf("AfterDispatch = %q, want continue", lifecycle.AfterDispatch)
	}
	if len(signals) != 1 {
		t.Fatalf("len(signals) = %d, want 1", len(signals))
	}
	if signals[0].CooldownSeconds != 3600 {
		t.Fatalf("CooldownSeconds = %d, want 3600", signals[0].CooldownSeconds)
	}
}
