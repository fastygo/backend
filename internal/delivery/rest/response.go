package rest

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/fastygo/framework/pkg/core"
	"github.com/google/uuid"
)

type errorDocument struct {
	Error errorBody `json:"error"`
}

type errorBody struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	RequestID string `json:"request_id"`
}

func writeError(response http.ResponseWriter, request *http.Request, err error) {
	domainError := core.NewDomainError(core.ErrorCodeInternal, "internal server error")
	var resolved core.DomainError
	if errors.As(err, &resolved) {
		domainError = resolved
		if domainError.Code == core.ErrorCodeInternal {
			domainError.Message = "internal server error"
		}
	}
	writeJSON(response, domainError.StatusCode(), errorDocument{Error: errorBody{
		Code: string(domainError.Code), Message: domainError.Message, RequestID: requestID(request),
	}})
}

func writeJSON(response http.ResponseWriter, status int, value any) {
	response.Header().Set("Content-Type", "application/json; charset=utf-8")
	response.Header().Set("X-Content-Type-Options", "nosniff")
	response.WriteHeader(status)
	_ = json.NewEncoder(response).Encode(value)
}

func requestID(request *http.Request) string {
	if requestID := request.Header.Get("X-Request-ID"); requestID != "" {
		return requestID
	}
	return uuid.NewString()
}
