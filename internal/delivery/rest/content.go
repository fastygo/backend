package rest

import (
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	application "github.com/fastygo/backend/internal/application/content"
	"github.com/fastygo/backend/internal/domain/authz"
	domaincontent "github.com/fastygo/backend/internal/domain/content"
	"github.com/fastygo/backend/internal/domain/schema"
	"github.com/fastygo/framework/pkg/core"
)

const maxRequestBody = 4 << 20

type PrincipalResolver interface {
	Resolve(*http.Request) (authz.Principal, error)
}

type ContentHandler struct {
	service   *application.Service
	principal PrincipalResolver
	manifest  schema.Manifest
}

func NewContentHandler(service *application.Service, principal PrincipalResolver, manifests ...schema.Manifest) (*ContentHandler, error) {
	if service == nil {
		return nil, errors.New("content service is required")
	}
	handler := &ContentHandler{service: service, principal: principal}
	if len(manifests) > 0 {
		if err := manifests[0].Validate(); err != nil {
			return nil, err
		}
		handler.manifest = manifests[0]
	}
	return handler, nil
}

func (handler *ContentHandler) Routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /go-json/data/v1/resources/{kind}", handler.list)
	mux.HandleFunc("POST /go-json/data/v1/resources/{kind}", handler.create)
	mux.HandleFunc("GET /go-json/data/v1/resources/{kind}/{id}", handler.get)
	mux.HandleFunc("PUT /go-json/data/v1/resources/{kind}/{id}", handler.update)
	mux.HandleFunc("PATCH /go-json/data/v1/resources/{kind}/{id}", handler.update)
	mux.HandleFunc("DELETE /go-json/data/v1/resources/{kind}/{id}", handler.trash)
	mux.HandleFunc("POST /go-json/data/v1/resources/{kind}/{id}/transitions", handler.transition)
	mux.HandleFunc("GET /go-json/data/v1/resources/{kind}/{id}/revisions", handler.revisions)
	mux.HandleFunc("POST /go-json/data/v1/resources/{kind}/{id}/revisions/{revision}/restore", handler.restoreRevision)
	mux.HandleFunc("GET /go-json/data/v1/audit", handler.listAudit)
	if handler.manifest.Name != "" {
		mux.HandleFunc("GET /go-json/data/v1/schema", handler.schemaIdentity)
		mux.HandleFunc("GET /go-json/data/v1/schema/{resource}", handler.resourceSchema)
		mux.HandleFunc("GET /go-json/data/v1/openapi.json", handler.openAPI)
		mux.HandleFunc("GET /go-json/data/v1/graphql/schema", handler.graphQLSchema)
	}
}

func (handler *ContentHandler) list(response http.ResponseWriter, request *http.Request) {
	principal, ok := handler.resolvePrincipal(response, request)
	if !ok {
		return
	}
	query, err := parseQuery(request)
	if err != nil {
		writeError(response, request, core.WrapDomainError(core.ErrorCodeValidation, "invalid query", err))
		return
	}
	query.Kinds = []domaincontent.Kind{domaincontent.Kind(request.PathValue("kind"))}
	result, err := handler.service.List(request.Context(), principal, query)
	if err != nil {
		writeError(response, request, err)
		return
	}
	writeJSON(response, http.StatusOK, map[string]any{
		"data":       projectRecords(result.Entries),
		"pagination": projectPage(result.Page),
	})
}

func (handler *ContentHandler) get(response http.ResponseWriter, request *http.Request) {
	principal, ok := handler.resolvePrincipal(response, request)
	if !ok {
		return
	}
	entry, err := handler.service.Get(request.Context(), principal, domaincontent.ID(request.PathValue("id")))
	if err != nil {
		writeError(response, request, err)
		return
	}
	if entry.Kind != domaincontent.Kind(request.PathValue("kind")) {
		writeError(response, request, core.NewDomainError(core.ErrorCodeNotFound, "content was not found"))
		return
	}
	writeJSON(response, http.StatusOK, map[string]any{"data": projectRecord(entry)})
}

func (handler *ContentHandler) create(response http.ResponseWriter, request *http.Request) {
	principal, ok := handler.resolvePrincipal(response, request)
	if !ok {
		return
	}
	var entry domaincontent.Entry
	if err := decodeEntryJSON(response, request, &entry); err != nil {
		writeError(response, request, err)
		return
	}
	entry.Kind = domaincontent.Kind(request.PathValue("kind"))
	created, err := handler.service.Create(request.Context(), principal, entry)
	if err != nil {
		writeError(response, request, err)
		return
	}
	response.Header().Set("Location", request.URL.Path+"/"+string(created.ID))
	writeJSON(response, http.StatusCreated, map[string]any{"data": projectRecord(created)})
}

