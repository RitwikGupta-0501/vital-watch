package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/RitwikGupta-0501/vital-watch/internal/audit"
	"github.com/RitwikGupta-0501/vital-watch/internal/models"
	"github.com/RitwikGupta-0501/vital-watch/internal/notifications"
	"github.com/RitwikGupta-0501/vital-watch/internal/telehealth"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// GetPatientAppointments returns a paginated list of appointments for the authenticated patient.
func (h *Handler) GetPatientAppointments(c *gin.Context) {
	userIDVal, ok := c.Get("userID")
	if !ok {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "User ID not found in context"})
		return
	}
	patientID, ok := userIDVal.(uuid.UUID)
	if !ok {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "Invalid user ID type in context"})
		return
	}

	limit, offset := parsePagination(c)
	appointments, err := h.Repo.GetAppointmentsByPatientID(c.Request.Context(), patientID, limit, offset)
	if err != nil {
		slog.ErrorContext(c.Request.Context(), "Internal error in GetPatientAppointments", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch appointments"})
		return
	}

	// Audit patient encounter history read (HIPAA § 164.312(b))
	h.audit(c, audit.ActionViewAppointments, "appointment", nil, &patientID, http.StatusOK, map[string]interface{}{
		"limit":  limit,
		"offset": offset,
		"count":  len(appointments),
	})

	c.JSON(http.StatusOK, gin.H{
		"data":   appointments,
		"limit":  limit,
		"offset": offset,
	})
}

