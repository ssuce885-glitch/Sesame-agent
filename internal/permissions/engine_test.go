package permissions

import "testing"

func TestNewEngineDefaultsToTrustedLocal(t *testing.T) {
	engine := NewEngine()
	if got := engine.Profile(); got != ProfileTrustedLocal {
		t.Fatalf("Profile() = %q, want %q", got, ProfileTrustedLocal)
	}
	if got := engine.Decide("shell_command"); got != DecisionAllow {
		t.Fatalf("Decide(shell_command) = %q, want %q", got, DecisionAllow)
	}
	if !engine.AllowsAll() {
		t.Fatal("AllowsAll() = false, want true")
	}
}

func TestReadOnlyProfileStillRestrictsTools(t *testing.T) {
	engine := NewEngine(ProfileReadOnly)
	if got := engine.Decide("file_read"); got != DecisionAllow {
		t.Fatalf("Decide(file_read) = %q, want %q", got, DecisionAllow)
	}
	if got := engine.Decide("shell_command"); got != DecisionDeny {
		t.Fatalf("Decide(shell_command) = %q, want %q", got, DecisionDeny)
	}
	if engine.AllowsAll() {
		t.Fatal("AllowsAll() = true, want false")
	}
}

func TestDefaultModeAllows(t *testing.T) {
	if got := DefaultMode(); got != DecisionAllow {
		t.Fatalf("DefaultMode() = %q, want %q", got, DecisionAllow)
	}
}
