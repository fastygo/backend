package identity

import (
	"context"
	"time"

	"github.com/fastygo/backend/internal/domain/audit"
	domainidentity "github.com/fastygo/backend/internal/domain/identity"
)

type Repository interface {
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

type AuditRepository interface {
	NextID(context.Context) (audit.ID, error)
	Save(context.Context, audit.Event) error
}

type Transaction interface {
	Identity() Repository
	Audit() AuditRepository
}

type Transactor interface {
	WithinIdentityTransaction(context.Context, func(Transaction) error) error
}

type Clock interface {
	Now() time.Time
}