// CreateAppointment books a new clinical appointment for the authenticated patient.
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

	userIDVal, ok := c.Get("userID")
	if !ok {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "User ID not found in context"})
		return
	}
	patientID, ok := userIDVal.(uuid.UUID)
	if !ok {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "Invalid user ID type in context"})
		return
	}

	// Self-Booking Guard
	if patientID == req.DoctorID {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Cannot book an appointment with yourself"})
		return
	}

	if req.StartTime.IsZero() || req.EndTime.IsZero() {
		c.JSON(http.StatusBadRequest, gin.H{"error": "start_time and end_time are required"})
		return
	}

	// API-04: start_time must be in the future
	now := time.Now()
	if !req.StartTime.After(now) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Appointment start time must be in the future"})
		return
	}

	// 15-minute lead time enforcement (same rule as slot availability listing)
	if !req.StartTime.After(now.Add(15 * time.Minute)) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Appointments must be booked at least 15 minutes in advance"})
		return
	}

	// Booking horizon: cannot book more than 1 year in advance
	if req.StartTime.After(now.AddDate(1, 0, 0)) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Appointment cannot be booked more than 1 year in advance"})
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

	// CRITICAL-03: Validate target doctor is active (GetDoctorByID filters is_active=true) and available.
	doc, dErr := h.Repo.GetDoctorByID(c.Request.Context(), req.DoctorID)
	if dErr != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid doctor: doctor not found or inactive"})
		return
	}
	if !doc.Available {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Doctor is not currently available for bookings"})
		return
	}

	allScheds, aErr := h.Repo.GetDoctorSchedules(c.Request.Context(), req.DoctorID)
	if aErr != nil {
		slog.ErrorContext(c.Request.Context(), "Internal error checking doctor schedules", "error", aErr)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to verify doctor schedule"})
		return
	}
	// CRITICAL-03: Zero-schedule bypass fix — require at least one schedule entry.
	if len(allScheds) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Doctor has no schedule configured"})
		return
	}

	var matchingSched *models.DoctorSchedule
	for _, s := range allScheds {
		tz := s.Timezone
		if tz == "" {
			tz = "UTC"
		}
		loc, lErr := time.LoadLocation(tz)
		if lErr != nil {
			loc = time.UTC
		}
		if int(req.StartTime.In(loc).Weekday()) == s.DayOfWeek {
			schedCopy := s
			matchingSched = &schedCopy
			break
		}
	}

	if matchingSched == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Doctor has no office hours configured on this day"})
		return
	}
	if !matchingSched.IsActive {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Doctor is not available on this day"})
		return
	}

	docTz := matchingSched.Timezone
	if docTz == "" {
		docTz = "UTC"
	}
	loc, lErr := time.LoadLocation(docTz)
	if lErr != nil {
		loc = time.UTC
	}

	// Truncate seconds/nanoseconds from requested time for slot alignment check.
	req.StartTime = req.StartTime.Truncate(time.Minute)
	req.EndTime = req.EndTime.Truncate(time.Minute)

	localStart := req.StartTime.In(loc)
	localEnd := req.EndTime.In(loc)

	stParsed, err1 := time.Parse("15:04", matchingSched.StartTime)
	etParsed, err2 := time.Parse("15:04", matchingSched.EndTime)
	if err1 == nil && err2 == nil {
		windowStart := time.Date(localStart.Year(), localStart.Month(), localStart.Day(), stParsed.Hour(), stParsed.Minute(), 0, 0, loc)
		windowEnd := time.Date(localStart.Year(), localStart.Month(), localStart.Day(), etParsed.Hour(), etParsed.Minute(), 0, 0, loc)

		if localStart.Before(windowStart) || localEnd.After(windowEnd) {
			c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("Appointment is outside doctor's active working hours (%s - %s %s)", matchingSched.StartTime, matchingSched.EndTime, matchingSched.Timezone)})
			return
		}

		// HIGH-01: Slot alignment — start and duration must be multiples of slotDuration.
		slotDur := time.Duration(matchingSched.SlotDuration) * time.Minute
		if slotDur > 0 {
			offsetFromWindow := localStart.Sub(windowStart)
			if offsetFromWindow%slotDur != 0 {
				c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("Appointment start time must align to the doctor's %d-minute slot boundaries", matchingSched.SlotDuration)})
				return
			}
			if duration%slotDur != 0 {
				c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("Appointment duration must be a multiple of the doctor's %d-minute slot duration", matchingSched.SlotDuration)})
				return
			}
		}
	}

	appointmentID := uuid.New()
	var meetingLink, meetingID string
	if req.Type == "virtual" && h.Telehealth != nil {
		room, rErr := h.Telehealth.CreateRoom(c.Request.Context(), appointmentID, req.StartTime, req.EndTime.Sub(req.StartTime))
		if rErr != nil {
			slog.WarnContext(c.Request.Context(), "Failed to create telehealth room", "error", rErr)
		} else if room != nil {
			meetingLink = room.MeetingLink
			meetingID = room.MeetingID
		}
	}

	newID, err := h.Repo.CreateAppointment(c.Request.Context(), appointmentID, patientID, req.DoctorID, req.StartTime, req.EndTime, req.Type, meetingLink, meetingID)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			if pgErr.Code == "23P01" {
				msg := "Doctor is already booked for this time slot"
				if strings.Contains(pgErr.ConstraintName, "patient") || strings.Contains(pgErr.ConstraintName, "uq_patient") {
					msg = "You already have an appointment during this time slot"
				}
				c.JSON(http.StatusConflict, gin.H{"error": msg})
				return
			}
			if pgErr.Code == "23503" {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid doctor ID: specified doctor does not exist"})
				return
			}
		}
		if strings.Contains(err.Error(), "appointments_doctor_id_tstzrange_excl") || strings.Contains(err.Error(), "conflicting key") {
			c.JSON(http.StatusConflict, gin.H{"error": "Doctor is already booked for this time slot"})
			return
		}
		if strings.Contains(err.Error(), "uq_patient_no_overlap") {
			c.JSON(http.StatusConflict, gin.H{"error": "You already have an appointment during this time slot"})
			return
		}
		if strings.Contains(err.Error(), "appointments_doctor_id_fkey") {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid doctor ID: specified doctor does not exist"})
			return
		}
		slog.ErrorContext(c.Request.Context(), "Internal error in CreateAppointment", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create appointment"})
		return
	}

	// Audit appointment creation (HIPAA § 164.312(b))
	h.audit(c, audit.ActionCreateAppointment, "appointment", &newID, &patientID, http.StatusCreated, map[string]interface{}{
		"doctor_id":  req.DoctorID.String(),
		"start_time": req.StartTime.Format(time.RFC3339),
		"end_time":   req.EndTime.Format(time.RFC3339),
		"type":       req.Type,
	})

	// SSE: notify both participants of new booking
	if h.Notifier != nil {
		event := notifications.NotificationEvent{
			Type:          notifications.EventAppointmentBooked,
			AppointmentID: newID,
			PatientID:     patientID,
			DoctorID:      req.DoctorID,
			Message:       "A new appointment has been booked",
		}
		h.Notifier.Publish(event)
	}

	resp := gin.H{"id": newID}
	if meetingLink != "" {
		resp["meeting_link"] = meetingLink
		resp["meeting_id"] = meetingID
	}
	c.JSON(http.StatusCreated, resp)
}

