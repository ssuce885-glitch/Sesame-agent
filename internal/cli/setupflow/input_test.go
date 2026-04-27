package setupflow

import (
	"bufio"
	"bytes"
	"strings"
	"testing"
)

func TestChooseArrowOptionAcceptsNumberedSelection(t *testing.T) {
	var out bytes.Buffer

	got, err := chooseArrowOption(
		bufio.NewReader(strings.NewReader("2\n")),
		&out,
		"Select section",
		[]string{"Model Setup", "Third-Party Integrations", "Save and Exit"},
		0,
	)
	if err != nil {
		t.Fatalf("chooseArrowOption returned error: %v", err)
	}
	if got != 1 {
		t.Fatalf("selection = %d, want 1", got)
	}
	if !strings.Contains(out.String(), "2) Third-Party Integrations") {
		t.Fatalf("output did not include numbered options:\n%s", out.String())
	}
}

func TestChooseArrowOptionUsesDefaultOnEnter(t *testing.T) {
	got, err := chooseArrowOption(
		bufio.NewReader(strings.NewReader("\n")),
		&bytes.Buffer{},
		"Enable Discord integration",
		[]string{"Enabled", "Disabled"},
		1,
	)
	if err != nil {
		t.Fatalf("chooseArrowOption returned error: %v", err)
	}
	if got != 1 {
		t.Fatalf("selection = %d, want default 1", got)
	}
}

func TestChooseArrowOptionRepromptsInvalidSelection(t *testing.T) {
	var out bytes.Buffer

	got, err := chooseArrowOption(
		bufio.NewReader(strings.NewReader("\x1b[A\n3\n")),
		&out,
		"Select section",
		[]string{"Model Setup", "Third-Party Integrations", "Save and Exit"},
		0,
	)
	if err != nil {
		t.Fatalf("chooseArrowOption returned error: %v", err)
	}
	if got != 2 {
		t.Fatalf("selection = %d, want 2", got)
	}
	if !strings.Contains(out.String(), "Invalid selection") {
		t.Fatalf("output did not include invalid selection guidance:\n%s", out.String())
	}
}
