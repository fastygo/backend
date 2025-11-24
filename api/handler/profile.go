package handler

import (
	"encoding/json"
	"net/http"

	"github.com/valyala/fasthttp"
	"go.uber.org/zap"

	"github.com/e-go/backend/api/transport"
	"github.com/e-go/backend/domain"
	"github.com/e-go/backend/pkg/httpcontext"
	profileUC "github.com/e-go/backend/usecase/profile"
)

type ProfileHandler struct {
	baseHandler
	uc *profileUC.UseCase
}

func NewProfileHandler(uc *profileUC.UseCase, adapter *httpcontext.Adapter, logger *zap.Logger) *ProfileHandler {
	return &ProfileHandler{
		baseHandler: newBaseHandler(adapter, logger),
		uc:          uc,
	}
}

// @Summary Get profile
// @Tags profile
// @Success 200 {object} transport.Envelope
// @Router /api/v1/profile [get]
func (h *ProfileHandler) GetProfile(ctx *fasthttp.RequestCtx) {
	userID := string(ctx.Request.Header.Peek("X-User-ID"))
	if userID == "" {
		h.respondJSON(ctx, http.StatusUnauthorized, transport.NewError(string(domain.ErrCodeUnauthorized), "missing user id", nil))
		return
	}

	stdCtx, cancel := h.requestContext(ctx)
	defer cancel()

	user, err := h.uc.GetProfile(stdCtx, userID)
	if err != nil {
		h.respondError(ctx, err)
		return
	}
	h.respondSuccess(ctx, http.StatusOK, user)
}

// @Summary Update profile
// @Tags profile
// @Accept json
// @Produce json
// @Router /api/v1/profile [put]
func (h *ProfileHandler) UpdateProfile(ctx *fasthttp.RequestCtx) {
	userID := string(ctx.Request.Header.Peek("X-User-ID"))
	if userID == "" {
		h.respondJSON(ctx, http.StatusUnauthorized, transport.NewError(string(domain.ErrCodeUnauthorized), "missing user id", nil))
		return
	}

	var req transport.ProfileUpdateRequest
	if err := json.Unmarshal(ctx.PostBody(), &req); err != nil {
		h.respondJSON(ctx, http.StatusBadRequest, transport.NewError(string(domain.ErrCodeInvalid), "invalid payload", nil))
		return
	}

	user := &domain.User{
		ID:       userID,
		Email:    req.Email,
		Role:     req.Role,
		Status:   req.Status,
		Metadata: req.Meta,
	}

	stdCtx, cancel := h.requestContext(ctx)
	defer cancel()

	updated, err := h.uc.UpdateProfile(stdCtx, user)
	if err != nil {
		h.respondError(ctx, err)
		return
	}
	h.respondSuccess(ctx, http.StatusOK, updated)
}

