package rest

import (
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"

	applicationmedia "github.com/fastygo/backend/internal/application/media"
	"github.com/fastygo/backend/internal/domain/authz"
	"github.com/fastygo/backend/internal/domain/content"
	domainmedia "github.com/fastygo/backend/internal/domain/media"
	"github.com/fastygo/framework/pkg/core"
)

type MediaHandler struct {
	service   *applicationmedia.Service
	principal PrincipalResolver
	maxBytes  int64
}

func NewMediaHandler(
	service *applicationmedia.Service,
	principal PrincipalResolver,
	maxBytes int64,
) (*MediaHandler, error) {
	if service == nil {
		return nil, core.NewDomainError(core.ErrorCodeInternal, "media service is required")
	}
	if maxBytes <= 0 {
		maxBytes = 32 << 20
	}
	return &MediaHandler{service: service, principal: principal, maxBytes: maxBytes}, nil
}

func (handler *MediaHandler) Routes(mux *http.ServeMux) {
	mux.HandleFunc("POST /go-json/go/v2/media", handler.upload)
	mux.HandleFunc("GET /go-json/go/v2/media/{id}/content", handler.download)
}

func (handler *MediaHandler) upload(response http.ResponseWriter, request *http.Request) {
	principal, ok := resolvePrincipal(handler.principal, response, request)
	if !ok {
		return
	}
	request.Body = http.MaxBytesReader(response, request.Body, handler.maxBytes+(1<<20))
	if err := request.ParseMultipartForm(handler.maxBytes); err != nil {
		writeError(response, request, core.WrapDomainError(core.ErrorCodeValidation, "invalid media upload", err))
		return
	}
	if request.MultipartForm != nil {
		defer request.MultipartForm.RemoveAll()
	}
	file, header, err := request.FormFile("file")
	if err != nil {
		writeError(response, request, core.WrapDomainError(core.ErrorCodeValidation, "media file is required", err))
		return
	}
	defer file.Close()
	locale := strings.TrimSpace(request.FormValue("locale"))
	if locale == "" {
		locale = "en"
	}
	alt := content.LocalizedText{}
	if value := strings.TrimSpace(request.FormValue("alt")); value != "" {
		alt[locale] = value
	}
	asset, err := handler.service.Upload(request.Context(), principal, applicationmedia.Upload{
		Filename: header.Filename, MIMEType: header.Header.Get("Content-Type"),
		Alt:        alt,
		Status:     content.Status(request.FormValue("status")),
		Visibility: content.Visibility(request.FormValue("visibility")),
		Reader:     file, MaxBytes: handler.maxBytes,
	})
	if err != nil {
		writeError(response, request, err)
		return
	}
	response.Header().Set("Location", "/go-json/go/v2/media/"+string(asset.ID)+"/content")
	writeJSON(response, http.StatusCreated, map[string]any{"data": publicMedia(asset)})
}

func (handler *MediaHandler) download(response http.ResponseWriter, request *http.Request) {
	principal, ok := resolvePrincipal(handler.principal, response, request)
	if !ok {
		return
	}
	download, err := handler.service.Open(request.Context(), principal, content.ID(request.PathValue("id")))
	if err != nil {
		writeError(response, request, err)
		return
	}
	defer download.Body.Close()
	response.Header().Set("Content-Type", download.Asset.MIMEType)
	response.Header().Set("Content-Length", strconv.FormatInt(download.Asset.Size, 10))
	response.Header().Set("ETag", `"sha256-`+download.Asset.Checksum+`"`)
	response.Header().Set("X-Content-Type-Options", "nosniff")
	if disposition := mime.FormatMediaType("inline", map[string]string{"filename": download.Asset.Filename}); disposition != "" {
		response.Header().Set("Content-Disposition", disposition)
	}
	response.WriteHeader(http.StatusOK)
	_, _ = io.Copy(response, download.Body)
}

func resolvePrincipal(
	resolver PrincipalResolver,
	response http.ResponseWriter,
	request *http.Request,
) (authz.Principal, bool) {
	if resolver == nil {
		return authz.Anonymous(), true
	}
	principal, err := resolver.Resolve(request)
	if err != nil {
		writeError(response, request, core.WrapDomainError(core.ErrorCodeUnauthorized, "authentication failed", err))
		return authz.Principal{}, false
	}
	return principal, true
}

func publicMedia(asset domainmedia.Asset) map[string]any {
	return map[string]any{
		"id": asset.ID, "status": asset.Status, "visibility": asset.Visibility,
		"filename": asset.Filename, "mime_type": asset.MIMEType, "size": asset.Size,
		"checksum_sha256": asset.Checksum, "alt": asset.Alt, "version": asset.Version,
	}
}
