package rest

import (
	"net/http"

	"github.com/fastygo/backend/internal/persist"
	"github.com/fastygo/framework/pkg/core"
)

func (handler *ContentHandler) schemaIdentity(response http.ResponseWriter, request *http.Request) {
	digest, err := handler.manifest.Digest()
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
