package api

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"github.com/RitwikGupta-0501/vital-watch/internal/models"
	"github.com/RitwikGupta-0501/vital-watch/internal/repository"
	"github.com/RitwikGupta-0501/vital-watch/utils"
)

type Handler struct {
	Repo             *repository.Repository
	S3Client         *s3.Client
	BucketName       string
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

// Patient Portal Handlers
func (h *Handler) GetDoctors(c *gin.Context) {
	doctors, err := h.Repo.GetDoctors(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch doctors"})
		return
	}
	c.JSON(http.StatusOK, doctors)
}

func (h *Handler) GetPatientAppointments(c *gin.Context) {
	userIDVal, ok := c.Get("userID")
	if !ok {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "User ID not found in context"})
		return
	}
	patientID := userIDVal.(uuid.UUID)

	appointments, err := h.Repo.GetAppointmentsByPatientID(c.Request.Context(), patientID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch appointments"})
		return
	}
	c.JSON(http.StatusOK, appointments)
}

func (h *Handler) GetPatientPrescriptions(c *gin.Context) {
	userIDVal, ok := c.Get("userID")
	if !ok {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "User ID not found in context"})
		return
	}
	patientID := userIDVal.(uuid.UUID)

	prescriptions, err := h.Repo.GetPrescriptionsByPatientID(c.Request.Context(), patientID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch prescriptions"})
		return
	}
	c.JSON(http.StatusOK, prescriptions)
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

	if req.StartTime.IsZero() || req.EndTime.IsZero() || !req.EndTime.After(req.StartTime) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "end_time must be strictly after start_time"})
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
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create appointment"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"id": newID})
}

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
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "You are not authorized to download this file"})
		return
	}

	getObjectInput := &s3.GetObjectInput{
		Bucket: aws.String(h.BucketName),
		Key:    aws.String(filename),
	}
	out, err := h.S3Client.GetObject(ctx, getObjectInput)
	if err != nil {
		log.Printf("Failed to get object from S3: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve file"})
		return
	}
	defer out.Body.Close()

	c.Header("Content-Disposition", "attachment; filename="+filename)
	c.Header("Content-Type", *out.ContentType)
	c.Header("Content-Length", strconv.FormatInt(*out.ContentLength, 10))

	io.Copy(c.Writer, out.Body)
}

// Doctor Portal Handlers
func (h *Handler) GetDoctorAppointments(c *gin.Context) {
	userIDVal, ok := c.Get("userID")
	if !ok {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "User ID not found in context"})
		return
	}
	doctorID := userIDVal.(uuid.UUID)

	appointments, err := h.Repo.GetAppointmentsByDoctorID(c.Request.Context(), doctorID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch appointments"})
		return
	}
	c.JSON(http.StatusOK, appointments)
}

func (h *Handler) GetDoctorPatients(c *gin.Context) {
	userIDVal, ok := c.Get("userID")
	if !ok {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "User ID not found in context"})
		return
	}
	doctorID := userIDVal.(uuid.UUID)

	patients, err := h.Repo.GetPatientsByDoctorID(c.Request.Context(), doctorID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch patients"})
		return
	}
	c.JSON(http.StatusOK, patients)
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

	appointments, err := h.Repo.GetAppointmentsForPatient(c.Request.Context(), doctorID, patientID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch appointments"})
		return
	}

	c.JSON(http.StatusOK, appointments)
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

	prescriptions, err := h.Repo.GetPrescriptionsForPatient(c.Request.Context(), doctorID, patientID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch prescriptions"})
		return
	}

	c.JSON(http.StatusOK, prescriptions)
}

func (h *Handler) CreatePrescription(c *gin.Context) {
	userIDVal, ok := c.Get("userID")
	if !ok {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "User ID not found in context"})
		return
	}
	doctorID := userIDVal.(uuid.UUID)

	if err := c.Request.ParseMultipartForm(10 << 20); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to parse form"})
		return
	}

	patientIDStr := c.Request.FormValue("patientID")
	if patientIDStr == "" {
		patientIDStr = c.Request.FormValue("patient_id")
	}
	patientID, err := uuid.Parse(patientIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid patient ID"})
		return
	}

	medication := c.Request.FormValue("medication")
	notes := c.Request.FormValue("notes")

	file, header, err := c.Request.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "File is required"})
		return
	}
	defer file.Close()

	ext := filepath.Ext(header.Filename)
	uniqueFilename := fmt.Sprintf("prescription-%s-%s%s", patientID.String(), uuid.New().String(), ext)

	ctx := c.Request.Context()
	putObjectInput := &s3.PutObjectInput{
		Bucket:        aws.String(h.BucketName),
		Key:           aws.String(uniqueFilename),
		Body:          file,
		ContentLength: aws.Int64(header.Size),
		ContentType:   aws.String(header.Header.Get("Content-Type")),
	}

	_, err = h.S3Client.PutObject(ctx, putObjectInput)
	if err != nil {
		log.Printf("Failed to upload file to S3: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save file"})
		return
	}

	newID, err := h.Repo.CreatePrescription(ctx, patientID, doctorID, medication, notes, uniqueFilename)
	if err != nil {
		log.Printf("Failed to create prescription in DB: %v", err)
		// If DB save fails, roll back S3 upload with timeout
		go func() {
			rbCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()

			deleteObjectInput := &s3.DeleteObjectInput{
				Bucket: aws.String(h.BucketName),
				Key:    aws.String(uniqueFilename),
			}
			_, delErr := h.S3Client.DeleteObject(rbCtx, deleteObjectInput)
			if delErr != nil {
				log.Printf("CRITICAL: Failed to rollback S3 upload: %v", delErr)
			}
		}()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create prescription record"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"id": newID, "filename": uniqueFilename})
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
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to mark appointment as completed"})
		return
	}

	if !updated {
		c.JSON(http.StatusNotFound, gin.H{"error": "Appointment not found or not assigned to you"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Appointment marked as completed"})
}

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
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "You are not authorized to download this file"})
		return
	}

	getObjectInput := &s3.GetObjectInput{
		Bucket: aws.String(h.BucketName),
		Key:    aws.String(filename),
	}
	out, err := h.S3Client.GetObject(ctx, getObjectInput)
	if err != nil {
		log.Printf("Failed to get object from S3: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve file"})
		return
	}
	defer out.Body.Close()

	c.Header("Content-Disposition", "attachment; filename="+filename)
	c.Header("Content-Type", *out.ContentType)
	c.Header("Content-Length", strconv.FormatInt(*out.ContentLength, 10))

	io.Copy(c.Writer, out.Body)
}
