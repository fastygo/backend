package persist

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/fastygo/backend/internal/domain/content"
	"github.com/fastygo/backend/internal/domain/revision"
)

type Revision struct {
	ID        revision.ID `json:"id"`
	EntryID   content.ID  `json:"entry_id"`
	Version   uint64      `json:"version"`
	Snapshot  Entry       `json:"snapshot"`
	AuthorID  string      `json:"author_id"`
	Reason    string      `json:"reason,omitempty"`
	CreatedAt time.Time   `json:"created_at"`
}

func RevisionFromDomain(item revision.Revision) Revision {
	return Revision{
		ID: item.ID, EntryID: item.EntryID, Version: item.Version,
		Snapshot: EntryFromDomain(item.Snapshot), AuthorID: item.AuthorID,
		Reason: item.Reason, CreatedAt: item.CreatedAt,
	}
}

func (item Revision) Domain() revision.Revision {
	return revision.Revision{
		ID: item.ID, EntryID: item.EntryID, Version: item.Version,
		Snapshot: item.Snapshot.Domain(), AuthorID: item.AuthorID,
		Reason: item.Reason, CreatedAt: item.CreatedAt,
	}
}

func EncodeRevision(item revision.Revision) ([]byte, error) {
	encoded, err := json.Marshal(RevisionFromDomain(item))
	if err != nil {
		return nil, fmt.Errorf("failed to encode revision: %w", err)
	}
	return encoded, nil
}

func DecodeRevision(encoded []byte) (revision.Revision, error) {
	var document Revision
	if err := json.Unmarshal(encoded, &document); err != nil {
		return revision.Revision{}, fmt.Errorf("failed to decode revision: %w", err)
	}
	return document.Domain(), nil
}
