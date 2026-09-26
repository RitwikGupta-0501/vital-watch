package api

import (
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/RitwikGupta-0501/vital-watch/internal/audit"
	"github.com/RitwikGupta-0501/vital-watch/internal/models"
	"github.com/RitwikGupta-0501/vital-watch/internal/notifications"
	"github.com/RitwikGupta-0501/vital-watch/internal/schedule"
)

// ==========================================
// DOMAIN 5: PATIENT CARE, VITALS & ADHERENCE
// ==========================================

// GetDoctors retrieves the directory of registered doctors with pagination.
func (h *Handler) GetDoctors(c *gin.Context) {
	limit, offset := parsePagination(c)
	doctors, err := h.Repo.GetDoctors(c.Request.Context(), limit, offset)
	if err != nil {
		slog.ErrorContext(c.Request.Context(), "Internal error in GetDoctors", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch doctors"})
		return
	}
	h.audit(c, audit.ActionViewDoctorProfile, "doctor_directory", nil, nil, http.StatusOK, map[string]interface{}{"count": len(doctors)})
	c.JSON(http.StatusOK, gin.H{
		"data":   doctors,
		"limit":  limit,
		"offset": offset,
	})
}

// GetDoctorPatients retrieves the active patient roster for the authenticated doctor.
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
	h.audit(c, audit.ActionViewPatientProfile, "patient_roster", nil, nil, http.StatusOK, map[string]interface{}{"count": len(patients)})
	c.JSON(http.StatusOK, gin.H{
		"data":   patients,
		"limit":  limit,
		"offset": offset,
	})
}

// GetPatientHistoryAppointments retrieves the appointment history for a given patient.
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

	h.audit(c, audit.ActionViewAppointments, "appointment_history", nil, &patientID, http.StatusOK, map[string]interface{}{"count": len(appointments)})

	c.JSON(http.StatusOK, gin.H{
		"data":   appointments,
		"limit":  limit,
		"offset": offset,
	})
}

// StreamNotifications provides a real-time SSE stream for prescription and clinic status events.
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

// VitalInput represents biometric data submitted for patient health tracking.
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

// CreatePatientVital records biometric measurements with physiological boundary validation.
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
		if in.RecordedAt.Before(time.Now().AddDate(-5, 0, 0)) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "recorded_at cannot be older than 5 years"})
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

	isCritical, criticalAlerts := evaluateVitalAlerts(vital)

	if isCritical && h.Notifier != nil {
		alertMsg := "Critical vital signs recorded: " + strings.Join(criticalAlerts, "; ")
		h.Notifier.Publish(notifications.NotificationEvent{
			Type:      notifications.EventVitalAlert,
			PatientID: targetPatientID,
			Message:   alertMsg,
			Data: map[string]interface{}{
				"vital_id":        newID,
				"is_critical":     true,
				"critical_alerts": criticalAlerts,
			},
		})
	}

	auditMeta := map[string]interface{}{
		"is_critical": isCritical,
	}
	if isCritical {
		auditMeta["critical_alerts"] = criticalAlerts
	}
	h.audit(c, audit.ActionRecordVitals, "vital", &newID, &targetPatientID, http.StatusCreated, auditMeta)

	resp := gin.H{
		"id":          newID,
		"is_critical": isCritical,
	}
	if isCritical {
		resp["critical_alerts"] = criticalAlerts
	}
	c.JSON(http.StatusCreated, resp)
}

