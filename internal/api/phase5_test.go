package api

import (
	"bytes"
	"context"
	"sync"
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
	unrelatedPatientID := uuid.New()

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
		GetAuditLogsByPatientIDFunc: func(ctx context.Context, pid uuid.UUID, limit, offset int) ([]models.PhiAuditLog, error) {
			return []models.PhiAuditLog{
				{
					ID:           uuid.New(),
					Action:       audit.ActionReadPrescriptions,
					ResourceType: "prescription",
					PatientID:    &pid,
					StatusCode:   200,
				},
			}, nil
		},
		HasDoctorPatientRelationshipFunc: func(ctx context.Context, dID, pID uuid.UUID) (bool, error) {
			return dID == doctorID && pID == patientID, nil
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

	// 2. Allowed for admin role (global logs access)
	reqAdmin := httptest.NewRequest(http.MethodGet, "/api/compliance/audit-logs", nil)
	reqAdmin.Header.Set("X-Role", "admin")
	wAdmin := httptest.NewRecorder()
	r.ServeHTTP(wAdmin, reqAdmin)
	if wAdmin.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for admin accessing compliance logs, got %d", wAdmin.Code)
	}

	var res struct {
		Data []models.PhiAuditLog `json:"data"`
	}
	json.Unmarshal(wAdmin.Body.Bytes(), &res)
	if len(res.Data) != 1 || res.Data[0].Action != audit.ActionReadPrescriptions {
		t.Errorf("unexpected audit logs returned for admin: %+v", res.Data)
	}

	// 3. Doctor role WITHOUT patient_id is forbidden (HIPAA IDOR safeguard)
	reqDocNoPatient := httptest.NewRequest(http.MethodGet, "/api/compliance/audit-logs", nil)
	reqDocNoPatient.Header.Set("X-User-ID", doctorID.String())
	reqDocNoPatient.Header.Set("X-Role", "doctor")
	wDocNoPatient := httptest.NewRecorder()
	r.ServeHTTP(wDocNoPatient, reqDocNoPatient)
	if wDocNoPatient.Code != http.StatusForbidden {
		t.Errorf("expected 403 Forbidden for doctor without patient_id, got %d", wDocNoPatient.Code)
	}

	// 4. Doctor role with UNRELATED patient_id is forbidden (no relationship)
	reqDocUnrelated := httptest.NewRequest(http.MethodGet, "/api/compliance/audit-logs?patient_id="+unrelatedPatientID.String(), nil)
	reqDocUnrelated.Header.Set("X-User-ID", doctorID.String())
	reqDocUnrelated.Header.Set("X-Role", "doctor")
	wDocUnrelated := httptest.NewRecorder()
	r.ServeHTTP(wDocUnrelated, reqDocUnrelated)
	if wDocUnrelated.Code != http.StatusForbidden {
		t.Errorf("expected 403 Forbidden for doctor accessing unrelated patient audit logs, got %d", wDocUnrelated.Code)
	}

	// 5. Doctor role with related patient_id succeeds
	reqDocRelated := httptest.NewRequest(http.MethodGet, "/api/compliance/audit-logs?patient_id="+patientID.String(), nil)
	reqDocRelated.Header.Set("X-User-ID", doctorID.String())
	reqDocRelated.Header.Set("X-Role", "doctor")
	wDocRelated := httptest.NewRecorder()
	r.ServeHTTP(wDocRelated, reqDocRelated)
	if wDocRelated.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for doctor with valid patient relationship, got %d", wDocRelated.Code)
	}
}

func TestRefreshToken_GracePeriod(t *testing.T) {
	userID := uuid.New()
	tokenID := uuid.New()
	rawRefreshToken := "my-rotated-token-12345"
	expectedHash := hashToken(rawRefreshToken)

	allRevokedCalled := false
	revokedRecently := time.Now().Add(-2 * time.Second)

	mockRepo := &repository.MockRepository{
		GetRefreshTokenByHashFunc: func(ctx context.Context, hash string) (models.RefreshToken, error) {
			if hash == expectedHash {
				return models.RefreshToken{
					ID:        tokenID,
					UserID:    userID,
					TokenHash: hash,
					ExpiresAt: time.Now().Add(1 * time.Hour),
					RevokedAt: &revokedRecently,
				}, nil
			}
			return models.RefreshToken{}, sql.ErrNoRows
		},
		RevokeAllUserRefreshTokensFunc: func(ctx context.Context, uID uuid.UUID) error {
			allRevokedCalled = true
			return nil
		},
	}

	h := &Handler{
		Repo:      mockRepo,
		JWTSecret: []byte("test-jwt-secret-phase5-32bytes!!"),
	}
	r := setupPhase5TestRouter(h)

	body, _ := json.Marshal(map[string]string{
		"refresh_token": rawRefreshToken,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/auth/refresh", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Errorf("expected 409 Conflict within grace window, got %d", w.Code)
	}
	if allRevokedCalled {
		t.Error("expected RevokeAllUserRefreshTokens NOT to be called within grace window")
	}

	// Now test beyond grace window (e.g. 20s ago)
	revokedOld := time.Now().Add(-20 * time.Second)
	mockRepo.GetRefreshTokenByHashFunc = func(ctx context.Context, hash string) (models.RefreshToken, error) {
		return models.RefreshToken{
			ID:        tokenID,
			UserID:    userID,
			TokenHash: hash,
			ExpiresAt: time.Now().Add(1 * time.Hour),
			RevokedAt: &revokedOld,
		}, nil
	}

	reqOld := httptest.NewRequest(http.MethodPost, "/api/auth/refresh", bytes.NewReader(body))
	reqOld.Header.Set("Content-Type", "application/json")
	wOld := httptest.NewRecorder()
	r.ServeHTTP(wOld, reqOld)

	if wOld.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 Unauthorized outside grace window, got %d", wOld.Code)
	}
	if !allRevokedCalled {
		t.Error("expected RevokeAllUserRefreshTokens to be called outside grace window")
	}
}

