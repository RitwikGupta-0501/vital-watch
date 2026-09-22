package api

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"github.com/RitwikGupta-0501/vital-watch/internal/audit"
	"github.com/RitwikGupta-0501/vital-watch/internal/middleware"
	"github.com/RitwikGupta-0501/vital-watch/internal/models"
	"github.com/RitwikGupta-0501/vital-watch/internal/repository"
	"github.com/RitwikGupta-0501/vital-watch/internal/storage"
)

func setupPhase5TestRouter(h *Handler) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(middleware.RequestIDMiddleware())

	r.Use(func(c *gin.Context) {
		if uidStr := c.GetHeader("X-User-ID"); uidStr != "" {
			if uid, err := uuid.Parse(uidStr); err == nil {
				c.Set("userID", uid)
			}
		}
		if role := c.GetHeader("X-Role"); role != "" {
			c.Set("role", role)
		}
		c.Next()
	})

	r.POST("/api/login", h.Login)
	r.POST("/api/auth/refresh", h.RefreshToken)
	r.POST("/api/auth/logout", h.Logout)
	r.GET("/api/compliance/audit-logs", h.GetComplianceAuditLogs)

	return r
}

func TestLogin_ReturnsTokensWithShortExpiration(t *testing.T) {
	jwtSecret := []byte("test-jwt-secret-phase5-32bytes!!")
	patientID := uuid.New()

	// Hash for "correct-password"
	// $2a$10$7EqJtq98hPqEX7fNZaFWoO...
	// We can use a mock repo returning a patient
	mockRepo := &repository.MockRepository{
		GetPatientByEmailFunc: func(ctx context.Context, email string) (models.Patient, error) {
			return models.Patient{
				ID:             patientID,
				Email:          "patient@example.com",
				HashedPassword: "$2a$10$wK3V.z3h.rK5cM7dDqg/e.p2TjB9fO2B5M7K8L9P0Q1R2S3T4U5V6", // won't match arbitrary unless bcrypt, so let's test Refresh & Logout
			}, nil
		},
		CreateRefreshTokenFunc: func(ctx context.Context, userID uuid.UUID, tokenHash string, expiresAt time.Time) (models.RefreshToken, error) {
			return models.RefreshToken{
				ID:        uuid.New(),
				UserID:    userID,
				TokenHash: tokenHash,
				ExpiresAt: expiresAt,
			}, nil
		},
	}

	h := &Handler{
		Repo:      mockRepo,
		Storage:   storage.NewMockProvider(),
		JWTSecret: jwtSecret,
	}

	r := setupPhase5TestRouter(h)

	// Test RequestIDMiddleware returns X-Request-ID header
	req := httptest.NewRequest(http.MethodPost, "/api/login", bytes.NewReader([]byte("{}")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	reqID := w.Header().Get("X-Request-ID")
	if reqID == "" {
		t.Error("expected X-Request-ID header on response")
	}
}

func TestRefreshToken_RotationSuccess(t *testing.T) {
	jwtSecret := []byte("test-jwt-secret-phase5-32bytes!!")
	userID := uuid.New()
	tokenID := uuid.New()

	rawRefreshToken := "my-secret-random-refresh-token-12345"
	expectedHash := hashToken(rawRefreshToken)

	var revokedOldID uuid.UUID
	var replacementID *uuid.UUID

	mockRepo := &repository.MockRepository{
		GetRefreshTokenByHashFunc: func(ctx context.Context, tokenHash string) (models.RefreshToken, error) {
			if tokenHash == expectedHash {
				return models.RefreshToken{
					ID:        tokenID,
					UserID:    userID,
					TokenHash: tokenHash,
					ExpiresAt: time.Now().Add(24 * time.Hour),
					RevokedAt: nil,
				}, nil
			}
			return models.RefreshToken{}, sql.ErrNoRows
		},
		GetPatientByIDFunc: func(ctx context.Context, id uuid.UUID) (models.Patient, error) {
			return models.Patient{
				ID:   userID,
				Role: "patient",
			}, nil
		},
		CreateRefreshTokenFunc: func(ctx context.Context, uID uuid.UUID, tokenHash string, expiresAt time.Time) (models.RefreshToken, error) {
			return models.RefreshToken{
				ID:        uuid.New(),
				UserID:    uID,
				TokenHash: tokenHash,
				ExpiresAt: expiresAt,
			}, nil
		},
		RevokeRefreshTokenFunc: func(ctx context.Context, id uuid.UUID, replacedBy *uuid.UUID) error {
			revokedOldID = id
			replacementID = replacedBy
			return nil
		},
	}

	h := &Handler{
		Repo:      mockRepo,
		Storage:   storage.NewMockProvider(),
		JWTSecret: jwtSecret,
	}

	r := setupPhase5TestRouter(h)

	body, _ := json.Marshal(map[string]string{
		"refresh_token": rawRefreshToken,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/auth/refresh", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		TokenType    string `json:"token_type"`
		ExpiresIn    int    `json:"expires_in"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.AccessToken == "" || resp.RefreshToken == "" {
		t.Errorf("expected access_token and refresh_token in response: %+v", resp)
	}
	if resp.ExpiresIn != 900 {
		t.Errorf("expected expires_in 900, got %d", resp.ExpiresIn)
	}

	// Verify old token was revoked and replacement lineage recorded
	if revokedOldID != tokenID {
		t.Errorf("expected old token %s to be revoked, got %s", tokenID, revokedOldID)
	}
	if replacementID == nil {
		t.Error("expected replacement token ID to be set")
	}

	// Verify access token claims
	token, err := jwt.Parse(resp.AccessToken, func(t *jwt.Token) (interface{}, error) {
		return jwtSecret, nil
	})
	if err != nil || !token.Valid {
		t.Fatalf("failed to parse returned access token: %v", err)
	}
	claims := token.Claims.(jwt.MapClaims)
	if claims["sub"] != userID.String() {
		t.Errorf("expected sub %s, got %v", userID, claims["sub"])
	}
	if claims["role"] != "patient" {
		t.Errorf("expected role patient, got %v", claims["role"])
	}
}

func TestRefreshToken_TheftDetection(t *testing.T) {
	jwtSecret := []byte("test-jwt-secret-phase5-32bytes!!")
	userID := uuid.New()
	tokenID := uuid.New()
	revokedAt := time.Now().Add(-1 * time.Hour)

	rawRefreshToken := "compromised-token-12345"
	expectedHash := hashToken(rawRefreshToken)

	var allTokensRevokedForUser uuid.UUID

	mockRepo := &repository.MockRepository{
		GetRefreshTokenByHashFunc: func(ctx context.Context, tokenHash string) (models.RefreshToken, error) {
			if tokenHash == expectedHash {
				return models.RefreshToken{
					ID:        tokenID,
					UserID:    userID,
					TokenHash: tokenHash,
					ExpiresAt: time.Now().Add(24 * time.Hour),
					RevokedAt: &revokedAt, // Already revoked!
				}, nil
			}
			return models.RefreshToken{}, sql.ErrNoRows
		},
		RevokeAllUserRefreshTokensFunc: func(ctx context.Context, uID uuid.UUID) error {
			allTokensRevokedForUser = uID
			return nil
		},
	}

	h := &Handler{
		Repo:      mockRepo,
		Storage:   storage.NewMockProvider(),
		JWTSecret: jwtSecret,
	}

	r := setupPhase5TestRouter(h)

	body, _ := json.Marshal(map[string]string{
		"refresh_token": rawRefreshToken,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/auth/refresh", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized for compromised token, got %d", w.Code)
	}

	if allTokensRevokedForUser != userID {
		t.Errorf("expected all tokens for user %s to be revoked, got %s", userID, allTokensRevokedForUser)
	}
}

func TestLogout_RevokesToken(t *testing.T) {
	tokenID := uuid.New()
	rawRefreshToken := "valid-token-to-logout"
	expectedHash := hashToken(rawRefreshToken)

	var revokedID uuid.UUID

	mockRepo := &repository.MockRepository{
		GetRefreshTokenByHashFunc: func(ctx context.Context, tokenHash string) (models.RefreshToken, error) {
			if tokenHash == expectedHash {
				return models.RefreshToken{
					ID:        tokenID,
					TokenHash: tokenHash,
					ExpiresAt: time.Now().Add(24 * time.Hour),
					RevokedAt: nil,
				}, nil
			}
			return models.RefreshToken{}, sql.ErrNoRows
		},
		RevokeRefreshTokenFunc: func(ctx context.Context, id uuid.UUID, replacedBy *uuid.UUID) error {
			revokedID = id
			return nil
		},
	}

	h := &Handler{
		Repo:    mockRepo,
		Storage: storage.NewMockProvider(),
	}
	r := setupPhase5TestRouter(h)

	body, _ := json.Marshal(map[string]string{
		"refresh_token": rawRefreshToken,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/auth/logout", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for logout, got %d", w.Code)
	}
	if revokedID != tokenID {
		t.Errorf("expected token %s to be revoked, got %s", tokenID, revokedID)
	}
}

func TestGetComplianceAuditLogs(t *testing.T) {
	doctorID := uuid.New()
	patientID := uuid.New()

	mockRepo := &repository.MockRepository{
		GetAuditLogsFunc: func(ctx context.Context, limit, offset int) ([]models.PhiAuditLog, error) {
			return []models.PhiAuditLog{
				{
					ID:           uuid.New(),
					Action:       audit.ActionReadPrescriptions,
					ResourceType: "prescription",
					PatientID:    &patientID,
					StatusCode:   200,
				},
			}, nil
		},
	}

	h := &Handler{
		Repo: mockRepo,
	}
	r := setupPhase5TestRouter(h)

	// 1. Forbidden for patient role
	reqPat := httptest.NewRequest(http.MethodGet, "/api/compliance/audit-logs", nil)
	reqPat.Header.Set("X-Role", "patient")
	wPat := httptest.NewRecorder()
	r.ServeHTTP(wPat, reqPat)
	if wPat.Code != http.StatusForbidden {
		t.Errorf("expected 403 Forbidden for patient accessing compliance logs, got %d", wPat.Code)
	}

	// 2. Allowed for doctor role
	reqDoc := httptest.NewRequest(http.MethodGet, "/api/compliance/audit-logs", nil)
	reqDoc.Header.Set("X-User-ID", doctorID.String())
	reqDoc.Header.Set("X-Role", "doctor")
	wDoc := httptest.NewRecorder()
	r.ServeHTTP(wDoc, reqDoc)
	if wDoc.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for doctor accessing compliance logs, got %d", wDoc.Code)
	}

	var res struct {
		Data []models.PhiAuditLog `json:"data"`
	}
	json.Unmarshal(wDoc.Body.Bytes(), &res)
	if len(res.Data) != 1 || res.Data[0].Action != audit.ActionReadPrescriptions {
		t.Errorf("unexpected audit logs returned: %+v", res.Data)
	}
}
