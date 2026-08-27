package rest

import (
	"encoding/json"
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
	"github.com/fastygo/backend/internal/persist"
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
	mux.HandleFunc("GET /go-json/go/v2/audit", handler.listAudit)
	if handler.manifest.Name != "" {
		mux.HandleFunc("GET /go-json/go/v2/schema", handler.schemaIdentity)
		mux.HandleFunc("GET /go-json/go/v2/types/{resource}/json-schema", handler.resourceSchema)
		mux.HandleFunc("GET /go-json/go/v2/types/{resource}/form", handler.resourceForm)
		mux.HandleFunc("POST /go-json/go/v2/types/{resource}/form/bind", handler.bindForm)
		mux.HandleFunc("GET /go-json/go/v2/openapi.json", handler.openAPI)
		mux.HandleFunc("GET /go-json/go/v2/graphql.sdl", handler.graphQLSchema)
	}
}

func (handler *ContentHandler) kindFromCollection(collection string) (domaincontent.Kind, error) {
	for _, resource := range handler.manifest.Resources {
		if resource.Collection == collection {
			return domaincontent.Kind(resource.ID), nil
		}
	}
	return "", core.NewDomainError(core.ErrorCodeNotFound, "resource collection was not found")
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
	var document persist.Entry
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&document); err != nil {
		return core.WrapDomainError(core.ErrorCodeValidation, "invalid JSON body", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return core.NewDomainError(core.ErrorCodeValidation, "request body must contain one JSON object")
	}
	*target = document.Domain()
	return nil
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
