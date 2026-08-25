package taxonomy

import (
	"context"
	"time"

	contentapplication "github.com/fastygo/backend/internal/application/content"
	"github.com/fastygo/backend/internal/domain/audit"
	"github.com/fastygo/backend/internal/domain/taxonomy"
)

type Repository interface {
	GetDefinition(context.Context, string) (taxonomy.Definition, error)
	ListDefinitions(context.Context) ([]taxonomy.Definition, error)
	SaveDefinition(context.Context, taxonomy.Definition, uint64) error
	DeleteDefinition(context.Context, string, uint64) error
	GetTerm(context.Context, taxonomy.ID) (taxonomy.Term, error)
	ListTerms(context.Context, string) ([]taxonomy.Term, error)
	SaveTerm(context.Context, taxonomy.Term, uint64) error
	DeleteTerm(context.Context, taxonomy.ID, uint64) error
}

type ContentRepository interface {
	List(context.Context, contentapplication.Query) (contentapplication.ListResult, error)
}

type AuditRepository interface {
	NextID(context.Context) (audit.ID, error)
	Save(context.Context, audit.Event) error
}

type Transaction interface {
	Taxonomies() Repository
	Content() ContentRepository
	Audit() AuditRepository
}

type Transactor interface {
	WithinTaxonomyTransaction(context.Context, func(Transaction) error) error
}

type Clock interface {
	Now() time.Time
}
