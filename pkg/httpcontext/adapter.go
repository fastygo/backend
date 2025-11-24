package httpcontext

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/valyala/fasthttp"

	appLogger "github.com/e-go/backend/pkg/logger"
)

// Key represents a context value key exported for reuse.
type Key string

const (
	KeyRemoteAddr Key = "remote_addr"
	KeyUserAgent  Key = "user_agent"
)

// Adapter converts fasthttp.RequestCtx into a stdlib context with deadlines and metadata.
type Adapter struct {
	timeout time.Duration
}

// NewAdapter constructs a new Adapter using the provided timeout.
func NewAdapter(timeout time.Duration) *Adapter {
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	return &Adapter{
		timeout: timeout,
	}
}

// Attach creates a context with timeout derived from the adapter and enriches it with request metadata.
func (a *Adapter) Attach(ctx *fasthttp.RequestCtx) (context.Context, context.CancelFunc) {
	base := context.Background()

	stdCtx, cancel := context.WithTimeout(base, a.timeout)

	reqID := getRequestID(ctx)
	stdCtx = appLogger.ContextWithRequestID(stdCtx, reqID)
	ctx.Response.Header.Set("X-Request-ID", reqID)

	if remoteAddr := ctx.RemoteAddr(); remoteAddr != nil {
		stdCtx = context.WithValue(stdCtx, KeyRemoteAddr, remoteAddr.String())
	}
	if ua := string(ctx.Request.Header.UserAgent()); ua != "" {
		stdCtx = context.WithValue(stdCtx, KeyUserAgent, ua)
	}

	return stdCtx, cancel
}

func getRequestID(ctx *fasthttp.RequestCtx) string {
	if ctx == nil {
		return uuid.NewString()
	}
	if header := string(ctx.Request.Header.Peek("X-Request-ID")); strings.TrimSpace(header) != "" {
		return header
	}
	return uuid.NewString()
}
