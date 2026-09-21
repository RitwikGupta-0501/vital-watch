package api

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"github.com/RitwikGupta-0501/vital-watch/internal/models"
	"github.com/RitwikGupta-0501/vital-watch/internal/notifications"
	"github.com/RitwikGupta-0501/vital-watch/internal/pdf"
	"github.com/RitwikGupta-0501/vital-watch/internal/repository"
	"github.com/RitwikGupta-0501/vital-watch/internal/safety"
	"github.com/RitwikGupta-0501/vital-watch/internal/storage"
	"github.com/RitwikGupta-0501/vital-watch/utils"
)

type Handler struct {
	Repo             repository.Repository
	Storage          storage.Provider
	JWTSecret        []byte
	DoctorInviteCode string
	OCREnabled       bool
	PDFGenerator     pdf.Generator
	SafetyChecker    safety.Checker
	Notifier         notifications.Broker
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

// Generic Handlers
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
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid role"})
		return
	}

	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid credentials"})
		return
	}

	if !utils.CheckPasswordHash(req.Password, user.GetHashedPassword()) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid credentials"})
		return
	}

	claims := jwt.MapClaims{
		"sub":  user.GetID().String(),
		"role": req.Role,
		"iat":  time.Now().Unix(),
		"exp":  time.Now().Add(time.Hour * 24 * 7).Unix(),
		"iss":  "vital-watch",
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString(h.JWTSecret)
	if err != nil {
		log.Println("Failed to sign token:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate token"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"token": tokenString,
	})
}

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
	if req.Email == "" || !strings.Contains(req.Email, "@") {
		c.JSON(http.StatusBadRequest, gin.H{"error": "A valid email address is required"})
		return
	}

	if len(req.Password) < 8 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Password must be at least 8 characters long"})
		return
	}

	if strings.TrimSpace(req.FirstName) == "" || strings.TrimSpace(req.LastName) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "First name and last name are required"})
		return
	}

	ctx := c.Request.Context()
	var newID uuid.UUID
	var err error
	switch req.Role {
	case "patient":
		hashed, err := utils.HashPassword(req.Password)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to hash password"})
			return
		}
		newID, err = h.Repo.CreatePatient(ctx, req.FirstName, req.LastName, req.Email, hashed)
	case "doctor":
		if h.DoctorInviteCode == "" || req.InviteCode != h.DoctorInviteCode {
			c.JSON(http.StatusForbidden, gin.H{"error": "Doctor registration is restricted or invalid invite code"})
			return
		}
		hashed, err := utils.HashPassword(req.Password)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to hash password"})
			return
		}
		newID, err = h.Repo.CreateDoctor(ctx, req.FirstName, req.LastName, req.Email, hashed, req.Specialty, req.Experience)
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid role"})
		return
	}

	if err != nil {
		if strings.Contains(err.Error(), "duplicate key") || strings.Contains(err.Error(), "users_email_key") {
			c.JSON(http.StatusConflict, gin.H{"error": "An account with this email already exists"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create user"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"id": newID})
}

func (h *Handler) GetUserProfile(c *gin.Context) {
	userIDVal, ok := c.Get("userID")
	if !ok {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "User ID not found in context"})
		return
	}
	userID := userIDVal.(uuid.UUID)

	role, ok := c.Get("role")
	if !ok {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "Role not found in context"})
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
		c.JSON(http.StatusOK, patient)

	case "doctor":
		doctor, err := h.Repo.GetDoctorByID(ctx, userID)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "Doctor profile not found"})
			return
		}
		c.JSON(http.StatusOK, doctor)

	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid user role"})
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

// Patient Portal Handlers
func (h *Handler) GetDoctors(c *gin.Context) {
	limit, offset := parsePagination(c)
	doctors, err := h.Repo.GetDoctors(c.Request.Context(), limit, offset)
	if err != nil {
		log.Printf("Internal error in GetDoctors: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch doctors"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"data":   doctors,
		"limit":  limit,
		"offset": offset,
	})
}

func (h *Handler) GetPatientAppointments(c *gin.Context) {
	userIDVal, ok := c.Get("userID")
	if !ok {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "User ID not found in context"})
		return
	}
	patientID := userIDVal.(uuid.UUID)

	limit, offset := parsePagination(c)
	appointments, err := h.Repo.GetAppointmentsByPatientID(c.Request.Context(), patientID, limit, offset)
	if err != nil {
		log.Printf("Internal error in GetPatientAppointments: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch appointments"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"data":   appointments,
		"limit":  limit,
		"offset": offset,
	})
}

