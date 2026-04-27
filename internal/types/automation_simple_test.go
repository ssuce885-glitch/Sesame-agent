package types

import "testing"

func TestNormalizeRoleAutomationOwnerAcceptsRole(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"role:log_repairer", "role:log_repairer"},
		{"role:foo-bar_1", "role:foo-bar_1"},
		{" role:foo ", "role:foo"},
		{"role: foo", "role:foo"},
	}
	for _, tc := range cases {
		if got := NormalizeRoleAutomationOwner(tc.in); got != tc.want {
			t.Fatalf("NormalizeRoleAutomationOwner(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestNormalizeRoleAutomationOwnerRejectsBareRoleName(t *testing.T) {
	if got := NormalizeRoleAutomationOwner("log_repairer"); got != "" {
		t.Fatalf("NormalizeRoleAutomationOwner returned %q, want empty", got)
	}
}

func TestNormalizeRoleAutomationOwnerRejectsEmptyMainAgentAndInvalidRoleForms(t *testing.T) {
	cases := []string{
		"",
		"   ",
		"main_agent",
		"role:",
		"role:   ",
		"role:Foo",
		"role:foo/bar",
	}
	for _, in := range cases {
		if got := NormalizeRoleAutomationOwner(in); got != "" {
			t.Fatalf("NormalizeRoleAutomationOwner(%q) = %q, want empty", in, got)
		}
	}
}