// GetDoctorAppointments returns a paginated list of appointments for the authenticated doctor.
func (h *Handler) GetDoctorAppointments(c *gin.Context) {
	userIDVal, ok := c.Get("userID")
	if !ok {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "User ID not found in context"})
		return
	}
	doctorID, ok := userIDVal.(uuid.UUID)
	if !ok {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "Invalid user ID type in context"})
		return
	}

	limit, offset := parsePagination(c)
	appointments, err := h.Repo.GetAppointmentsByDoctorID(c.Request.Context(), doctorID, limit, offset)
	if err != nil {
		slog.ErrorContext(c.Request.Context(), "Internal error in GetDoctorAppointments", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch appointments"})
		return
	}

	// Audit clinical appointment ledger read (HIPAA § 164.312(b))
	h.audit(c, audit.ActionViewAppointments, "appointment", nil, nil, http.StatusOK, map[string]interface{}{
		"limit":  limit,
		"offset": offset,
		"count":  len(appointments),
	})

	c.JSON(http.StatusOK, gin.H{
		"data":   appointments,
		"limit":  limit,
		"offset": offset,
	})
}

// MarkAppointmentAsCompleted updates an appointment status to completed (Assigned Doctor only).
func (h *Handler) MarkAppointmentAsCompleted(c *gin.Context) {
	userIDVal, ok := c.Get("userID")
	if !ok {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Doctor ID not found in context"})
		return
	}
	doctorID, ok := userIDVal.(uuid.UUID)
	if !ok {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "Invalid user ID type in context"})
		return
	}

	appointmentID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid appointment ID"})
		return
	}

	// Fetch the appointment and immediately enforce authorization (BOLA prevention).
	// Auth check MUST precede temporal check: checking time before owner identity
	// allows Doctor B to probe Doctor A's appointment UUIDs and learn their
	// scheduling details via the 400 vs 404 response delta.
	appt, aErr := h.Repo.GetAppointmentByID(c.Request.Context(), appointmentID)
	if aErr != nil || appt.DoctorID != doctorID {
		c.JSON(http.StatusNotFound, gin.H{"error": "Appointment not found or not assigned to you"})
		return
	}

	if appt.Status != "upcoming" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Only upcoming appointments can be completed"})
		return
	}

	if time.Now().Before(appt.StartTime) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Cannot complete an appointment that has not started yet"})
		return
	}

	updated, err := h.Repo.UpdateAppointmentAsCompletedForDoctor(c.Request.Context(), appointmentID, doctorID)
	if err != nil {
		slog.ErrorContext(c.Request.Context(), "Internal error in MarkAppointmentAsCompleted", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to mark appointment as completed"})
		return
	}

	if !updated {
		c.JSON(http.StatusNotFound, gin.H{"error": "Appointment not found or not assigned to you"})
		return
	}

	// Audit appointment completion milestone (HIPAA § 164.312(b))
	h.audit(c, audit.ActionCompleteAppointment, "appointment", &appointmentID, &appt.PatientID, http.StatusOK, map[string]interface{}{
		"doctor_id": doctorID.String(),
	})

	// SSE: notify both participants of completion
	if h.Notifier != nil {
		event := notifications.NotificationEvent{
			Type:          notifications.EventAppointmentCompleted,
			AppointmentID: appointmentID,
			PatientID:     appt.PatientID,
			DoctorID:      doctorID,
			Message:       "Appointment has been marked as completed",
		}
		h.Notifier.Publish(event)
	}

	c.JSON(http.StatusOK, gin.H{"message": "Appointment marked as completed"})
}

