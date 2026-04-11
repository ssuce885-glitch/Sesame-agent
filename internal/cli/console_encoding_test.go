package cli

import "testing"

func TestConfigureConsoleUTF8SetsInputAndOutputCodePages(t *testing.T) {
	var calls []uint32
	setter := func(codePage uint32, output bool) error {
		calls = append(calls, codePage)
		return nil
	}

	if err := configureConsoleUTF8(setter); err != nil {
		t.Fatalf("configureConsoleUTF8() error = %v", err)
	}
	if len(calls) != 2 {
		t.Fatalf("len(calls) = %d, want 2", len(calls))
	}
	if calls[0] != 65001 || calls[1] != 65001 {
		t.Fatalf("calls = %#v, want both code pages set to 65001", calls)
	}
}
