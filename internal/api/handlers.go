package api

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"github.com/RitwikGupta-0501/vital-watch/internal/models"
	"github.com/RitwikGupta-0501/vital-watch/internal/repository"
	"github.com/RitwikGupta-0501/vital-watch/internal/storage"
	"github.com/RitwikGupta-0501/vital-watch/utils"
)

type Handler struct {
	Repo             repository.Repository
	Storage          storage.Provider
	JWTSecret        []byte
	DoctorInviteCode string
}

func (h *Handler) Ping(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"message": "pong from the api layer!"})
}

func AuthMiddleware(jwtSecret []byte) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Authorization header missing"})
			return
		}

		parts := strings.Split(authHeader, " ")
		if len(parts) != 2 || parts[0] != "Bearer" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Invalid token format"})
			return
		}
		tokenString := parts[1]

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

// DownloadPrescription provides a pre-signed direct download URL to the authorized patient
func (h *Handler) DownloadPrescription(c *gin.Context) {
	filename := c.Param("filename")

	userIDVal, ok := c.Get("userID")
	if !ok {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "User ID not found in context"})
		return
	}
	patientID := userIDVal.(uuid.UUID)

	ctx := c.Request.Context()
	_, err := h.Repo.GetPrescriptionByFilename(ctx, patientID, filename)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": "Prescription not found or access denied"})
			return
		}
		log.Printf("Database error fetching prescription: %v", err)
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "Internal database error"})
		return
	}

	downloadURL, err := h.Storage.GenerateDownloadURL(ctx, filename, 5*time.Minute)
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

	limit, offset := parsePagination(c)
	prescriptions, err := h.Repo.GetPrescriptionsForPatient(c.Request.Context(), doctorID, patientID, limit, offset)
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

// CreatePrescription persists the prescription record once the file has been uploaded
func (h *Handler) CreatePrescription(c *gin.Context) {
	userIDVal, ok := c.Get("userID")
	if !ok {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "User ID not found in context"})
		return
	}
	doctorID := userIDVal.(uuid.UUID)

	var req struct {
		PatientID  uuid.UUID `json:"patient_id"`
		Medication string    `json:"medication"`
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

	req.Medication = strings.TrimSpace(req.Medication)
	if req.Medication == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Medication is required"})
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
	newID, err := h.Repo.CreatePrescription(ctx, req.PatientID, doctorID, req.Medication, req.Notes, req.FileName)
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

	c.JSON(http.StatusCreated, gin.H{"id": newID, "filename": req.FileName})
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

// DoctorDownloadPrescription provides a pre-signed direct download URL to the authorized doctor
func (h *Handler) DoctorDownloadPrescription(c *gin.Context) {
	filename := c.Param("filename")

	userIDVal, ok := c.Get("userID")
	if !ok {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "User ID not found in context"})
		return
	}
	doctorID := userIDVal.(uuid.UUID)

	ctx := c.Request.Context()
	_, err := h.Repo.GetPrescriptionByFilenameForDoctor(ctx, doctorID, filename)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": "Prescription not found or access denied"})
			return
		}
		log.Printf("Database error fetching prescription: %v", err)
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "Internal database error"})
		return
	}

	downloadURL, err := h.Storage.GenerateDownloadURL(ctx, filename, 5*time.Minute)
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
