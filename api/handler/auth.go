package handler

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/valyala/fasthttp"
	"go.uber.org/zap"

	"github.com/e-go/backend/api/transport"
	"github.com/e-go/backend/domain"
	"github.com/e-go/backend/pkg/httpcontext"
	authUC "github.com/e-go/backend/usecase/auth"
)

type AuthHandler struct {
	baseHandler
	uc        *authUC.UseCase
	defaultTTL time.Duration
}

func NewAuthHandler(uc *authUC.UseCase, adapter *httpcontext.Adapter, logger *zap.Logger, ttl time.Duration) *AuthHandler {
	if ttl <= 0 {
		ttl = time.Hour
	}
	return &AuthHandler{
		baseHandler: newBaseHandler(adapter, logger),
		uc:          uc,
		defaultTTL:  ttl,
	}
}

// @Summary Issue a new session
// @Tags auth
// @Router /api/v1/auth/login [post]
func (h *AuthHandler) Login(ctx *fasthttp.RequestCtx) {
	var req transport.AuthLoginRequest
	if err := json.Unmarshal(ctx.PostBody(), &req); err != nil || req.UserID == "" {
		h.respondJSON(ctx, http.StatusBadRequest, transport.NewError(string(domain.ErrCodeInvalid), "invalid payload", nil))
		return
	}

	ttl := h.ttlFromRequest(req.TTL)

	stdCtx, cancel := h.requestContext(ctx)
	defer cancel()

	session, err := h.uc.CreateSession(stdCtx, req.UserID, ttl)
	if err != nil {
		h.respondError(ctx, err)
		return
	}
	h.respondSuccess(ctx, http.StatusCreated, session)
}

// @Summary Refresh an existing session
// @Tags auth
// @Router /api/v1/auth/refresh [post]
func (h *AuthHandler) Refresh(ctx *fasthttp.RequestCtx) {
	var req transport.RefreshRequest
	if err := json.Unmarshal(ctx.PostBody(), &req); err != nil || req.SessionID == "" {
		h.respondJSON(ctx, http.StatusBadRequest, transport.NewError(string(domain.ErrCodeInvalid), "invalid payload", nil))
		return
	}

	ttl := h.ttlFromRequest(req.TTL)

	stdCtx, cancel := h.requestContext(ctx)
	defer cancel()

	session, err := h.uc.RefreshSession(stdCtx, req.SessionID, ttl)
	if err != nil {
		h.respondError(ctx, err)
		return
	}
	h.respondSuccess(ctx, http.StatusOK, session)
}

func (h *AuthHandler) ttlFromRequest(ttlSeconds int) time.Duration {
	if ttlSeconds <= 0 {
		return h.defaultTTL
	}
	return time.Duration(ttlSeconds) * time.Second
}

