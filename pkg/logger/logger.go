package logger

import (
	"context"
	"os"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type ctxKey string

const requestIDKey ctxKey = "request_id"

// Config mirrors logger.LoggerConfig but avoids importing the config package here.
type Config struct {
	Level    string
	Encoding string
}

// New builds a zap.Logger using the provided configuration.
func New(cfg Config) (*zap.Logger, error) {
	encoderCfg := zap.NewProductionEncoderConfig()
	encoderCfg.TimeKey = "timestamp"
	encoderCfg.EncodeTime = zapcore.ISO8601TimeEncoder

	level := zapcore.InfoLevel
	if err := level.Set(cfg.Level); err != nil {
		// fall back to info level if parsing fails
		level = zapcore.InfoLevel
	}

	var encoder zapcore.Encoder
	switch cfg.Encoding {
	case "console":
		encoder = zapcore.NewConsoleEncoder(encoderCfg)
	default:
		encoder = zapcore.NewJSONEncoder(encoderCfg)
	}

	core := zapcore.NewCore(
		encoder,
		zapcore.AddSync(zapcore.Lock(os.Stdout)),
		level,
	)

	return zap.New(core, zap.AddCaller()), nil
}

// ContextWithRequestID attaches a request ID to the provided context.
func ContextWithRequestID(ctx context.Context, requestID string) context.Context {
	return context.WithValue(ctx, requestIDKey, requestID)
}

// WithRequestID enriches the logger with the request ID stored in the context.
func WithRequestID(ctx context.Context, base *zap.Logger) *zap.Logger {
	if ctx == nil || base == nil {
		return base
	}
	if reqID, ok := ctx.Value(requestIDKey).(string); ok && reqID != "" {
		return base.With(zap.String("request_id", reqID))
	}
	return base
}