// GetAppointmentMeetingRoom generates or retrieves the video consultation room link.
func (h *Handler) GetAppointmentMeetingRoom(c *gin.Context) {
	idStr := c.Param("id")
	apptID, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid appointment ID"})
		return
	}

	userIDVal, ok := c.Get("userID")
	if !ok {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "User ID not found in context"})
		return
	}
	callerID, ok := userIDVal.(uuid.UUID)
	if !ok {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "Invalid user ID type in context"})
		return
	}

	appt, err := h.Repo.GetAppointmentByID(c.Request.Context(), apptID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Appointment not found"})
		return
	}

	if appt.PatientID != callerID && appt.DoctorID != callerID {
		c.JSON(http.StatusForbidden, gin.H{"error": "Access denied: you are not a participant in this appointment"})
		return
	}

	if appt.Status == "cancelled" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Appointment has been cancelled"})
		return
	}

	if appt.Type != "virtual" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Appointment is not a virtual consultation"})
		return
	}

	// Status & Temporal Guards: Reject cancelled, completed, or expired consultations
	if appt.Status == "completed" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Appointment has already been completed"})
		return
	}
	now := time.Now()
	if !appt.EndTime.IsZero() && now.After(appt.EndTime.Add(24*time.Hour)) {
		c.JSON(http.StatusGone, gin.H{"error": "Consultation window has expired"})
		return
	}

	// HIGH-04: Prevent meeting token pre-minting weeks in advance.
	// Access is only allowed within 15 minutes of appointment start.
	if !appt.StartTime.IsZero() && now.Before(appt.StartTime.Add(-15*time.Minute)) {
		c.JSON(http.StatusForbidden, gin.H{"error": "Meeting room access is only available within 15 minutes of appointment start"})
		return
	}

	if appt.MeetingLink == "" {
		if h.Telehealth != nil {
			duration := appt.EndTime.Sub(appt.StartTime)
			if duration <= 0 {
				duration = 30 * time.Minute
			}
			generator := func() (string, string, error) {
				room, rErr := h.Telehealth.CreateRoom(c.Request.Context(), appt.ID, appt.StartTime, duration)
				if rErr != nil {
					return "", "", rErr
				}
				if room == nil {
					return "", "", nil
				}
				return room.MeetingLink, room.MeetingID, nil
			}
			lockedAppt, lErr := h.Repo.GetOrGenerateAppointmentMeetingRoom(c.Request.Context(), appt.ID, generator)
			if lErr != nil {
				slog.WarnContext(c.Request.Context(), "Failed to atomically create telehealth room", "error", lErr)
			} else if lockedAppt.MeetingLink != "" {
				appt = lockedAppt
			}
		}
		if appt.MeetingLink == "" {
			c.JSON(http.StatusNotFound, gin.H{"error": "No meeting room configured for this appointment"})
			return
		}
	}

	meetingLink := appt.MeetingLink
	if tp, ok := h.Telehealth.(telehealth.TokenProvider); ok && appt.MeetingID != "" {
		isOwner := callerID == appt.DoctorID
		token, tErr := tp.CreateMeetingToken(c.Request.Context(), appt.MeetingID, isOwner, appt.EndTime.Add(30*time.Minute))
		if tErr != nil {
			slog.ErrorContext(c.Request.Context(), "Failed to generate meeting token", "error", tErr)
			c.JSON(http.StatusBadGateway, gin.H{"error": "Failed to generate meeting access token for video consultation"})
			return
		}
		if token != "" {
			separator := "?"
			if strings.Contains(meetingLink, "?") {
				separator = "&"
			}
			meetingLink = fmt.Sprintf("%s%st=%s", meetingLink, separator, token)
		}
	}

	providerName := ""
	if h.Telehealth != nil {
		providerName = h.Telehealth.Name()
	}
	h.audit(c, audit.ActionAccessMeetingRoom, "appointment", &appt.ID, &appt.PatientID, http.StatusOK, map[string]interface{}{
		"provider":   providerName,
		"meeting_id": appt.MeetingID,
	})

	c.JSON(http.StatusOK, gin.H{
		"meeting_link": meetingLink,
		"meeting_id":   appt.MeetingID,
		"status":       appt.Status,
		"start_time":   appt.StartTime,
		"end_time":     appt.EndTime,
	})
}

