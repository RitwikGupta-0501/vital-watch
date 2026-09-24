package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
	"log/slog"
	"math"
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
	"github.com/RitwikGupta-0501/vital-watch/internal/audit"
	"github.com/RitwikGupta-0501/vital-watch/internal/notifications"
	"github.com/RitwikGupta-0501/vital-watch/internal/pdf"
	"github.com/RitwikGupta-0501/vital-watch/internal/repository"
	"github.com/RitwikGupta-0501/vital-watch/internal/safety"
	"github.com/RitwikGupta-0501/vital-watch/internal/schedule"
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

// Patient Portal Handlers
func (h *Handler) GetDoctors(c *gin.Context) {
	limit, offset := parsePagination(c)
	doctors, err := h.Repo.GetDoctors(c.Request.Context(), limit, offset)
	if err != nil {
		slog.ErrorContext(c.Request.Context(), "Internal error in GetDoctors", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch doctors"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"data":   doctors,
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
		slog.ErrorContext(c.Request.Context(), "Internal error in GetPatientPrescriptions", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch prescriptions"})
		return
	}
	h.audit(c, audit.ActionReadPrescriptions, "prescription", nil, &patientID, http.StatusOK, map[string]interface{}{"count": len(prescriptions)})
	c.JSON(http.StatusOK, gin.H{
		"data":   prescriptions,
		"limit":  limit,
		"offset": offset,
	})
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
	// auditPrescID and auditPatientID carry IDs for the HIPAA audit record.
	var auditPrescID *uuid.UUID
	var auditPatientID *uuid.UUID

	// If identifier is a valid UUID, look up by prescription ID
	if prescUUID, parseErr := uuid.Parse(identifier); parseErr == nil {
		presc, err := h.Repo.GetPrescriptionByID(ctx, prescUUID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) || errors.Is(err, pgx.ErrNoRows) {
				c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": "Prescription not found or access denied"})
				return
			}
			slog.ErrorContext(ctx, "Database error fetching prescription by ID", "error", err)
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
				hasRel, relErr := h.Repo.HasDoctorPatientRelationship(ctx, callerID, presc.PatientID)
				if relErr != nil || !hasRel {
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
		// Capture IDs for the audit record
		auditPrescID = &presc.ID
		auditPatientID = &presc.PatientID
	} else {
		if role == "patient" {
			presc, err := h.Repo.GetPrescriptionByFilename(ctx, callerID, identifier)
			if err != nil {
				if errors.Is(err, sql.ErrNoRows) || errors.Is(err, pgx.ErrNoRows) {
					c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": "Prescription not found or access denied"})
					return
				}
				slog.ErrorContext(ctx, "Database error fetching prescription", "error", err)
				c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "Internal database error"})
				return
			}
			auditPrescID = &presc.ID
			auditPatientID = &presc.PatientID
		} else if role == "doctor" {
			presc, err := h.Repo.GetPrescriptionByFilenameForDoctor(ctx, callerID, identifier)
			if err != nil {
				if errors.Is(err, sql.ErrNoRows) || errors.Is(err, pgx.ErrNoRows) {
					c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": "Prescription not found or access denied"})
					return
				}
				slog.ErrorContext(ctx, "Database error fetching prescription", "error", err)
				c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "Internal database error"})
				return
			}
			auditPrescID = &presc.ID
			auditPatientID = &presc.PatientID
		} else {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "Access denied"})
			return
		}
	}

	downloadURL, err := h.Storage.GenerateDownloadURL(ctx, targetFilename, 5*time.Minute)
	if err != nil {
		slog.ErrorContext(ctx, "Failed to generate download URL", "error", err, "filename", targetFilename)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate download URL"})
		return
	}

	// auditPrescID and auditPatientID are populated by the UUID lookup path above.
	h.audit(c, audit.ActionDownloadPrescriptionFile, "prescription_file", auditPrescID, auditPatientID, http.StatusOK, map[string]interface{}{
		"filename": targetFilename,
	})

	c.JSON(http.StatusOK, gin.H{
		"download_url": downloadURL,
		"expires_in":   300,
	})
}

