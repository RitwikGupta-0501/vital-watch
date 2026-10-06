package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/RitwikGupta-0501/vital-watch/internal/models"
	"github.com/RitwikGupta-0501/vital-watch/internal/audit"
	"github.com/RitwikGupta-0501/vital-watch/internal/crypto"
	"github.com/RitwikGupta-0501/vital-watch/internal/notifications"
	"github.com/RitwikGupta-0501/vital-watch/internal/pdf"
	"github.com/RitwikGupta-0501/vital-watch/internal/repository"
	"github.com/RitwikGupta-0501/vital-watch/internal/safety"
	"github.com/RitwikGupta-0501/vital-watch/internal/storage"
	"github.com/RitwikGupta-0501/vital-watch/internal/telehealth"
)

type activeCacheEntry struct {
	active    bool
	checkedAt time.Time
}

type userActiveChecker interface {
	IsUserActive(ctx context.Context, userID uuid.UUID, role string) bool
}

type Handler struct {
	Repo             repository.Repository
	Storage          storage.Provider
	JWTSecret        []byte
	DoctorInviteCode string
	AdminInviteCode  string
	OCREnabled       bool
	PDFGenerator     pdf.Generator
	SafetyChecker    safety.Checker
	Notifier         notifications.Broker
	Telehealth       telehealth.Provider
	Auditor          audit.Auditor
	CipherService    *crypto.CipherService
	Pool             *pgxpool.Pool // for connection pool telemetry in /healthz
	activeUserCache  sync.Map      // uuid.UUID -> activeCacheEntry
}

// InvalidateUserActiveCache clears the cached active status of a user upon administrative deactivation/reactivation.
func (h *Handler) InvalidateUserActiveCache(userID uuid.UUID) {
	h.activeUserCache.Delete(userID)
}

// IsUserActive determines whether an account is active, using a thread-safe 30s TTL in-memory cache
// to prevent per-request database lookup overhead while guaranteeing fast lockout.
func (h *Handler) IsUserActive(ctx context.Context, userID uuid.UUID, role string) bool {
	if h == nil || h.Repo == nil {
		return true
	}

	if val, ok := h.activeUserCache.Load(userID); ok {
		entry := val.(activeCacheEntry)
		if time.Since(entry.checkedAt) < 30*time.Second {
			return entry.active
		}
	}

	var active bool
	switch role {
	case "patient":
		_, err := h.Repo.GetPatientByID(ctx, userID)
		active = (err == nil)
	case "doctor":
		_, err := h.Repo.GetDoctorByID(ctx, userID)
		active = (err == nil)
	case "admin", "tenant_admin", "platform_admin":
		_, err := h.Repo.GetAdminByID(ctx, userID)
		active = (err == nil)
	default:
		active = false
	}

	h.activeUserCache.Store(userID, activeCacheEntry{
		active:    active,
		checkedAt: time.Now(),
	})
	return active
}

func (h *Handler) Ping(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"message": "pong from the api layer!"})
}

// HealthCheck verifies backend liveness, database connectivity, and connection
// pool health for container probes and SRE dashboards.
//
// A plain pool.Ping() succeeds even when all connections are checked out and
// requests are queuing. The pool stat fields below give Kubernetes readiness
// gates and SRE alerting real visibility into connection saturation.
func (h *Handler) HealthCheck(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
	defer cancel()

	if h.Repo == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"status":   "degraded",
			"database": "down",
			"error":    "repository not initialized",
		})
		return
	}

	if err := h.Repo.Ping(ctx); err != nil {
		slog.Error("HealthCheck: database ping failed", "error", err)
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"status":   "degraded",
			"database": "down",
			"error":    err.Error(),
		})
		return
	}

	resp := gin.H{
		"status":   "healthy",
		"database": "up",
	}

	// Attach connection pool telemetry when the pool reference is available.
	// pool_empty_acquires is a leading indicator of saturation: it counts requests
	// that had to wait because all connections were checked out.
	if h.Pool != nil {
		stat := h.Pool.Stat()
		resp["pool_total_conns"] = stat.TotalConns()
		resp["pool_acquired_conns"] = stat.AcquiredConns()
		resp["pool_idle_conns"] = stat.IdleConns()
		resp["pool_max_conns"] = stat.MaxConns()
		resp["pool_empty_acquires"] = stat.EmptyAcquireCount()
	}

	c.JSON(http.StatusOK, resp)
}


func AuthMiddleware(jwtSecret []byte) gin.HandlerFunc {
	return parseTokenMiddleware(jwtSecret, nil)
}

// SSEAuthMiddleware allows JWT authentication via Authorization header or ?token= query parameter, specifically for EventSource connections
func SSEAuthMiddleware(jwtSecret []byte) gin.HandlerFunc {
	return parseTokenMiddleware(jwtSecret, nil)
}

// AuthMiddleware on Handler enforces JWT verification and immediate user active status verification
func (h *Handler) AuthMiddleware() gin.HandlerFunc {
	return parseTokenMiddleware(h.JWTSecret, h)
}

// SSEAuthMiddleware on Handler enforces query/header JWT verification and immediate user active status verification
func (h *Handler) SSEAuthMiddleware() gin.HandlerFunc {
	return parseTokenMiddleware(h.JWTSecret, h)
}

