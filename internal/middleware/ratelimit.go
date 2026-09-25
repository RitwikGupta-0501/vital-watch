package middleware

import (
	"fmt"
	"math"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/RitwikGupta-0501/vital-watch/internal/audit"
	"github.com/RitwikGupta-0501/vital-watch/internal/models"
	"github.com/gin-gonic/gin"
)

type tokenBucket struct {
	tokens     float64
	lastUpdate time.Time
}

// RateLimiter provides memory-safe token-bucket rate limiting per client IP
// with automatic background pruning to prevent memory exhaustion (DoS).
type RateLimiter struct {
	mu       sync.Mutex
	clients  map[string]*tokenBucket
	rate     float64       // token replenishment rate (tokens/sec)
	capacity float64       // burst limit
	window   time.Duration
	limit    int
	auditor  audit.Auditor
	stopChan chan struct{}
}

// NewRateLimiter creates a new RateLimiter allowing `limit` requests per `window` duration per IP.
func NewRateLimiter(limit int, window time.Duration, auditor audit.Auditor) *RateLimiter {
	if limit <= 0 {
		limit = 60
	}
	if window <= 0 {
		window = time.Minute
	}

	rate := float64(limit) / window.Seconds()
	rl := &RateLimiter{
		clients:  make(map[string]*tokenBucket),
		rate:     rate,
		capacity: float64(limit),
		window:   window,
		limit:    limit,
		auditor:  auditor,
		stopChan: make(chan struct{}),
	}

	// Prune inactive clients every 2x window duration to avoid memory leaks
	cleanupInterval := window * 2
	if cleanupInterval < 500*time.Millisecond {
		cleanupInterval = 500 * time.Millisecond
	}
	go rl.cleanupRoutine(cleanupInterval)

	return rl
}

// Close stops the background eviction worker.
func (rl *RateLimiter) Close() {
	select {
	case <-rl.stopChan:
		// already closed
	default:
		close(rl.stopChan)
	}
}

func (rl *RateLimiter) cleanupRoutine(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-rl.stopChan:
			return
		case now := <-ticker.C:
			rl.mu.Lock()
			for ip, bucket := range rl.clients {
				if now.Sub(bucket.lastUpdate) > interval {
					delete(rl.clients, ip)
				}
			}
			rl.mu.Unlock()
		}
	}
}

// Middleware returns a Gin middleware that enforces the configured rate limit.
func (rl *RateLimiter) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		ip := c.ClientIP()
		if ip == "" {
			ip = "unknown"
		}

		rl.mu.Lock()
		now := time.Now()
		bucket, exists := rl.clients[ip]
		if !exists {
			bucket = &tokenBucket{
				tokens:     rl.capacity,
				lastUpdate: now,
			}
			rl.clients[ip] = bucket
		}

		// Replenish tokens based on elapsed duration
		elapsed := now.Sub(bucket.lastUpdate).Seconds()
		bucket.tokens = math.Min(rl.capacity, bucket.tokens+elapsed*rl.rate)
		bucket.lastUpdate = now

		if bucket.tokens < 1.0 {
			// Limit exceeded: calculate retry-after seconds
			retryAfter := int(math.Ceil((1.0 - bucket.tokens) / rl.rate))
			if retryAfter < 1 {
				retryAfter = 1
			}
			rl.mu.Unlock()

			c.Header("Retry-After", strconv.Itoa(retryAfter))
			c.Header("X-RateLimit-Limit", strconv.Itoa(rl.limit))
			c.Header("X-RateLimit-Remaining", "0")

			// Audit the rate limit violation for HIPAA security compliance
			if rl.auditor != nil {
				rl.auditor.Log(models.PhiAuditLog{
					Action:     audit.ActionRateLimitExceeded,
					IPAddress:  ip,
					UserAgent:  c.Request.UserAgent(),
					StatusCode: http.StatusTooManyRequests,
					Metadata:   fmt.Sprintf(`{"path":%q,"method":%q,"retry_after":%d}`, c.Request.URL.Path, c.Request.Method, retryAfter),
				})
			}

			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"error": "Too many requests. Please try again later.",
			})
			return
		}

		// Consume 1 token
		bucket.tokens -= 1.0
		remaining := int(math.Floor(bucket.tokens))
		rl.mu.Unlock()

		c.Header("X-RateLimit-Limit", strconv.Itoa(rl.limit))
		c.Header("X-RateLimit-Remaining", strconv.Itoa(remaining))

		c.Next()
	}
}
