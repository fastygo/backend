package revision

import (
	"errors"
	"strings"
	"time"

	"github.com/fastygo/backend/internal/domain/content"
)

type ID string

type Revision struct {
	ID        ID
	EntryID   content.ID
	Version   uint64
	Snapshot  content.Entry
	AuthorID  string
	Reason    string
	CreatedAt time.Time
}

func (revision Revision) Validate() error {
	switch {
	case strings.TrimSpace(string(revision.ID)) == "":
		return errors.New("revision id is required")
	case revision.EntryID == "":
		return errors.New("revision entry id is required")
	case revision.EntryID != revision.Snapshot.ID:
		return errors.New("revision snapshot entry does not match")
	case revision.Version == 0 || revision.Version != revision.Snapshot.Version:
		return errors.New("revision version does not match snapshot")
	case strings.TrimSpace(revision.AuthorID) == "":
		return errors.New("revision author is required")
	case revision.CreatedAt.IsZero():
		return errors.New("revision created_at is required")
	default:
		return nil
	}
}
