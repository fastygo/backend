package handler

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/valyala/fasthttp"
	"go.uber.org/zap"

	"github.com/fastygo/backend/api/transport"
	"github.com/fastygo/backend/domain"
	"github.com/fastygo/backend/pkg/httpcontext"
)

type baseHandler struct {
	adapter *httpcontext.Adapter
	logger  *zap.Logger
}

func newBaseHandler(adapter *httpcontext.Adapter, logger *zap.Logger) baseHandler {
	if logger == nil {
		logger = zap.NewNop()
	}
	return baseHandler{adapter: adapter, logger: logger}
}

func (h baseHandler) requestContext(ctx *fasthttp.RequestCtx) (context.Context, context.CancelFunc) {
	if h.adapter != nil {
		return h.adapter.Attach(ctx)
	}
	return context.WithCancel(context.Background())
}

func (h baseHandler) respondJSON(ctx *fasthttp.RequestCtx, status int, payload transport.Envelope) {
	ctx.Response.Header.SetContentType("application/json")
	ctx.SetStatusCode(status)
	body, _ := json.Marshal(payload)
	ctx.SetBody(body)
}

func (h baseHandler) respondSuccess(ctx *fasthttp.RequestCtx, status int, data interface{}) {
	h.respondJSON(ctx, status, transport.NewSuccess(data, nil))
}

func (h baseHandler) respondError(ctx *fasthttp.RequestCtx, err error) {
	status, code := mapError(err)
	h.respondJSON(ctx, status, transport.NewError(code, err.Error(), nil))
}

func mapError(err error) (int, string) {
	switch {
	case domain.IsDomainError(err, domain.ErrCodeUnauthorized):
		return http.StatusUnauthorized, string(domain.ErrCodeUnauthorized)
	case domain.IsDomainError(err, domain.ErrCodeForbidden):
		return http.StatusForbidden, string(domain.ErrCodeForbidden)
	case domain.IsDomainError(err, domain.ErrCodeInvalid):
		return http.StatusBadRequest, string(domain.ErrCodeInvalid)
	case domain.IsDomainError(err, domain.ErrCodeNotFound):
		return http.StatusNotFound, string(domain.ErrCodeNotFound)
	default:
		return http.StatusInternalServerError, string(domain.ErrCodeInternal)
	}
}