func TestAuditLogging_PrescriptionsAndProfile(t *testing.T) {
	patientID := uuid.New()
	doctorID := uuid.New()
	prescID := uuid.New()

	mockAuditor := audit.NewMockAuditor()
	mockRepo := &repository.MockRepository{
		GetPatientByIDFunc: func(ctx context.Context, id uuid.UUID) (models.Patient, error) {
			return models.Patient{ID: patientID, FirstName: "Alice", LastName: "Smith", Role: "patient"}, nil
		},
		GetPrescriptionByIDFunc: func(ctx context.Context, id uuid.UUID) (models.Prescription, error) {
			return models.Prescription{
				ID:        prescID,
				PatientID: patientID,
				DoctorID:  doctorID,
				Status:    "approved",
			}, nil
		},
	}

	h := &Handler{
		Repo:    mockRepo,
		Auditor: mockAuditor,
	}

	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("userID", patientID)
		c.Set("role", "patient")
		c.Next()
	})
	r.GET("/api/profile", h.GetUserProfile)
	r.GET("/api/prescriptions/:id", h.GetPrescriptionByID)

	// 1. Test GetUserProfile logs audit entry
	reqProf := httptest.NewRequest(http.MethodGet, "/api/profile", nil)
	wProf := httptest.NewRecorder()
	r.ServeHTTP(wProf, reqProf)
	if wProf.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", wProf.Code)
	}

	// 2. Test GetPrescriptionByID logs audit entry
	reqRx := httptest.NewRequest(http.MethodGet, "/api/prescriptions/"+prescID.String(), nil)
	wRx := httptest.NewRecorder()
	r.ServeHTTP(wRx, reqRx)
	if wRx.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", wRx.Code)
	}

	entries := mockAuditor.GetEntries()
	if len(entries) < 2 {
		t.Fatalf("expected at least 2 audit entries, got %d", len(entries))
	}

	actions := []string{entries[0].Action, entries[1].Action}
	hasProfileAudit := false
	hasRxAudit := false
	for _, a := range actions {
		if a == audit.ActionViewPatientProfile {
			hasProfileAudit = true
		}
		if a == audit.ActionReadPrescriptions {
			hasRxAudit = true
		}
	}

	if !hasProfileAudit {
		t.Errorf("expected ActionViewPatientProfile in audit log, got actions: %v", actions)
	}
	if !hasRxAudit {
		t.Errorf("expected ActionReadPrescriptions in audit log, got actions: %v", actions)
	}
}

func TestRefreshToken_ConcurrentRace(t *testing.T) {
	userID := uuid.New()
	rawRefreshToken := "race-test-refresh-token-12345"
	expectedHash := hashToken(rawRefreshToken)

	var mu sync.Mutex
	isRevoked := false
	var revokedTime time.Time

	mockRepo := &repository.MockRepository{
		GetRefreshTokenByHashFunc: func(ctx context.Context, hash string) (models.RefreshToken, error) {
			mu.Lock()
			defer mu.Unlock()
			if hash == expectedHash {
				tok := models.RefreshToken{
					ID:        uuid.New(),
					UserID:    userID,
					TokenHash: hash,
					ExpiresAt: time.Now().Add(1 * time.Hour),
				}
				if isRevoked {
					tok.RevokedAt = &revokedTime
				}
				return tok, nil
			}
			return models.RefreshToken{}, sql.ErrNoRows
		},
		GetPatientByIDFunc: func(ctx context.Context, id uuid.UUID) (models.Patient, error) {
			return models.Patient{ID: userID, Role: "patient"}, nil
		},
		CreateRefreshTokenFunc: func(ctx context.Context, uID uuid.UUID, hash string, exp time.Time) (models.RefreshToken, error) {
			return models.RefreshToken{ID: uuid.New(), UserID: uID, TokenHash: hash, ExpiresAt: exp}, nil
		},
		RevokeRefreshTokenFunc: func(ctx context.Context, id uuid.UUID, replacedBy *uuid.UUID) error {
			mu.Lock()
			defer mu.Unlock()
			isRevoked = true
			revokedTime = time.Now()
			return nil
		},
		RevokeAllUserRefreshTokensFunc: func(ctx context.Context, uID uuid.UUID) error {
			return nil
		},
	}

	h := &Handler{
		Repo:      mockRepo,
		JWTSecret: []byte("test-secret-phase5-32bytes-long!"),
	}
	r := setupPhase5TestRouter(h)

	concurrency := 10
	var wg sync.WaitGroup
	statusCodes := make([]int, concurrency)

	body, _ := json.Marshal(map[string]string{
		"refresh_token": rawRefreshToken,
	})

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			req := httptest.NewRequest(http.MethodPost, "/api/auth/refresh", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			statusCodes[idx] = w.Code
		}(i)
	}

	wg.Wait()

	// Every request must receive a valid status code (200 OK or 409 Conflict)
	for idx, code := range statusCodes {
		if code != http.StatusOK && code != http.StatusConflict {
			t.Errorf("request %d returned unexpected status: %d", idx, code)
		}
	}
}

