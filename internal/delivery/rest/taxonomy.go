package rest

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	applicationtaxonomy "github.com/fastygo/backend/internal/application/taxonomy"
	"github.com/fastygo/backend/internal/domain/authz"
	domaintaxonomy "github.com/fastygo/backend/internal/domain/taxonomy"
	"github.com/fastygo/backend/internal/persist"
	"github.com/fastygo/framework/pkg/core"
)

type TaxonomyHandler struct {
	service   *applicationtaxonomy.Service
	principal PrincipalResolver
}

func NewTaxonomyHandler(
	service *applicationtaxonomy.Service,
	principal PrincipalResolver,
) (*TaxonomyHandler, error) {
	if service == nil {
		return nil, errors.New("taxonomy service is required")
	}
	return &TaxonomyHandler{service: service, principal: principal}, nil
}

func (handler *TaxonomyHandler) Routes(mux *http.ServeMux) {
	mux.HandleFunc("POST /go-json/go/v2/taxonomies", handler.createDefinition)
	mux.HandleFunc("PUT /go-json/go/v2/taxonomies/{taxonomy}", handler.updateDefinition)
	mux.HandleFunc("DELETE /go-json/go/v2/taxonomies/{taxonomy}", handler.deleteDefinition)
	mux.HandleFunc("POST /go-json/go/v2/taxonomies/{taxonomy}/terms", handler.createTerm)
	mux.HandleFunc("PUT /go-json/go/v2/taxonomies/{taxonomy}/terms/{term}", handler.updateTerm)
	mux.HandleFunc("DELETE /go-json/go/v2/taxonomies/{taxonomy}/terms/{term}", handler.deleteTerm)
}

func (handler *TaxonomyHandler) createDefinition(response http.ResponseWriter, request *http.Request) {
	principal, ok := handler.resolvePrincipal(response, request)
	if !ok {
		return
	}
	var document persist.Definition
	if err := decodeTaxonomyJSON(response, request, &document); err != nil {
		writeError(response, request, err)
		return
	}
	saved, err := handler.service.SaveDefinition(request.Context(), principal, document.Domain(), 0)
	if err != nil {
		writeError(response, request, err)
		return
	}
	response.Header().Set("ETag", versionETag(saved.Version))
	response.Header().Set("Location", request.URL.Path+"/"+saved.ID)
	writeJSON(response, http.StatusCreated, map[string]any{"data": persist.DefinitionFromDomain(saved)})
}

func (handler *TaxonomyHandler) updateDefinition(response http.ResponseWriter, request *http.Request) {
	principal, ok := handler.resolvePrincipal(response, request)
	if !ok {
		return
	}
	var document persist.Definition
	if err := decodeTaxonomyJSON(response, request, &document); err != nil {
		writeError(response, request, err)
		return
	}
	item := document.Domain()
	item.ID = request.PathValue("taxonomy")
	version, err := expectedVersion(request, item.Version)
	if err != nil {
		writeError(response, request, core.WrapDomainError(core.ErrorCodeValidation, "version is required", err))
		return
	}
	saved, err := handler.service.SaveDefinition(request.Context(), principal, item, version)
	if err != nil {
		writeError(response, request, err)
		return
	}
	response.Header().Set("ETag", versionETag(saved.Version))
	writeJSON(response, http.StatusOK, map[string]any{"data": persist.DefinitionFromDomain(saved)})
}

func (handler *TaxonomyHandler) deleteDefinition(response http.ResponseWriter, request *http.Request) {
	principal, ok := handler.resolvePrincipal(response, request)
	if !ok {
		return
	}
	version, err := expectedVersion(request, 0)
	if err != nil {
		writeError(response, request, core.WrapDomainError(core.ErrorCodeValidation, "version is required", err))
		return
	}
	if err := handler.service.DeleteDefinition(request.Context(), principal, request.PathValue("taxonomy"), version); err != nil {
		writeError(response, request, err)
		return
	}
	response.WriteHeader(http.StatusNoContent)
}

func (handler *TaxonomyHandler) createTerm(response http.ResponseWriter, request *http.Request) {
	principal, ok := handler.resolvePrincipal(response, request)
	if !ok {
		return
	}
	var document persist.Term
	if err := decodeTaxonomyJSON(response, request, &document); err != nil {
		writeError(response, request, err)
		return
	}
	item := document.Domain()
	item.TaxonomyID = request.PathValue("taxonomy")
	saved, err := handler.service.SaveTerm(request.Context(), principal, item, 0)
	if err != nil {
		writeError(response, request, err)
		return
	}
	response.Header().Set("ETag", versionETag(saved.Version))
	response.Header().Set("Location", request.URL.Path+"/"+string(saved.ID))
	writeJSON(response, http.StatusCreated, map[string]any{"data": persist.TermFromDomain(saved)})
}

func (handler *TaxonomyHandler) updateTerm(response http.ResponseWriter, request *http.Request) {
	principal, ok := handler.resolvePrincipal(response, request)
	if !ok {
		return
	}
	var document persist.Term
	if err := decodeTaxonomyJSON(response, request, &document); err != nil {
		writeError(response, request, err)
		return
	}
	item := document.Domain()
	item.ID = domaintaxonomy.ID(request.PathValue("term"))
	item.TaxonomyID = request.PathValue("taxonomy")
	version, err := expectedVersion(request, item.Version)
	if err != nil {
		writeError(response, request, core.WrapDomainError(core.ErrorCodeValidation, "version is required", err))
		return
	}
	saved, err := handler.service.SaveTerm(request.Context(), principal, item, version)
	if err != nil {
		writeError(response, request, err)
		return
	}
	response.Header().Set("ETag", versionETag(saved.Version))
	writeJSON(response, http.StatusOK, map[string]any{"data": persist.TermFromDomain(saved)})
}

func (handler *TaxonomyHandler) deleteTerm(response http.ResponseWriter, request *http.Request) {
	principal, ok := handler.resolvePrincipal(response, request)
	if !ok {
		return
	}
	version, err := expectedVersion(request, 0)
	if err != nil {
		writeError(response, request, core.WrapDomainError(core.ErrorCodeValidation, "version is required", err))
		return
	}
	if err := handler.service.DeleteTerm(
		request.Context(),
		principal,
		domaintaxonomy.ID(request.PathValue("term")),
		version,
	); err != nil {
		writeError(response, request, err)
		return
	}
	response.WriteHeader(http.StatusNoContent)
}

func (handler *TaxonomyHandler) resolvePrincipal(
	response http.ResponseWriter,
	request *http.Request,
) (authz.Principal, bool) {
	if handler.principal == nil {
		return authz.Anonymous(), true
	}
	principal, err := handler.principal.Resolve(request)
	if err != nil {
		writeError(response, request, err)
		return authz.Principal{}, false
	}
	return principal, true
}

func decodeTaxonomyJSON(response http.ResponseWriter, request *http.Request, target any) error {
	request.Body = http.MaxBytesReader(response, request.Body, maxRequestBody)
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return core.WrapDomainError(core.ErrorCodeValidation, "invalid JSON body", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return core.NewDomainError(core.ErrorCodeValidation, "request body must contain one JSON value")
	}
	return nil
}
