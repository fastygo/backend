package rest

import (
	"testing"

	application "github.com/fastygo/backend/internal/application/content"
	domainidentity "github.com/fastygo/backend/internal/domain/identity"
)

func TestProjectPageAndPublicUser(t *testing.T) {
	t.Parallel()
	cases := map[string]struct {
		page application.Page
		want paginationDocument
	}{
		"first page": {
			page: application.Page{Number: 1, PerPage: 20, Total: 21, TotalPages: 2},
			want: paginationDocument{Page: 1, PerPage: 20, Total: 21, TotalPages: 2},
		},
		"empty": {
			page: application.Page{Number: 1, PerPage: 10},
			want: paginationDocument{Page: 1, PerPage: 10},
		},
	}
	for name, test := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			got := projectPage(test.page)
			if got != test.want {
				t.Fatalf("pagination DTO=%#v want %#v", got, test.want)
			}
		})
	}
	user := projectUser(domainidentity.User{
		ID: "user_1", Email: "a@example.com", DisplayName: "A",
		PasswordHash: "must-not-copy", RoleIDs: []string{"editor"}, Active: true, Version: 3,
	})
	if user.Email != "a@example.com" || user.Version != 3 || user.DisplayName != "A" {
		t.Fatalf("public user DTO leaked or dropped fields: %#v", user)
	}
}
