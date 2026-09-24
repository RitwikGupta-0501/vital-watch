package api

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/mail"
	"strings"
	"time"

	"github.com/RitwikGupta-0501/vital-watch/internal/audit"
	"github.com/RitwikGupta-0501/vital-watch/internal/models"
	"github.com/RitwikGupta-0501/vital-watch/internal/repository"
	"github.com/RitwikGupta-0501/vital-watch/utils"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
)

// Precomputed bcrypt hash used to equalize timing when non-existent users are queried,
// preventing side-channel email enumeration probes (OWASP A07).
var dummyBcryptHash string

func init() {
	hash, err := utils.HashPassword("vital-watch-timing-mitigation-dummy-password")
	if err != nil {
		dummyBcryptHash = "$2a$10$7EqJtq98hPqEX7fNZaFWoO.8/kSfvj7g.XgP1W03tYF1wT7aK9i7."
	} else {
		dummyBcryptHash = hash
	}
}

func generateSecureRandomToken() (raw string, hash string, err error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", "", err
	}
	raw = hex.EncodeToString(b)
	h := sha256.Sum256([]byte(raw))
	hash = hex.EncodeToString(h[:])
	return raw, hash, nil
}

func hashToken(raw string) string {
	h := sha256.Sum256([]byte(strings.TrimSpace(raw)))
	return hex.EncodeToString(h[:])
}

func handleRegisterDBError(c *gin.Context, err error) {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		c.JSON(http.StatusConflict, gin.H{"error": "An account with this email already exists"})
		return
	}
	errStr := err.Error()
	if strings.Contains(errStr, "duplicate key") || strings.Contains(errStr, "unique constraint") || strings.Contains(errStr, "users_email_key") {
		c.JSON(http.StatusConflict, gin.H{"error": "An account with this email already exists"})
		return
	}
	c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create user"})
}

