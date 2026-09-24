package api

import (
	"github.com/RitwikGupta-0501/vital-watch/internal/audit"

	"github.com/golang-jwt/jwt/v5"
	"database/sql"
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/RitwikGupta-0501/vital-watch/internal/models"
	"github.com/RitwikGupta-0501/vital-watch/internal/repository"
	"github.com/RitwikGupta-0501/vital-watch/utils"
)

func setupAdminTestRouter(h *Handler, jwtSecret []byte) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()

	r.POST("/api/register", h.Register)
	r.POST("/api/login", h.Login)

	authGroup := r.Group("/api")
	authGroup.Use(AuthMiddleware(jwtSecret))
	{
		authGroup.GET("/profile", h.GetUserProfile)

		doctorGroup := authGroup.Group("")
		doctorGroup.Use(RequireRole("doctor"))
		{
			doctorGroup.GET("/doctor/appointments", h.GetDoctorAppointments)
			doctorGroup.POST("/prescriptions/digital", h.CreateDigitalPrescription)
		}

		adminGroup := authGroup.Group("/admin")
		adminGroup.Use(RequireRole("admin"))
		{
			adminGroup.GET("/users", h.ListUsers)
			adminGroup.PATCH("/users/:id/status", h.ToggleUserStatus)
		}
	}

	return r
}

func TestAdminRegister_ValidInviteCode(t *testing.T) {
	jwtSecret := []byte("test-jwt-secret-key-32bytes-long!")
	adminID := uuid.New()

	mockRepo := &repository.MockRepository{
		CreateAdminFunc: func(ctx context.Context, firstName, lastName, email, hashedPassword, department string) (uuid.UUID, error) {
			if firstName != "Sarah" || lastName != "Connor" {
				t.Errorf("unexpected name: %s %s", firstName, lastName)
			}
			if department != "Compliance" {
				t.Errorf("unexpected department: %s", department)
			}
			return adminID, nil
		},
	}

	h := &Handler{
		Repo:            mockRepo,
		JWTSecret:       jwtSecret,
		AdminInviteCode: "valid-admin-secret-invite-1234",
	}

	r := setupAdminTestRouter(h, jwtSecret)

	body := map[string]interface{}{
		"role":        "admin",
		"first_name":  "Sarah",
		"last_name":   "Connor",
		"email":       "sarah.admin@vitalwatch.internal",
		"password":    "SecureAdminPass123!",
		"specialty":   "Compliance",
		"invite_code": "valid-admin-secret-invite-1234",
	}
	bodyBytes, _ := json.Marshal(body)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/api/register", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected status 201 Created, got %d. Body: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["id"] != adminID.String() {
		t.Errorf("expected id %s, got %v", adminID, resp["id"])
	}
}

func TestAdminRegister_InvalidInviteCode(t *testing.T) {
	jwtSecret := []byte("test-jwt-secret-key-32bytes-long!")
	h := &Handler{
		Repo:            &repository.MockRepository{},
		JWTSecret:       jwtSecret,
		AdminInviteCode: "super-secret-admin-code",
	}

	r := setupAdminTestRouter(h, jwtSecret)

	// Wrong invite code
	body := map[string]interface{}{
		"role":        "admin",
		"first_name":  "Intruder",
		"last_name":   "User",
		"email":       "intruder@evil.corp",
		"password":    "Password123!",
		"invite_code": "wrong-code",
	}
	bodyBytes, _ := json.Marshal(body)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/api/register", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected status 403 Forbidden, got %d. Body: %s", w.Code, w.Body.String())
	}
}