// Doctor Portal Handlers
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
		slog.ErrorContext(c.Request.Context(), "Internal error in GetDoctorPatients", "error", err)
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
		slog.ErrorContext(c.Request.Context(), "Internal error in GetPatientHistoryAppointments", "error", err)
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
		slog.ErrorContext(c.Request.Context(), "Internal error in GetPatientHistoryPrescriptions", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch prescriptions"})
		return
	}

	h.audit(c, audit.ActionReadPrescriptions, "prescription", nil, &patientID, http.StatusOK, map[string]interface{}{
		"count":  len(prescriptions),
		"status": status,
	})

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
		slog.ErrorContext(c.Request.Context(), "Failed to generate upload URL", "error", err)
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

		hasRel, relErr := h.Repo.HasDoctorPatientRelationship(ctx, doctorID, req.PatientID)
		if relErr != nil || !hasRel {
			c.JSON(http.StatusForbidden, gin.H{"error": "You can only prescribe to patients who have booked appointments with you"})
			return
		}
	}

	exists, err := h.Storage.ObjectExists(ctx, req.FileName)
	if err != nil {
		slog.ErrorContext(c.Request.Context(), "Storage error verifying object existence", "error", err)
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
		slog.ErrorContext(c.Request.Context(), "Failed to create prescription in DB", "error", err)
		// Clean up uploaded file if DB record insertion fails
		go func() {
			rbCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			if delErr := h.Storage.DeleteFile(rbCtx, req.FileName); delErr != nil {
				slog.ErrorContext(rbCtx, "CRITICAL: Failed to rollback storage file", "error", delErr, "filename", req.FileName)
			}
		}()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create prescription record"})
		return
	}

	h.audit(c, audit.ActionCreatePrescription, "prescription", &newID, &req.PatientID, http.StatusCreated, map[string]interface{}{
		"source":   "upload",
		"filename": req.FileName,
	})

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

	// Verify doctor has an active clinical relationship with the patient
	hasRel, relErr := h.Repo.HasDoctorPatientRelationship(ctx, doctorID, req.PatientID)
	if relErr != nil || !hasRel {
		c.JSON(http.StatusForbidden, gin.H{"error": "You can only prescribe to patients who have booked appointments with you"})
		return
	}

	// Safety Pre-flight Check (DDI and Allergies)
	safetyReport, safetyErr := h.checkSafety(ctx, req.PatientID, items)
	if safetyErr == nil && safetyReport != nil && (safetyReport.HasHighSeverityAlerts || safetyReport.ServiceDegraded) && !req.OverrideSafety {
		errMsg := "Critical drug interaction or allergy contraindication detected"
		if safetyReport.ServiceDegraded {
			errMsg = "Drug safety check service is degraded; automated interaction checks incomplete"
		}
		c.JSON(http.StatusConflict, gin.H{
			"error":             errMsg,
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
		slog.ErrorContext(c.Request.Context(), "Failed to create digital prescription", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create digital prescription"})
		return
	}

	// Auto-generate standardized prescription PDF
	var linkedFileName string
	if h.PDFGenerator != nil && h.Storage != nil {
		targetFileName := fmt.Sprintf("prescription-%s-%s.pdf", req.PatientID.String(), newID.String())
		doc, docErr := h.Repo.GetDoctorByID(ctx, doctorID)
		if docErr != nil {
			slog.WarnContext(c.Request.Context(), "Could not fetch doctor details for PDF", "error", docErr)
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
			slog.WarnContext(c.Request.Context(), "Failed to generate digital prescription PDF", "error", genErr)
		} else if saveErr := h.Storage.SaveFile(ctx, targetFileName, pdfBytes, "application/pdf"); saveErr != nil {
			slog.ErrorContext(c.Request.Context(), "Failed to persist generated PDF to storage", "error", saveErr)
		} else if updateErr := h.Repo.UpdatePrescriptionFileName(ctx, newID, targetFileName); updateErr != nil {
			slog.ErrorContext(c.Request.Context(), "Failed to link prescription PDF filename in DB", "error", updateErr)
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

	h.audit(c, audit.ActionCreatePrescription, "prescription", &newID, &req.PatientID, http.StatusCreated, map[string]interface{}{
		"source":          "digital",
		"override_safety": req.OverrideSafety,
	})
	if linkedFileName != "" {
		h.audit(c, audit.ActionExportPrescriptionPDF, "prescription_pdf", &newID, &req.PatientID, http.StatusCreated, map[string]interface{}{
			"filename": linkedFileName,
		})
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
		slog.ErrorContext(c.Request.Context(), "Internal error in GetPendingReviewPrescriptions", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch pending prescriptions"})
		return
	}

	h.audit(c, audit.ActionReadPrescriptions, "prescription", nil, nil, http.StatusOK, map[string]interface{}{
		"count": len(prescriptions),
	})

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
			if safetyErr == nil && safetyReport != nil && (safetyReport.HasHighSeverityAlerts || safetyReport.ServiceDegraded) && !req.OverrideSafety {
				errMsg := "Critical drug interaction or allergy contraindication detected"
				if safetyReport.ServiceDegraded {
					errMsg = "Drug safety check service is degraded; automated interaction checks incomplete"
				}
				c.JSON(http.StatusConflict, gin.H{
					"error":             errMsg,
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
		slog.ErrorContext(c.Request.Context(), "Internal error in VerifyPrescription", "error", err)
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
				hasRel, relErr := h.Repo.HasDoctorPatientRelationship(ctx, doctorID, existing.PatientID)
				isAuthorized = relErr == nil && hasRel
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
						slog.WarnContext(c.Request.Context(), "Failed to fetch doctor details for PDF", "error", docErr)
					}
					pat, patErr := h.Repo.GetPatientByID(ctx, existing.PatientID)
					if patErr != nil {
						slog.WarnContext(c.Request.Context(), "Failed to fetch patient details for PDF", "error", patErr)
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
							slog.ErrorContext(c.Request.Context(), "Failed to persist verified prescription PDF to storage", "error", saveErr)
						} else {
							if updateErr := h.Repo.UpdatePrescriptionFileName(ctx, prescriptionID, pdfFileName); updateErr != nil {
								slog.ErrorContext(c.Request.Context(), "Failed to link verified prescription PDF filename in DB", "error", updateErr)
							}
						}
					} else {
						slog.WarnContext(c.Request.Context(), "Failed to generate verified prescription PDF", "error", genErr)
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

	h.audit(c, audit.ActionVerifyPrescription, "prescription", &prescriptionID, &existing.PatientID, http.StatusOK, map[string]interface{}{
		"status":          req.Status,
		"override_safety": req.OverrideSafety,
	})

	c.JSON(http.StatusOK, gin.H{
		"message": "Prescription verified successfully",
		"status":  req.Status,
	})
}

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
		slog.ErrorContext(c.Request.Context(), "Internal error in GetPrescriptionByID", "error", err)
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
			hasRel, relErr := h.Repo.HasDoctorPatientRelationship(c.Request.Context(), userID, prescription.PatientID)
			if relErr != nil || !hasRel {
				c.JSON(http.StatusNotFound, gin.H{"error": "Prescription not found or access denied"})
				return
			}
		}
	} else {
		c.JSON(http.StatusForbidden, gin.H{"error": "Access denied"})
		return
	}

	h.audit(c, audit.ActionReadPrescriptions, "prescription", &prescription.ID, &prescription.PatientID, http.StatusOK, nil)

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

// ==========================================
// PHASE 4: ADVANCED SCHEDULING & TELEHEALTH
// ==========================================

// ==========================================
// PHASE 4: LONGITUDINAL PATIENT VITALS
// ==========================================

type VitalInput struct {
	PatientID        *uuid.UUID `json:"patient_id,omitempty"`
	RecordedAt       *time.Time `json:"recorded_at,omitempty"`
	SystolicBP       *int       `json:"systolic_bp,omitempty"`
	DiastolicBP      *int       `json:"diastolic_bp,omitempty"`
	HeartRate        *int       `json:"heart_rate,omitempty"`
	BloodGlucose     *float64   `json:"blood_glucose,omitempty"`
	OxygenSaturation *float64   `json:"oxygen_saturation,omitempty"`
	Temperature      *float64   `json:"temperature,omitempty"`
	WeightKg         *float64   `json:"weight_kg,omitempty"`
	Notes            string     `json:"notes,omitempty"`
}

func (h *Handler) CreatePatientVital(c *gin.Context) {
	userIDVal, ok := c.Get("userID")
	roleVal, roleOk := c.Get("role")
	if !ok || !roleOk {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "User context missing"})
		return
	}
	callerID := userIDVal.(uuid.UUID)
	callerRole := roleVal.(string)

	var in VitalInput
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	var targetPatientID uuid.UUID
	if callerRole == "patient" {
		targetPatientID = callerID
	} else if callerRole == "doctor" {
		if in.PatientID == nil || *in.PatientID == uuid.Nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "patient_id is required when recording vitals as a doctor"})
			return
		}
		targetPatientID = *in.PatientID
		hasRel, err := h.Repo.HasDoctorPatientRelationship(c.Request.Context(), callerID, targetPatientID)
		if err != nil {
			slog.ErrorContext(c.Request.Context(), "Internal error checking doctor-patient relationship", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to verify patient relationship"})
			return
		}
		if !hasRel {
			c.JSON(http.StatusForbidden, gin.H{"error": "Access denied: no clinical relationship with this patient"})
			return
		}
	} else {
		c.JSON(http.StatusForbidden, gin.H{"error": "Unauthorized role for recording vitals"})
		return
	}

	if len([]rune(strings.TrimSpace(in.Notes))) > 2000 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "notes exceeds maximum length of 2000 characters"})
		return
	}

	hasMetric := in.SystolicBP != nil || in.DiastolicBP != nil || in.HeartRate != nil ||
		in.BloodGlucose != nil || in.OxygenSaturation != nil || in.Temperature != nil || in.WeightKg != nil
	if !hasMetric {
		c.JSON(http.StatusBadRequest, gin.H{"error": "At least one vital measurement must be provided"})
		return
	}

	isInvalidFloat := func(f *float64) bool {
		return f != nil && (math.IsNaN(*f) || math.IsInf(*f, 0))
	}
	if isInvalidFloat(in.BloodGlucose) || isInvalidFloat(in.OxygenSaturation) || isInvalidFloat(in.Temperature) || isInvalidFloat(in.WeightKg) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Vital measurements must be valid finite numbers"})
		return
	}

	if in.SystolicBP != nil && (*in.SystolicBP < 50 || *in.SystolicBP > 300) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "systolic_bp must be between 50 and 300 mmHg"})
		return
	}
	if in.DiastolicBP != nil && (*in.DiastolicBP < 30 || *in.DiastolicBP > 200) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "diastolic_bp must be between 30 and 200 mmHg"})
		return
	}
	if in.SystolicBP != nil && in.DiastolicBP != nil && *in.SystolicBP <= *in.DiastolicBP {
		c.JSON(http.StatusBadRequest, gin.H{"error": "systolic_bp must be strictly greater than diastolic_bp"})
		return
	}
	if in.HeartRate != nil && (*in.HeartRate < 30 || *in.HeartRate > 250) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "heart_rate must be between 30 and 250 bpm"})
		return
	}
	if in.BloodGlucose != nil && (*in.BloodGlucose < 10.0 || *in.BloodGlucose > 1000.0) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "blood_glucose must be between 10 and 1000 mg/dL"})
		return
	}
	if in.OxygenSaturation != nil && (*in.OxygenSaturation < 50.0 || *in.OxygenSaturation > 100.0) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "oxygen_saturation must be between 50% and 100%"})
		return
	}
	if in.Temperature != nil && (*in.Temperature < 30.0 || *in.Temperature > 45.0) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "temperature must be between 30.0 and 45.0 °C"})
		return
	}
	if in.WeightKg != nil && (*in.WeightKg < 1.0 || *in.WeightKg > 500.0) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "weight_kg must be between 1.0 and 500.0 kg"})
		return
	}

	recAt := time.Now()
	if in.RecordedAt != nil && !in.RecordedAt.IsZero() {
		if in.RecordedAt.After(time.Now().Add(5 * time.Minute)) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "recorded_at cannot be in the future"})
			return
		}
		recAt = *in.RecordedAt
	}

	vital := models.PatientVital{
		PatientID:        targetPatientID,
		RecordedBy:       callerID,
		RecordedAt:       recAt,
		SystolicBP:       in.SystolicBP,
		DiastolicBP:      in.DiastolicBP,
		HeartRate:        in.HeartRate,
		BloodGlucose:     in.BloodGlucose,
		OxygenSaturation: in.OxygenSaturation,
		Temperature:      in.Temperature,
		WeightKg:         in.WeightKg,
		Notes:            strings.TrimSpace(in.Notes),
	}

	newID, err := h.Repo.CreatePatientVital(c.Request.Context(), vital)
	if err != nil {
		slog.ErrorContext(c.Request.Context(), "Internal error in CreatePatientVital", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to record vital measurement"})
		return
	}

	h.audit(c, audit.ActionRecordVitals, "vital", &newID, &targetPatientID, http.StatusCreated, nil)
	c.JSON(http.StatusCreated, gin.H{"id": newID})
}

