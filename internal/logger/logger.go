package logger

import (
	"context"
	"log/slog"
	"os"
	"strings"
)

type contextKey string

const RequestIDKey contextKey = "request_id"

var DefaultLogger *slog.Logger

func init() {
	DefaultLogger = InitLogger()
	slog.SetDefault(DefaultLogger)
}

func InitLogger() *slog.Logger {
	level := slog.LevelInfo
	if strings.ToLower(os.Getenv("LOG_LEVEL")) == "debug" {
		level = slog.LevelDebug
	} else if strings.ToLower(os.Getenv("LOG_LEVEL")) == "warn" {
		level = slog.LevelWarn
	} else if strings.ToLower(os.Getenv("LOG_LEVEL")) == "error" {
		level = slog.LevelError
	}

	opts := &slog.HandlerOptions{
		Level: level,
	}

	var handler slog.Handler
	format := strings.ToLower(os.Getenv("LOG_FORMAT"))
	if format == "text" {
		handler = slog.NewTextHandler(os.Stdout, opts)
	} else {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	}

	return slog.New(handler)
}

func WithRequestID(ctx context.Context, requestID string) context.Context {
	return context.WithValue(ctx, RequestIDKey, requestID)
}

func GetRequestID(ctx context.Context) string {
	if val, ok := ctx.Value(RequestIDKey).(string); ok {
		return val
	}
	return ""
}

func FromContext(ctx context.Context) *slog.Logger {
	reqID := GetRequestID(ctx)
	if reqID != "" {
		return DefaultLogger.With("request_id", reqID)
	}
	return DefaultLogger
}