type ScheduleInput struct {
	DayOfWeek    int    `json:"day_of_week"`
	StartTime    string `json:"start_time"`
	EndTime      string `json:"end_time"`
	SlotDuration int    `json:"slot_duration"`
	Timezone     string `json:"timezone"`
	IsActive     *bool  `json:"is_active"`
}

func validateScheduleInput(in ScheduleInput) (models.DoctorSchedule, error) {
	if in.DayOfWeek < 0 || in.DayOfWeek > 6 {
		return models.DoctorSchedule{}, errors.New("day_of_week must be between 0 (Sunday) and 6 (Saturday)")
	}
	in.StartTime = strings.TrimSpace(in.StartTime)
	in.EndTime = strings.TrimSpace(in.EndTime)
	if in.StartTime == "" || in.EndTime == "" {
		return models.DoctorSchedule{}, errors.New("start_time and end_time are required (format HH:MM)")
	}

	st, err := time.Parse("15:04", in.StartTime)
	if err != nil {
		return models.DoctorSchedule{}, fmt.Errorf("invalid start_time format (must be HH:MM): %w", err)
	}
	et, err := time.Parse("15:04", in.EndTime)
	if err != nil {
		return models.DoctorSchedule{}, fmt.Errorf("invalid end_time format (must be HH:MM): %w", err)
	}
	if !et.After(st) {
		return models.DoctorSchedule{}, errors.New("end_time must be after start_time")
	}

	if in.SlotDuration == 0 {
		in.SlotDuration = 30
	}
	allowedDurations := map[int]bool{15: true, 20: true, 30: true, 45: true, 60: true}
	if !allowedDurations[in.SlotDuration] {
		return models.DoctorSchedule{}, errors.New("slot_duration must be 15, 20, 30, 45, or 60 minutes")
	}

	windowDuration := et.Sub(st)
	if windowDuration < time.Duration(in.SlotDuration)*time.Minute {
		return models.DoctorSchedule{}, fmt.Errorf("working window duration (%v) must be at least equal to slot_duration (%d minutes)", windowDuration, in.SlotDuration)
	}

	tz := strings.TrimSpace(in.Timezone)
	if tz == "" {
		tz = "UTC"
	}
	if _, err := time.LoadLocation(tz); err != nil {
		return models.DoctorSchedule{}, fmt.Errorf("invalid timezone: %w", err)
	}

	active := true
	if in.IsActive != nil {
		active = *in.IsActive
	}

	return models.DoctorSchedule{
		DayOfWeek:    in.DayOfWeek,
		StartTime:    in.StartTime,
		EndTime:      in.EndTime,
		SlotDuration: in.SlotDuration,
		Timezone:     tz,
		IsActive:     active,
	}, nil
}

// UpsertDoctorSchedule configures recurring office hours for the authenticated doctor.
func (h *Handler) UpsertDoctorSchedule(c *gin.Context) {
	userIDVal, ok := c.Get("userID")
	if !ok {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "User ID not found in context"})
		return
	}
	doctorID, ok := userIDVal.(uuid.UUID)
	if !ok {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "Invalid user ID type in context"})
		return
	}

	bodyBytes, err := c.GetRawData()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to read request body"})
		return
	}

	var inputs []ScheduleInput
	if err := json.Unmarshal(bodyBytes, &inputs); err != nil {
		var single ScheduleInput
		if errSingle := json.Unmarshal(bodyBytes, &single); errSingle != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body: expected schedule object or array of schedules"})
			return
		}
		inputs = []ScheduleInput{single}
	}

	if len(inputs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "At least one schedule item is required"})
		return
	}

	var validated []models.DoctorSchedule
	seenDays := make(map[int]bool)
	for idx, in := range inputs {
		if seenDays[in.DayOfWeek] {
			c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("Duplicate schedule entry for day_of_week %d", in.DayOfWeek)})
			return
		}
		seenDays[in.DayOfWeek] = true

		validSched, err := validateScheduleInput(in)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("Schedule [%d]: %s", idx, err.Error())})
			return
		}
		validSched.DoctorID = doctorID
		validated = append(validated, validSched)
	}

	saved, err := h.Repo.UpsertDoctorSchedulesTx(c.Request.Context(), doctorID, validated)
	if err != nil {
		slog.ErrorContext(c.Request.Context(), "Internal error in UpsertDoctorSchedule", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save schedule"})
		return
	}

	// Audit schedule configuration change
	h.audit(c, audit.ActionUpdateSchedule, "doctor_schedule", nil, nil, http.StatusOK, map[string]interface{}{
		"doctor_id": doctorID.String(),
		"count":     len(saved),
	})

	c.JSON(http.StatusOK, gin.H{"data": saved})
}

