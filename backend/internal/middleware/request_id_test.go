package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func TestRequestIDMiddleware_GeneratedIfMissing(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(RequestIDMiddleware())

	var capturedID string
	r.GET("/test", func(c *gin.Context) {
		capturedID = c.GetString("requestIDStr")
		c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", w.Code)
	}

	headerID := w.Header().Get(HeaderXRequestID)
	if headerID == "" {
		t.Fatal("expected X-Request-ID header in response")
	}

	if _, err := uuid.Parse(headerID); err != nil {
		t.Fatalf("expected valid UUID in X-Request-ID header, got %s", headerID)
	}

	if capturedID != headerID {
		t.Errorf("context ID %s does not match header ID %s", capturedID, headerID)
	}
}

func TestRequestIDMiddleware_PreservedIfValid(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(RequestIDMiddleware())

	customID := uuid.New().String()
	r.GET("/test", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set(HeaderXRequestID, customID)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Header().Get(HeaderXRequestID) != customID {
		t.Errorf("expected preserved header %s, got %s", customID, w.Header().Get(HeaderXRequestID))
	}
}
