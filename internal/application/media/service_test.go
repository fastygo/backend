package media_test

import (
	"context"
	"io"
	"path/filepath"
	"strings"
	"testing"
	"time"

	contentapplication "github.com/fastygo/backend/internal/application/content"
	applicationmedia "github.com/fastygo/backend/internal/application/media"
	"github.com/fastygo/backend/internal/domain/authz"
	"github.com/fastygo/backend/internal/domain/content"
	bboltstorage "github.com/fastygo/backend/internal/storage/bbolt"
	"github.com/fastygo/backend/internal/storage/localmedia"
)

func TestMediaUploadPersistsMetadataAndEnforcesPrivateAccess(t *testing.T) {
	database, err := bboltstorage.Open(filepath.Join(t.TempDir(), "media.db"), 0o600, nil)
	if err != nil {
		t.Fatalf("open metadata storage: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	contentService, _ := contentapplication.NewService(database, nil, nil)
	blobs, _ := localmedia.Open(filepath.Join(t.TempDir(), "blobs"))
	service, _ := applicationmedia.NewService(contentService, blobs)
	uploader := authz.NewPrincipal("editor", authz.CapabilityMediaUpload)

	asset, err := service.Upload(context.Background(), uploader, applicationmedia.Upload{
		Filename: "photo.png", MIMEType: "image/png",
		Alt: content.LocalizedText{"en": "Photo"}, Reader: strings.NewReader("image"),
		Status: content.StatusDraft, Visibility: content.VisibilityPrivate,
	})
	if err != nil {
		t.Fatalf("upload media: %v", err)
	}
	if asset.Size != 5 || len(asset.Checksum) != 64 || asset.StorageKey == "" {
		t.Fatalf("unexpected media asset: %#v", asset)
	}
	if _, err := service.Open(context.Background(), authz.Anonymous(), asset.ID); err == nil {
		t.Fatalf("anonymous user opened private media")
	}

	reader := authz.NewPrincipal("reader", authz.CapabilityMediaReadPrivate)
	download, err := service.Open(context.Background(), reader, asset.ID)
	if err != nil {
		t.Fatalf("open private media: %v", err)
	}
	body, _ := io.ReadAll(download.Body)
	_ = download.Body.Close()
	if string(body) != "image" {
		t.Fatalf("unexpected media body: %q", body)
	}
}

func TestPublicPublishedMediaIsReadableAnonymously(t *testing.T) {
	database, _ := bboltstorage.Open(filepath.Join(t.TempDir(), "public.db"), 0o600, nil)
	t.Cleanup(func() { _ = database.Close() })
	contentService, _ := contentapplication.NewService(database, nil, mediaClock{time.Now().UTC()})
	blobs, _ := localmedia.Open(filepath.Join(t.TempDir(), "public-blobs"))
	service, _ := applicationmedia.NewService(contentService, blobs)
	uploader := authz.NewPrincipal(
		"publisher", authz.CapabilityMediaUpload, authz.CapabilityContentPublish,
	)
	asset, err := service.Upload(context.Background(), uploader, applicationmedia.Upload{
		Filename: "manual.pdf", MIMEType: "application/pdf", Reader: strings.NewReader("document"),
		Status: content.StatusPublished, Visibility: content.VisibilityPublic,
	})
	if err != nil {
		t.Fatalf("upload public media: %v", err)
	}
	download, err := service.Open(context.Background(), authz.Anonymous(), asset.ID)
	if err != nil {
		t.Fatalf("open public media: %v", err)
	}
	_ = download.Body.Close()
}

type mediaClock struct {
	value time.Time
}

func (clock mediaClock) Now() time.Time {
	return clock.value
}