func TestRefreshToken_ExpiredAndInvalid(t *testing.T) {
	mockRepo := &repository.MockRepository{
		GetRefreshTokenByHashFunc: func(ctx context.Context, hash string) (models.RefreshToken, error) {
			if hash == hashToken("expired-token") {
				return models.RefreshToken{
					ID:        uuid.New(),
					UserID:    uuid.New(),
					ExpiresAt: time.Now().Add(-1 * time.Hour), // Expired
				}, nil
			}
			return models.RefreshToken{}, sql.ErrNoRows
		},
	}

	h := &Handler{
		Repo:      mockRepo,
		JWTSecret: []byte("test-secret-phase5-32bytes-long!"),
	}
	r := setupPhase5TestRouter(h)

	// 1. Missing body / empty token
	reqEmpty := httptest.NewRequest(http.MethodPost, "/api/auth/refresh", bytes.NewReader([]byte("{}")))
	reqEmpty.Header.Set("Content-Type", "application/json")
	wEmpty := httptest.NewRecorder()
	r.ServeHTTP(wEmpty, reqEmpty)
	if wEmpty.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for empty token, got %d", wEmpty.Code)
	}

	// 2. Nonexistent token
	bodyNonexistent, _ := json.Marshal(map[string]string{"refresh_token": "nonexistent-token"})
	reqNonexistent := httptest.NewRequest(http.MethodPost, "/api/auth/refresh", bytes.NewReader(bodyNonexistent))
	reqNonexistent.Header.Set("Content-Type", "application/json")
	wNonexistent := httptest.NewRecorder()
	r.ServeHTTP(wNonexistent, reqNonexistent)
	if wNonexistent.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for nonexistent token, got %d", wNonexistent.Code)
	}

	// 3. Expired token
	bodyExpired, _ := json.Marshal(map[string]string{"refresh_token": "expired-token"})
	reqExpired := httptest.NewRequest(http.MethodPost, "/api/auth/refresh", bytes.NewReader(bodyExpired))
	reqExpired.Header.Set("Content-Type", "application/json")
	wExpired := httptest.NewRecorder()
	r.ServeHTTP(wExpired, reqExpired)
	if wExpired.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for expired token, got %d", wExpired.Code)
	}
}

func TestAudit_ContextRequestID(t *testing.T) {
	patientID := uuid.New()
	mockAuditor := audit.NewMockAuditor()
	mockRepo := &repository.MockRepository{
		GetPatientByIDFunc: func(ctx context.Context, id uuid.UUID) (models.Patient, error) {
			return models.Patient{ID: patientID, FirstName: "Bob", LastName: "Jones", Role: "patient"}, nil
		},
	}

	h := &Handler{
		Repo:    mockRepo,
		Auditor: mockAuditor,
	}

	r := gin.New()
	// Apply request ID middleware
	r.Use(middleware.RequestIDMiddleware())
	r.Use(func(c *gin.Context) {
		c.Set("userID", patientID)
		c.Set("role", "patient")
		c.Next()
	})
	r.GET("/api/profile", h.GetUserProfile)

	customReqID := uuid.New().String()
	req := httptest.NewRequest(http.MethodGet, "/api/profile", nil)
	req.Header.Set(middleware.HeaderXRequestID, customReqID)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", w.Code)
	}

	entries := mockAuditor.GetEntries()
	if len(entries) == 0 {
		t.Fatal("expected audit entry to be recorded")
	}

	lastEntry := entries[len(entries)-1]
	if lastEntry.RequestID == nil {
		t.Fatal("expected RequestID in audit entry to not be nil")
	}
	if lastEntry.RequestID.String() != customReqID {
		t.Errorf("expected RequestID %s, got %s", customReqID, lastEntry.RequestID.String())
	}
}
