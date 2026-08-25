package rest

import (
	"net/http"
	"time"

	application "github.com/fastygo/backend/internal/application/content"
	"github.com/fastygo/framework/pkg/core"
)

func (handler *ContentHandler) listAudit(response http.ResponseWriter, request *http.Request) {
	principal, ok := handler.resolvePrincipal(response, request)
	if !ok {
		return
	}
	values := request.URL.Query()
	page, err := positiveInteger(values.Get("page"), 1)
	if err != nil {
		writeError(response, request, core.WrapDomainError(core.ErrorCodeValidation, "invalid audit page", err))
		return
	}
	perPage, err := positiveInteger(values.Get("per_page"), 20)
	if err != nil || perPage > 100 {
		writeError(response, request, core.NewDomainError(core.ErrorCodeValidation, "invalid audit per_page"))
		return
	}
	query := application.AuditQuery{
		ActorID: values.Get("actor_id"), Action: values.Get("action"),
		Resource: values.Get("resource"), ResourceID: values.Get("resource_id"),
		Page: page, PerPage: perPage,
	}
	if raw := values.Get("after"); raw != "" {
		parsed, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			writeError(response, request, core.WrapDomainError(core.ErrorCodeValidation, "invalid audit after", err))
			return
		}
		query.After = &parsed
	}
	if raw := values.Get("before"); raw != "" {
		parsed, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			writeError(response, request, core.WrapDomainError(core.ErrorCodeValidation, "invalid audit before", err))
			return
		}
		query.Before = &parsed
	}
	events, pagination, err := handler.service.ListAudit(request.Context(), principal, query)
	if err != nil {
		writeError(response, request, err)
		return
	}
	writeJSON(response, http.StatusOK, map[string]any{"data": events, "pagination": pagination})
}
