package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestSecurityHeadersMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	r.Use(SecurityHeadersMiddleware())
	r.GET("/ping", func(c *gin.Context) {
		c.String(http.StatusOK, "pong")
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/ping", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	expectedHeaders := map[string]string{
		"X-Content-Type-Options":    "nosniff",
		"X-Frame-Options":           "DENY",
		"X-XSS-Protection":          "0",
		"Referrer-Policy":           "strict-origin-when-cross-origin",
		"Strict-Transport-Security": "max-age=31536000; includeSubDomains",
		"Content-Security-Policy":   "default-src 'self'",
	}

	for header, expectedVal := range expectedHeaders {
		actualVal := w.Header().Get(header)
		if actualVal != expectedVal {
			t.Errorf("header %s: expected %q, got %q", header, expectedVal, actualVal)
		}
	}
}

func TestSensitiveCacheControlMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	r.Use(SensitiveCacheControlMiddleware())
	r.GET("/phi", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"patient": "data"})
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/phi", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	expectedCacheControl := "no-store, no-cache, must-revalidate, private"
	if actual := w.Header().Get("Cache-Control"); actual != expectedCacheControl {
		t.Errorf("Cache-Control: expected %q, got %q", expectedCacheControl, actual)
	}
	if actual := w.Header().Get("Pragma"); actual != "no-cache" {
		t.Errorf("Pragma: expected %q, got %q", "no-cache", actual)
	}
	if actual := w.Header().Get("Expires"); actual != "0" {
		t.Errorf("Expires: expected %q, got %q", "0", actual)
	}
}
