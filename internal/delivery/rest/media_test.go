package rest

import (
	"bytes"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	contentapplication "github.com/fastygo/backend/internal/application/content"
	applicationmedia "github.com/fastygo/backend/internal/application/media"
	"github.com/fastygo/backend/internal/domain/authz"
	bboltstorage "github.com/fastygo/backend/internal/storage/bbolt"
	"github.com/fastygo/backend/internal/storage/localmedia"
)

func TestMediaRESTUploadAndAnonymousDownload(t *testing.T) {
	database, _ := bboltstorage.Open(filepath.Join(t.TempDir(), "media.db"), 0o600, nil)
	t.Cleanup(func() { _ = database.Close() })
	contentService, _ := contentapplication.NewService(database, nil, nil)
	blobs, _ := localmedia.Open(filepath.Join(t.TempDir(), "blobs"))
	mediaService, _ := applicationmedia.NewService(contentService, blobs)
	uploader := authz.NewPrincipal(
		"publisher", authz.CapabilityMediaUpload, authz.CapabilityContentPublish,
	)
	handler, _ := NewMediaHandler(mediaService, fixedPrincipal{principal: uploader}, 1<<20)
	mux := http.NewServeMux()
	handler.Routes(mux)

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	_ = writer.WriteField("status", "published")
	_ = writer.WriteField("visibility", "public")
	_ = writer.WriteField("alt", "Manual")
	file, _ := writer.CreateFormFile("file", "manual.txt")
	_, _ = io.WriteString(file, "documentation")
	_ = writer.Close()
		request := httptest.NewRequest(http.MethodPost, "/go-json/go/v2/media", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, request)
	if response.Code != http.StatusCreated {
		t.Fatalf("upload status %d: %s", response.Code, response.Body.String())
	}
	if response.Header().Get("Location") == "" || bytes.Contains(response.Body.Bytes(), []byte("storage_key")) {
		t.Fatalf("media response leaked storage details or omitted location")
	}

	publicHandler, _ := NewMediaHandler(mediaService, nil, 1<<20)
	publicMux := http.NewServeMux()
	publicHandler.Routes(publicMux)
	download := httptest.NewRecorder()
	publicMux.ServeHTTP(download, httptest.NewRequest(http.MethodGet, response.Header().Get("Location"), nil))
	if download.Code != http.StatusOK || download.Body.String() != "documentation" {
		t.Fatalf("download failed: %d %q", download.Code, download.Body.String())
	}
	if download.Header().Get("ETag") == "" || download.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("download security headers are missing")
	}
}