func TestAdminLogin_Success(t *testing.T) {
	jwtSecret := []byte("test-jwt-secret-key-32bytes-long!")
	adminID := uuid.New()
	hashedPass, _ := utils.HashPassword("CorrectAdminPass123!")

	mockRepo := &repository.MockRepository{
		GetAdminByEmailFunc: func(ctx context.Context, email string) (models.Admin, error) {
			return models.Admin{
				ID:             adminID,
				Email:          "admin@vitalwatch.internal",
				FirstName:      "Chief",
				LastName:       "Admin",
				Department:     "Security",
				HashedPassword: hashedPass,
				Role:           "admin",
				CreatedAt:      time.Now(),
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
		JWTSecret: jwtSecret,
	}

	r := setupAdminTestRouter(h, jwtSecret)

	body := map[string]interface{}{
		"role":     "admin",
		"email":    "admin@vitalwatch.internal",
		"password": "CorrectAdminPass123!",
	}
	bodyBytes, _ := json.Marshal(body)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/api/login", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200 OK, got %d. Body: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)

	if resp["access_token"] == nil || resp["access_token"] == "" {
		t.Errorf("expected access_token in login response")
	}
	if resp["refresh_token"] == nil || resp["refresh_token"] == "" {
		t.Errorf("expected refresh_token in login response")
	}
}

func TestAdminLogin_WrongCredentials(t *testing.T) {
	jwtSecret := []byte("test-jwt-secret-key-32bytes-long!")
	adminID := uuid.New()
	hashedPass, _ := utils.HashPassword("CorrectAdminPass123!")

	mockRepo := &repository.MockRepository{
		GetAdminByEmailFunc: func(ctx context.Context, email string) (models.Admin, error) {
			return models.Admin{
				ID:             adminID,
				Email:          "admin@vitalwatch.internal",
				HashedPassword: hashedPass,
				Role:           "admin",
			}, nil
		},
	}

	h := &Handler{
		Repo:      mockRepo,
		JWTSecret: jwtSecret,
	}

	r := setupAdminTestRouter(h, jwtSecret)

	// Wrong password
	body := map[string]interface{}{
		"role":     "admin",
		"email":    "admin@vitalwatch.internal",
		"password": "WrongPassword!",
	}
	bodyBytes, _ := json.Marshal(body)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/api/login", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401 Unauthorized, got %d", w.Code)
	}
}

func TestAdminProfile_Success(t *testing.T) {
	jwtSecret := []byte("test-jwt-secret-key-32bytes-long!")
	adminID := uuid.New()

	mockRepo := &repository.MockRepository{
		GetAdminByIDFunc: func(ctx context.Context, id uuid.UUID) (models.Admin, error) {
			return models.Admin{
				ID:         adminID,
				Email:      "audit.lead@vitalwatch.internal",
				FirstName:  "Marcus",
				LastName:   "Vance",
				Department: "Compliance",
				Role:       "admin",
				CreatedAt:  time.Now(),
			}, nil
		},
	}

	h := &Handler{
		Repo:      mockRepo,
		JWTSecret: jwtSecret,
	}

	r := setupAdminTestRouter(h, jwtSecret)

	token := generateTestToken(jwtSecret, adminID, "admin", false)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/api/profile", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200 OK, got %d. Body: %s", w.Code, w.Body.String())
	}

	var resp models.Admin
	_ = json.Unmarshal(w.Body.Bytes(), &resp)

	if resp.Email != "audit.lead@vitalwatch.internal" {
		t.Errorf("expected email audit.lead@vitalwatch.internal, got %s", resp.Email)
	}
	if resp.FirstName != "Marcus" || resp.LastName != "Vance" {
		t.Errorf("expected name Marcus Vance, got %s %s", resp.FirstName, resp.LastName)
	}
	if resp.Department != "Compliance" {
		t.Errorf("expected department Compliance, got %s", resp.Department)
	}
	if resp.Role != "admin" {
		t.Errorf("expected role admin, got %s", resp.Role)
	}
}

func TestAdmin_RBAC_SeparationOfDuties(t *testing.T) {
	jwtSecret := []byte("test-jwt-secret-key-32bytes-long!")
	adminID := uuid.New()
	doctorID := uuid.New()
	patientID := uuid.New()

	h := &Handler{
		Repo:      &repository.MockRepository{},
		JWTSecret: jwtSecret,
	}

	r := setupAdminTestRouter(h, jwtSecret)

	adminToken := generateTestToken(jwtSecret, adminID, "admin", false)
	doctorToken := generateTestToken(jwtSecret, doctorID, "doctor", false)
	patientToken := generateTestToken(jwtSecret, patientID, "patient", false)

	// 1. Admin CANNOT access Doctor-only endpoints
	w1 := httptest.NewRecorder()
	req1, _ := http.NewRequest(http.MethodGet, "/api/doctor/appointments", nil)
	req1.Header.Set("Authorization", "Bearer "+adminToken)
	r.ServeHTTP(w1, req1)
	if w1.Code != http.StatusForbidden {
		t.Errorf("expected 403 Forbidden when admin accesses doctor appointments, got %d", w1.Code)
	}

	// 2. Doctor CANNOT access Admin-only endpoints
	w2 := httptest.NewRecorder()
	req2, _ := http.NewRequest(http.MethodGet, "/api/admin/users", nil)
	req2.Header.Set("Authorization", "Bearer "+doctorToken)
	r.ServeHTTP(w2, req2)
	if w2.Code != http.StatusForbidden {
		t.Errorf("expected 403 Forbidden when doctor accesses admin users, got %d", w2.Code)
	}

	// 3. Patient CANNOT access Admin-only endpoints
	w3 := httptest.NewRecorder()
	req3, _ := http.NewRequest(http.MethodGet, "/api/admin/users", nil)
	req3.Header.Set("Authorization", "Bearer "+patientToken)
	r.ServeHTTP(w3, req3)
	if w3.Code != http.StatusForbidden {
		t.Errorf("expected 403 Forbidden when patient accesses admin users, got %d", w3.Code)
	}
}