// evaluateVitalAlerts evaluates biometric readings against emergency physiological crisis thresholds.
func evaluateVitalAlerts(vital models.PatientVital) (bool, []string) {
	var alerts []string

	// Blood Pressure: Hypertensive Crisis (>= 180 SBP or >= 120 DBP)
	if (vital.SystolicBP != nil && *vital.SystolicBP >= 180) || (vital.DiastolicBP != nil && *vital.DiastolicBP >= 120) {
		alerts = append(alerts, fmt.Sprintf("Hypertensive Crisis: Blood pressure is dangerously elevated (%d/%d mmHg). Seek emergency medical care immediately.",
			safeIntDeref(vital.SystolicBP), safeIntDeref(vital.DiastolicBP)))
	} else if vital.SystolicBP != nil && vital.DiastolicBP != nil && *vital.SystolicBP < 80 && *vital.DiastolicBP < 50 {
		alerts = append(alerts, fmt.Sprintf("Hypotensive Shock Warning: Blood pressure is critically low (%d/%d mmHg). Seek emergency medical care.",
			*vital.SystolicBP, *vital.DiastolicBP))
	}

	// Oxygen Saturation: Severe Hypoxemia (< 90%)
	if vital.OxygenSaturation != nil && *vital.OxygenSaturation < 90.0 {
		alerts = append(alerts, fmt.Sprintf("Severe Hypoxemia: Blood oxygen saturation is critically low (%.1f%%). Seek emergency medical attention immediately.",
			*vital.OxygenSaturation))
	}

	// Heart Rate: Critical Bradycardia (< 40 bpm) or Severe Tachycardia (> 140 bpm)
	if vital.HeartRate != nil {
		if *vital.HeartRate < 40 {
			alerts = append(alerts, fmt.Sprintf("Severe Bradycardia: Heart rate is dangerously low (%d bpm). Seek immediate medical evaluation.", *vital.HeartRate))
		} else if *vital.HeartRate > 140 {
			alerts = append(alerts, fmt.Sprintf("Severe Tachycardia: Heart rate is dangerously high (%d bpm). Seek immediate medical evaluation.", *vital.HeartRate))
		}
	}

	// Blood Glucose: Severe Hypoglycemia (< 54 mg/dL) or Extreme Hyperglycemia (>= 350 mg/dL)
	if vital.BloodGlucose != nil {
		if *vital.BloodGlucose < 54.0 {
			alerts = append(alerts, fmt.Sprintf("Severe Hypoglycemia: Blood glucose is critically low (%.1f mg/dL). Ingest fast-acting carbohydrates immediately and seek medical attention.", *vital.BloodGlucose))
		} else if *vital.BloodGlucose >= 350.0 {
			alerts = append(alerts, fmt.Sprintf("Critical Hyperglycemia: Blood glucose is severely elevated (%.1f mg/dL). High risk of diabetic ketoacidosis. Contact healthcare provider immediately.", *vital.BloodGlucose))
		}
	}

	// Temperature: Hyperpyrexia (>= 40.0 C) or Severe Hypothermia (<= 35.0 C)
	if vital.Temperature != nil {
		if *vital.Temperature >= 40.0 {
			alerts = append(alerts, fmt.Sprintf("Hyperpyrexia: Body temperature is dangerously high (%.1f °C). Seek immediate medical care.", *vital.Temperature))
		} else if *vital.Temperature <= 35.0 {
			alerts = append(alerts, fmt.Sprintf("Severe Hypothermia: Body temperature is dangerously low (%.1f °C). Seek immediate medical care.", *vital.Temperature))
		}
	}

	return len(alerts) > 0, alerts
}

func safeIntDeref(val *int) int {
	if val == nil {
		return 0
	}
	return *val
}

// GetPatientVitals retrieves longitudinal biometric vital signs for a patient.
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

// GetPatientMedicationSchedule generates the daily adherence medication schedule for a patient.
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

	now := time.Now()
	if targetDate.Before(now.AddDate(-2, 0, 0)) || targetDate.After(now.AddDate(1, 0, 0)) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "date must be within the past 2 years and next 1 year"})
		return
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

// MedicationLogInput represents patient confirmation of a prescribed medication dose.
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

// LogMedicationAdherence records a patient medication compliance entry.
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

	now := time.Now()
	if schedDate.After(now.Add(24 * time.Hour)) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "scheduled_date cannot be more than 1 day in the future"})
		return
	}
	if schedDate.Before(now.AddDate(-2, 0, 0)) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "scheduled_date cannot be older than 2 years"})
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
	if doseNum > 24 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "dose_number cannot exceed 24 doses per day"})
		return
	}

	var takenAt *time.Time
	if in.Status == "taken" {
		if in.TakenAt != nil && !in.TakenAt.IsZero() {
			if in.TakenAt.After(now.Add(5 * time.Minute)) {
				c.JSON(http.StatusBadRequest, gin.H{"error": "taken_at cannot be in the future"})
				return
			}
			if in.TakenAt.Before(now.AddDate(-2, 0, 0)) {
				c.JSON(http.StatusBadRequest, gin.H{"error": "taken_at cannot be older than 2 years"})
				return
			}
			takenAt = in.TakenAt
		} else {
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

	if h.Notifier != nil {
		h.Notifier.Publish(notifications.NotificationEvent{
			Type:      notifications.EventMedicationLogged,
			PatientID: patientID,
			Message:   fmt.Sprintf("Medication adherence logged: %s (%s)", in.Status, in.TimeOfDay),
			Data: map[string]interface{}{
				"log_id":               saved.ID,
				"prescription_item_id": in.PrescriptionItemID,
				"status":               in.Status,
				"time_of_day":          in.TimeOfDay,
			},
		})
	}

	h.audit(c, audit.ActionLogMedicationAdherence, "schedule", &saved.ID, &patientID, http.StatusOK, map[string]interface{}{"status": in.Status, "time_of_day": in.TimeOfDay})
	c.JSON(http.StatusOK, gin.H{"data": saved})
}
