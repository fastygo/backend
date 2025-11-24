package handler

import (
	"net/http"
	"time"

	"github.com/valyala/fasthttp"
	"go.uber.org/zap"

	"github.com/e-go/backend/api/transport"
	"github.com/e-go/backend/internal/infrastructure/monitor"
	"github.com/e-go/backend/pkg/httpcontext"
)

type HealthHandler struct {
	baseHandler
	monitor *monitor.Monitor
}

func NewHealthHandler(mon *monitor.Monitor, adapter *httpcontext.Adapter, logger *zap.Logger) *HealthHandler {
	return &HealthHandler{
		baseHandler: newBaseHandler(adapter, logger),
		monitor:     mon,
	}
}

// @Summary Health check
// @Tags health
// @Router /health [get]
func (h *HealthHandler) Check(ctx *fasthttp.RequestCtx) {
	status := h.monitor.GetStatus()
	payload := map[string]interface{}{
		"timestamp": time.Now().UTC(),
		"services": map[string]interface{}{
			"postgresql": status.PostgreSQL,
			"redis":      status.Redis,
			"buffer": map[string]interface{}{
				"online": status.Buffer,
				"size":   status.BufferSize,
			},
		},
	}

	if status.PostgreSQL && status.Redis {
		h.respondSuccess(ctx, http.StatusOK, payload)
		return
	}
	h.respondJSON(ctx, http.StatusServiceUnavailable, transport.NewError("DEGRADED", "dependencies unhealthy", payload))
}