func (h *Handler) GetPatientPrescriptions(c *gin.Context) {
	userIDVal, ok := c.Get("userID")
	if !ok {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "User ID not found in context"})
		return
	}
	patientID := userIDVal.(uuid.UUID)

	limit, offset := parsePagination(c)
	prescriptions, err := h.Repo.GetPrescriptionsByPatientID(c.Request.Context(), patientID, limit, offset)
	if err != nil {
		log.Printf("Internal error in GetPatientPrescriptions: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch prescriptions"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"data":   prescriptions,
		"limit":  limit,
		"offset": offset,
	})
}

func (h *Handler) CreateAppointment(c *gin.Context) {
	var req struct {
		DoctorID  uuid.UUID `json:"doctor_id"`
		StartTime time.Time `json:"start_time"`
		EndTime   time.Time `json:"end_time"`
		Type      string    `json:"type"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	if req.DoctorID == uuid.Nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Doctor ID is required"})
		return
	}

	if req.StartTime.IsZero() || req.EndTime.IsZero() {
		c.JSON(http.StatusBadRequest, gin.H{"error": "start_time and end_time are required"})
		return
	}

	// API-04: start_time must be in the future
	if !req.StartTime.After(time.Now()) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Appointment start time must be in the future"})
		return
	}

	// API-04: end_time must be after start_time
	if !req.EndTime.After(req.StartTime) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Appointment end time must be after start time"})
		return
	}

	// API-04: duration validation (15 min - 4 hours)
	duration := req.EndTime.Sub(req.StartTime)
	if duration < 15*time.Minute {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Appointment duration must be at least 15 minutes"})
		return
	}
	if duration > 4*time.Hour {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Appointment duration cannot exceed 4 hours"})
		return
	}

	// API-04: appointment type validation
	req.Type = strings.ToLower(strings.TrimSpace(req.Type))
	if req.Type == "" {
		req.Type = "in_person"
	}
	if req.Type != "in_person" && req.Type != "virtual" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid appointment type: must be in_person or virtual"})
		return
	}

	userIDVal, ok := c.Get("userID")
	if !ok {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "User ID not found in context"})
		return
	}
	patientID := userIDVal.(uuid.UUID)

	newID, err := h.Repo.CreateAppointment(c.Request.Context(), patientID, req.DoctorID, req.StartTime, req.EndTime, req.Type)
	if err != nil {
		if strings.Contains(err.Error(), "appointments_doctor_id_tstzrange_excl") || strings.Contains(err.Error(), "conflicting key") {
			c.JSON(http.StatusConflict, gin.H{"error": "Doctor is already booked for this time slot"})
			return
		}
		if strings.Contains(err.Error(), "appointments_doctor_id_fkey") {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid doctor ID: specified doctor does not exist"})
			return
		}
		log.Printf("Internal error in CreateAppointment: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create appointment"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"id": newID})
}

func validatePrescriptionItems(inputs []PrescriptionItemInput) ([]models.PrescriptionItem, error) {
	if len(inputs) == 0 {
		return nil, errors.New("at least one medication item is required")
	}
	items := make([]models.PrescriptionItem, 0, len(inputs))
	for idx, it := range inputs {
		medName := strings.TrimSpace(it.MedicationName)
		if medName == "" {
			return nil, fmt.Errorf("medication name is required for item at index %d", idx)
		}
		if len([]rune(medName)) > 255 {
			return nil, fmt.Errorf("medication name at index %d exceeds maximum length of 255 characters", idx)
		}
		dosage := strings.TrimSpace(it.Dosage)
		if len([]rune(dosage)) > 100 {
			return nil, fmt.Errorf("dosage for medication %q exceeds maximum length of 100 characters", medName)
		}
		frequency := strings.TrimSpace(it.Frequency)
		if len([]rune(frequency)) > 100 {
			return nil, fmt.Errorf("frequency for medication %q exceeds maximum length of 100 characters", medName)
		}
		duration := strings.TrimSpace(it.Duration)
		if len([]rune(duration)) > 100 {
			return nil, fmt.Errorf("duration for medication %q exceeds maximum length of 100 characters", medName)
		}
		timing := strings.TrimSpace(it.Timing)
		if len([]rune(timing)) > 100 {
			return nil, fmt.Errorf("timing for medication %q exceeds maximum length of 100 characters", medName)
		}
		items = append(items, models.PrescriptionItem{
			MedicationName: medName,
			Dosage:         dosage,
			Frequency:      frequency,
			Duration:       duration,
			Timing:         timing,
			Instructions:   strings.TrimSpace(it.Instructions),
		})
	}
	return items, nil
}

func clampString(s string, maxLen int) string {
	s = strings.TrimSpace(s)
	runes := []rune(s)
	if len(runes) > maxLen {
		return string(runes[:maxLen])
	}
	return s
}

// DownloadPrescription provides a pre-signed direct download URL to authorized patients and doctors
func (h *Handler) DownloadPrescription(c *gin.Context) {
	identifier := c.Param("filename")
	if identifier == "" {
		identifier = c.Param("id")
	}

	userIDVal, ok := c.Get("userID")
	if !ok {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "User ID not found in context"})
		return
	}
	callerID := userIDVal.(uuid.UUID)
	roleVal, _ := c.Get("role")
	role, _ := roleVal.(string)

	ctx := c.Request.Context()
	targetFilename := identifier

	// If identifier is a valid UUID, look up by prescription ID
	if prescUUID, parseErr := uuid.Parse(identifier); parseErr == nil {
		presc, err := h.Repo.GetPrescriptionByID(ctx, prescUUID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) || errors.Is(err, pgx.ErrNoRows) {
				c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": "Prescription not found or access denied"})
				return
			}
			log.Printf("Database error fetching prescription by ID: %v", err)
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "Internal database error"})
			return
		}

		if role == "patient" {
			if presc.PatientID != callerID || presc.Status != "approved" {
				c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": "Prescription not found or access denied"})
				return
			}
		} else if role == "doctor" {
			if presc.DoctorID != callerID {
				appts, apptErr := h.Repo.GetAppointmentsForPatient(ctx, callerID, presc.PatientID, 1, 0)
				if apptErr != nil || len(appts) == 0 {
					c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": "Prescription not found or access denied"})
					return
				}
			}
		} else {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "Access denied"})
			return
		}

		if presc.FileName == "" {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "This digital prescription has no uploaded file attachment"})
			return
		}
		targetFilename = presc.FileName
	} else {
		if role == "patient" {
			_, err := h.Repo.GetPrescriptionByFilename(ctx, callerID, identifier)
			if err != nil {
				if errors.Is(err, sql.ErrNoRows) || errors.Is(err, pgx.ErrNoRows) {
					c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": "Prescription not found or access denied"})
					return
				}
				log.Printf("Database error fetching prescription: %v", err)
				c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "Internal database error"})
				return
			}
		} else if role == "doctor" {
			_, err := h.Repo.GetPrescriptionByFilenameForDoctor(ctx, callerID, identifier)
			if err != nil {
				if errors.Is(err, sql.ErrNoRows) || errors.Is(err, pgx.ErrNoRows) {
					c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": "Prescription not found or access denied"})
					return
				}
				log.Printf("Database error fetching prescription: %v", err)
				c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "Internal database error"})
				return
			}
		} else {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "Access denied"})
			return
		}
	}

	downloadURL, err := h.Storage.GenerateDownloadURL(ctx, targetFilename, 5*time.Minute)
	if err != nil {
		log.Printf("Failed to generate download URL: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate download URL"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"download_url": downloadURL,
		"expires_in":   300,
	})
}

// Doctor Portal Handlers
func (h *Handler) GetDoctorAppointments(c *gin.Context) {
	userIDVal, ok := c.Get("userID")
	if !ok {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "User ID not found in context"})
		return
	}
	doctorID := userIDVal.(uuid.UUID)

	limit, offset := parsePagination(c)
	appointments, err := h.Repo.GetAppointmentsByDoctorID(c.Request.Context(), doctorID, limit, offset)
	if err != nil {
		log.Printf("Internal error in GetDoctorAppointments: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch appointments"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"data":   appointments,
		"limit":  limit,
		"offset": offset,
	})
}

func (h *Handler) GetDoctorPatients(c *gin.Context) {
	userIDVal, ok := c.Get("userID")
	if !ok {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "User ID not found in context"})
		return
	}
	doctorID := userIDVal.(uuid.UUID)

	limit, offset := parsePagination(c)
	patients, err := h.Repo.GetPatientsByDoctorID(c.Request.Context(), doctorID, limit, offset)
	if err != nil {
		log.Printf("Internal error in GetDoctorPatients: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch patients"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"data":   patients,
		"limit":  limit,
		"offset": offset,
	})
}

func (h *Handler) GetPatientHistoryAppointments(c *gin.Context) {
	userIDVal, ok := c.Get("userID")
	if !ok {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "User ID not found in context"})
		return
	}
	doctorID := userIDVal.(uuid.UUID)

	patientID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid patient ID"})
		return
	}

	limit, offset := parsePagination(c)
	appointments, err := h.Repo.GetAppointmentsForPatient(c.Request.Context(), doctorID, patientID, limit, offset)
	if err != nil {
		log.Printf("Internal error in GetPatientHistoryAppointments: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch appointments"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data":   appointments,
		"limit":  limit,
		"offset": offset,
	})
}

func (h *Handler) GetPatientHistoryPrescriptions(c *gin.Context) {
	userIDVal, ok := c.Get("userID")
	if !ok {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "User ID not found in context"})
		return
	}
	doctorID := userIDVal.(uuid.UUID)

	patientID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid patient ID"})
		return
	}

	status := strings.ToLower(strings.TrimSpace(c.Query("status")))
	if status != "" && status != "pending_ocr" && status != "needs_review" && status != "approved" && status != "rejected" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid status filter. Allowed values: pending_ocr, needs_review, approved, rejected"})
		return
	}

	limit, offset := parsePagination(c)
	prescriptions, err := h.Repo.GetPrescriptionsForPatient(c.Request.Context(), doctorID, patientID, status, limit, offset)
	if err != nil {
		log.Printf("Internal error in GetPatientHistoryPrescriptions: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch prescriptions"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data":   prescriptions,
		"limit":  limit,
		"offset": offset,
	})
}

// GetPrescriptionUploadURL generates a guarded pre-signed upload URL for direct cloud upload
func (h *Handler) GetPrescriptionUploadURL(c *gin.Context) {
	var req struct {
		PatientID   uuid.UUID `json:"patient_id"`
		Filename    string    `json:"filename"`
		ContentType string    `json:"content_type"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	if req.PatientID == uuid.Nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "A valid patient_id is required"})
		return
	}

	if err := storage.ValidatePrescriptionFileType(req.Filename, req.ContentType); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if h.Repo != nil {
		userIDVal, ok := c.Get("userID")
		if !ok {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "Doctor ID not found in context"})
			return
		}
		doctorID, ok := userIDVal.(uuid.UUID)
		if !ok {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "Invalid doctor ID in context"})
			return
		}

		ctx := c.Request.Context()
		patient, err := h.Repo.GetPatientByID(ctx, req.PatientID)
		if err != nil || patient.ID == uuid.Nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "Patient not found"})
			return
		}

		appointments, err := h.Repo.GetAppointmentsForPatient(ctx, doctorID, req.PatientID, 1, 0)
		if err != nil || len(appointments) == 0 {
			c.JSON(http.StatusForbidden, gin.H{"error": "You can only generate upload URLs for patients who have booked appointments with you"})
			return
		}
	}

	ext := strings.ToLower(filepath.Ext(req.Filename))
	uniqueFilename := fmt.Sprintf("prescription-%s-%s%s", req.PatientID.String(), uuid.New().String(), ext)

	ctx := c.Request.Context()
	uploadURL, err := h.Storage.GenerateUploadURL(ctx, uniqueFilename, req.ContentType, 5*time.Minute)
	if err != nil {
		log.Printf("Failed to generate upload URL: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate upload URL"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"upload_url": uploadURL,
		"file_key":   uniqueFilename,
		"expires_in": 300,
	})
}

