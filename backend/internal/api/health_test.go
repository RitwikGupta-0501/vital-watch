package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

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

func TestMetricsProtectionMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)

	jwtSecret := []byte("super-secret-jwt-key-minimum-32-bytes!!")
	scrapeToken := "test-scrape-token-12345"

	r := gin.New()
	r.GET("/metrics", MetricsProtectionMiddleware(jwtSecret, scrapeToken), func(c *gin.Context) {
		c.String(http.StatusOK, "metrics-payload-ok")
	})

	// 1. Unauthenticated request -> 401 Unauthorized
	req1, _ := http.NewRequest(http.MethodGet, "/metrics", nil)
	w1 := httptest.NewRecorder()
	r.ServeHTTP(w1, req1)
	if w1.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for unauthenticated /metrics request, got %d", w1.Code)
	}

	// 2. Request with valid scrape token via Bearer header -> 200 OK
	req2, _ := http.NewRequest(http.MethodGet, "/metrics", nil)
	req2.Header.Set("Authorization", "Bearer "+scrapeToken)
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req2)
	if w2.Code != http.StatusOK || w2.Body.String() != "metrics-payload-ok" {
		t.Errorf("expected 200 with valid scrape token, got code %d body %s", w2.Code, w2.Body.String())
	}

	// 3. Request with valid scrape token via direct Authorization header -> 200 OK
	req3, _ := http.NewRequest(http.MethodGet, "/metrics", nil)
	req3.Header.Set("Authorization", scrapeToken)
	w3 := httptest.NewRecorder()
	r.ServeHTTP(w3, req3)
	if w3.Code != http.StatusOK {
		t.Errorf("expected 200 with direct scrape token header, got %d", w3.Code)
	}

	// 4. Request with invalid scrape token -> 401 Unauthorized
	req4, _ := http.NewRequest(http.MethodGet, "/metrics", nil)
	req4.Header.Set("Authorization", "Bearer wrong-token")
	w4 := httptest.NewRecorder()
	r.ServeHTTP(w4, req4)
	if w4.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 with wrong scrape token, got %d", w4.Code)
	}

	// 5. Request with platform_admin JWT -> 200 OK
	adminClaims := jwt.MapClaims{
		"sub":       uuid.New().String(),
		"role":      "platform_admin",
		"tenant_id": uuid.New().String(),
		"iss":       "vital-watch",
		"exp":       time.Now().Add(15 * time.Minute).Unix(),
	}
	adminTokObj := jwt.NewWithClaims(jwt.SigningMethodHS256, adminClaims)
	adminToken, err := adminTokObj.SignedString(jwtSecret)
	if err != nil {
		t.Fatalf("failed to sign admin token: %v", err)
	}
	req5, _ := http.NewRequest(http.MethodGet, "/metrics", nil)
	req5.Header.Set("Authorization", "Bearer "+adminToken)
	w5 := httptest.NewRecorder()
	r.ServeHTTP(w5, req5)
	if w5.Code != http.StatusOK {
		t.Errorf("expected 200 with platform_admin JWT, got %d", w5.Code)
	}

	// 6. Request with non-platform_admin JWT (e.g. patient or tenant_admin) -> 401 Unauthorized
	patientClaims := jwt.MapClaims{
		"sub":       uuid.New().String(),
		"role":      "patient",
		"tenant_id": uuid.New().String(),
		"iss":       "vital-watch",
		"exp":       time.Now().Add(15 * time.Minute).Unix(),
	}
	patientTokObj := jwt.NewWithClaims(jwt.SigningMethodHS256, patientClaims)
	patientToken, err := patientTokObj.SignedString(jwtSecret)
	if err != nil {
		t.Fatalf("failed to sign patient token: %v", err)
	}
	req6, _ := http.NewRequest(http.MethodGet, "/metrics", nil)
	req6.Header.Set("Authorization", "Bearer "+patientToken)
	w6 := httptest.NewRecorder()
	r.ServeHTTP(w6, req6)
	if w6.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 with patient JWT, got %d", w6.Code)
	}
}