func TestAdminListUsers_Success(t *testing.T) {
	jwtSecret := []byte("test-jwt-secret-key-32bytes-long!")
	adminID := uuid.New()

	mockRepo := &repository.MockRepository{
		GetAllUsersFunc: func(ctx context.Context, limit, offset int) ([]models.User, error) {
			return []models.User{
				{ID: uuid.New(), Email: "pat@example.com", Role: "patient", IsActive: true},
				{ID: uuid.New(), Email: "doc@example.com", Role: "doctor", IsActive: true},
				{ID: adminID, Email: "admin@vitalwatch.internal", Role: "admin", IsActive: true},
			}, nil
		},
	}

	h := &Handler{
		Repo:      mockRepo,
		JWTSecret: jwtSecret,
	}

	r := setupAdminTestRouter(h, jwtSecret)
	adminToken := generateTestToken(jwtSecret, adminID, "admin", false)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/api/admin/users?limit=10&offset=0", nil)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200 OK, got %d. Body: %s", w.Code, w.Body.String())
	}

	var res struct {
		Data   []models.User `json:"data"`
		Limit  int           `json:"limit"`
		Offset int           `json:"offset"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &res)

	if len(res.Data) != 3 {
		t.Errorf("expected 3 users, got %d", len(res.Data))
	}
	if res.Limit != 10 {
		t.Errorf("expected limit 10, got %d", res.Limit)
	}
}

func TestAdminToggleUserStatus_Success(t *testing.T) {
	jwtSecret := []byte("test-jwt-secret-key-32bytes-long!")
	adminID := uuid.New()
	targetUserID := uuid.New()
	statusUpdated := false

	mockRepo := &repository.MockRepository{
		UpdateUserActiveStatusFunc: func(ctx context.Context, id uuid.UUID, isActive bool) error {
			if id == targetUserID && !isActive {
				statusUpdated = true
			}
			return nil
		},
	}

	h := &Handler{
		Repo:      mockRepo,
		JWTSecret: jwtSecret,
	}

	r := setupAdminTestRouter(h, jwtSecret)
	adminToken := generateTestToken(jwtSecret, adminID, "admin", false)

	bodyBytes, _ := json.Marshal(map[string]bool{"is_active": false})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPatch, "/api/admin/users/"+targetUserID.String()+"/status", bytes.NewReader(bodyBytes))
	req.Header.Set("Authorization", "Bearer "+adminToken)
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200 OK, got %d. Body: %s", w.Code, w.Body.String())
	}
	if !statusUpdated {
		t.Errorf("expected UpdateUserActiveStatus to be called with is_active = false")
	}
}

func TestRefreshToken_AdminRole(t *testing.T) {
	jwtSecret := []byte("test-jwt-secret-key-32bytes-long!")
	adminID := uuid.New()
	rawRefreshToken := "admin-refresh-token-12345"
	expectedHash := hashToken(rawRefreshToken)

	mockRepo := &repository.MockRepository{
		GetRefreshTokenByHashFunc: func(ctx context.Context, hash string) (models.RefreshToken, error) {
			if hash == expectedHash {
				return models.RefreshToken{
					ID:        uuid.New(),
					UserID:    adminID,
					TokenHash: hash,
					ExpiresAt: time.Now().Add(1 * time.Hour),
				}, nil
			}
			return models.RefreshToken{}, sql.ErrNoRows
		},
		GetAdminByIDFunc: func(ctx context.Context, id uuid.UUID) (models.Admin, error) {
			return models.Admin{
				ID:         adminID,
				Email:      "admin@vitalwatch.org",
				FirstName:  "System",
				LastName:   "Admin",
				Department: "IT",
				Role:       "admin",
			}, nil
		},
		RotateRefreshTokenFunc: func(ctx context.Context, oldTokenID, userID uuid.UUID, newHash string, expiresAt time.Time) (models.RefreshToken, error) {
			return models.RefreshToken{
				ID:        uuid.New(),
				UserID:    userID,
				TokenHash: newHash,
				ExpiresAt: expiresAt,
			}, nil
		},
	}

	h := &Handler{
		Repo:      mockRepo,
		JWTSecret: jwtSecret,
	}

	r := gin.New()
	r.POST("/api/auth/refresh", h.RefreshToken)

	bodyBytes, _ := json.Marshal(map[string]string{"refresh_token": rawRefreshToken})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/api/auth/refresh", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for admin refresh token, got %d. Body: %s", w.Code, w.Body.String())
	}

	var resp struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.AccessToken == "" || resp.RefreshToken == "" {
		t.Fatalf("expected access_token and refresh_token in response")
	}

	token, err := jwt.Parse(resp.AccessToken, func(t *jwt.Token) (interface{}, error) {
		return jwtSecret, nil
	})
	if err != nil || !token.Valid {
		t.Fatalf("failed to parse returned access token: %v", err)
	}
	claims := token.Claims.(jwt.MapClaims)
	if claims["role"] != "admin" {
		t.Errorf("expected role admin in refreshed access token, got %v", claims["role"])
	}
}


func TestToggleUserStatus_SelfDeactivationPrevented(t *testing.T) {
	adminID := uuid.New()
	mockRepo := &repository.MockRepository{}

	h := &Handler{
		Repo: mockRepo,
	}

	r := gin.New()
	r.PATCH("/api/admin/users/:id/status", func(c *gin.Context) {
		c.Set("userID", adminID)
		c.Set("role", "admin")
		h.ToggleUserStatus(c)
	})

	bodyBytes, _ := json.Marshal(map[string]bool{"is_active": false})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPatch, "/api/admin/users/"+adminID.String()+"/status", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request for self-deactivation, got %d. Body: %s", w.Code, w.Body.String())
	}
}

func TestToggleUserStatus_DeactivationRevokesAllSessions(t *testing.T) {
	adminID := uuid.New()
	targetUserID := uuid.New()
	revokedSessions := false

	mockRepo := &repository.MockRepository{
		UpdateUserActiveStatusFunc: func(ctx context.Context, id uuid.UUID, isActive bool) error {
			if id != targetUserID || isActive != false {
				t.Errorf("unexpected status update: %s -> %v", id, isActive)
			}
			return nil
		},
		RevokeAllUserRefreshTokensFunc: func(ctx context.Context, userID uuid.UUID) error {
			if userID == targetUserID {
				revokedSessions = true
			}
			return nil
		},
	}

	h := &Handler{
		Repo: mockRepo,
	}

	r := gin.New()
	r.PATCH("/api/admin/users/:id/status", func(c *gin.Context) {
		c.Set("userID", adminID)
		c.Set("role", "admin")
		h.ToggleUserStatus(c)
	})

	bodyBytes, _ := json.Marshal(map[string]bool{"is_active": false})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPatch, "/api/admin/users/"+targetUserID.String()+"/status", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK on user deactivation, got %d. Body: %s", w.Code, w.Body.String())
	}
	if !revokedSessions {
		t.Fatalf("expected RevokeAllUserRefreshTokens to be called upon deactivating user")
	}
}

func TestListUsers_AuditLogged(t *testing.T) {
	adminID := uuid.New()
	mockAuditor := audit.NewMockAuditor()

	mockRepo := &repository.MockRepository{
		GetAllUsersFunc: func(ctx context.Context, limit, offset int) ([]models.User, error) {
			return []models.User{
				{ID: uuid.New(), Email: "u1@hospital.org", Role: "patient", IsActive: true},
			}, nil
		},
	}

	h := &Handler{
		Repo:    mockRepo,
		Auditor: mockAuditor,
	}

	r := gin.New()
	r.GET("/api/admin/users", func(c *gin.Context) {
		c.Set("userID", adminID)
		c.Set("role", "admin")
		h.ListUsers(c)
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/api/admin/users", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", w.Code)
	}

	entries := mockAuditor.GetEntries()
	found := false
	for _, entry := range entries {
		if entry.Action == audit.ActionViewUserDirectory {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected ActionViewUserDirectory in audit log, got: %+v", entries)
	}
}

func TestGetComplianceAuditLogs_LimitCapped(t *testing.T) {
	adminID := uuid.New()
	capturedLimit := 0

	mockRepo := &repository.MockRepository{
		GetAuditLogsFunc: func(ctx context.Context, limit, offset int) ([]models.PhiAuditLog, error) {
			capturedLimit = limit
			return []models.PhiAuditLog{}, nil
		},
	}

	h := &Handler{
		Repo: mockRepo,
	}

	r := gin.New()
	r.GET("/api/compliance/audit-logs", func(c *gin.Context) {
		c.Set("userID", adminID)
		c.Set("role", "admin")
		h.GetComplianceAuditLogs(c)
	})

	// Query with limit=500 -> must be capped to 100
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/api/compliance/audit-logs?limit=500", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", w.Code)
	}
	if capturedLimit != 100 {
		t.Fatalf("expected limit to be clamped to 100, got %d", capturedLimit)
	}
}
