package rest

import (
	"encoding/json"
	"net/http"

	"github.com/fastygo/backend/internal/application/forms"
	domaincontent "github.com/fastygo/backend/internal/domain/content"
	"github.com/fastygo/backend/internal/domain/schema"
	"github.com/fastygo/backend/internal/persist"
	"github.com/fastygo/formset"
	"github.com/fastygo/framework/pkg/core"
)

func (handler *ContentHandler) schemaIdentity(response http.ResponseWriter, request *http.Request) {
	digest, err := persist.ManifestDigest(handler.manifest)
	if err != nil {
		writeError(response, request, err)
		return
	}
	document := persist.ManifestFromDomain(handler.manifest)
	writeJSON(response, http.StatusOK, map[string]any{
		"name": document.Name, "version": document.Version, "digest": digest,
		"resources": document.Resources,
	})
}

func (handler *ContentHandler) resourceSchema(response http.ResponseWriter, request *http.Request) {
	document, err := handler.manifest.JSONSchema(request.PathValue("resource"))
	if err != nil {
		writeError(response, request, core.WrapDomainError(core.ErrorCodeNotFound, "resource schema was not found", err))
		return
	}
	writeJSON(response, http.StatusOK, document)
}

func (handler *ContentHandler) resourceForm(response http.ResponseWriter, request *http.Request) {
	resource, ok := handler.manifest.Resource(request.PathValue("resource"))
	if !ok {
		writeError(response, request, core.NewDomainError(core.ErrorCodeNotFound, "resource schema was not found"))
		return
	}
	document, err := formDocument(resource)
	if err != nil {
		writeError(response, request, err)
		return
	}
	writeJSON(response, http.StatusOK, document)
}

func (handler *ContentHandler) bindForm(response http.ResponseWriter, request *http.Request) {
	resource, ok := handler.manifest.Resource(request.PathValue("resource"))
	if !ok {
		writeError(response, request, core.NewDomainError(core.ErrorCodeNotFound, "resource schema was not found"))
		return
	}
	request.Body = http.MaxBytesReader(response, request.Body, maxRequestBody)
	var documents formset.Documents
	if err := json.NewDecoder(request.Body).Decode(&documents); err != nil {
		writeError(response, request, core.WrapDomainError(core.ErrorCodeValidation, "invalid JSON body", err))
		return
	}
	form, err := forms.Bind(resource, documents)
	if err != nil {
		writeError(response, request, core.WrapDomainError(core.ErrorCodeValidation, "form schema is invalid", err))
		return
	}
	document, err := formDocument(resource)
	if err != nil {
		writeError(response, request, err)
		return
	}
	document["form"] = form
	document["payloads"] = form.PayloadDocuments()
	writeJSON(response, http.StatusOK, document)
}

func (handler *ContentHandler) entryForm(response http.ResponseWriter, request *http.Request) {
	principal, ok := handler.resolvePrincipal(response, request)
	if !ok {
		return
	}
	resource, exists := handler.manifest.Resource(request.PathValue("kind"))
	if !exists {
		writeError(response, request, core.NewDomainError(core.ErrorCodeNotFound, "resource schema was not found"))
		return
	}
	entry, err := handler.service.Get(request.Context(), principal, domaincontent.ID(request.PathValue("id")))
	if err != nil {
		writeError(response, request, err)
		return
	}
	if entry.Kind != domaincontent.Kind(resource.ID) {
		writeError(response, request, core.NewDomainError(core.ErrorCodeNotFound, "content was not found"))
		return
	}
	form, err := forms.BindEntry(resource, entry)
	if err != nil {
		writeError(response, request, core.WrapDomainError(core.ErrorCodeValidation, "form schema is invalid", err))
		return
	}
	document, err := formDocument(resource)
	if err != nil {
		writeError(response, request, err)
		return
	}
	document["form"] = form
	writeJSON(response, http.StatusOK, document)
}

func formDocument(resource schema.Resource) (map[string]any, error) {
	jsonSchema, err := forms.Schema(resource)
	if err != nil {
		return nil, err
	}
	record := forms.Record(resource)
	return map[string]any{
		"record": record,
		"schema": jsonSchema,
		"fields": record.Fields,
	}, nil
}

func (handler *ContentHandler) openAPI(response http.ResponseWriter, request *http.Request) {
	document, err := handler.manifest.OpenAPI()
	if err != nil {
		writeError(response, request, err)
		return
	}
	writeJSON(response, http.StatusOK, document)
}

func (handler *ContentHandler) graphQLSchema(response http.ResponseWriter, request *http.Request) {
	document, err := handler.manifest.GraphQLSDL()
	if err != nil {
		writeError(response, request, err)
		return
	}
	response.Header().Set("Content-Type", "text/plain; charset=utf-8")
	response.Header().Set("X-Content-Type-Options", "nosniff")
	response.WriteHeader(http.StatusOK)
	_, _ = response.Write([]byte(document))
}
