package rest

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	application "github.com/fastygo/backend/internal/application/content"
	domaincontent "github.com/fastygo/backend/internal/domain/content"
	"github.com/fastygo/backend/internal/domain/revision"
	"github.com/fastygo/backend/internal/persist"
	"github.com/fastygo/framework/pkg/core"
)

type transitionDocument struct {
	Status          domaincontent.Status `json:"status"`
	PublishAt       *time.Time           `json:"publish_at,omitempty"`
	ExpectedVersion uint64               `json:"expected_version"`
	Reason          string               `json:"reason,omitempty"`
}

type restoreDocument struct {
	ExpectedVersion uint64 `json:"expected_version"`
}

func (handler *ContentHandler) transition(response http.ResponseWriter, request *http.Request) {
	principal, ok := handler.resolvePrincipal(response, request)
	if !ok {
		return
	}
	var document transitionDocument
	if err := decodeDocument(response, request, &document); err != nil {
		writeError(response, request, err)
		return
	}
	kind, err := handler.kindFromCollection(request.PathValue("collection"))
	if err != nil {
		writeError(response, request, err)
		return
	}
	entry, err := handler.service.Transition(request.Context(), principal, domaincontent.ID(request.PathValue("id")), application.Transition{
		Kind: kind, Status: document.Status, PublishAt: document.PublishAt,
		ExpectedVersion: document.ExpectedVersion, Reason: document.Reason,
	})
	if err != nil {
		writeError(response, request, err)
		return
	}
	response.Header().Set("ETag", versionETag(entry.Version))
	writeJSON(response, http.StatusOK, map[string]any{"data": persist.EntryFromDomain(entry)})
}

func (handler *ContentHandler) trash(response http.ResponseWriter, request *http.Request) {
	principal, ok := handler.resolvePrincipal(response, request)
	if !ok {
		return
	}
	version, err := expectedVersion(request, 0)
	if err != nil {
		writeError(response, request, core.WrapDomainError(core.ErrorCodeValidation, "invalid expected version", err))
		return
	}
	kind, err := handler.kindFromCollection(request.PathValue("collection"))
	if err != nil {
		writeError(response, request, err)
		return
	}
	_, err = handler.service.Transition(request.Context(), principal, domaincontent.ID(request.PathValue("id")), application.Transition{
		Kind: kind, Status: domaincontent.StatusTrashed, ExpectedVersion: version, Reason: "REST delete",
	})
	if err != nil {
		writeError(response, request, err)
		return
	}
	response.WriteHeader(http.StatusNoContent)
}

func (handler *ContentHandler) revisions(response http.ResponseWriter, request *http.Request) {
	principal, ok := handler.resolvePrincipal(response, request)
	if !ok {
		return
	}
	page, err := positiveInteger(request.URL.Query().Get("page"), 1)
	if err != nil {
		writeError(response, request, core.WrapDomainError(core.ErrorCodeValidation, "invalid revision page", err))
		return
	}
	perPage, err := positiveInteger(request.URL.Query().Get("per_page"), 20)
	if err != nil || perPage > 100 {
		writeError(response, request, core.NewDomainError(core.ErrorCodeValidation, "invalid revision per_page"))
		return
	}
	kind, err := handler.kindFromCollection(request.PathValue("collection"))
	if err != nil {
		writeError(response, request, err)
		return
	}
	items, pagination, err := handler.service.Revisions(
		request.Context(), principal, domaincontent.ID(request.PathValue("id")),
		kind, page, perPage,
	)
	if err != nil {
		writeError(response, request, err)
		return
	}
	writeJSON(response, http.StatusOK, map[string]any{"data": projectRevisions(items), "pagination": projectPage(pagination)})
}

func (handler *ContentHandler) restoreRevision(response http.ResponseWriter, request *http.Request) {
	principal, ok := handler.resolvePrincipal(response, request)
	if !ok {
		return
	}
	var document restoreDocument
	if err := decodeDocument(response, request, &document); err != nil {
		writeError(response, request, err)
		return
	}
	kind, err := handler.kindFromCollection(request.PathValue("collection"))
	if err != nil {
		writeError(response, request, err)
		return
	}
	entry, err := handler.service.RestoreRevision(
		request.Context(), principal, domaincontent.ID(request.PathValue("id")),
		kind, revision.ID(request.PathValue("revision")), document.ExpectedVersion,
	)
	if err != nil {
		writeError(response, request, err)
		return
	}
	response.Header().Set("ETag", versionETag(entry.Version))
	writeJSON(response, http.StatusOK, map[string]any{"data": persist.EntryFromDomain(entry)})
}

func decodeDocument(response http.ResponseWriter, request *http.Request, target any) error {
	request.Body = http.MaxBytesReader(response, request.Body, maxRequestBody)
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return core.WrapDomainError(core.ErrorCodeValidation, "invalid JSON body", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return core.NewDomainError(core.ErrorCodeValidation, "request body must contain one JSON object")
	}
	return nil
}
