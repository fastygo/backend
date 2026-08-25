package content

import (
	"context"
	"time"

	"github.com/fastygo/backend/internal/domain/audit"
	"github.com/fastygo/backend/internal/domain/content"
	domainidentity "github.com/fastygo/backend/internal/domain/identity"
	"github.com/fastygo/backend/internal/domain/revision"
	"github.com/fastygo/backend/internal/domain/taxonomy"
)

type Query struct {
	Kinds         []content.Kind
	Statuses      []content.Status
	AuthorID      string
	TaxonomyID    string
	TermID        string
	Search        string
	Locale        string
	After         *time.Time
	Before        *time.Time
	PublishBefore *time.Time
	Page          int
	PerPage       int
	Sort          string
	Descending    bool
	PublicOnly    bool
	PublicAt      time.Time
}

type Page struct {
	Number     int `json:"page"`
	PerPage    int `json:"per_page"`
	Total      int `json:"total"`
	TotalPages int `json:"total_pages"`
}

type ListResult struct {
	Entries []content.Entry
	Page    Page
}

// Repository is owned by the content application and implemented by storage adapters.
type Repository interface {
	Get(context.Context, content.ID) (content.Entry, error)
	GetBySlug(context.Context, content.Kind, string, string) (content.Entry, error)
	List(context.Context, Query) (ListResult, error)
	Create(context.Context, content.Entry) error
	Update(context.Context, content.Entry, uint64) error
	Delete(context.Context, content.ID, uint64) error
	SlugExists(context.Context, content.Kind, string, string, content.ID) (bool, error)
}

type RevisionRepository interface {
	NextID(context.Context) (revision.ID, error)
	Save(context.Context, revision.Revision) error
	Get(context.Context, revision.ID) (revision.Revision, error)
	List(context.Context, content.ID, int, int) ([]revision.Revision, Page, error)
}

type Transaction interface {
	Content() Repository
	Revisions() RevisionRepository
	Audit() AuditRepository
	Taxonomies() TaxonomyRepository
	Identity() IdentityRepository
}

type Transactor interface {
	WithinTransaction(context.Context, func(Transaction) error) error
}

type LifecycleEvent struct {
	Name      string
	Principal string
	Before    *content.Entry
	After     *content.Entry
}

type Hooks interface {
	Before(context.Context, LifecycleEvent) error
	After(context.Context, LifecycleEvent) error
}

type Clock interface {
	Now() time.Time
}

type AuditQuery struct {
	ActorID    string
	Action     string
	Resource   string
	ResourceID string
	After      *time.Time
	Before     *time.Time
	Page       int
	PerPage    int
}

type AuditRepository interface {
	NextID(context.Context) (audit.ID, error)
	Save(context.Context, audit.Event) error
	List(context.Context, AuditQuery) ([]audit.Event, Page, error)
}

type TaxonomyRepository interface {
	GetDefinition(context.Context, string) (taxonomy.Definition, error)
	ListDefinitions(context.Context) ([]taxonomy.Definition, error)
	SaveDefinition(context.Context, taxonomy.Definition, uint64) error
	DeleteDefinition(context.Context, string, uint64) error
	GetTerm(context.Context, taxonomy.ID) (taxonomy.Term, error)
	ListTerms(context.Context, string) ([]taxonomy.Term, error)
	SaveTerm(context.Context, taxonomy.Term, uint64) error
	DeleteTerm(context.Context, taxonomy.ID, uint64) error
}

type IdentityRepository interface {
	GetUser(context.Context, string) (domainidentity.User, error)
	GetUserByEmail(context.Context, string) (domainidentity.User, error)
	ListUsers(context.Context) ([]domainidentity.User, error)
	SaveUser(context.Context, domainidentity.User, uint64) error
	DeleteUser(context.Context, string, uint64) error
	GetRole(context.Context, string) (domainidentity.Role, error)
	ListRoles(context.Context) ([]domainidentity.Role, error)
	SaveRole(context.Context, domainidentity.Role, uint64) error
	DeleteRole(context.Context, string, uint64) error
}
