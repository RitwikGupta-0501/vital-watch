package api

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/RitwikGupta-0501/vital-watch/internal/audit"
	"github.com/RitwikGupta-0501/vital-watch/internal/models"
	"github.com/RitwikGupta-0501/vital-watch/internal/notifications"
	"github.com/RitwikGupta-0501/vital-watch/internal/pdf"
	"github.com/RitwikGupta-0501/vital-watch/internal/safety"
	"github.com/RitwikGupta-0501/vital-watch/internal/storage"
)

// ==========================================
// DOMAIN 4: PRESCRIPTIONS, VISION AI OCR & SAFETY
// ==========================================

// PrescriptionItemInput represents client input for prescription medication items.
type PrescriptionItemInput struct {
	MedicationName string `json:"medication_name"`
	Dosage         string `json:"dosage"`
	Frequency      string `json:"frequency"`
	Duration       string `json:"duration"`
	Timing         string `json:"timing"`
	Instructions   string `json:"instructions"`
}

// CreateDigitalPrescriptionRequest represents client input for digital e-prescribing.
type CreateDigitalPrescriptionRequest struct {
	PatientID      uuid.UUID               `json:"patient_id"`
	Notes          string                  `json:"notes"`
	Items          []PrescriptionItemInput `json:"items"`
	OverrideSafety bool                    `json:"override_safety"`
	OverrideReason string                  `json:"override_reason"`
}

// VerifyPrescriptionRequest represents doctor approval or rejection of an OCR/uploaded slip.
type VerifyPrescriptionRequest struct {
	Status         string                  `json:"status"` // "approved" or "rejected"
	Notes          string                  `json:"notes"`
	Items          []PrescriptionItemInput `json:"items"`
	OverrideSafety bool                    `json:"override_safety"`
	OverrideReason string                  `json:"override_reason"`
}

func validatePrescriptionItems(inputs []PrescriptionItemInput) ([]models.PrescriptionItem, error) {
	if len(inputs) == 0 {
		return nil, errors.New("at least one medication item is required")
	}
	if len(inputs) > 50 {
		return nil, errors.New("a prescription cannot contain more than 50 medication items")
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

// GetPatientPrescriptions retrieves prescriptions issued to the authenticated patient.
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

// DownloadPrescription provides a pre-signed direct download URL to authorized patients and doctors.
func (h *Handler) DownloadPrescription(c *gin.Context) {
	identifier := c.Param("filename")
	if identifier == "" {
		identifier = c.Param("id")
	}

	cleaned := filepath.Base(filepath.Clean(identifier))
	if cleaned != identifier || cleaned == "." || cleaned == "/" || strings.Contains(identifier, "..") {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "Invalid filename or prescription identifier"})
		return
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

	h.audit(c, audit.ActionDownloadPrescriptionFile, "prescription_file", auditPrescID, auditPatientID, http.StatusOK, map[string]interface{}{
		"filename": targetFilename,
	})

	c.JSON(http.StatusOK, gin.H{
		"download_url": downloadURL,
		"expires_in":   300,
	})
}

// DoctorDownloadPrescription allows authorized doctors to retrieve prescription files.
func (h *Handler) DoctorDownloadPrescription(c *gin.Context) {
	h.DownloadPrescription(c)
}

// GetPatientHistoryPrescriptions retrieves historical prescriptions for a patient from a doctor perspective.
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

// GetPrescriptionUploadURL generates a guarded pre-signed upload URL for direct cloud upload.
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

	h.audit(c, audit.ActionCreatePrescription, "prescription_upload_url", nil, &req.PatientID, http.StatusOK, map[string]interface{}{
		"filename": uniqueFilename,
	})

	c.JSON(http.StatusOK, gin.H{
		"upload_url": uploadURL,
		"file_key":   uniqueFilename,
		"expires_in": 300,
	})
}