func (h *Handler) GetPatientVitals(c *gin.Context) {
	userIDVal, ok := c.Get("userID")
	roleVal, roleOk := c.Get("role")
	if !ok || !roleOk {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "User context missing"})
		return
	}
	callerID := userIDVal.(uuid.UUID)
	callerRole := roleVal.(string)

	var targetPatientID uuid.UUID
	if callerRole == "patient" {
		targetPatientID = callerID
	} else if callerRole == "doctor" {
		patientIDStr := c.Query("patient_id")
		if patientIDStr == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "patient_id query parameter is required for doctors"})
			return
		}
		parsedID, err := uuid.Parse(patientIDStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid patient_id"})
			return
		}
		targetPatientID = parsedID
		hasRel, err := h.Repo.HasDoctorPatientRelationship(c.Request.Context(), callerID, targetPatientID)
		if err != nil {
			slog.ErrorContext(c.Request.Context(), "Internal error checking doctor-patient relationship", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to verify patient relationship"})
			return
		}
		if !hasRel {
			c.JSON(http.StatusForbidden, gin.H{"error": "Access denied: no clinical relationship with this patient"})
			return
		}
	} else {
		c.JSON(http.StatusForbidden, gin.H{"error": "Access denied"})
		return
	}

	var startDate, endDate *time.Time
	if sStr := strings.TrimSpace(c.Query("start_date")); sStr != "" {
		if t, err := time.Parse(time.RFC3339, sStr); err == nil {
			startDate = &t
		} else if t, err := time.Parse("2006-01-02", sStr); err == nil {
			startDate = &t
		} else {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid start_date format (RFC3339 or YYYY-MM-DD)"})
			return
		}
	}
	if eStr := strings.TrimSpace(c.Query("end_date")); eStr != "" {
		if t, err := time.Parse(time.RFC3339, eStr); err == nil {
			endDate = &t
		} else if t, err := time.Parse("2006-01-02", eStr); err == nil {
			eEnd := t.Add(24*time.Hour - time.Microsecond)
			endDate = &eEnd
		} else {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid end_date format (RFC3339 or YYYY-MM-DD)"})
			return
		}
	}
	if startDate != nil && endDate != nil && endDate.Before(*startDate) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "end_date cannot be before start_date"})
		return
	}

	limit := 20
	offset := 0
	if lStr := c.Query("limit"); lStr != "" {
		if l, err := strconv.Atoi(lStr); err == nil && l > 0 {
			limit = l
			if limit > 100 {
				limit = 100
			}
		}
	}
	if oStr := c.Query("offset"); oStr != "" {
		if o, err := strconv.Atoi(oStr); err == nil && o >= 0 {
			offset = o
		}
	}

	vitals, err := h.Repo.GetPatientVitals(c.Request.Context(), targetPatientID, startDate, endDate, limit, offset)
	if err != nil {
		slog.ErrorContext(c.Request.Context(), "Internal error in GetPatientVitals", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve vitals"})
		return
	}

	h.audit(c, audit.ActionViewVitals, "vital", nil, &targetPatientID, http.StatusOK, map[string]interface{}{"count": len(vitals)})
	c.JSON(http.StatusOK, gin.H{
		"data":   vitals,
		"limit":  limit,
		"offset": offset,
	})
}