func (handler *ContentHandler) update(response http.ResponseWriter, request *http.Request) {
	principal, ok := handler.resolvePrincipal(response, request)
	if !ok {
		return
	}
	var entry domaincontent.Entry
	if err := decodeEntryJSON(response, request, &entry); err != nil {
		writeError(response, request, err)
		return
	}
	entry.ID = domaincontent.ID(request.PathValue("id"))
	entry.Kind = domaincontent.Kind(request.PathValue("kind"))
	expectedVersion, err := expectedVersion(request, entry.Version)
	if err != nil {
		writeError(response, request, core.WrapDomainError(core.ErrorCodeValidation, "invalid expected version", err))
		return
	}
	updated, err := handler.service.Update(
		request.Context(),
		principal,
		entry,
		expectedVersion,
		request.Header.Get("X-Revision-Reason"),
	)
	if err != nil {
		writeError(response, request, err)
		return
	}
	response.Header().Set("ETag", versionETag(updated.Version))
	writeJSON(response, http.StatusOK, map[string]any{"data": projectRecord(updated)})
}

func (handler *ContentHandler) resolvePrincipal(response http.ResponseWriter, request *http.Request) (authz.Principal, bool) {
	return resolvePrincipal(handler.principal, response, request)
}

func parseQuery(request *http.Request) (application.Query, error) {
	values := request.URL.Query()
	page, err := positiveInteger(values.Get("page"), 1)
	if err != nil {
		return application.Query{}, err
	}
	perPage, err := positiveInteger(values.Get("per_page"), 20)
	if err != nil || perPage > 100 {
		return application.Query{}, errors.New("per_page must be between 1 and 100")
	}
	query := application.Query{
		Page: page, PerPage: perPage, Search: values.Get("search"), Locale: values.Get("locale"),
		AuthorID: values.Get("author_id"), TaxonomyID: values.Get("taxonomy"), TermID: values.Get("term"),
		Sort: values.Get("sort"), Descending: values.Get("order") != "asc",
	}
	for _, status := range values["status"] {
		query.Statuses = append(query.Statuses, domaincontent.Status(status))
	}
	if raw := values.Get("after"); raw != "" {
		parsed, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			return application.Query{}, err
		}
		query.After = &parsed
	}
	if raw := values.Get("before"); raw != "" {
		parsed, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			return application.Query{}, err
		}
		query.Before = &parsed
	}
	return query, nil
}

func decodeEntryJSON(response http.ResponseWriter, request *http.Request, target *domaincontent.Entry) error {
	if mediaType := request.Header.Get("Content-Type"); !strings.HasPrefix(mediaType, "application/json") {
		return core.NewDomainError(core.ErrorCodeValidation, "Content-Type must be application/json")
	}
	request.Body = http.MaxBytesReader(response, request.Body, maxRequestBody)
	encoded, err := io.ReadAll(request.Body)
	if err != nil {
		return core.WrapDomainError(core.ErrorCodeValidation, "invalid JSON body", err)
	}
	entry, err := decodeEntryRequest(encoded)
	if err != nil {
		return err
	}
	*target = entry
	return nil
}

func projectRecords(entries []domaincontent.Entry) []resourceRecord {
	records := make([]resourceRecord, 0, len(entries))
	for _, entry := range entries {
		records = append(records, projectRecord(entry))
	}
	return records
}

func expectedVersion(request *http.Request, bodyVersion uint64) (uint64, error) {
	value := strings.TrimSpace(request.Header.Get("If-Match"))
	if value == "" {
		if bodyVersion == 0 {
			return 0, errors.New("If-Match or body version is required")
		}
		return bodyVersion, nil
	}
	value = strings.Trim(value, `"`)
	value = strings.TrimPrefix(value, "v")
	version, err := strconv.ParseUint(value, 10, 64)
	if err != nil || version == 0 {
		return 0, errors.New("If-Match must contain a positive version")
	}
	return version, nil
}

func positiveInteger(value string, fallback int) (int, error) {
	if value == "" {
		return fallback, nil
	}
	resolved, err := strconv.Atoi(value)
	if err != nil || resolved < 1 {
		return 0, errors.New("value must be a positive integer")
	}
	return resolved, nil
}

func versionETag(version uint64) string {
	return `"v` + strconv.FormatUint(version, 10) + `"`
}
