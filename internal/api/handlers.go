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
			userIDFloat, ok := claims["sub"].(float64)
			if !ok {
				c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Invalid token claims"})
				return
			}

			// Get User Role
			role, ok := claims["role"].(string)
			if !ok {
				c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Invalid token claims (role)"})
				return
			}

			c.Set("userID", int(userIDFloat))
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

	switch req.Role {
	case "patient":
		user, err = h.Repo.GetPatientByEmail(req.Email)
	case "doctor":
		user, err = h.Repo.GetDoctorByEmail(req.Email)
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
		"sub":  user.GetID(),                              // "subject" (who the token is for)
		"role": req.Role,                                  // custom claim for user role
		"iat":  time.Now().Unix(),                         // "issued at"
		"exp":  time.Now().Add(time.Hour * 24 * 7).Unix(), // "expires at" (e.g., 7 days)
		"iss":  "vital-watch",                             // "issuer"
	}

	// Create the token
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

	// Sign the token with the secret key
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
		Specialty  string `json:"specialty,omitempty"`   // For doctors
		Experience int    `json:"experience,omitempty"`  // For doctors
		InviteCode string `json:"invite_code,omitempty"` // For doctors
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

	var id int
	var err error
	switch req.Role {
	case "patient":
		hashed, err := utils.HashPassword(req.Password)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to hash password"})
			return
		}
		id, err = h.Repo.CreatePatient(req.FirstName, req.LastName, req.Email, hashed)
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
		id, err = h.Repo.CreateDoctor(req.FirstName, req.LastName, req.Email, hashed, req.Specialty, req.Experience)
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid role"})
		return
	}

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create user"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"id": id})
}

func (h *Handler) GetUserProfile(c *gin.Context) {
	userID, ok := c.Get("userID")
	if !ok {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "User ID not found in context"})
		return
	}

	role, ok := c.Get("role")
	if !ok {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "Role not found in context"})
		return
	}

	switch role {
	case "patient":
		patient, err := h.Repo.GetPatientByID(userID.(int))
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "Patient profile not found"})
			return
		}
		c.JSON(http.StatusOK, patient)

	case "doctor":
		doctor, err := h.Repo.GetDoctorByID(userID.(int))
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
	doctors, err := h.Repo.GetDoctors()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch doctors", "err": err.Error()})
		return
	}
	c.JSON(http.StatusOK, doctors)
}

func (h *Handler) GetPatientAppointments(c *gin.Context) {
	patientID, ok := c.Get("userID")
	if !ok {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "User ID not found in context"})
		return
	}

	appointments, err := h.Repo.GetAppointmentsByPatientID(patientID.(int))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch appointments"})
		return
	}
	c.JSON(http.StatusOK, appointments)
}

func (h *Handler) GetPatientPrescriptions(c *gin.Context) {
	patientID, ok := c.Get("userID")
	if !ok {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "User ID not found in context"})
		return
	}

	prescriptions, err := h.Repo.GetPrescriptionsByPatientID(patientID.(int))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch prescriptions"})
		return
	}
	c.JSON(http.StatusOK, prescriptions)
}

