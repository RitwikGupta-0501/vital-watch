package middleware

import (
	"log/slog"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/RitwikGupta-0501/vital-watch/internal/logger"
)

const HeaderXRequestID = "X-Request-ID"

// RequestIDMiddleware extracts or generates a unique UUID X-Request-ID for every incoming request
func RequestIDMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		reqIDStr := strings.TrimSpace(c.GetHeader(HeaderXRequestID))
		var reqID uuid.UUID
		var err error

		if reqIDStr != "" {
			reqID, err = uuid.Parse(reqIDStr)
		}
		if reqIDStr == "" || err != nil {
			reqID = uuid.New()
			reqIDStr = reqID.String()
		}

		// Inject into Gin context and Request Context
		c.Set("requestID", reqID)
		c.Set("requestIDStr", reqIDStr)
		ctx := logger.WithRequestID(c.Request.Context(), reqIDStr)
		c.Request = c.Request.WithContext(ctx)

		// Set outgoing response header
		c.Writer.Header().Set(HeaderXRequestID, reqIDStr)

		start := time.Now()
		c.Next()
		duration := time.Since(start)

		// Structured access logging
		status := c.Writer.Status()
		logAttrs := []any{
			"method", c.Request.Method,
			"path", c.Request.URL.Path,
			"status", status,
			"duration_ms", duration.Milliseconds(),
			"ip", c.ClientIP(),
			"request_id", reqIDStr,
		}

		if status >= 500 {
			slog.ErrorContext(ctx, "HTTP Server Error", logAttrs...)
		} else if status >= 400 {
			slog.WarnContext(ctx, "HTTP Client Error", logAttrs...)
		} else {
			slog.InfoContext(ctx, "HTTP Request Completed", logAttrs...)
		}
	}
}
