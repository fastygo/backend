package authz

import "testing"

func TestPrincipalCanEdit(t *testing.T) {
	t.Parallel()
	cases := map[string]struct {
		principal Principal
		authorID  string
		allowed   bool
	}{
		"edit others": {
			principal: NewPrincipal("editor", CapabilityContentEditOthers),
			authorID:  "author",
			allowed:   true,
		},
		"edit own": {
			principal: NewPrincipal("author", CapabilityContentEditOwn),
			authorID:  "author",
			allowed:   true,
		},
		"edit own of another author": {
			principal: NewPrincipal("editor", CapabilityContentEditOwn),
			authorID:  "author",
		},
		"anonymous": {
			principal: Anonymous(),
			authorID:  "author",
		},
	}
	for name, test := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if got := test.principal.CanEdit(test.authorID); got != test.allowed {
				t.Fatalf("CanEdit=%v want %v", got, test.allowed)
			}
		})
	}
}

func TestRoleValidation(t *testing.T) {
	t.Parallel()
	cases := map[string]struct {
		role      Role
		wantError bool
	}{
		"valid editor": {role: EditorRole()},
		"missing label": {
			role:      Role{ID: "reviewer", Capabilities: []Capability{CapabilityContentReadPrivate}},
			wantError: true,
		},
		"unknown capability": {
			role:      Role{ID: "reviewer", Label: "Reviewer", Capabilities: []Capability{"not.a.capability"}},
			wantError: true,
		},
		"duplicate capability": {
			role: Role{
				ID: "reviewer", Label: "Reviewer",
				Capabilities: []Capability{CapabilityContentReadPrivate, CapabilityContentReadPrivate},
			},
			wantError: true,
		},
	}
	for name, test := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			err := test.role.Validate()
			if test.wantError && err == nil {
				t.Fatalf("expected role validation error")
			}
			if !test.wantError && err != nil {
				t.Fatalf("valid role rejected: %v", err)
			}
		})
	}
}