// GetDoctorSchedules retrieves all configured office hour schedules for the authenticated doctor.
func (h *Handler) GetDoctorSchedules(c *gin.Context) {
	userIDVal, ok := c.Get("userID")
	if !ok {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "User ID not found in context"})
		return
	}
	doctorID, ok := userIDVal.(uuid.UUID)
	if !ok {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "Invalid user ID type in context"})
		return
	}

	schedules, err := h.Repo.GetDoctorSchedules(c.Request.Context(), doctorID)
	if err != nil {
		slog.ErrorContext(c.Request.Context(), "Internal error in GetDoctorSchedules", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve schedules"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": schedules})
}

// DeleteDoctorSchedule removes office hours for a given day of the week (0=Sunday to 6=Saturday).
func (h *Handler) DeleteDoctorSchedule(c *gin.Context) {
	userIDVal, ok := c.Get("userID")
	if !ok {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "User ID not found in context"})
		return
	}
	doctorID, ok := userIDVal.(uuid.UUID)
	if !ok {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "Invalid user ID type in context"})
		return
	}

	dayStr := c.Param("day")
	day, err := strconv.Atoi(dayStr)
	if err != nil || day < 0 || day > 6 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid day parameter: must be between 0 (Sunday) and 6 (Saturday)"})
		return
	}

	if err := h.Repo.DeleteDoctorScheduleByDay(c.Request.Context(), doctorID, day); err != nil {
		slog.ErrorContext(c.Request.Context(), "Internal error in DeleteDoctorSchedule", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete schedule"})
		return
	}

	h.audit(c, audit.ActionUpdateSchedule, "doctor_schedule", nil, nil, http.StatusOK, map[string]interface{}{
		"doctor_id": doctorID.String(),
		"day":       day,
	})

	c.JSON(http.StatusOK, gin.H{"message": "Doctor schedule deleted successfully"})
}

