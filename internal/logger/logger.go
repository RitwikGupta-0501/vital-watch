package logger

import (
	"context"
	"io"
	"log/slog"
	"os"
	"strings"
)

type contextKey string

const RequestIDKey contextKey = "request_id"

var DefaultLogger *slog.Logger

// ContextHandler wraps an slog.Handler and automatically extracts
// contextual attributes such as request_id from context.Context.
type ContextHandler struct {
	handler slog.Handler
}

// NewContextHandler creates a new ContextHandler wrapping the given slog.Handler.
func NewContextHandler(h slog.Handler) *ContextHandler {
	return &ContextHandler{handler: h}
}

func (h *ContextHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.handler.Enabled(ctx, level)
}

func (h *ContextHandler) Handle(ctx context.Context, r slog.Record) error {
	if reqID := GetRequestID(ctx); reqID != "" {
		hasReqID := false
		r.Attrs(func(a slog.Attr) bool {
			if a.Key == "request_id" {
				hasReqID = true
				return false
			}
			return true
		})
		if !hasReqID {
			r.AddAttrs(slog.String("request_id", reqID))
		}
	}
	return h.handler.Handle(ctx, r)
}

func (h *ContextHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &ContextHandler{handler: h.handler.WithAttrs(attrs)}
}

func (h *ContextHandler) WithGroup(name string) slog.Handler {
	return &ContextHandler{handler: h.handler.WithGroup(name)}
}

func init() {
	DefaultLogger = InitLogger()
	slog.SetDefault(DefaultLogger)
}

// InitLogger initializes DefaultLogger writing to stdout based on LOG_LEVEL and LOG_FORMAT.
func InitLogger() *slog.Logger {
	return InitLoggerWithWriter(os.Stdout)
}

// InitLoggerWithWriter initializes a structured logger with an arbitrary writer for testability.
func InitLoggerWithWriter(w io.Writer) *slog.Logger {
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

	var baseHandler slog.Handler
	format := strings.ToLower(os.Getenv("LOG_FORMAT"))
	if format == "text" {
		baseHandler = slog.NewTextHandler(w, opts)
	} else {
		baseHandler = slog.NewJSONHandler(w, opts)
	}

	return slog.New(NewContextHandler(baseHandler))
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
