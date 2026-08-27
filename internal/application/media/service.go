package media

import (
	"context"
	"errors"
	"io"
	"mime"
	"path/filepath"
	"strings"
	"time"

	contentapplication "github.com/fastygo/backend/internal/application/content"
	"github.com/fastygo/backend/internal/domain/authz"
	"github.com/fastygo/backend/internal/domain/content"
	domainmedia "github.com/fastygo/backend/internal/domain/media"
	"github.com/fastygo/framework/pkg/core"
	"github.com/google/uuid"
)

type StoredObject struct {
	Key      string
	Size     int64
	Checksum string
}

type BlobStore interface {
	Put(context.Context, string, io.Reader, int64) (StoredObject, error)
	Open(context.Context, string) (io.ReadCloser, error)
	Delete(context.Context, string) error
}

type Upload struct {
	Filename   string
	MIMEType   string
	Alt        content.LocalizedText
	Status     content.Status
	Visibility content.Visibility
	Reader     io.Reader
	MaxBytes   int64
}

type Download struct {
	Asset domainmedia.Asset
	Body  io.ReadCloser
}

type Service struct {
	content *contentapplication.Service
	blobs   BlobStore
	now     func() time.Time
}

func NewService(contentService *contentapplication.Service, blobs BlobStore) (*Service, error) {
	if contentService == nil || blobs == nil {
		return nil, errors.New("media content service and blob store are required")
	}
	return &Service{content: contentService, blobs: blobs, now: time.Now}, nil
}

func (service *Service) Upload(
	ctx context.Context,
	principal authz.Principal,
	input Upload,
) (domainmedia.Asset, error) {
	if !principal.Has(authz.CapabilityMediaUpload) {
		return domainmedia.Asset{}, core.NewDomainError(core.ErrorCodeForbidden, "media.upload is required")
	}
	input.Filename = filepath.Base(strings.TrimSpace(input.Filename))
	if input.Filename == "" || input.Filename == "." || input.Reader == nil {
		return domainmedia.Asset{}, core.NewDomainError(core.ErrorCodeValidation, "media file is required")
	}
	if input.MaxBytes <= 0 {
		input.MaxBytes = 32 << 20
	}
	if input.Status == "" {
		input.Status = content.StatusDraft
	}
	if input.Visibility == "" {
		input.Visibility = content.VisibilityPrivate
	}
	if strings.TrimSpace(input.MIMEType) == "" {
		input.MIMEType = mime.TypeByExtension(filepath.Ext(input.Filename))
	}
	if input.MIMEType == "" {
		input.MIMEType = "application/octet-stream"
	}
	id := content.ID("media_" + uuid.NewString())
	key := storageKey(id, filepath.Ext(input.Filename))
	stored, err := service.blobs.Put(ctx, key, input.Reader, input.MaxBytes)
	if err != nil {
		return domainmedia.Asset{}, core.WrapDomainError(core.ErrorCodeValidation, "media upload failed", err)
	}
	title := content.LocalizedText{}
	for locale, value := range input.Alt {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			title[locale] = trimmed
		}
	}
	if len(title) == 0 {
		title = content.LocalizedText{"en": input.Filename}
	}
	now := service.now().UTC()
	entry := content.Entry{
		ID: id, Kind: "media", Status: input.Status, Visibility: input.Visibility,
		Slug:  content.LocalizedText{"en": content.NormalizeSlug(strings.TrimSuffix(input.Filename, filepath.Ext(input.Filename))) + "-" + string(id[len(id)-8:])},
		Title: title, AuthorID: principal.ID, Version: 1, CreatedAt: now, UpdatedAt: now,
		Metadata: map[string]content.MetadataValue{
			domainmedia.MetadataFilename:   {Value: input.Filename},
			domainmedia.MetadataMIMEType:   {Value: input.MIMEType},
			domainmedia.MetadataSize:       {Value: stored.Size},
			domainmedia.MetadataChecksum:   {Value: stored.Checksum},
			domainmedia.MetadataStorageKey: {Value: stored.Key, Private: true},
		},
	}
	created, err := service.content.Create(ctx, principal, entry)
	if err != nil {
		_ = service.blobs.Delete(context.WithoutCancel(ctx), stored.Key)
		return domainmedia.Asset{}, err
	}
	asset, err := domainmedia.FromEntry(created)
	if err != nil {
		return domainmedia.Asset{}, core.WrapDomainError(core.ErrorCodeInternal, "created media is invalid", err)
	}
	return asset, nil
}

func (service *Service) Open(
	ctx context.Context,
	principal authz.Principal,
	id content.ID,
) (Download, error) {
	entry, err := service.content.GetAuthorized(ctx, principal, id, authz.CapabilityMediaReadPrivate)
	if err != nil {
		return Download{}, err
	}
	asset, err := domainmedia.FromEntry(entry)
	if err != nil {
		return Download{}, core.WrapDomainError(core.ErrorCodeNotFound, "media was not found", err)
	}
	body, err := service.blobs.Open(ctx, asset.StorageKey)
	if err != nil {
		return Download{}, core.WrapDomainError(core.ErrorCodeNotFound, "media blob was not found", err)
	}
	return Download{Asset: asset, Body: body}, nil
}

func storageKey(id content.ID, extension string) string {
	extension = strings.ToLower(extension)
	if len(extension) > 16 {
		extension = ""
	}
	value := string(id)
	return value[:min(10, len(value))] + "/" + value + extension
}
