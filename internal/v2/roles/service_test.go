package roles

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestServiceListAndGet(t *testing.T) {
	root := t.TempDir()
	roleDir := filepath.Join(root, "roles", "doc_writer")
	if err := os.MkdirAll(roleDir, 0o755); err != nil {
		t.Fatal(err)
	}
	roleYAML := []byte(`
name: Documentation Writer
description: Keeps docs current.
model: test-model
permission_profile: workspace
denied_paths:
  - "*.go"
allowed_paths:
  - "docs/*"
budget:
  max_tool_calls: 12
  max_runtime: 90
  max_context_tokens: 80000
policy:
  can_delegate: true
  automation_ownership:
    - doc_writer
`)
	if err := os.WriteFile(filepath.Join(roleDir, "role.yaml"), roleYAML, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(roleDir, "prompt.md"), []byte("Write clear docs."), 0o644); err != nil {
		t.Fatal(err)
	}

	service := NewService(root)
	specs, err := service.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(specs) != 1 {
		t.Fatalf("expected one role, got %+v", specs)
	}
	if specs[0].ID != "doc_writer" || specs[0].Name != "Documentation Writer" || specs[0].MaxToolCalls != 12 || specs[0].MaxRuntime != 90 {
		t.Fatalf("unexpected role spec: %+v", specs[0])
	}
	if specs[0].MaxContextTokens != 80000 || !specs[0].CanDelegate {
		t.Fatalf("unexpected role budget/policy: %+v", specs[0])
	}
	if len(specs[0].AutomationOwners) != 1 || specs[0].AutomationOwners[0] != "doc_writer" {
		t.Fatalf("unexpected automation ownership: %+v", specs[0].AutomationOwners)
	}
	if len(specs[0].DeniedPaths) != 1 || specs[0].DeniedPaths[0] != "*.go" {
		t.Fatalf("unexpected denied paths: %+v", specs[0].DeniedPaths)
	}
	if len(specs[0].AllowedPaths) != 1 || specs[0].AllowedPaths[0] != "docs/*" {
		t.Fatalf("unexpected allowed paths: %+v", specs[0].AllowedPaths)
	}

	spec, ok, err := service.Get("doc_writer")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !ok {
		t.Fatal("expected role to exist")
	}
	if spec.SystemPrompt != "Write clear docs." {
		t.Fatalf("unexpected prompt: %q", spec.SystemPrompt)
	}

	_, ok, err = service.Get("missing")
	if err != nil {
		t.Fatalf("Get missing: %v", err)
	}
	if ok {
		t.Fatal("expected missing role")
	}
}

func TestServiceInstallRoleFromPath(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(t.TempDir(), "researcher")
	if err := os.MkdirAll(filepath.Join(source, "automations"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "role.yaml"), []byte("display_name: Researcher\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "prompt.md"), []byte("Research carefully."), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "automations", "watch.sh"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	service := NewService(root)
	if err := service.InstallRoleFromPath(context.Background(), source); err != nil {
		t.Fatalf("InstallRoleFromPath: %v", err)
	}
	spec, ok, err := service.Get("researcher")
	if err != nil {
		t.Fatalf("Get installed: %v", err)
	}
	if !ok || spec.Name != "Researcher" {
		t.Fatalf("unexpected installed role: ok=%v spec=%+v", ok, spec)
	}
	if _, err := os.Stat(filepath.Join(root, "roles", "researcher", "automations", "watch.sh")); err != nil {
		t.Fatalf("expected automation asset copied: %v", err)
	}
}

func TestServiceCreateAndUpdateRole(t *testing.T) {
	root := t.TempDir()
	service := NewService(root)

	created, err := service.Create(context.Background(), SaveInput{
		ID:                "frontend_reviewer",
		Name:              "Frontend Reviewer",
		Description:       "Reviews web UI changes.",
		SystemPrompt:      "Review UI with care.",
		PermissionProfile: "workspace",
		Model:             "test-model",
		MaxToolCalls:      8,
		MaxRuntime:        120,
		MaxContextTokens:  70000,
		SkillNames:        []string{"react", "react"},
		AllowedTools:      []string{"file_read"},
		DeniedPaths:       []string{"secrets/*"},
		CanDelegate:       true,
		AutomationOwners:  []string{"frontend_reviewer"},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.ID != "frontend_reviewer" || created.Version != 1 {
		t.Fatalf("unexpected created role: %+v", created)
	}
	if len(created.SkillNames) != 1 || created.SkillNames[0] != "react" {
		t.Fatalf("expected deduped skills: %+v", created.SkillNames)
	}

	roleYAML, err := os.ReadFile(filepath.Join(root, "roles", "frontend_reviewer", "role.yaml"))
	if err != nil {
		t.Fatalf("read role.yaml: %v", err)
	}
	if !strings.Contains(string(roleYAML), "display_name: Frontend Reviewer") {
		t.Fatalf("role.yaml missing display name:\n%s", roleYAML)
	}
	if !strings.Contains(string(roleYAML), "max_context_tokens: 70000") || !strings.Contains(string(roleYAML), "can_delegate: true") {
		t.Fatalf("role.yaml missing budget/policy:\n%s", roleYAML)
	}
	prompt, err := os.ReadFile(filepath.Join(root, "roles", "frontend_reviewer", "prompt.md"))
	if err != nil {
		t.Fatalf("read prompt.md: %v", err)
	}
	if string(prompt) != "Review UI with care." {
		t.Fatalf("unexpected prompt: %q", prompt)
	}
	if _, err := os.Stat(filepath.Join(root, "roles", "frontend_reviewer", ".role-versions", "000001.yaml")); err != nil {
		t.Fatalf("expected version snapshot: %v", err)
	}

	updated, err := service.Update(context.Background(), "frontend_reviewer", SaveInput{
		ID:           "frontend_reviewer",
		Name:         "Frontend Reviewer",
		Description:  "Reviews UI and interaction changes.",
		SystemPrompt: "Review UI and accessibility.",
		MaxToolCalls: 9,
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.Version != 2 || updated.Description != "Reviews UI and interaction changes." || updated.MaxToolCalls != 9 {
		t.Fatalf("unexpected updated role: %+v", updated)
	}
	if _, err := os.Stat(filepath.Join(root, "roles", "frontend_reviewer", ".role-versions", "000002.yaml")); err != nil {
		t.Fatalf("expected second version snapshot: %v", err)
	}
}

func TestServiceCreateRoleValidation(t *testing.T) {
	service := NewService(t.TempDir())
	_, err := service.Create(context.Background(), SaveInput{
		ID:           "../bad",
		Name:         "Bad",
		SystemPrompt: "Bad prompt.",
	})
	if !errors.Is(err, ErrInvalidRole) {
		t.Fatalf("expected invalid role error, got %v", err)
	}

	_, err = service.Update(context.Background(), "missing", SaveInput{
		ID:           "missing",
		Name:         "Missing",
		SystemPrompt: "Prompt.",
	})
	if !errors.Is(err, ErrRoleNotFound) {
		t.Fatalf("expected role not found, got %v", err)
	}
}