// PrescriptionItemInput represents client input for prescription medication items
type PrescriptionItemInput struct {
	MedicationName string `json:"medication_name"`
	Dosage         string `json:"dosage"`
	Frequency      string `json:"frequency"`
	Duration       string `json:"duration"`
	Timing         string `json:"timing"`
	Instructions   string `json:"instructions"`
}

// CreatePrescription persists an uploaded prescription record and enqueues an OCR task
func (h *Handler) CreatePrescription(c *gin.Context) {
	userIDVal, ok := c.Get("userID")
	if !ok {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "User ID not found in context"})
		return
	}
	doctorID := userIDVal.(uuid.UUID)

	var req struct {
		PatientID  uuid.UUID `json:"patient_id"`
		Medication string    `json:"medication"` // Optional for uploads; OCR or clinician extracts items
		Notes      string    `json:"notes"`
		FileName   string    `json:"file_name"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	if req.PatientID == uuid.Nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "A valid patient_id is required"})
		return
	}

	req.FileName = strings.TrimSpace(req.FileName)
	if req.FileName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "File name (storage key) is required"})
		return
	}

	if !storage.IsValidPrescriptionKey(req.PatientID, req.FileName) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid or unauthorized file key"})
		return
	}

	ctx := c.Request.Context()

	if h.Repo != nil {
		patient, err := h.Repo.GetPatientByID(ctx, req.PatientID)
		if err != nil || patient.ID == uuid.Nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "Patient not found"})
			return
		}

		appointments, err := h.Repo.GetAppointmentsForPatient(ctx, doctorID, req.PatientID, 1, 0)
		if err != nil || len(appointments) == 0 {
			c.JSON(http.StatusForbidden, gin.H{"error": "You can only prescribe to patients who have booked appointments with you"})
			return
		}
	}

	exists, err := h.Storage.ObjectExists(ctx, req.FileName)
	if err != nil {
		log.Printf("Storage error verifying object existence: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to verify file upload status"})
		return
	}
	if !exists {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Uploaded file not found in storage. Please upload the file first"})
		return
	}

	notes := strings.TrimSpace(req.Notes)
	if req.Medication = strings.TrimSpace(req.Medication); req.Medication != "" {
		if notes == "" {
			notes = req.Medication
		} else {
			notes = notes + " (" + req.Medication + ")"
		}
	}

	newID, status, err := h.Repo.CreateUploadedPrescriptionWithJob(ctx, req.PatientID, doctorID, req.FileName, notes, h.OCREnabled)
	if err != nil {
		log.Printf("Failed to create prescription in DB: %v", err)
		// Clean up uploaded file if DB record insertion fails
		go func() {
			rbCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			if delErr := h.Storage.DeleteFile(rbCtx, req.FileName); delErr != nil {
				log.Printf("CRITICAL: Failed to rollback storage file: %v", delErr)
			}
		}()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create prescription record"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"id":       newID,
		"filename": req.FileName,
		"status":   status,
	})
}

// CreateDigitalPrescriptionRequest represents client input for digital e-prescribing
type CreateDigitalPrescriptionRequest struct {
	PatientID      uuid.UUID               `json:"patient_id"`
	Notes          string                  `json:"notes"`
	Items          []PrescriptionItemInput `json:"items"`
	OverrideSafety bool                    `json:"override_safety"`
	OverrideReason string                  `json:"override_reason"`
}

func (h *Handler) checkSafety(ctx context.Context, patientID uuid.UUID, items []models.PrescriptionItem) (*safety.SafetyReport, error) {
	if h.SafetyChecker == nil || len(items) == 0 {
		return nil, nil
	}
	newMedNames := make([]string, 0, len(items))
	for _, it := range items {
		newMedNames = append(newMedNames, it.MedicationName)
	}
	activeMedNames := make([]string, 0)
	activeRxs, err := h.Repo.GetPrescriptionsByPatientID(ctx, patientID, 50, 0)
	if err == nil {
		for _, rx := range activeRxs {
			if rx.Status != "approved" {
				continue
			}
			for _, it := range rx.Items {
				activeMedNames = append(activeMedNames, it.MedicationName)
			}
		}
	}
	// TODO: Pass patient documented allergies once allergy schema is added to patient records
	return h.SafetyChecker.CheckPrescriptionSafety(ctx, newMedNames, activeMedNames, nil)
}

// CreateDigitalPrescription allows a doctor to author a digital native prescription
func (h *Handler) CreateDigitalPrescription(c *gin.Context) {
	userIDVal, ok := c.Get("userID")
	if !ok {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "Doctor ID not found in context"})
		return
	}
	doctorID := userIDVal.(uuid.UUID)

	var req CreateDigitalPrescriptionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	if req.PatientID == uuid.Nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "A valid patient_id is required"})
		return
	}

	items, err := validatePrescriptionItems(req.Items)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ctx := c.Request.Context()

	// Verify patient exists
	patient, err := h.Repo.GetPatientByID(ctx, req.PatientID)
	if err != nil || patient.ID == uuid.Nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Patient not found"})
		return
	}

	// Verify doctor has an appointment with the patient
	appointments, err := h.Repo.GetAppointmentsForPatient(ctx, doctorID, req.PatientID, 1, 0)
	if err != nil || len(appointments) == 0 {
		c.JSON(http.StatusForbidden, gin.H{"error": "You can only prescribe to patients who have booked appointments with you"})
		return
	}

	// Safety Pre-flight Check (DDI and Allergies)
	safetyReport, safetyErr := h.checkSafety(ctx, req.PatientID, items)
	if safetyErr == nil && safetyReport != nil && safetyReport.HasHighSeverityAlerts && !req.OverrideSafety {
		c.JSON(http.StatusConflict, gin.H{
			"error":             "Critical drug interaction or allergy contraindication detected",
			"safety_report":     safetyReport,
			"requires_override": true,
		})
		return
	}

	if req.OverrideSafety && strings.TrimSpace(req.OverrideReason) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "An override_reason is required when overriding safety alerts"})
		return
	}

	notes := strings.TrimSpace(req.Notes)
	if req.OverrideSafety && strings.TrimSpace(req.OverrideReason) != "" {
		if notes != "" {
			notes = notes + " [Safety Override: " + strings.TrimSpace(req.OverrideReason) + "]"
		} else {
			notes = "[Safety Override: " + strings.TrimSpace(req.OverrideReason) + "]"
		}
	}

	newID, err := h.Repo.CreateDigitalPrescription(ctx, req.PatientID, doctorID, notes, items)
	if err != nil {
		log.Printf("Failed to create digital prescription: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create digital prescription"})
		return
	}

	// Auto-generate standardized prescription PDF
	var linkedFileName string
	if h.PDFGenerator != nil && h.Storage != nil {
		targetFileName := fmt.Sprintf("prescription-%s-%s.pdf", req.PatientID.String(), newID.String())
		doc, docErr := h.Repo.GetDoctorByID(ctx, doctorID)
		if docErr != nil {
			log.Printf("Warning: Could not fetch doctor details for PDF: %v", docErr)
		}
		baseURL := os.Getenv("APP_BASE_URL")
		if baseURL == "" {
			baseURL = "https://vitalwatch.internal"
		}
		baseURL = strings.TrimRight(baseURL, "/")

		pdfBytes, genErr := h.PDFGenerator.GeneratePrescriptionPDF(pdf.PrescriptionData{
			PrescriptionID:  newID,
			Date:            time.Now().UTC(),
			DoctorName:      strings.TrimSpace(doc.FirstName + " " + doc.LastName),
			DoctorSpecialty: doc.Specialty,
			DoctorEmail:     doc.Email,
			PatientName:     strings.TrimSpace(patient.FirstName + " " + patient.LastName),
			PatientEmail:    patient.Email,
			Notes:           notes,
			Items:           items,
			VerificationURL: fmt.Sprintf("%s/verify/rx/%s", baseURL, newID),
		})
		if genErr != nil {
			log.Printf("Warning: Failed to generate digital prescription PDF: %v", genErr)
		} else if saveErr := h.Storage.SaveFile(ctx, targetFileName, pdfBytes, "application/pdf"); saveErr != nil {
			log.Printf("Error: Failed to persist generated PDF to storage: %v", saveErr)
		} else if updateErr := h.Repo.UpdatePrescriptionFileName(ctx, newID, targetFileName); updateErr != nil {
			log.Printf("Error: Failed to link prescription PDF filename in DB: %v", updateErr)
		} else {
			linkedFileName = targetFileName
		}
	}

	// Publish real-time SSE notification
	if h.Notifier != nil {
		eventData := map[string]interface{}{
			"source": "digital",
		}
		if linkedFileName != "" {
			eventData["file_name"] = linkedFileName
		}
		h.Notifier.Publish(notifications.NotificationEvent{
			Type:           notifications.EventPrescriptionApproved,
			PrescriptionID: newID,
			PatientID:      req.PatientID,
			DoctorID:       doctorID,
			Message:        "A new digital prescription has been issued",
			Data:           eventData,
		})
	}

	resp := gin.H{
		"id":     newID,
		"source": "digital",
		"status": "approved",
	}
	if linkedFileName != "" {
		resp["file_name"] = linkedFileName
	}
	if safetyReport != nil {
		resp["safety_report"] = safetyReport
	}

	c.JSON(http.StatusCreated, resp)
}

// GetPendingReviewPrescriptions retrieves prescriptions waiting for doctor HITL verification
func (h *Handler) GetPendingReviewPrescriptions(c *gin.Context) {
	userIDVal, ok := c.Get("userID")
	if !ok {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "Doctor ID not found in context"})
		return
	}
	doctorID := userIDVal.(uuid.UUID)

	limit, offset := parsePagination(c)
	prescriptions, err := h.Repo.GetPrescriptionsPendingReview(c.Request.Context(), doctorID, limit, offset)
	if err != nil {
		log.Printf("Internal error in GetPendingReviewPrescriptions: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch pending prescriptions"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data":   prescriptions,
		"limit":  limit,
		"offset": offset,
	})
}

// VerifyPrescriptionRequest represents doctor approval or rejection of an OCR/uploaded slip
type VerifyPrescriptionRequest struct {
	Status         string                  `json:"status"` // "approved" or "rejected"
	Notes          string                  `json:"notes"`
	Items          []PrescriptionItemInput `json:"items"`
	OverrideSafety bool                    `json:"override_safety"`
	OverrideReason string                  `json:"override_reason"`
}

// VerifyPrescription implements clinician sign-off on uploaded prescriptions
func (h *Handler) VerifyPrescription(c *gin.Context) {
	userIDVal, ok := c.Get("userID")
	if !ok {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "Doctor ID not found in context"})
		return
	}
	doctorID := userIDVal.(uuid.UUID)

	prescriptionID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid prescription ID"})
		return
	}

	var req VerifyPrescriptionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	if req.Status != "approved" && req.Status != "rejected" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Status must be 'approved' or 'rejected'"})
		return
	}

	ctx := c.Request.Context()
	var existing models.Prescription
	var getErr error
	var items []models.PrescriptionItem

	if req.Status == "approved" {
		existing, getErr = h.Repo.GetPrescriptionByID(ctx, prescriptionID)
		if getErr != nil || existing.ID == uuid.Nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "Prescription not found"})
			return
		}

		var itemsToCheck []models.PrescriptionItem

		if req.Items != nil {
			validated, valErr := validatePrescriptionItems(req.Items)
			if valErr != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": valErr.Error()})
				return
			}
			items = validated
			itemsToCheck = validated
		} else {
			if existing.Status == "needs_review" && len(existing.Items) == 0 {
				c.JSON(http.StatusBadRequest, gin.H{"error": "No medications were detected by OCR. You must provide at least one medication item to approve this prescription."})
				return
			}
			itemsToCheck = existing.Items
			items = nil // Retain existing OCR items in repository
		}

		if len(itemsToCheck) > 0 {
			safetyReport, safetyErr := h.checkSafety(ctx, existing.PatientID, itemsToCheck)
			if safetyErr == nil && safetyReport != nil && safetyReport.HasHighSeverityAlerts && !req.OverrideSafety {
				c.JSON(http.StatusConflict, gin.H{
					"error":             "Critical drug interaction or allergy contraindication detected",
					"safety_report":     safetyReport,
					"requires_override": true,
				})
				return
			}
		}
	} else if req.Status == "rejected" {
		// Clean up unverified items from OCR on rejection
		items = []models.PrescriptionItem{}
	}

	if req.OverrideSafety && strings.TrimSpace(req.OverrideReason) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "An override_reason is required when overriding safety alerts"})
		return
	}

	notes := strings.TrimSpace(req.Notes)
	if req.OverrideSafety && strings.TrimSpace(req.OverrideReason) != "" {
		if notes != "" {
			notes = notes + " [Safety Override: " + strings.TrimSpace(req.OverrideReason) + "]"
		} else {
			notes = "[Safety Override: " + strings.TrimSpace(req.OverrideReason) + "]"
		}
	}

	updated, err := h.Repo.VerifyPrescription(ctx, prescriptionID, doctorID, req.Status, notes, items)
	if err != nil {
		log.Printf("Internal error in VerifyPrescription: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to verify prescription"})
		return
	}

	if !updated {
		if getErr != nil || existing.ID == uuid.Nil {
			existing, getErr = h.Repo.GetPrescriptionByID(ctx, prescriptionID)
		}
		if getErr == nil {
			isAuthorized := existing.DoctorID == doctorID
			if !isAuthorized {
				appts, apptErr := h.Repo.GetAppointmentsForPatient(ctx, doctorID, existing.PatientID, 1, 0)
				isAuthorized = apptErr == nil && len(appts) > 0
			}
			if isAuthorized {
				if existing.Status == "pending_ocr" {
					c.JSON(http.StatusConflict, gin.H{"error": "Prescription OCR processing is in progress. Verification is only available once OCR completes."})
					return
				}
				if existing.Status == "approved" || existing.Status == "rejected" {
					c.JSON(http.StatusConflict, gin.H{"error": fmt.Sprintf("Prescription has already been verified (status: %s)", existing.Status)})
					return
				}
			}
		}
		c.JSON(http.StatusNotFound, gin.H{"error": "Prescription not found or access denied"})
		return
	}

	// Reuse existing prescription record or fetch once if not yet loaded (e.g. rejection)
	if existing.ID == uuid.Nil {
		existing, getErr = h.Repo.GetPrescriptionByID(ctx, prescriptionID)
	} else if req.Items != nil {
		existing.Items = items
	}

	if getErr == nil {
		if req.Status == "approved" {
			// Only generate and link standardized PDF if there is no existing uploaded slip file
			if existing.FileName == "" {
				pdfFileName := fmt.Sprintf("prescription-%s-%s.pdf", existing.PatientID.String(), prescriptionID.String())
				if h.PDFGenerator != nil && h.Storage != nil {
					doc, docErr := h.Repo.GetDoctorByID(ctx, doctorID)
					if docErr != nil {
						log.Printf("Warning: Failed to fetch doctor details for PDF: %v", docErr)
					}
					pat, patErr := h.Repo.GetPatientByID(ctx, existing.PatientID)
					if patErr != nil {
						log.Printf("Warning: Failed to fetch patient details for PDF: %v", patErr)
					}

					baseURL := os.Getenv("APP_BASE_URL")
					if baseURL == "" {
						baseURL = "https://vitalwatch.internal"
					}
					baseURL = strings.TrimRight(baseURL, "/")

					pdfBytes, genErr := h.PDFGenerator.GeneratePrescriptionPDF(pdf.PrescriptionData{
						PrescriptionID:  prescriptionID,
						Date:            time.Now().UTC(),
						DoctorName:      strings.TrimSpace(doc.FirstName + " " + doc.LastName),
						DoctorSpecialty: doc.Specialty,
						DoctorEmail:     doc.Email,
						PatientName:     strings.TrimSpace(pat.FirstName + " " + pat.LastName),
						PatientEmail:    pat.Email,
						Notes:           notes,
						Items:           existing.Items,
						VerificationURL: fmt.Sprintf("%s/verify/rx/%s", baseURL, prescriptionID),
					})
					if genErr == nil {
						if saveErr := h.Storage.SaveFile(ctx, pdfFileName, pdfBytes, "application/pdf"); saveErr != nil {
							log.Printf("Error: Failed to persist verified prescription PDF to storage: %v", saveErr)
						} else {
							if updateErr := h.Repo.UpdatePrescriptionFileName(ctx, prescriptionID, pdfFileName); updateErr != nil {
								log.Printf("Error: Failed to link verified prescription PDF filename in DB: %v", updateErr)
							}
						}
					} else {
						log.Printf("Warning: Failed to generate verified prescription PDF: %v", genErr)
					}
				}
			}

			if h.Notifier != nil {
				h.Notifier.Publish(notifications.NotificationEvent{
					Type:           notifications.EventPrescriptionApproved,
					PrescriptionID: prescriptionID,
					PatientID:      existing.PatientID,
					DoctorID:       doctorID,
					Message:        "Prescription verified and approved by clinician",
				})
			}
		} else if req.Status == "rejected" {
			if h.Notifier != nil {
				h.Notifier.Publish(notifications.NotificationEvent{
					Type:           notifications.EventPrescriptionRejected,
					PrescriptionID: prescriptionID,
					PatientID:      existing.PatientID,
					DoctorID:       doctorID,
					Message:        "Prescription was rejected by clinician",
				})
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Prescription verified successfully",
		"status":  req.Status,
	})
}

func (h *Handler) MarkAppointmentAsCompleted(c *gin.Context) {
	userIDVal, ok := c.Get("userID")
	if !ok {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Doctor ID not found in context"})
		return
	}
	doctorID := userIDVal.(uuid.UUID)

	appointmentID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid appointment ID"})
		return
	}

	updated, err := h.Repo.UpdateAppointmentAsCompletedForDoctor(c.Request.Context(), appointmentID, doctorID)
	if err != nil {
		log.Printf("Internal error in MarkAppointmentAsCompleted: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to mark appointment as completed"})
		return
	}

	if !updated {
		c.JSON(http.StatusNotFound, gin.H{"error": "Appointment not found or not assigned to you"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Appointment marked as completed"})
}

// DoctorDownloadPrescription provides a pre-signed direct download URL to the authorized doctor (alias)
func (h *Handler) DoctorDownloadPrescription(c *gin.Context) {
	h.DownloadPrescription(c)
}

// GetPrescriptionByID retrieves a single prescription by ID with ownership access control
func (h *Handler) GetPrescriptionByID(c *gin.Context) {
	userIDVal, ok := c.Get("userID")
	if !ok {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "User ID not found in context"})
		return
	}
	userID := userIDVal.(uuid.UUID)
	roleVal, _ := c.Get("role")
	role, _ := roleVal.(string)

	prescriptionID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid prescription ID"})
		return
	}

	prescription, err := h.Repo.GetPrescriptionByID(c.Request.Context(), prescriptionID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) || errors.Is(err, pgx.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Prescription not found or access denied"})
			return
		}
		log.Printf("Internal error in GetPrescriptionByID: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch prescription"})
		return
	}

	if role == "patient" {
		if prescription.PatientID != userID || prescription.Status != "approved" {
			c.JSON(http.StatusNotFound, gin.H{"error": "Prescription not found or access denied"})
			return
		}
	} else if role == "doctor" {
		if prescription.DoctorID != userID {
			appts, apptErr := h.Repo.GetAppointmentsForPatient(c.Request.Context(), userID, prescription.PatientID, 1, 0)
			if apptErr != nil || len(appts) == 0 {
				c.JSON(http.StatusNotFound, gin.H{"error": "Prescription not found or access denied"})
				return
			}
		}
	} else {
		c.JSON(http.StatusForbidden, gin.H{"error": "Access denied"})
		return
	}

	c.JSON(http.StatusOK, prescription)
}

// StreamNotifications provides a real-time SSE stream for prescription status events
func (h *Handler) StreamNotifications(c *gin.Context) {
	userIDVal, ok := c.Get("userID")
	if !ok {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "User ID not found in context"})
		return
	}
	userID := userIDVal.(uuid.UUID)

	if h.Notifier == nil {
		c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{"error": "Notification service unavailable"})
		return
	}

	notifications.ServeSSE(h.Notifier, c, userID)
}
