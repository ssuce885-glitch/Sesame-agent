package model

import (
	"testing"

	"go-agent/internal/config"
)

func TestNewFromConfigRejectsEmptyProvider(t *testing.T) {
	_, err := NewFromConfig(config.Config{})
	if err == nil {
		t.Fatal("NewFromConfig() error = nil, want error")
	}
}