func parseTokenMiddleware(jwtSecret []byte, checker userActiveChecker) gin.HandlerFunc {
	if len(jwtSecret) == 0 {
		panic("api: jwtSecret cannot be empty")
	}
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		tokenString := ""
		
		if authHeader != "" {
			parts := strings.Split(authHeader, " ")
			if len(parts) == 2 && parts[0] == "Bearer" {
				tokenString = parts[1]
			}
		}
		
		if tokenString == "" {
			cookieToken, err := c.Cookie("access_token")
			if err == nil && cookieToken != "" {
				tokenString = cookieToken
			}
		}

		if tokenString == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Authorization token missing"})
			return
		}

		token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
			if token.Method.Alg() != jwt.SigningMethodHS256.Alg() {
				return nil, jwt.ErrSignatureInvalid
			}
			return jwtSecret, nil
		}, jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}))

		if err != nil || !token.Valid {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Invalid token"})
			return
		}

		if claims, ok := token.Claims.(jwt.MapClaims); ok {
			// Validate issuer if present
			if iss, hasIss := claims["iss"].(string); hasIss && iss != "" && iss != "vital-watch" {
				c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Invalid token issuer"})
				return
			}

			subStr, ok := claims["sub"].(string)
			if !ok {
				c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Invalid token claims (sub)"})
				return
			}

			tenantID := uuid.Nil
			if tenantStr, ok := claims["tenant_id"].(string); ok {
				if parsed, err := uuid.Parse(tenantStr); err == nil {
					tenantID = parsed
				}
			}
			c.Set("tenant_id", tenantID.String())
			c.Request = c.Request.WithContext(context.WithValue(c.Request.Context(), "tenant_id", tenantID))

			userID, err := uuid.Parse(subStr)
			if err != nil {
				c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Invalid user ID in token"})
				return
			}

			role, ok := claims["role"].(string)
			if !ok {
				c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Invalid token claims (role)"})
				return
			}

			// Active account check: immediate rejection of soft-deactivated users
			if checker != nil && !checker.IsUserActive(c.Request.Context(), userID, role) {
				c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Account is inactive or deactivated"})
				return
			}

			c.Set("userID", userID)
			c.Set("role", role)
		}

		c.Next()
	}
}

func RequireRole(allowedRoles ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		userRole, exists := c.Get("role")
		if !exists {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Role not found in token context"})
			return
		}

		roleStr, ok := userRole.(string)
		if !ok {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Invalid role format in context"})
			return
		}

		for _, allowed := range allowedRoles {
			if roleStr == allowed {
				c.Next()
				return
			}
		}

		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "Access denied: insufficient permissions for this endpoint"})
	}
}

func parsePagination(c *gin.Context) (int, int) {
	limit := 20
	if limitStr := c.Query("limit"); limitStr != "" {
		if parsed, err := strconv.Atoi(limitStr); err == nil && parsed > 0 {
			limit = parsed
		}
	}
	if limit > 100 {
		limit = 100
	}

	offset := 0
	if offsetStr := c.Query("offset"); offsetStr != "" {
		if parsed, err := strconv.Atoi(offsetStr); err == nil && parsed >= 0 {
			offset = parsed
		}
	}
	return limit, offset
}

// ==========================================
// PHASE 5: TOKEN MANAGEMENT & HIPAA AUDITING
// ==========================================

func (h *Handler) audit(c *gin.Context, action, resourceType string, resourceID, patientID *uuid.UUID, statusCode int, metadata map[string]interface{}) {
	if h.Auditor == nil {
		return
	}
	var uid *uuid.UUID
	var role string
	if uVal, ok := c.Get("userID"); ok {
		if u, ok := uVal.(uuid.UUID); ok {
			uid = &u
		}
	}
	if rVal, ok := c.Get("role"); ok {
		if r, ok := rVal.(string); ok {
			role = r
		}
	}
	var reqID *uuid.UUID
	if rVal, ok := c.Get("requestID"); ok {
		if r, ok := rVal.(uuid.UUID); ok {
			reqID = &r
		}
	}
	var metaStr string
	if metadata != nil {
		if b, err := json.Marshal(metadata); err == nil {
			metaStr = string(b)
		}
	}
	h.Auditor.Log(models.PhiAuditLog{
		UserID:       uid,
		UserRole:     role,
		Action:       action,
		ResourceType: resourceType,
		ResourceID:   resourceID,
		PatientID:    patientID,
		IPAddress:    c.ClientIP(),
		UserAgent:    c.Request.UserAgent(),
		RequestID:    reqID,
		StatusCode:   statusCode,
		Metadata:     json.RawMessage(metaStr),
	})
}


func RequirePlatformAdmin() gin.HandlerFunc {
	return func(c *gin.Context) {
		userRole, exists := c.Get("role")
		if !exists || userRole != "platform_admin" {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "Platform Administrator privileges required"})
			return
		}
		c.Next()
	}
}

func RequireTenantAdmin() gin.HandlerFunc {
	return func(c *gin.Context) {
		userRole, exists := c.Get("role")
		if !exists || userRole != "tenant_admin" {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "Tenant Administrator privileges required"})
			return
		}
		c.Next()
	}
}
