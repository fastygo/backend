package backup

import (
	"context"

	contentapplication "github.com/fastygo/backend/internal/application/content"
	"github.com/fastygo/backend/internal/domain/audit"
	"github.com/fastygo/backend/internal/domain/content"
	domainidentity "github.com/fastygo/backend/internal/domain/identity"
	"github.com/fastygo/backend/internal/domain/revision"
	"github.com/fastygo/backend/internal/domain/taxonomy"
)

type ContentRepository interface {
	List(context.Context, contentapplication.Query) (contentapplication.ListResult, error)
	Create(context.Context, content.Entry) error
}

type RevisionRepository interface {
	List(context.Context, content.ID, int, int) ([]revision.Revision, contentapplication.Page, error)
	Save(context.Context, revision.Revision) error
}

type AuditRepository interface {
	List(context.Context, contentapplication.AuditQuery) ([]audit.Event, contentapplication.Page, error)
	Save(context.Context, audit.Event) error
}

type TaxonomyRepository interface {
	ListDefinitions(context.Context) ([]taxonomy.Definition, error)
	SaveDefinition(context.Context, taxonomy.Definition, uint64) error
	ListTerms(context.Context, string) ([]taxonomy.Term, error)
	SaveTerm(context.Context, taxonomy.Term, uint64) error
}

type IdentityRepository interface {
	ListUsers(context.Context) ([]domainidentity.User, error)
	SaveUser(context.Context, domainidentity.User, uint64) error
	ListRoles(context.Context) ([]domainidentity.Role, error)
	SaveRole(context.Context, domainidentity.Role, uint64) error
}

type Transaction interface {
	Content() ContentRepository
	Revisions() RevisionRepository
	Audit() AuditRepository
	Taxonomies() TaxonomyRepository
	Identity() IdentityRepository
}

type Transactor interface {
	WithinBackupTransaction(context.Context, func(Transaction) error) error
}