// CreatePrescription persists an uploaded prescription record and enqueues an OCR task.
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

	// HIGH-3: Check if file_name is already associated with an existing prescription
	fileAlreadyUsed, chkErr := h.Repo.CheckPrescriptionFileNameExists(ctx, req.FileName)
	if chkErr == nil && fileAlreadyUsed {
		c.JSON(http.StatusConflict, gin.H{"error": "A prescription with this file name already exists"})
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
		go func() {
			rbCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			// Guard: only delete file if no database record references it
			inUse, cErr := h.Repo.CheckPrescriptionFileNameExists(rbCtx, req.FileName)
			if cErr == nil && inUse {
				return
			}
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

func (h *Handler) checkSafety(ctx context.Context, patientID uuid.UUID, items []models.PrescriptionItem) (*safety.SafetyReport, error) {
	if h.SafetyChecker == nil || len(items) == 0 {
		return nil, nil
	}
	newMedNames := make([]string, 0, len(items))
	for _, it := range items {
		newMedNames = append(newMedNames, it.MedicationName)
	}
	activeMedNames := make([]string, 0)
	activeItems, err := h.Repo.GetActivePrescriptionItemsForPatient(ctx, patientID, time.Now())
	if err == nil && len(activeItems) > 0 {
		for _, it := range activeItems {
			activeMedNames = append(activeMedNames, it.MedicationName)
		}
	} else {
		// Fallback for repositories / mocks where only GetPrescriptionsByPatientID is implemented
		if prescriptions, pErr := h.Repo.GetPrescriptionsByPatientID(ctx, patientID, 50, 0); pErr == nil {
			for _, p := range prescriptions {
				if p.Status == "approved" {
					for _, it := range p.Items {
						activeMedNames = append(activeMedNames, it.MedicationName)
					}
				}
			}
		}
	}
	return h.SafetyChecker.CheckPrescriptionSafety(ctx, newMedNames, activeMedNames, nil)
}

// CreateDigitalPrescription allows a doctor to author a digital native prescription.
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

// GetPendingReviewPrescriptions retrieves prescriptions waiting for doctor HITL verification.
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

// VerifyPrescription implements clinician sign-off on uploaded prescriptions.
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
		c.JSON(http.StatusBadRequest, gin.H{"error": "Status must be approved or rejected"})
		return
	}

	ctx := c.Request.Context()
	existing, getErr := h.Repo.GetPrescriptionByID(ctx, prescriptionID)
	if getErr != nil || existing.ID == uuid.Nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Prescription not found"})
		return
	}

	// CRITICAL-1: Enforce authorization immediately before any safety calculation or processing.
	// Prevents unauthorized doctors from probing prescription UUIDs to extract patient medication/allergy history.
	isAuthorized := existing.DoctorID == doctorID
	if !isAuthorized {
		hasRel, relErr := h.Repo.HasDoctorPatientRelationship(ctx, doctorID, existing.PatientID)
		isAuthorized = relErr == nil && hasRel
	}
	if !isAuthorized {
		c.JSON(http.StatusForbidden, gin.H{"error": "Access denied: you are not authorized to verify prescriptions for this patient"})
		return
	}

	if existing.Status == "pending_ocr" {
		c.JSON(http.StatusConflict, gin.H{"error": "Prescription OCR processing is in progress. Verification is only available once OCR completes."})
		return
	}
	if existing.Status == "approved" || existing.Status == "rejected" {
		c.JSON(http.StatusConflict, gin.H{"error": fmt.Sprintf("Prescription has already been verified (status: %s)", existing.Status)})
		return
	}

	var items []models.PrescriptionItem

	if req.Status == "approved" {
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
			items = nil
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
		c.JSON(http.StatusConflict, gin.H{"error": "Prescription could not be updated or was modified concurrently"})
		return
	}

	if req.Status == "approved" {
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

	h.audit(c, audit.ActionVerifyPrescription, "prescription", &prescriptionID, &existing.PatientID, http.StatusOK, map[string]interface{}{
		"status":          req.Status,
		"override_safety": req.OverrideSafety,
	})

	c.JSON(http.StatusOK, gin.H{
		"message": "Prescription verified successfully",
		"status":  req.Status,
	})
}

var safetyOverrideRegex = regexp.MustCompile(`(?i)\s*\[Safety Override:.*?\]`)

func sanitizePublicNotes(notes string) string {
	return strings.TrimSpace(safetyOverrideRegex.ReplaceAllString(notes, ""))
}

// GetPrescriptionByID retrieves a single prescription by ID with ownership access control,
// supporting dual-mode public QR verification when unauthenticated.
func (h *Handler) GetPrescriptionByID(c *gin.Context) {
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

	userIDVal, ok := c.Get("userID")
	if !ok {
		// Dual-mode resolution: Public QR code verification (/verify/rx/:id)
		if prescription.Status != "approved" {
			c.JSON(http.StatusNotFound, gin.H{"error": "Prescription not found or unverified"})
			return
		}
		h.audit(c, audit.ActionReadPrescriptions, "prescription", &prescription.ID, nil, http.StatusOK, map[string]interface{}{
			"mode": "public_qr_verification",
		})
		patientIDStr := prescription.PatientID.String()
		maskedPatientID := "********-****-****-****-" + patientIDStr[len(patientIDStr)-12:]
		c.JSON(http.StatusOK, gin.H{
			"id":                prescription.ID,
			"status":            prescription.Status,
			"created_at":        prescription.CreatedAt,
			"doctor_id":         prescription.DoctorID,
			"patient_id":        maskedPatientID,
			"masked_patient_id": maskedPatientID,
			"verified":          true,
			"items":             prescription.Items,
			"notes":             sanitizePublicNotes(prescription.Notes),
		})
		return
	}
	userID := userIDVal.(uuid.UUID)
	roleVal, _ := c.Get("role")
	role, _ := roleVal.(string)

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
