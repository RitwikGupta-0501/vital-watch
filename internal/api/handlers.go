package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"github.com/RitwikGupta-0501/vital-watch/internal/models"
	"github.com/RitwikGupta-0501/vital-watch/internal/audit"
	"github.com/RitwikGupta-0501/vital-watch/internal/notifications"
	"github.com/RitwikGupta-0501/vital-watch/internal/pdf"
	"github.com/RitwikGupta-0501/vital-watch/internal/repository"
	"github.com/RitwikGupta-0501/vital-watch/internal/safety"
	"github.com/RitwikGupta-0501/vital-watch/internal/storage"
	"github.com/RitwikGupta-0501/vital-watch/internal/telehealth"
)

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
}

func (h *Handler) Ping(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"message": "pong from the api layer!"})
}

func AuthMiddleware(jwtSecret []byte) gin.HandlerFunc {
	return parseTokenMiddleware(jwtSecret, false)
}

// SSEAuthMiddleware allows JWT authentication via Authorization header or ?token= query parameter, specifically for EventSource connections
func SSEAuthMiddleware(jwtSecret []byte) gin.HandlerFunc {
	return parseTokenMiddleware(jwtSecret, true)
}

func parseTokenMiddleware(jwtSecret []byte, allowQueryToken bool) gin.HandlerFunc {
	if len(jwtSecret) == 0 {
		panic("api: jwtSecret cannot be empty")
	}
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		tokenString := ""
		if authHeader != "" {
			parts := strings.Split(authHeader, " ")
			if len(parts) != 2 || parts[0] != "Bearer" {
				c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Invalid token format"})
				return
			}
			tokenString = parts[1]
		} else if allowQueryToken && c.Query("token") != "" {
			tokenString = strings.TrimSpace(c.Query("token"))
		} else {
			errMsg := "Authorization header missing"
			if allowQueryToken {
				errMsg = "Authorization header or token query parameter missing"
			}
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": errMsg})
			return
		}

		token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, jwt.ErrSignatureInvalid
			}
			return jwtSecret, nil
		})

		if err != nil || !token.Valid {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Invalid token"})
			return
		}

		if claims, ok := token.Claims.(jwt.MapClaims); ok {
			subStr, ok := claims["sub"].(string)
			if !ok {
				c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Invalid token claims (sub)"})
				return
			}

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
		Metadata:     metaStr,
	})
}

