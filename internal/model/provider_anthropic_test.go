package model

import "testing"

func TestAnthropicProviderRejectsMissingAPIKey(t *testing.T) {
	_, err := NewAnthropicProvider(Config{
		APIKey: "",
		Model:  "claude-sonnet-4-5",
	})
	if err == nil {
		t.Fatal("expected error for missing API key")
	}
}
