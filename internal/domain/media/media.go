package media

import (
	"errors"
	"strings"

	"github.com/fastygo/backend/internal/domain/content"
)

const (
	MetadataFilename   = "media_filename"
	MetadataMIMEType   = "media_mime_type"
	MetadataSize       = "media_size"
	MetadataChecksum   = "media_checksum_sha256"
	MetadataStorageKey = "media_storage_key"
)

type Asset struct {
	ID         content.ID            `json:"id"`
	Status     content.Status        `json:"status"`
	Visibility content.Visibility    `json:"visibility"`
	Filename   string                `json:"filename"`
	MIMEType   string                `json:"mime_type"`
	Size       int64                 `json:"size"`
	Checksum   string                `json:"checksum_sha256"`
	StorageKey string                `json:"storage_key"`
	Alt        content.LocalizedText `json:"alt"`
	Version    uint64                `json:"version"`
}

func FromEntry(entry content.Entry) (Asset, error) {
	if entry.Kind != "media" {
		return Asset{}, errors.New("content entry is not media")
	}
	asset := Asset{
		ID: entry.ID, Status: entry.Status, Visibility: entry.Visibility,
		Filename:   stringMetadata(entry, MetadataFilename),
		MIMEType:   stringMetadata(entry, MetadataMIMEType),
		Checksum:   stringMetadata(entry, MetadataChecksum),
		StorageKey: stringMetadata(entry, MetadataStorageKey),
		Alt:        entry.Title, Version: entry.Version,
	}
	switch value := entry.Metadata[MetadataSize].Value.(type) {
	case float64:
		asset.Size = int64(value)
	case int64:
		asset.Size = value
	case int:
		asset.Size = int64(value)
	default:
		return Asset{}, errors.New("media size metadata is invalid")
	}
	if err := asset.Validate(); err != nil {
		return Asset{}, err
	}
	return asset, nil
}

func (asset Asset) Validate() error {
	switch {
	case asset.ID == "":
		return errors.New("media id is required")
	case strings.TrimSpace(asset.Filename) == "":
		return errors.New("media filename is required")
	case strings.TrimSpace(asset.MIMEType) == "":
		return errors.New("media MIME type is required")
	case asset.Size < 0:
		return errors.New("media size is invalid")
	case len(asset.Checksum) != 64:
		return errors.New("media checksum is invalid")
	case strings.TrimSpace(asset.StorageKey) == "":
		return errors.New("media storage key is required")
	default:
		return nil
	}
}

func stringMetadata(entry content.Entry, key string) string {
	value, exists := entry.Metadata[key]
	if !exists {
		return ""
	}
	resolved, _ := value.Value.(string)
	return resolved
}
