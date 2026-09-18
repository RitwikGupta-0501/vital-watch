package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func generateTestToken(secret []byte, userID uuid.UUID, role string, expired bool) string {
	exp := time.Now().Add(time.Hour * 1).Unix()
	if expired {
		exp = time.Now().Add(-time.Hour * 1).Unix()
	}

	claims := jwt.MapClaims{
		"sub":  userID.String(),
		"role": role,
		"iat":  time.Now().Unix(),
		"exp":  exp,
		"iss":  "vital-watch",
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, _ := token.SignedString(secret)
	return tokenString
}

func TestAuthMiddleware(t *testing.T) {
	testSecret := []byte("correct-test-secret-key")
	wrongSecret := []byte("wrong-test-secret-key")
	testID := uuid.New()

	r := gin.New()
	r.Use(AuthMiddleware(testSecret))
	r.GET("/test-auth", func(c *gin.Context) {
		userID, _ := c.Get("userID")
		role, _ := c.Get("role")
		c.JSON(http.StatusOK, gin.H{
			"userID": userID,
			"role":   role,
		})
	})

	tests := []struct {
		name           string
		authHeader     string
		expectedStatus int
	}{
		{
			name:           "Missing Authorization header",
			authHeader:     "",
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:           "Malformed Authorization header",
			authHeader:     "Basic 12345",
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:           "Token signed with wrong secret",
			authHeader:     "Bearer " + generateTestToken(wrongSecret, testID, "patient", false),
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:           "Expired token",
			authHeader:     "Bearer " + generateTestToken(testSecret, testID, "patient", true),
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:           "Valid token with UUID",
			authHeader:     "Bearer " + generateTestToken(testSecret, testID, "doctor", false),
			expectedStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			req, _ := http.NewRequest(http.MethodGet, "/test-auth", nil)
			if tt.authHeader != "" {
				req.Header.Set("Authorization", tt.authHeader)
			}
			r.ServeHTTP(w, req)

			if w.Code != tt.expectedStatus {
				t.Fatalf("expected status %d, got %d. Body: %s", tt.expectedStatus, w.Code, w.Body.String())
			}
		})
	}
}

func TestRequireRoleMiddleware(t *testing.T) {
	testSecret := []byte("role-test-secret-key")

	r := gin.New()
	authGroup := r.Group("/api")
	authGroup.Use(AuthMiddleware(testSecret))

	doctorOnly := authGroup.Group("/doctor")
	doctorOnly.Use(RequireRole("doctor"))
	doctorOnly.GET("/dashboard", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "doctor granted"})
	})

	patientOnly := authGroup.Group("/patient")
	patientOnly.Use(RequireRole("patient"))
	patientOnly.GET("/records", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "patient granted"})
	})

	// 1. Doctor token accessing doctor-only endpoint -> Expect 200
	doctorToken := generateTestToken(testSecret, uuid.New(), "doctor", false)
	w1 := httptest.NewRecorder()
	req1, _ := http.NewRequest(http.MethodGet, "/api/doctor/dashboard", nil)
	req1.Header.Set("Authorization", "Bearer "+doctorToken)
	r.ServeHTTP(w1, req1)
	if w1.Code != http.StatusOK {
		t.Fatalf("expected 200 for doctor on doctor route, got %d", w1.Code)
	}

	// 2. Doctor token attempting to access patient-only endpoint -> Expect 403 Forbidden
	w2 := httptest.NewRecorder()
	req2, _ := http.NewRequest(http.MethodGet, "/api/patient/records", nil)
	req2.Header.Set("Authorization", "Bearer "+doctorToken)
	r.ServeHTTP(w2, req2)
	if w2.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for doctor on patient route, got %d", w2.Code)
	}

	// 3. Patient token accessing patient-only endpoint -> Expect 200
	patientToken := generateTestToken(testSecret, uuid.New(), "patient", false)
	w3 := httptest.NewRecorder()
	req3, _ := http.NewRequest(http.MethodGet, "/api/patient/records", nil)
	req3.Header.Set("Authorization", "Bearer "+patientToken)
	r.ServeHTTP(w3, req3)
	if w3.Code != http.StatusOK {
		t.Fatalf("expected 200 for patient on patient route, got %d", w3.Code)
	}

	// 4. Patient token attempting to access doctor-only endpoint -> Expect 403 Forbidden
	w4 := httptest.NewRecorder()
	req4, _ := http.NewRequest(http.MethodGet, "/api/doctor/dashboard", nil)
	req4.Header.Set("Authorization", "Bearer "+patientToken)
	r.ServeHTTP(w4, req4)
	if w4.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for patient on doctor route, got %d", w4.Code)
	}
}

func TestDoctorRegistrationValidation(t *testing.T) {
	h := &Handler{
		DoctorInviteCode: "valid-invite-code",
	}

	r := gin.New()
	r.POST("/register", h.Register)

	// 1. Password shorter than 8 chars -> 400 Bad Request
	w1 := httptest.NewRecorder()
	body1 := strings.NewReader(`{"role":"patient","first_name":"Jane","last_name":"Doe","email":"jane@example.com","password":"short"}`)
	req1, _ := http.NewRequest(http.MethodPost, "/register", body1)
	req1.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w1, req1)
	if w1.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for short password, got %d", w1.Code)
	}

	// 2. Invalid email format -> 400 Bad Request
	w2 := httptest.NewRecorder()
	body2 := strings.NewReader(`{"role":"patient","first_name":"Jane","last_name":"Doe","email":"invalid-email","password":"ValidPassword123!"}`)
	req2, _ := http.NewRequest(http.MethodPost, "/register", body2)
	req2.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w2, req2)
	if w2.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid email, got %d", w2.Code)
	}

	// 3. Doctor registration with wrong invite code -> 403 Forbidden
	w3 := httptest.NewRecorder()
	body3 := strings.NewReader(`{"role":"doctor","first_name":"Dr","last_name":"Smith","email":"dr@example.com","password":"ValidPassword123!","invite_code":"wrong"}`)
	req3, _ := http.NewRequest(http.MethodPost, "/register", body3)
	req3.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w3, req3)
	if w3.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for wrong doctor invite code, got %d", w3.Code)
	}

	// 4. Doctor registration when invite code is empty in config (fail-closed) -> 403 Forbidden
	hUnset := &Handler{
		DoctorInviteCode: "",
	}
	rUnset := gin.New()
	rUnset.POST("/register", hUnset.Register)

	w4 := httptest.NewRecorder()
	body4 := strings.NewReader(`{"role":"doctor","first_name":"Dr","last_name":"Smith","email":"dr@example.com","password":"ValidPassword123!","invite_code":""}`)
	req4, _ := http.NewRequest(http.MethodPost, "/register", body4)
	req4.Header.Set("Content-Type", "application/json")
	rUnset.ServeHTTP(w4, req4)
	if w4.Code != http.StatusForbidden {
		t.Fatalf("expected 403 fail-closed when DoctorInviteCode is empty in config, got %d", w4.Code)
	}
}