// Register creates a new patient, doctor, or admin account with role-segregated validation.
func (h *Handler) Register(c *gin.Context) {
	var req struct {
		Role       string `json:"role"`
		FirstName  string `json:"first_name"`
		LastName   string `json:"last_name"`
		Email      string `json:"email"`
		Password   string `json:"password"`
		Specialty  string `json:"specialty,omitempty"`
		Experience int    `json:"experience,omitempty"`
		InviteCode string `json:"invite_code,omitempty"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	req.Email = strings.TrimSpace(req.Email)
	if req.Email == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "A valid email address is required"})
		return
	}

	parsedAddr, err := mail.ParseAddress(req.Email)
	if err != nil || parsedAddr.Address != req.Email || !strings.Contains(req.Email, "@") {
		c.JSON(http.StatusBadRequest, gin.H{"error": "A valid email address is required"})
		return
	}
	parts := strings.Split(parsedAddr.Address, "@")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" || !strings.Contains(parts[1], ".") {
		c.JSON(http.StatusBadRequest, gin.H{"error": "A valid email address is required"})
		return
	}
	req.Email = strings.ToLower(parsedAddr.Address)

	if len(req.Password) < 8 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Password must be at least 8 characters long"})
		return
	}
	if len(req.Password) > 72 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Password cannot exceed 72 bytes"})
		return
	}

	req.FirstName = strings.TrimSpace(req.FirstName)
	req.LastName = strings.TrimSpace(req.LastName)
	if req.FirstName == "" || req.LastName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "First name and last name are required"})
		return
	}
	if len(req.FirstName) > 100 || len(req.LastName) > 100 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "First name and last name must not exceed 100 characters"})
		return
	}

	ctx := c.Request.Context()
	var newID uuid.UUID
	switch req.Role {
	case "patient":
		hashed, err := utils.HashPassword(req.Password)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to hash password"})
			return
		}
		newID, err = h.Repo.CreatePatient(ctx, req.FirstName, req.LastName, req.Email, hashed)
		if err != nil {
			handleRegisterDBError(c, err)
			return
		}

	case "doctor":
		if h.DoctorInviteCode == "" || subtle.ConstantTimeCompare([]byte(req.InviteCode), []byte(h.DoctorInviteCode)) != 1 {
			c.JSON(http.StatusForbidden, gin.H{"error": "Doctor registration is restricted or invalid invite code"})
			return
		}
		hashed, err := utils.HashPassword(req.Password)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to hash password"})
			return
		}
		newID, err = h.Repo.CreateDoctor(ctx, req.FirstName, req.LastName, req.Email, hashed, req.Specialty, req.Experience)
		if err != nil {
			handleRegisterDBError(c, err)
			return
		}

	case "admin":
		if h.AdminInviteCode == "" || subtle.ConstantTimeCompare([]byte(req.InviteCode), []byte(h.AdminInviteCode)) != 1 {
			c.JSON(http.StatusForbidden, gin.H{"error": "Admin registration is restricted or invalid invite code"})
			return
		}
		hashed, err := utils.HashPassword(req.Password)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to hash password"})
			return
		}
		dept := req.Specialty
		if dept == "" {
			dept = "Operations"
		}
		newID, err = h.Repo.CreateAdmin(ctx, req.FirstName, req.LastName, req.Email, hashed, dept)
		if err != nil {
			handleRegisterDBError(c, err)
			return
		}

	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid role"})
		return
	}

	// Audit successful registration (HIPAA § 164.312(b))
	if h.Auditor != nil {
		h.Auditor.Log(models.PhiAuditLog{
			UserID:     &newID,
			UserRole:   req.Role,
			Action:     audit.ActionUserRegister,
			IPAddress:  c.ClientIP(),
			UserAgent:  c.Request.UserAgent(),
			StatusCode: http.StatusCreated,
			Metadata:   fmt.Sprintf(`{"role":%q,"email":%q}`, req.Role, req.Email),
		})
	}

	c.JSON(http.StatusCreated, gin.H{"id": newID})
}

// Login authenticates a user, issuing a 15-minute access JWT and a rotating 7-day refresh token.
func (h *Handler) Login(c *gin.Context) {
	var req struct {
		Role     string `json:"role"`
		Email    string `json:"email"`
		Password string `json:"password"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	req.Email = strings.ToLower(strings.TrimSpace(req.Email))
	if req.Email == "" || len(req.Password) == 0 || len(req.Password) > 72 {
		_ = utils.CheckPasswordHash("dummy", dummyBcryptHash)
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid credentials"})
		return
	}

	var (
		user models.Authenticatable
		err  error
	)

	ctx := c.Request.Context()
	switch req.Role {
	case "patient":
		user, err = h.Repo.GetPatientByEmail(ctx, req.Email)
	case "doctor":
		user, err = h.Repo.GetDoctorByEmail(ctx, req.Email)
	case "admin":
		user, err = h.Repo.GetAdminByEmail(ctx, req.Email)
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid role"})
		return
	}

	if err != nil {
		// Evaluate dummy bcrypt to equalize timing and mitigate side-channel enumeration
		_ = utils.CheckPasswordHash(req.Password, dummyBcryptHash)

		// Audit failed login attempt (HIPAA § 164.312(b))
	if h.Auditor != nil {
			h.Auditor.Log(models.PhiAuditLog{
				Action:     audit.ActionUserLogin,
				IPAddress:  c.ClientIP(),
				UserAgent:  c.Request.UserAgent(),
				StatusCode: http.StatusUnauthorized,
				Metadata:   fmt.Sprintf(`{"email":%q,"role":%q,"reason":"user_not_found"}`, req.Email, req.Role),
			})
		}
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid credentials"})
		return
	}

	if !utils.CheckPasswordHash(req.Password, user.GetHashedPassword()) || (user.GetRole() != "" && user.GetRole() != req.Role) {
		// Audit failed login attempt (HIPAA § 164.312(b))
		userID := user.GetID()
		if h.Auditor != nil {
			h.Auditor.Log(models.PhiAuditLog{
				UserID:     &userID,
				Action:     audit.ActionUserLogin,
				IPAddress:  c.ClientIP(),
				UserAgent:  c.Request.UserAgent(),
				StatusCode: http.StatusUnauthorized,
				Metadata:   fmt.Sprintf(`{"email":%q,"role":%q,"reason":"bad_credentials"}`, req.Email, req.Role),
			})
		}
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid credentials"})
		return
	}

	// Access token lifetime: 15 minutes (SEC-09)
	claims := jwt.MapClaims{
		"sub":  user.GetID().String(),
		"role": req.Role,
		"iat":  time.Now().Unix(),
		"exp":  time.Now().Add(15 * time.Minute).Unix(),
		"iss":  "vital-watch",
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString(h.JWTSecret)
	if err != nil {
		slog.ErrorContext(ctx, "Failed to sign access token", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate token"})
		return
	}

	// Cryptographically secure refresh token (7-day validity)
	rawRefresh, refreshHash, err := generateSecureRandomToken()
	if err != nil {
		slog.ErrorContext(ctx, "Failed to generate refresh token", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate session tokens"})
		return
	}

	_, err = h.Repo.CreateRefreshToken(ctx, user.GetID(), refreshHash, time.Now().Add(7*24*time.Hour))
	if err != nil {
		slog.ErrorContext(ctx, "Failed to store refresh token", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to store session tokens"})
		return
	}

	// Audit successful login (HIPAA § 164.312(b))
	if h.Auditor != nil {
		successUserID := user.GetID()
		h.Auditor.Log(models.PhiAuditLog{
			UserID:     &successUserID,
			UserRole:   req.Role,
			Action:     audit.ActionUserLogin,
			IPAddress:  c.ClientIP(),
			UserAgent:  c.Request.UserAgent(),
			StatusCode: http.StatusOK,
			Metadata:   fmt.Sprintf(`{"role":%q}`, req.Role),
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"access_token":  tokenString,
		"refresh_token": rawRefresh,
		"token_type":    "Bearer",
		"expires_in":    900,
		"token":         tokenString,
	})
}

// RefreshToken atomically rotates a single-use refresh token and issues a new access token.
func (h *Handler) RefreshToken(c *gin.Context) {
	var req struct {
		RefreshToken string `json:"refresh_token"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.RefreshToken) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "refresh_token is required"})
		return
	}

	req.RefreshToken = strings.TrimSpace(req.RefreshToken)
	if len(req.RefreshToken) > 512 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid refresh token format"})
		return
	}

	tokenHash := hashToken(req.RefreshToken)
	existingToken, err := h.Repo.GetRefreshTokenByHash(c.Request.Context(), tokenHash)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid refresh token"})
		return
	}

	// TOKEN THEFT DETECTION: If a previously revoked token is reused, revoke all tokens for this user
	if existingToken.RevokedAt != nil {
		if time.Since(*existingToken.RevokedAt) >= 10*time.Second {
		slog.WarnContext(c.Request.Context(), "SECURITY ALERT: Reuse of revoked refresh token outside grace window! Revoking all sessions.", "user_id", existingToken.UserID)
			// Audit the security breach event (HIPAA § 164.312(b))
			if h.Auditor != nil {
				h.Auditor.Log(models.PhiAuditLog{
					UserID:     &existingToken.UserID,
					Action:     audit.ActionTokenReuseAlert,
					IPAddress:  c.ClientIP(),
					UserAgent:  c.Request.UserAgent(),
					StatusCode: http.StatusUnauthorized,
					Metadata:   fmt.Sprintf(`{"token_id":%q}`, existingToken.ID.String()),
				})
			}
			_ = h.Repo.RevokeAllUserRefreshTokens(c.Request.Context(), existingToken.UserID)
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Compromised token detected; all sessions have been terminated"})
			return
		}
		// If within 10-second grace window, do not terminate immediately.
		// RotateRefreshToken will atomically verify if the replacement token was consumed.
	}

	if existingToken.ExpiresAt.Before(time.Now()) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Refresh token has expired"})
		return
	}

	// Look up user role (Patient, Doctor, or Admin). Active status is asserted by Get*ByID queries.
	var role string
	if p, pErr := h.Repo.GetPatientByID(c.Request.Context(), existingToken.UserID); pErr == nil {
		role = p.Role
	} else if d, dErr := h.Repo.GetDoctorByID(c.Request.Context(), existingToken.UserID); dErr == nil {
		role = d.Role
	} else if a, aErr := h.Repo.GetAdminByID(c.Request.Context(), existingToken.UserID); aErr == nil {
		role = a.Role
	} else {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User account no longer active"})
		return
	}

	// Generate replacement refresh token
	newRawRefresh, newRefreshHash, err := generateSecureRandomToken()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate replacement token"})
		return
	}

	// RotateRefreshToken atomically revokes the old token and inserts the new one
	// in a single Postgres transaction with a SELECT FOR UPDATE lock, preventing
	// concurrent rotation races and recovering lost mobile retry responses if unconsumed.
	newTok, err := h.Repo.RotateRefreshToken(c.Request.Context(), existingToken.ID, existingToken.UserID, newRefreshHash, time.Now().Add(7*24*time.Hour))
	if err != nil {
		if errors.Is(err, repository.ErrTokenAlreadyRotated) {
			slog.WarnContext(c.Request.Context(), "SECURITY ALERT: Token rotation replay detected (replacement was already consumed). Revoking all sessions.", "user_id", existingToken.UserID)
			if h.Auditor != nil {
				h.Auditor.Log(models.PhiAuditLog{
					UserID:     &existingToken.UserID,
					Action:     audit.ActionTokenReuseAlert,
					IPAddress:  c.ClientIP(),
					UserAgent:  c.Request.UserAgent(),
					StatusCode: http.StatusUnauthorized,
					Metadata:   fmt.Sprintf(`{"token_id":%q}`, existingToken.ID.String()),
				})
			}
			_ = h.Repo.RevokeAllUserRefreshTokens(c.Request.Context(), existingToken.UserID)
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Compromised token detected; all sessions have been terminated"})
			return
		}
		slog.ErrorContext(c.Request.Context(), "Failed to rotate refresh token", "error", err, "token_id", existingToken.ID)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to rotate session token"})
		return
	}
	_ = newTok

	// Issue new 15-minute access token
	claims := jwt.MapClaims{
		"sub":  existingToken.UserID.String(),
		"role": role,
		"iat":  time.Now().Unix(),
		"exp":  time.Now().Add(15 * time.Minute).Unix(),
		"iss":  "vital-watch",
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString(h.JWTSecret)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to sign access token"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"access_token":  tokenString,
		"refresh_token": newRawRefresh,
		"token_type":    "Bearer",
		"expires_in":    900,
		"token":         tokenString,
	})
}

// Logout revokes the caller's active refresh token or all user refresh tokens if requested.
func (h *Handler) Logout(c *gin.Context) {
	var req struct {
		RefreshToken string `json:"refresh_token"`
		AllDevices   bool   `json:"all_devices,omitempty"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.RefreshToken) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "refresh_token is required"})
		return
	}

	tokenHash := hashToken(req.RefreshToken)
	existingToken, err := h.Repo.GetRefreshTokenByHash(c.Request.Context(), tokenHash)
	if err == nil {
		if req.AllDevices {
			_ = h.Repo.RevokeAllUserRefreshTokens(c.Request.Context(), existingToken.UserID)
		} else if existingToken.RevokedAt == nil {
			_ = h.Repo.RevokeRefreshToken(c.Request.Context(), existingToken.ID, nil)
		}
	}

	h.audit(c, audit.ActionUserLogout, "session", nil, nil, http.StatusOK, map[string]interface{}{
		"all_devices": req.AllDevices,
	})
	c.JSON(http.StatusOK, gin.H{"message": "Successfully logged out"})
}

// GetUserProfile returns the authenticated user's profile based on their verified JWT claims.
func (h *Handler) GetUserProfile(c *gin.Context) {
	userIDVal, ok := c.Get("userID")
	if !ok {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "User ID not found in context"})
		return
	}
	userID, ok := userIDVal.(uuid.UUID)
	if !ok {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "Invalid user ID type in context"})
		return
	}

	roleVal, ok := c.Get("role")
	if !ok {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "Role not found in context"})
		return
	}
	role, ok := roleVal.(string)
	if !ok {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "Invalid role type in context"})
		return
	}

	ctx := c.Request.Context()
	switch role {
	case "patient":
		patient, err := h.Repo.GetPatientByID(ctx, userID)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "Patient profile not found"})
			return
		}
		h.audit(c, audit.ActionViewPatientProfile, "patient", &userID, &userID, http.StatusOK, nil)
		c.JSON(http.StatusOK, patient)

	case "doctor":
		doctor, err := h.Repo.GetDoctorByID(ctx, userID)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "Doctor profile not found"})
			return
		}
		h.audit(c, audit.ActionViewDoctorProfile, "doctor", &userID, nil, http.StatusOK, nil)
		c.JSON(http.StatusOK, doctor)

	case "admin":
		admin, err := h.Repo.GetAdminByID(ctx, userID)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "Admin profile not found"})
			return
		}
		h.audit(c, audit.ActionViewAdminProfile, "admin", &userID, nil, http.StatusOK, nil)
		c.JSON(http.StatusOK, admin)

	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid user role"})
	}
}
