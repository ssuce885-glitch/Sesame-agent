package cli

import "testing"

func TestParseOptionsSeparatesPromptFromFlags(t *testing.T) {
	opts, err := ParseOptions([]string{"--status", "check daemon"})
	if err != nil {
		t.Fatalf("ParseOptions() error = %v", err)
	}

	if !opts.ShowStatus {
		t.Fatal("ShowStatus = false, want true")
	}
	if opts.InitialPrompt != "check daemon" {
		t.Fatalf("InitialPrompt = %q, want %q", opts.InitialPrompt, "check daemon")
	}
}

func TestParseOptionsCapturesStartupOverrides(t *testing.T) {
	opts, err := ParseOptions([]string{
		"--resume", "sess_123",
		"--print",
		"--data-dir", "E:/tmp/agentd",
		"--model", "gpt-5.4",
		"--permission-mode", "trusted_local",
		"hello there",
	})
	if err != nil {
		t.Fatalf("ParseOptions() error = %v", err)
	}

	if opts.ResumeID != "sess_123" {
		t.Fatalf("ResumeID = %q, want %q", opts.ResumeID, "sess_123")
	}
	if !opts.PrintOnly {
		t.Fatal("PrintOnly = false, want true")
	}
	if opts.DataDir != "E:/tmp/agentd" {
		t.Fatalf("DataDir = %q, want %q", opts.DataDir, "E:/tmp/agentd")
	}
	if opts.Model != "gpt-5.4" {
		t.Fatalf("Model = %q, want %q", opts.Model, "gpt-5.4")
	}
	if opts.PermissionMode != "trusted_local" {
		t.Fatalf("PermissionMode = %q, want %q", opts.PermissionMode, "trusted_local")
	}
	if opts.InitialPrompt != "hello there" {
		t.Fatalf("InitialPrompt = %q, want %q", opts.InitialPrompt, "hello there")
	}
}
