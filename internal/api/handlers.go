package api

import (
	"encoding/json"
	"log/slog"
	"math"
	"net/http"
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