// ==========================================
// PHASE 4: MEDICATION SCHEDULE & ADHERENCE
// ==========================================

func (h *Handler) GetPatientMedicationSchedule(c *gin.Context) {
	userIDVal, ok := c.Get("userID")
	roleVal, roleOk := c.Get("role")
	if !ok || !roleOk {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "User context missing"})
		return
	}
	callerID := userIDVal.(uuid.UUID)
	callerRole := roleVal.(string)

	var targetPatientID uuid.UUID
	if callerRole == "patient" {
		targetPatientID = callerID
	} else if callerRole == "doctor" {
		patientIDStr := c.Query("patient_id")
		if patientIDStr == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "patient_id query parameter is required for doctors"})
			return
		}
		parsedID, err := uuid.Parse(patientIDStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid patient_id"})
			return
		}
		targetPatientID = parsedID
		hasRel, err := h.Repo.HasDoctorPatientRelationship(c.Request.Context(), callerID, targetPatientID)
		if err != nil {
			slog.ErrorContext(c.Request.Context(), "Internal error checking doctor-patient relationship", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to verify patient relationship"})
			return
		}
		if !hasRel {
			c.JSON(http.StatusForbidden, gin.H{"error": "Access denied: no clinical relationship with this patient"})
			return
		}
	} else {
		c.JSON(http.StatusForbidden, gin.H{"error": "Access denied"})
		return
	}

	targetDate := time.Now()
	if dateStr := strings.TrimSpace(c.Query("date")); dateStr != "" {
		if d, err := time.Parse("2006-01-02", dateStr); err == nil {
			targetDate = d
		} else {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid date format: must be YYYY-MM-DD"})
			return
		}
	}

	items, err := h.Repo.GetActivePrescriptionItemsForPatient(c.Request.Context(), targetPatientID, targetDate)
	if err != nil {
		slog.ErrorContext(c.Request.Context(), "Internal error fetching active prescription items", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch prescription medications"})
		return
	}

	existingLogs, err := h.Repo.GetMedicationLogsByDate(c.Request.Context(), targetPatientID, targetDate)
	if err != nil {
		slog.ErrorContext(c.Request.Context(), "Internal error fetching medication logs", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch medication logs"})
		return
	}

	dailySchedule := schedule.BuildDailySchedule(targetPatientID, targetDate, items, existingLogs)

	h.audit(c, audit.ActionViewMedicationSchedule, "schedule", nil, &targetPatientID, http.StatusOK, map[string]interface{}{"date": targetDate.Format("2006-01-02")})
	c.JSON(http.StatusOK, gin.H{
		"date":       targetDate.Format("2006-01-02"),
		"patient_id": targetPatientID,
		"schedule":   dailySchedule,
	})
}

type MedicationLogInput struct {
	PrescriptionItemID uuid.UUID  `json:"prescription_item_id"`
	ScheduledDate      string     `json:"scheduled_date"`
	TimeOfDay          string     `json:"time_of_day"`
	DoseNumber         int        `json:"dose_number"`
	MealTiming         string     `json:"meal_timing,omitempty"`
	Status             string     `json:"status"`
	TakenAt            *time.Time `json:"taken_at,omitempty"`
	Notes              string     `json:"notes,omitempty"`
}

func (h *Handler) LogMedicationAdherence(c *gin.Context) {
	userIDVal, ok := c.Get("userID")
	if !ok {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "User ID not found in context"})
		return
	}
	patientID := userIDVal.(uuid.UUID)

	var in MedicationLogInput
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	if in.PrescriptionItemID == uuid.Nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "prescription_item_id is required"})
		return
	}

	isOwner, err := h.Repo.VerifyPrescriptionItemOwnership(c.Request.Context(), in.PrescriptionItemID, patientID)
	if err != nil {
		slog.ErrorContext(c.Request.Context(), "Internal error verifying prescription item ownership", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to verify medication item"})
		return
	}
	if !isOwner {
		c.JSON(http.StatusForbidden, gin.H{"error": "Access denied: prescription item does not belong to this patient"})
		return
	}

	if len([]rune(strings.TrimSpace(in.Notes))) > 2000 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "notes exceeds maximum length of 2000 characters"})
		return
	}

	schedDate, err := time.Parse("2006-01-02", strings.TrimSpace(in.ScheduledDate))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid scheduled_date format: must be YYYY-MM-DD"})
		return
	}

	if schedDate.After(time.Now().Add(24 * time.Hour)) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "scheduled_date cannot be more than 1 day in the future"})
		return
	}

	in.TimeOfDay = strings.ToLower(strings.TrimSpace(in.TimeOfDay))
	allowedSlots := map[string]bool{"morning": true, "afternoon": true, "evening": true, "bedtime": true, "as_needed": true}
	if !allowedSlots[in.TimeOfDay] {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid time_of_day: must be morning, afternoon, evening, bedtime, or as_needed"})
		return
	}

	in.Status = strings.ToLower(strings.TrimSpace(in.Status))
	allowedStatuses := map[string]bool{"taken": true, "skipped": true, "pending": true}
	if !allowedStatuses[in.Status] {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid status: must be taken, skipped, or pending"})
		return
	}

	doseNum := in.DoseNumber
	if doseNum <= 0 {
		doseNum = 1
	}

	var takenAt *time.Time
	if in.Status == "taken" {
		if in.TakenAt != nil && !in.TakenAt.IsZero() {
			if in.TakenAt.After(time.Now().Add(5 * time.Minute)) {
				c.JSON(http.StatusBadRequest, gin.H{"error": "taken_at cannot be in the future"})
				return
			}
			takenAt = in.TakenAt
		} else {
			now := time.Now()
			takenAt = &now
		}
	}

	logEntry := models.MedicationLog{
		PatientID:          patientID,
		PrescriptionItemID: in.PrescriptionItemID,
		ScheduledDate:      schedDate,
		TimeOfDay:          in.TimeOfDay,
		DoseNumber:         doseNum,
		MealTiming:         strings.TrimSpace(in.MealTiming),
		Status:             in.Status,
		TakenAt:            takenAt,
		Notes:              strings.TrimSpace(in.Notes),
	}

	saved, err := h.Repo.UpsertMedicationLog(c.Request.Context(), logEntry)
	if err != nil {
		slog.ErrorContext(c.Request.Context(), "Internal error in UpsertMedicationLog", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to log medication adherence"})
		return
	}

	h.audit(c, audit.ActionLogMedicationAdherence, "schedule", &saved.ID, &patientID, http.StatusOK, map[string]interface{}{"status": in.Status, "time_of_day": in.TimeOfDay})
	c.JSON(http.StatusOK, gin.H{"data": saved})
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