// GetDoctorAvailableSlots returns open consultation slots for a doctor on a specific date.
func (h *Handler) GetDoctorAvailableSlots(c *gin.Context) {
	idStr := c.Param("id")
	doctorID, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid doctor ID"})
		return
	}

	dateStr := strings.TrimSpace(c.Query("date"))
	if dateStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "date query parameter is required (format: YYYY-MM-DD)"})
		return
	}

	targetDate, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid date format: must be YYYY-MM-DD"})
		return
	}

	// CRITICAL-03: Validate that the doctor is active and available before showing slots.
	doc, dErr := h.Repo.GetDoctorByID(c.Request.Context(), doctorID)
	if dErr != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Doctor not found or inactive"})
		return
	}
	if !doc.Available {
		c.JSON(http.StatusOK, gin.H{
			"date":        dateStr,
			"day_of_week": int(targetDate.Weekday()),
			"slots":       []models.TimeSlot{},
			"message":     "Doctor is not currently available for bookings",
		})
		return
	}

	dayOfWeek := int(targetDate.Weekday())

	schedule, err := h.Repo.GetDoctorScheduleByDay(c.Request.Context(), doctorID, dayOfWeek)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			c.JSON(http.StatusOK, gin.H{
				"date":        dateStr,
				"day_of_week": dayOfWeek,
				"slots":       []models.TimeSlot{},
				"message":     "Doctor is not available on this day",
			})
			return
		}
		slog.ErrorContext(c.Request.Context(), "Internal error in GetDoctorScheduleByDay", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to check doctor schedule"})
		return
	}
	if !schedule.IsActive {
		c.JSON(http.StatusOK, gin.H{
			"date":        dateStr,
			"day_of_week": dayOfWeek,
			"slots":       []models.TimeSlot{},
			"message":     "Doctor is not available on this day",
		})
		return
	}

	loc, err := time.LoadLocation(schedule.Timezone)
	if err != nil {
		loc = time.UTC
	}

	stParsed, err := time.Parse("15:04", schedule.StartTime)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to parse doctor start time"})
		return
	}
	etParsed, err := time.Parse("15:04", schedule.EndTime)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to parse doctor end time"})
		return
	}

	windowStart := time.Date(targetDate.Year(), targetDate.Month(), targetDate.Day(), stParsed.Hour(), stParsed.Minute(), 0, 0, loc)
	windowEnd := time.Date(targetDate.Year(), targetDate.Month(), targetDate.Day(), etParsed.Hour(), etParsed.Minute(), 0, 0, loc)

	bookedAppts, err := h.Repo.GetDoctorAppointmentsInRange(c.Request.Context(), doctorID, windowStart, windowEnd)
	if err != nil {
		slog.ErrorContext(c.Request.Context(), "Internal error fetching doctor appointments in range", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to check doctor bookings"})
		return
	}

	slotDuration := time.Duration(schedule.SlotDuration) * time.Minute
	if slotDuration <= 0 {
		slotDuration = 30 * time.Minute
	}

	now := time.Now()
	minLeadTime := now.Add(15 * time.Minute)
	slots := make([]models.TimeSlot, 0)

	for curr := windowStart; curr.Add(slotDuration).Before(windowEnd) || curr.Add(slotDuration).Equal(windowEnd); curr = curr.Add(slotDuration) {
		slotStart := curr
		slotEnd := curr.Add(slotDuration)

		available := slotStart.After(minLeadTime)
		if available {
			for _, b := range bookedAppts {
				if b.StartTime.Before(slotEnd) && b.EndTime.After(slotStart) {
					available = false
					break
				}
			}
		}

		slots = append(slots, models.TimeSlot{
			StartTime: slotStart,
			EndTime:   slotEnd,
			Available: available,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"date":          dateStr,
		"day_of_week":   dayOfWeek,
		"slot_duration": schedule.SlotDuration,
		"timezone":      schedule.Timezone,
		"slots":         slots,
	})
}

// CancelAppointment allows any participant (patient or doctor) to cancel an upcoming appointment.
func (h *Handler) CancelAppointment(c *gin.Context) {
	userIDVal, ok := c.Get("userID")
	if !ok {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "User ID not found in context"})
		return
	}
	callerID, ok := userIDVal.(uuid.UUID)
	if !ok {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "Invalid user ID type in context"})
		return
	}

	appointmentID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid appointment ID"})
		return
	}

	// Fetch appointment first for audit data and existence verification.
	appt, aErr := h.Repo.GetAppointmentByID(c.Request.Context(), appointmentID)
	if aErr != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Appointment not found"})
		return
	}

	// Verify caller is a participant.
	if appt.PatientID != callerID && appt.DoctorID != callerID {
		c.JSON(http.StatusNotFound, gin.H{"error": "Appointment not found"})
		return
	}

	cancelled, err := h.Repo.CancelAppointmentByParticipant(c.Request.Context(), appointmentID, callerID)
	if err != nil {
		slog.ErrorContext(c.Request.Context(), "Internal error in CancelAppointment", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to cancel appointment"})
		return
	}
	if !cancelled {
		// Row was found but status != 'upcoming' — already completed or already cancelled.
		c.JSON(http.StatusBadRequest, gin.H{"error": "Only upcoming appointments can be cancelled"})
		return
	}

	// Audit appointment cancellation (HIPAA § 164.312(b))
	h.audit(c, audit.ActionCancelAppointment, "appointment", &appointmentID, &appt.PatientID, http.StatusOK, map[string]interface{}{
		"cancelled_by": callerID.String(),
		"doctor_id":    appt.DoctorID.String(),
	})

	// SSE: notify both participants of cancellation
	if h.Notifier != nil {
		event := notifications.NotificationEvent{
			Type:          notifications.EventAppointmentCancelled,
			AppointmentID: appointmentID,
			PatientID:     appt.PatientID,
			DoctorID:      appt.DoctorID,
			Message:       "Appointment has been cancelled",
		}
		h.Notifier.Publish(event)
	}

	c.JSON(http.StatusOK, gin.H{"message": "Appointment cancelled successfully"})
}
