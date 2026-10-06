package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/RitwikGupta-0501/vital-watch/internal/repository"
)

func TestHealthCheck_Success(t *testing.T) {
	gin.SetMode(gin.TestMode)

	mockRepo := &repository.MockRepository{
		PingFunc: func(ctx context.Context) error {
			return nil
		},
	}

	h := &Handler{Repo: mockRepo}
	r := gin.New()
	r.GET("/healthz", h.HealthCheck)

	req, _ := http.NewRequest(http.MethodGet, "/healthz", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	var resp map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp["status"] != "healthy" || resp["database"] != "up" {
		t.Errorf("unexpected body: %+v", resp)
	}
}

func TestHealthCheck_DatabaseDown(t *testing.T) {
	gin.SetMode(gin.TestMode)

	mockRepo := &repository.MockRepository{
		PingFunc: func(ctx context.Context) error {
			return errors.New("connection pool exhausted or db unreachable")
		},
	}

	h := &Handler{Repo: mockRepo}
	r := gin.New()
	r.GET("/healthz", h.HealthCheck)

	req, _ := http.NewRequest(http.MethodGet, "/healthz", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected status 503, got %d", w.Code)
	}

	var resp map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp["status"] != "degraded" || resp["database"] != "down" {
		t.Errorf("unexpected body: %+v", resp)
	}
}

func TestHealthCheck_NilRepository(t *testing.T) {
	gin.SetMode(gin.TestMode)

	h := &Handler{Repo: nil}
	r := gin.New()
	r.GET("/healthz", h.HealthCheck)

	req, _ := http.NewRequest(http.MethodGet, "/healthz", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected status 503, got %d", w.Code)
	}

	var resp map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp["status"] != "degraded" || resp["database"] != "down" {
		t.Errorf("unexpected body: %+v", resp)
	}
}
