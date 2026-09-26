package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/RitwikGupta-0501/vital-watch/internal/api"
	"github.com/RitwikGupta-0501/vital-watch/internal/repository"
	"github.com/RitwikGupta-0501/vital-watch/internal/storage"
)

func TestSetupRouter_NoRouteConflicts(t *testing.T) {
	h := &api.Handler{
		Storage:   storage.NewMockProvider(),
		JWTSecret: []byte("test-secret"),
	}

	// Verify both local and S3 storage type configurations initialize without panicking
	rLocal := setupRouter(h, "local", []byte("test-secret"))
	if rLocal == nil {
		t.Fatal("expected non-nil gin engine for local storage")
	}

	rS3 := setupRouter(h, "s3", []byte("test-secret"))
	if rS3 == nil {
		t.Fatal("expected non-nil gin engine for s3 storage")
	}
}

func TestSetupRouter_HealthzRoutes(t *testing.T) {
	mockRepo := &repository.MockRepository{
		PingFunc: func(ctx context.Context) error {
			return nil
		},
	}

	h := &api.Handler{
		Repo:      mockRepo,
		Storage:   storage.NewMockProvider(),
		JWTSecret: []byte("test-secret-at-least-32-bytes-long!"),
	}

	r := setupRouter(h, "local", h.JWTSecret)

	for _, path := range []string{"/healthz", "/api/healthz"} {
		req, _ := http.NewRequest(http.MethodGet, path, nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected 200 for %s, got %d", path, w.Code)
		}
		if !strings.Contains(w.Body.String(), `"status":"healthy"`) {
			t.Errorf("expected body to contain healthy status for %s, got: %s", path, w.Body.String())
		}
	}
}

func TestSetupRouter_CORSTracingHeaders(t *testing.T) {
	h := &api.Handler{
		Storage:   storage.NewMockProvider(),
		JWTSecret: []byte("test-secret-at-least-32-bytes-long!"),
	}

	r := setupRouter(h, "local", h.JWTSecret)

	// Preflight OPTIONS request
	req, _ := http.NewRequest(http.MethodOptions, "/api/ping", nil)
	req.Header.Set("Origin", "http://localhost:3000")
	req.Header.Set("Access-Control-Request-Method", "GET")
	req.Header.Set("Access-Control-Request-Headers", "X-Request-ID, Content-Type")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	allowHeaders := strings.ToLower(w.Header().Get("Access-Control-Allow-Headers"))
	if !strings.Contains(allowHeaders, "x-request-id") {
		t.Errorf("expected Access-Control-Allow-Headers to contain x-request-id, got: %s", allowHeaders)
	}

	// Normal GET request exposing headers
	reqGet, _ := http.NewRequest(http.MethodGet, "/api/ping", nil)
	reqGet.Header.Set("Origin", "http://localhost:3000")
	wGet := httptest.NewRecorder()
	r.ServeHTTP(wGet, reqGet)

	exposeHeaders := strings.ToLower(wGet.Header().Get("Access-Control-Expose-Headers"))
	if !strings.Contains(exposeHeaders, "x-request-id") {
		t.Errorf("expected Access-Control-Expose-Headers to contain x-request-id, got: %s", exposeHeaders)
	}
}