func (h *Handler) CreateAppointment(c *gin.Context) {
	var req struct {
		DoctorID  int       `json:"doctor_id"`
		StartTime time.Time `json:"start_time"`
		EndTime   time.Time `json:"end_time"`
		Type      string    `json:"type"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	patientID, ok := c.Get("userID")
	if !ok {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "User ID not found in context"})
		return
	}

	newID, err := h.Repo.CreateAppointment(patientID.(int), req.DoctorID, req.StartTime, req.EndTime, req.Type)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create appointment", "err": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"id": newID})
}

func (h *Handler) DownloadPrescription(c *gin.Context) {
	filename := c.Param("filename")

	patientID, ok := c.Get("userID")
	if !ok {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "User ID not found in context"})
		return
	}

	// SECURITY CHECK: Verify this patient owns this file
	_, err := h.Repo.GetPrescriptionByFilename(patientID.(int), filename)
	if err != nil {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "You are not authorized to download this file"})
		return
	}

	// Get the file from S3
	getObjectInput := &s3.GetObjectInput{
		Bucket: aws.String(h.BucketName),
		Key:    aws.String(filename),
	}
	out, err := h.S3Client.GetObject(context.TODO(), getObjectInput)
	if err != nil {
		log.Printf("Failed to get object from S3: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve file"})
		return
	}
	defer out.Body.Close()

	// Set headers to tell the browser to download it
	c.Header("Content-Disposition", "attachment; filename="+filename)
	c.Header("Content-Type", *out.ContentType)
	c.Header("Content-Length", strconv.FormatInt(*out.ContentLength, 10))

	// Stream the file
	io.Copy(c.Writer, out.Body)
}

// Doctor Portal Handlers
func (h *Handler) GetDoctorAppointments(c *gin.Context) {
	doctorID, ok := c.Get("userID")
	if !ok {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "User ID not found in context"})
		return
	}

	// --- DEBUGGING: Log the doctorID ---
	log.Printf("GetDoctorAppointments: Fetching appointments for doctorID: %d", doctorID.(int))

	appointments, err := h.Repo.GetAppointmentsByDoctorID(doctorID.(int))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch appointments", "err": err.Error()})
		return
	}
	c.JSON(http.StatusOK, appointments)
}

func (h *Handler) GetDoctorPatients(c *gin.Context) {
	doctorID, ok := c.Get("userID")
	if !ok {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "User ID not found in context"})
		return
	}

	patients, err := h.Repo.GetPatientsByDoctorID(doctorID.(int))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch patients"})
		return
	}
	c.JSON(http.StatusOK, patients)
}

func (h *Handler) GetPatientHistoryAppointments(c *gin.Context) {
	doctorID, ok := c.Get("userID")
	if !ok {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "User ID not found in context"})
		return
	}

	patientIDStr := c.Param("id")
	patientID, err := strconv.Atoi(patientIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid patient ID"})
		return
	}

	appointments, err := h.Repo.GetAppointmentsForPatient(doctorID.(int), patientID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch appointments"})
		return
	}

	c.JSON(http.StatusOK, appointments)
}

func (h *Handler) GetPatientHistoryPrescriptions(c *gin.Context) {
	doctorID, ok := c.Get("userID")
	if !ok {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "User ID not found in context"})
		return
	}

	patientIDStr := c.Param("id")
	patientID, err := strconv.Atoi(patientIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid patient ID"})
		return
	}

	prescriptions, err := h.Repo.GetPrescriptionsForPatient(doctorID.(int), patientID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch prescriptions"})
		return
	}

	c.JSON(http.StatusOK, prescriptions)
}

func (h *Handler) CreatePrescription(c *gin.Context) {
	doctorID, ok := c.Get("userID")
	if !ok {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "User ID not found in context"})
		return
	}

	if err := c.Request.ParseMultipartForm(10 << 20); err != nil { // 10 MB Max File Size
		c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to parse form", "err": err.Error()})
		return
	}

	patientIDStr := c.Request.FormValue("patientID")
	medication := c.Request.FormValue("medication")
	notes := c.Request.FormValue("notes")

	patientID, err := strconv.Atoi(patientIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid patientID", "err": err.Error()})
		return
	}

	file, header, err := c.Request.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "File is required", "err": err.Error()})
		return
	}
	defer file.Close()

	// Generate a unique filename
	ext := filepath.Ext(header.Filename) // Get .pdf or .jpg
	uniqueFilename := fmt.Sprintf("prescription-%s-%s%s", patientIDStr, uuid.New().String(), ext)

	// Upload to S3
	putObjectInput := &s3.PutObjectInput{
		Bucket:        aws.String(h.BucketName),
		Key:           aws.String(uniqueFilename),
		Body:          file,
		ContentLength: aws.Int64(header.Size),
		ContentType:   aws.String(header.Header.Get("Content-Type")),
	}

	_, err = h.S3Client.PutObject(context.TODO(), putObjectInput)
	if err != nil {
		log.Printf("Failed to upload file to S3: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save file", "err": err.Error()})
		return
	}

	// Save metadata to database
	newID, err := h.Repo.CreatePrescription(patientID, doctorID.(int), medication, notes, uniqueFilename)
	if err != nil {
		log.Printf("Failed to create prescription in DB: %v", err)
		// If DB save fails, roll back S3 upload
		go func() {
			log.Printf("Rolling back S3 upload for key: %s", uniqueFilename)
			deleteObjectInput := &s3.DeleteObjectInput{
				Bucket: aws.String(h.BucketName),
				Key:    aws.String(uniqueFilename),
			}
			_, delErr := h.S3Client.DeleteObject(context.TODO(), deleteObjectInput)
			if delErr != nil {
				log.Printf("CRITICAL: Failed to rollback S3 upload: %v", delErr)
			}
		}()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create prescription record", "err": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"id": newID, "filename": uniqueFilename})
}

func (h *Handler) MarkAppointmentAsCompleted(c *gin.Context) {
	doctorID, ok := c.Get("userID")
	if !ok {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Doctor ID not found in context"})
		return
	}

	appointmentIDStr := c.Param("id")
	appointmentID, err := strconv.Atoi(appointmentIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid appointment ID"})
		return
	}

	updated, err := h.Repo.UpdateAppointmentAsCompletedForDoctor(appointmentID, doctorID.(int))
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

	doctorID, ok := c.Get("userID")
	if !ok {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "User ID not found in context"})
		return
	}

	// SECURITY CHECK: Verify this doctor is associated with this file
	_, err := h.Repo.GetPrescriptionByFilenameForDoctor(doctorID.(int), filename)
	if err != nil {
		log.Printf("Doctor download auth failed: %v", err)
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "You are not authorized to download this file"})
		return
	}

	// Get the file from S3
	getObjectInput := &s3.GetObjectInput{
		Bucket: aws.String(h.BucketName),
		Key:    aws.String(filename),
	}
	out, err := h.S3Client.GetObject(context.TODO(), getObjectInput)
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
