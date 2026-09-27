package middleware

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/RitwikGupta-0501/vital-watch/internal/models"
	"github.com/gin-gonic/gin"
)

type mockAuditor struct {
	loggedCount int32
}

func (m *mockAuditor) Log(entry models.PhiAuditLog) {
	atomic.AddInt32(&m.loggedCount, 1)
}

func (m *mockAuditor) Shutdown(ctx context.Context) error {
	return nil
}

func TestRateLimiter_AllowedUnderLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)

	limiter := NewRateLimiter(5, time.Minute, nil)
	defer limiter.Close()

	r := gin.New()
	r.Use(limiter.Middleware())
	r.GET("/test", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})

	for i := 0; i < 5; i++ {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodGet, "/test", nil)
		req.RemoteAddr = "192.0.2.1:12345"
		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("request %d: expected 200 OK, got %d", i+1, w.Code)
		}
		if w.Header().Get("X-RateLimit-Limit") != "5" {
			t.Errorf("expected X-RateLimit-Limit 5, got %s", w.Header().Get("X-RateLimit-Limit"))
		}
	}
}

func TestRateLimiter_ExceededReturns429AndAudits(t *testing.T) {
	gin.SetMode(gin.TestMode)

	aud := &mockAuditor{}
	limiter := NewRateLimiter(3, time.Minute, aud)
	defer limiter.Close()

	r := gin.New()
	r.Use(limiter.Middleware())
	r.GET("/auth/login", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})

	// First 3 requests must succeed
	for i := 0; i < 3; i++ {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodGet, "/auth/login", nil)
		req.RemoteAddr = "198.51.100.1:54321"
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("request %d: expected 200, got %d", i+1, w.Code)
		}
	}

	// 4th request must be rejected with 429
	w4 := httptest.NewRecorder()
	req4, _ := http.NewRequest(http.MethodGet, "/auth/login", nil)
	req4.RemoteAddr = "198.51.100.1:54321"
	r.ServeHTTP(w4, req4)

	if w4.Code != http.StatusTooManyRequests {
		t.Fatalf("request 4: expected 429 Too Many Requests, got %d", w4.Code)
	}

	if w4.Header().Get("Retry-After") == "" {
		t.Errorf("expected Retry-After header to be present on 429 response")
	}

	if atomic.LoadInt32(&aud.loggedCount) != 1 {
		t.Errorf("expected 1 audit log for rate limit exhaustion, got %d", atomic.LoadInt32(&aud.loggedCount))
	}

	// A different IP address should NOT be affected
	wOther := httptest.NewRecorder()
	reqOther, _ := http.NewRequest(http.MethodGet, "/auth/login", nil)
	reqOther.RemoteAddr = "198.51.100.2:54321"
	r.ServeHTTP(wOther, reqOther)
	if wOther.Code != http.StatusOK {
		t.Fatalf("different IP: expected 200 OK, got %d", wOther.Code)
	}
}

func TestRateLimiter_TokenRefillAllowsSubsequentRequests(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// High refill rate: 2 tokens per 50ms window
	limiter := NewRateLimiter(2, 50*time.Millisecond, nil)
	defer limiter.Close()

	r := gin.New()
	r.Use(limiter.Middleware())
	r.GET("/refill", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})

	// Consume 2 tokens
	for i := 0; i < 2; i++ {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodGet, "/refill", nil)
		req.RemoteAddr = "203.0.113.5:11111"
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("request %d: expected 200, got %d", i+1, w.Code)
		}
	}

	// 3rd request should fail immediately
	w3 := httptest.NewRecorder()
	req3, _ := http.NewRequest(http.MethodGet, "/refill", nil)
	req3.RemoteAddr = "203.0.113.5:11111"
	r.ServeHTTP(w3, req3)
	if w3.Code != http.StatusTooManyRequests {
		t.Fatalf("request 3: expected 429, got %d", w3.Code)
	}

	// Wait for window to replenish tokens
	time.Sleep(60 * time.Millisecond)

	// Next request must succeed again
	w4 := httptest.NewRecorder()
	req4, _ := http.NewRequest(http.MethodGet, "/refill", nil)
	req4.RemoteAddr = "203.0.113.5:11111"
	r.ServeHTTP(w4, req4)
	if w4.Code != http.StatusOK {
		t.Fatalf("request after wait: expected 200 OK, got %d", w4.Code)
	}
}

func TestRateLimiter_ConcurrencySafety(t *testing.T) {
	gin.SetMode(gin.TestMode)

	limiter := NewRateLimiter(50, time.Minute, nil)
	defer limiter.Close()

	r := gin.New()
	r.Use(limiter.Middleware())
	r.GET("/concurrent", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})

	const numGoroutines = 1000
	var wg sync.WaitGroup
	var successCount int32
	var rateLimitedCount int32

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			w := httptest.NewRecorder()
			req, _ := http.NewRequest(http.MethodGet, "/concurrent", nil)
			req.RemoteAddr = fmt.Sprintf("10.0.%d.%d:10000", idx%256, (idx/256)%256)
			r.ServeHTTP(w, req)
			if w.Code == http.StatusOK {
				atomic.AddInt32(&successCount, 1)
			} else if w.Code == http.StatusTooManyRequests {
				atomic.AddInt32(&rateLimitedCount, 1)
			}
		}(i)
	}

	wg.Wait()

	if total := atomic.LoadInt32(&successCount) + atomic.LoadInt32(&rateLimitedCount); total != numGoroutines {
		t.Fatalf("expected total %d responses, got %d", numGoroutines, total)
	}
}
