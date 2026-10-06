package api

import (
	"bytes"
	"strings"
	"sync"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/RitwikGupta-0501/vital-watch/internal/audit"
	"github.com/RitwikGupta-0501/vital-watch/internal/models"
	"github.com/RitwikGupta-0501/vital-watch/internal/notifications"
	"github.com/RitwikGupta-0501/vital-watch/internal/repository"
	"github.com/RitwikGupta-0501/vital-watch/internal/storage"
	"github.com/RitwikGupta-0501/vital-watch/internal/telehealth"
)

func setupPhase4TestRouter(h *Handler) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()

	r.Use(func(c *gin.Context) {
		// Mock auth context from headers if provided
		if uidStr := c.GetHeader("X-User-ID"); uidStr != "" {
			if uid, err := uuid.Parse(uidStr); err == nil {
				c.Set("userID", uid)
			}
		}
		if role := c.GetHeader("X-Role"); role != "" {
			c.Set("role", role)
		}
		c.Next()
	})

	r.POST("/api/appointments", h.CreateAppointment)
	r.GET("/api/appointments/:id/meeting-room", h.GetAppointmentMeetingRoom)
	r.PUT("/api/doctor/schedules", h.UpsertDoctorSchedule)
	r.GET("/api/doctor/schedules", h.GetDoctorSchedules)
	r.GET("/api/doctors/:id/available-slots", h.GetDoctorAvailableSlots)
	r.POST("/api/vitals", h.CreatePatientVital)
	r.GET("/api/vitals", h.GetPatientVitals)
	r.GET("/api/patients/medication-schedule", h.GetPatientMedicationSchedule)
	r.POST("/api/patients/medication-schedule/log", h.LogMedicationAdherence)
	r.DELETE("/api/doctor/schedules/:day", h.DeleteDoctorSchedule)

	return r
}

func TestTelehealthVirtualAppointmentFlow(t *testing.T) {
	patientID := uuid.New()
	doctorID := uuid.New()
	apptID := uuid.New()

	mockTelehealth := &telehealth.MockProvider{
		CustomID:   "room-xyz",
		CustomLink: "https://telehealth.vitalwatch.local/room-xyz",
	}

	var savedMeetingLink string
	var savedApptID uuid.UUID
	mockRepo := &repository.MockRepository{
		CreateAppointmentFunc: func(ctx context.Context, id, pID, dID uuid.UUID, start, end time.Time, apptType, meetingLink, meetingID string) (uuid.UUID, error) {
			savedMeetingLink = meetingLink
			savedApptID = id
			return id, nil
		},
		GetAppointmentByIDFunc: func(ctx context.Context, id uuid.UUID) (models.Appointment, error) {
			return models.Appointment{
				ID:          apptID,
				PatientID:   patientID,
				DoctorID:    doctorID,
				Type:        "virtual",
				MeetingLink: "https://telehealth.vitalwatch.local/room-xyz",
				MeetingID:   "room-xyz",
				Status:      "upcoming",
			}, nil
		},
		GetDoctorByIDFunc: func(ctx context.Context, id uuid.UUID) (models.Doctor, error) {
			return models.Doctor{ID: id, Available: true}, nil
		},
		GetDoctorSchedulesFunc: func(ctx context.Context, dID uuid.UUID) ([]models.DoctorSchedule, error) {
			return []models.DoctorSchedule{
				{
					DoctorID:     doctorID,
					DayOfWeek:    int(time.Now().Weekday()),
					StartTime:    "00:00",
					EndTime:      "23:59",
					SlotDuration: 15,
					IsActive:     true,
					Timezone:     "UTC",
				},
			}, nil
		},
	}

	h := &Handler{
		Repo:       mockRepo,
		Storage:    storage.NewMockProvider(),
		Telehealth: mockTelehealth,
	}

	r := setupPhase4TestRouter(h)

	// 1. Book Virtual Appointment
	nowSlot := time.Now().Truncate(time.Hour)
	reqBody := map[string]interface{}{
		"doctor_id":  doctorID.String(),
		"start_time": nowSlot.Add(2 * time.Hour).Format(time.RFC3339),
		"end_time":   nowSlot.Add(2*time.Hour + 30*time.Minute).Format(time.RFC3339),
		"type":       "virtual",
	}
	bodyBytes, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/api/appointments", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-User-ID", patientID.String())
	req.Header.Set("X-Role", "patient")

	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created, got %d: %s", w.Code, w.Body.String())
	}
	if savedMeetingLink != "https://telehealth.vitalwatch.local/room-xyz" {
		t.Errorf("expected meeting link saved, got %s", savedMeetingLink)
	}
	if savedApptID == uuid.Nil {
		t.Errorf("expected appointment ID to be non-nil")
	}

	// 2. Fetch Meeting Room as Patient
	reqRoom := httptest.NewRequest(http.MethodGet, "/api/appointments/"+apptID.String()+"/meeting-room", nil)
	reqRoom.Header.Set("X-User-ID", patientID.String())
	wRoom := httptest.NewRecorder()
	r.ServeHTTP(wRoom, reqRoom)

	if wRoom.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for patient room fetch, got %d", wRoom.Code)
	}

	// 3. Fetch Meeting Room as Unauthorized Third Party
	otherID := uuid.New()
	reqForbidden := httptest.NewRequest(http.MethodGet, "/api/appointments/"+apptID.String()+"/meeting-room", nil)
	reqForbidden.Header.Set("X-User-ID", otherID.String())
	wForbidden := httptest.NewRecorder()
	r.ServeHTTP(wForbidden, reqForbidden)

	if wForbidden.Code != http.StatusForbidden {
		t.Errorf("expected 403 Forbidden for stranger, got %d", wForbidden.Code)
	}

	// 4. Cancelled Appointment Denies Meeting Access
	cancelledID := uuid.New()
	mockRepo.GetAppointmentByIDFunc = func(ctx context.Context, id uuid.UUID) (models.Appointment, error) {
		return models.Appointment{
			ID:          cancelledID,
			PatientID:   patientID,
			DoctorID:    doctorID,
			Type:        "virtual",
			MeetingLink: "https://telehealth.vitalwatch.local/room-xyz",
			Status:      "cancelled",
		}, nil
	}
	reqCancelled := httptest.NewRequest(http.MethodGet, "/api/appointments/"+cancelledID.String()+"/meeting-room", nil)
	reqCancelled.Header.Set("X-User-ID", patientID.String())
	wCancelled := httptest.NewRecorder()
	r.ServeHTTP(wCancelled, reqCancelled)
	if wCancelled.Code != http.StatusBadRequest {
		t.Errorf("expected 400 Bad Request for cancelled appointment, got %d", wCancelled.Code)
	}
}

func TestDoctorScheduleAndAvailableSlots(t *testing.T) {
	doctorID := uuid.New()

	activeSchedule := models.DoctorSchedule{
		ID:           uuid.New(),
		DoctorID:     doctorID,
		DayOfWeek:    1, // Monday
		StartTime:    "09:00",
		EndTime:      "11:00",
		SlotDuration: 30,
		Timezone:     "UTC",
		IsActive:     true,
	}

	// Dynamic future Monday to ensure slots are not in the past
	targetMonday := time.Now().AddDate(0, 0, (8-int(time.Now().Weekday()))%7)
	if targetMonday.Before(time.Now().Add(24 * time.Hour)) {
		targetMonday = targetMonday.AddDate(0, 0, 7)
	}
	targetMonday = time.Date(targetMonday.Year(), targetMonday.Month(), targetMonday.Day(), 0, 0, 0, 0, time.UTC)

	mockRepo := &repository.MockRepository{
		UpsertDoctorScheduleFunc: func(ctx context.Context, s models.DoctorSchedule) (models.DoctorSchedule, error) {
			return s, nil
		},
		GetDoctorScheduleByDayFunc: func(ctx context.Context, dID uuid.UUID, day int) (models.DoctorSchedule, error) {
			if day == 1 {
				return activeSchedule, nil
			}
			return models.DoctorSchedule{IsActive: false}, nil
		},
		GetDoctorAppointmentsInRangeFunc: func(ctx context.Context, dID uuid.UUID, start, end time.Time) ([]models.Appointment, error) {
			// One existing appointment from 09:30 to 10:00
			return []models.Appointment{
				{
					StartTime: time.Date(targetMonday.Year(), targetMonday.Month(), targetMonday.Day(), 9, 30, 0, 0, time.UTC),
					EndTime:   time.Date(targetMonday.Year(), targetMonday.Month(), targetMonday.Day(), 10, 0, 0, 0, time.UTC),
					Status:    "upcoming",
				},
			}, nil
		},
		GetDoctorByIDFunc: func(ctx context.Context, id uuid.UUID) (models.Doctor, error) {
			return models.Doctor{ID: id, Available: true}, nil
		},
	}

	h := &Handler{
		Repo: mockRepo,
	}
	r := setupPhase4TestRouter(h)

	// 1. Configure Doctor Schedule
	schedPayload := []map[string]interface{}{
		{
			"day_of_week":   1,
			"start_time":    "09:00",
			"end_time":      "11:00",
			"slot_duration": 30,
			"timezone":      "UTC",
			"is_active":     true,
		},
	}
	bodyBytes, _ := json.Marshal(schedPayload)
	req := httptest.NewRequest(http.MethodPut, "/api/doctor/schedules", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-User-ID", doctorID.String())
	req.Header.Set("X-Role", "doctor")

	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK saving schedule, got %d: %s", w.Code, w.Body.String())
	}

	// 2. Query Available Slots for that Monday
	slotReq := httptest.NewRequest(http.MethodGet, "/api/doctors/"+doctorID.String()+"/available-slots?date="+targetMonday.Format("2006-01-02"), nil)
	wSlot := httptest.NewRecorder()
	r.ServeHTTP(wSlot, slotReq)

	if wSlot.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for available slots, got %d", wSlot.Code)
	}

	var slotResp struct {
		Slots []models.TimeSlot `json:"slots"`
	}
	json.Unmarshal(wSlot.Body.Bytes(), &slotResp)

	// Total slots from 09:00 to 11:00 with 30m slots: 4 slots
	// 09:00-09:30 (Available), 09:30-10:00 (Booked/Unavailable), 10:00-10:30 (Available), 10:30-11:00 (Available)
	if len(slotResp.Slots) != 4 {
		t.Fatalf("expected 4 slots, got %d", len(slotResp.Slots))
	}

	if !slotResp.Slots[0].Available {
		t.Errorf("expected slot 0 (09:00-09:30) to be available")
	}
	if slotResp.Slots[1].Available {
		t.Errorf("expected slot 1 (09:30-10:00) to be booked/unavailable")
	}
	if !slotResp.Slots[2].Available {
		t.Errorf("expected slot 2 (10:00-10:30) to be available")
	}

	// 3. Batch Pre-validation: Invalid item stops any persistence
	var upsertCallCount int
	mockRepo.UpsertDoctorScheduleFunc = func(ctx context.Context, s models.DoctorSchedule) (models.DoctorSchedule, error) {
		upsertCallCount++
		return s, nil
	}
	invalidBatch := []map[string]interface{}{
		{
			"day_of_week":   2,
			"start_time":    "09:00",
			"end_time":      "12:00",
			"slot_duration": 30,
		},
		{
			"day_of_week":   3,
			"start_time":    "invalid_time",
			"end_time":      "12:00",
			"slot_duration": 30,
		},
	}
	invBytes, _ := json.Marshal(invalidBatch)
	reqInv := httptest.NewRequest(http.MethodPut, "/api/doctor/schedules", bytes.NewReader(invBytes))
	reqInv.Header.Set("Content-Type", "application/json")
	reqInv.Header.Set("X-User-ID", doctorID.String())
	reqInv.Header.Set("X-Role", "doctor")
	wInv := httptest.NewRecorder()
	r.ServeHTTP(wInv, reqInv)
	if wInv.Code != http.StatusBadRequest {
		t.Errorf("expected 400 Bad Request for invalid batch schedule, got %d", wInv.Code)
	}
	if upsertCallCount != 0 {
		t.Errorf("expected 0 upsert calls due to pre-validation failure, got %d", upsertCallCount)
	}
}

func TestPatientVitalsRecordingAndValidation(t *testing.T) {
	patientID := uuid.New()
	vitalID := uuid.New()

	var savedVital models.PatientVital
	mockRepo := &repository.MockRepository{
		CreatePatientVitalFunc: func(ctx context.Context, v models.PatientVital) (uuid.UUID, error) {
			savedVital = v
			return vitalID, nil
		},
		GetPatientVitalsFunc: func(ctx context.Context, pID uuid.UUID, start, end *time.Time, limit, offset int) ([]models.PatientVital, error) {
			return []models.PatientVital{savedVital}, nil
		},
	}

	h := &Handler{
		Repo: mockRepo,
	}
	r := setupPhase4TestRouter(h)

	// 1. Invalid Vitals: Systolic <= Diastolic
	invalidBody := map[string]interface{}{
		"systolic_bp":  80,
		"diastolic_bp": 120, // Error: diastolic > systolic
	}
	b, _ := json.Marshal(invalidBody)
	req := httptest.NewRequest(http.MethodPost, "/api/vitals", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-User-ID", patientID.String())
	req.Header.Set("X-Role", "patient")

	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 Bad Request for systolic <= diastolic, got %d", w.Code)
	}

	// 2. Valid Vitals
	sys := 120
	dia := 80
	hr := 72
	gluc := 95.5
	validBody := map[string]interface{}{
		"systolic_bp":   sys,
		"diastolic_bp":  dia,
		"heart_rate":    hr,
		"blood_glucose": gluc,
		"notes":         "Routine morning test",
	}
	bValid, _ := json.Marshal(validBody)
	reqValid := httptest.NewRequest(http.MethodPost, "/api/vitals", bytes.NewReader(bValid))
	reqValid.Header.Set("Content-Type", "application/json")
	reqValid.Header.Set("X-User-ID", patientID.String())
	reqValid.Header.Set("X-Role", "patient")

	wValid := httptest.NewRecorder()
	r.ServeHTTP(wValid, reqValid)
	if wValid.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created for valid vitals, got %d: %s", wValid.Code, wValid.Body.String())
	}

	// 3. Get Vitals
	getReq := httptest.NewRequest(http.MethodGet, "/api/vitals", nil)
	getReq.Header.Set("X-User-ID", patientID.String())
	getReq.Header.Set("X-Role", "patient")
	wGet := httptest.NewRecorder()
	r.ServeHTTP(wGet, getReq)

	if wGet.Code != http.StatusOK {
		t.Fatalf("expected 200 OK retrieving vitals, got %d", wGet.Code)
	}

	// 4. Doctor without relationship rejected from viewing or recording vitals
	doctorID := uuid.New()
	mockRepo.HasDoctorPatientRelationshipFunc = func(ctx context.Context, dID, pID uuid.UUID) (bool, error) {
		return false, nil // No relationship
	}
	reqDocVitals := httptest.NewRequest(http.MethodGet, "/api/vitals?patient_id="+patientID.String(), nil)
	reqDocVitals.Header.Set("X-User-ID", doctorID.String())
	reqDocVitals.Header.Set("X-Role", "doctor")
	wDocVitals := httptest.NewRecorder()
	r.ServeHTTP(wDocVitals, reqDocVitals)
	if wDocVitals.Code != http.StatusForbidden {
		t.Errorf("expected 403 Forbidden for doctor without relationship, got %d", wDocVitals.Code)
	}

	// 5. Reject NaN or infinite measurements
	reqNaNBody := map[string]interface{}{
		"blood_glucose": "NaN",
	}
	bNaN, _ := json.Marshal(reqNaNBody)
	reqNaN := httptest.NewRequest(http.MethodPost, "/api/vitals", bytes.NewReader(bNaN))
	reqNaN.Header.Set("Content-Type", "application/json")
	reqNaN.Header.Set("X-User-ID", patientID.String())
	reqNaN.Header.Set("X-Role", "patient")
	wNaN := httptest.NewRecorder()
	r.ServeHTTP(wNaN, reqNaN)
	if wNaN.Code != http.StatusBadRequest {
		t.Errorf("expected 400 Bad Request for NaN vitals, got %d", wNaN.Code)
	}

	// 6. Reject future recorded_at
	futureTime := time.Now().Add(10 * time.Minute)
	futureBody := map[string]interface{}{
		"systolic_bp":  120,
		"diastolic_bp": 80,
		"recorded_at":  futureTime.Format(time.RFC3339),
	}
	bFut, _ := json.Marshal(futureBody)
	reqFut := httptest.NewRequest(http.MethodPost, "/api/vitals", bytes.NewReader(bFut))
	reqFut.Header.Set("Content-Type", "application/json")
	reqFut.Header.Set("X-User-ID", patientID.String())
	reqFut.Header.Set("X-Role", "patient")
	wFut := httptest.NewRecorder()
	r.ServeHTTP(wFut, reqFut)
	if wFut.Code != http.StatusBadRequest {
		t.Errorf("expected 400 Bad Request for future recorded_at, got %d", wFut.Code)
	}
}

func TestMedicationScheduleAndAdherenceLogging(t *testing.T) {
	patientID := uuid.New()
	rxItemID := uuid.New()

	mockRepo := &repository.MockRepository{
		GetActivePrescriptionItemsForPatientFunc: func(ctx context.Context, pID uuid.UUID, targetDate time.Time) ([]models.PrescriptionItem, error) {
			return []models.PrescriptionItem{
				{
					ID:             rxItemID,
					MedicationName: "Metformin",
					Dosage:         "500mg",
					Frequency:      "twice daily",
					Timing:         "with meals",
				},
			}, nil
		},
		GetMedicationLogsByDateFunc: func(ctx context.Context, pID uuid.UUID, d time.Time) ([]models.MedicationLog, error) {
			return []models.MedicationLog{}, nil
		},
		UpsertMedicationLogFunc: func(ctx context.Context, log models.MedicationLog) (models.MedicationLog, error) {
			log.ID = uuid.New()
			return log, nil
		},
	}

	h := &Handler{
		Repo: mockRepo,
	}
	r := setupPhase4TestRouter(h)

	// 1. Get Schedule for today
	req := httptest.NewRequest(http.MethodGet, "/api/patients/medication-schedule?date=2026-09-22", nil)
	req.Header.Set("X-User-ID", patientID.String())
	req.Header.Set("X-Role", "patient")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK fetching schedule, got %d", w.Code)
	}

	var res struct {
		Schedule []models.MedicationLog `json:"schedule"`
	}
	json.Unmarshal(w.Body.Bytes(), &res)
	if len(res.Schedule) != 2 {
		t.Fatalf("expected 2 doses (morning, evening) for twice daily, got %d", len(res.Schedule))
	}

	// 2. Log Dose as Taken
	logPayload := map[string]interface{}{
		"prescription_item_id": rxItemID.String(),
		"scheduled_date":       "2026-09-22",
		"time_of_day":          "morning",
		"status":               "taken",
		"notes":                "Taken with breakfast",
	}
	b, _ := json.Marshal(logPayload)
	reqLog := httptest.NewRequest(http.MethodPost, "/api/patients/medication-schedule/log", bytes.NewReader(b))
	reqLog.Header.Set("Content-Type", "application/json")
	reqLog.Header.Set("X-User-ID", patientID.String())
	reqLog.Header.Set("X-Role", "patient")

	wLog := httptest.NewRecorder()
	r.ServeHTTP(wLog, reqLog)

	if wLog.Code != http.StatusOK {
		t.Fatalf("expected 200 OK logging medication adherence, got %d: %s", wLog.Code, wLog.Body.String())
	}

	// 3. Reject Logging Adherence for Unowned Prescription Item
	mockRepo.VerifyPrescriptionItemOwnershipFunc = func(ctx context.Context, itemID, pID uuid.UUID) (bool, error) {
		return false, nil // Not owner
	}
	unownedPayload := map[string]interface{}{
		"prescription_item_id": uuid.New().String(),
		"scheduled_date":       "2026-09-22",
		"time_of_day":          "morning",
		"status":               "taken",
	}
	bUnowned, _ := json.Marshal(unownedPayload)
	reqUnowned := httptest.NewRequest(http.MethodPost, "/api/patients/medication-schedule/log", bytes.NewReader(bUnowned))
	reqUnowned.Header.Set("Content-Type", "application/json")
	reqUnowned.Header.Set("X-User-ID", patientID.String())
	reqUnowned.Header.Set("X-Role", "patient")
	wUnowned := httptest.NewRecorder()
	r.ServeHTTP(wUnowned, reqUnowned)
	if wUnowned.Code != http.StatusForbidden {
		t.Errorf("expected 403 Forbidden logging adherence for unowned medication item, got %d", wUnowned.Code)
	}

	// Reset ownership to true for subsequent validation tests
	mockRepo.VerifyPrescriptionItemOwnershipFunc = func(ctx context.Context, itemID, pID uuid.UUID) (bool, error) {
		return true, nil
	}

	// 4. Reject future taken_at
	futureTaken := time.Now().Add(10 * time.Minute)
	futTakenPayload := map[string]interface{}{
		"prescription_item_id": rxItemID.String(),
		"scheduled_date":       "2026-09-22",
		"time_of_day":          "morning",
		"status":               "taken",
		"taken_at":             futureTaken.Format(time.RFC3339),
	}
	bFutTaken, _ := json.Marshal(futTakenPayload)
	reqFutTaken := httptest.NewRequest(http.MethodPost, "/api/patients/medication-schedule/log", bytes.NewReader(bFutTaken))
	reqFutTaken.Header.Set("Content-Type", "application/json")
	reqFutTaken.Header.Set("X-User-ID", patientID.String())
	reqFutTaken.Header.Set("X-Role", "patient")
	wFutTaken := httptest.NewRecorder()
	r.ServeHTTP(wFutTaken, reqFutTaken)
	if wFutTaken.Code != http.StatusBadRequest {
		t.Errorf("expected 400 Bad Request for future taken_at, got %d", wFutTaken.Code)
	}

	// 5. Reject future scheduled_date
	futSchedPayload := map[string]interface{}{
		"prescription_item_id": rxItemID.String(),
		"scheduled_date":       time.Now().Add(48 * time.Hour).Format("2006-01-02"),
		"time_of_day":          "morning",
		"status":               "taken",
	}
	bFutSched, _ := json.Marshal(futSchedPayload)
	reqFutSched := httptest.NewRequest(http.MethodPost, "/api/patients/medication-schedule/log", bytes.NewReader(bFutSched))
	reqFutSched.Header.Set("Content-Type", "application/json")
	reqFutSched.Header.Set("X-User-ID", patientID.String())
	reqFutSched.Header.Set("X-Role", "patient")
	wFutSched := httptest.NewRecorder()
	r.ServeHTTP(wFutSched, reqFutSched)
	if wFutSched.Code != http.StatusBadRequest {
		t.Errorf("expected 400 Bad Request for future scheduled_date, got %d", wFutSched.Code)
	}
}

func TestTelehealthVirtualAppointmentFlow_LazyRoomGeneration(t *testing.T) {
	patientID := uuid.New()
	doctorID := uuid.New()
	apptID := uuid.New()

	var updatedLink, updatedID string
	mockRepo := &repository.MockRepository{
		GetAppointmentByIDFunc: func(ctx context.Context, id uuid.UUID) (models.Appointment, error) {
			return models.Appointment{
				ID:          apptID,
				PatientID:   patientID,
				DoctorID:    doctorID,
				Type:        "virtual",
				MeetingLink: "", // Initially empty (e.g. earlier provider failure)
				MeetingID:   "",
				Status:      "upcoming",
				StartTime:   time.Now().Add(10 * time.Minute),
				EndTime:     time.Now().Add(40 * time.Minute),
			}, nil
		},
		UpdateAppointmentMeetingRoomFunc: func(ctx context.Context, id uuid.UUID, link, meetingID string) error {
			updatedLink = link
			updatedID = meetingID
			return nil
		},
	}

	mockTelehealth := &telehealth.MockProvider{
		CustomID:   "lazy-room-456",
		CustomLink: "https://telehealth.vitalwatch.local/lazy-room-456",
	}

	h := &Handler{
		Repo:       mockRepo,
		Telehealth: mockTelehealth,
	}
	r := setupPhase4TestRouter(h)

	// Fetching room should trigger lazy creation and return the link
	req := httptest.NewRequest(http.MethodGet, "/api/appointments/"+apptID.String()+"/meeting-room", nil)
	req.Header.Set("X-User-ID", patientID.String())
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for lazy room generation, got %d: %s", w.Code, w.Body.String())
	}
	if updatedLink != "https://telehealth.vitalwatch.local/lazy-room-456" {
		t.Errorf("expected updatedLink to be saved, got %s", updatedLink)
	}
	if updatedID != "lazy-room-456" {
		t.Errorf("expected updatedID to be saved, got %s", updatedID)
	}
}

func TestCreateAppointment_DoctorScheduleEnforcement(t *testing.T) {
	patientID := uuid.New()
	doctorID := uuid.New()

	mockRepo := &repository.MockRepository{
		GetDoctorScheduleByDayFunc: func(ctx context.Context, dID uuid.UUID, day int) (models.DoctorSchedule, error) {
			if day == 1 { // Monday
				return models.DoctorSchedule{
					DoctorID:     doctorID,
					DayOfWeek:    1,
					StartTime:    "09:00",
					EndTime:      "17:00",
					SlotDuration: 30,
					Timezone:     "UTC",
					IsActive:     true,
				}, nil
			}
			// Other days doctor is inactive
			return models.DoctorSchedule{
				DoctorID:  doctorID,
				DayOfWeek: day,
				IsActive:  false,
			}, nil
		},
		CreateAppointmentFunc: func(ctx context.Context, id, pID, dID uuid.UUID, start, end time.Time, apptType, link, mID string) (uuid.UUID, error) {
			return id, nil
		},
		GetDoctorByIDFunc: func(ctx context.Context, id uuid.UUID) (models.Doctor, error) {
			return models.Doctor{ID: id, Available: true}, nil
		},
	}

	h := &Handler{
		Repo: mockRepo,
	}
	r := setupPhase4TestRouter(h)

	// Dynamic next Monday to guarantee lead-time validity
	targetMonday := time.Now().AddDate(0, 0, (8-int(time.Now().Weekday()))%7)
	if targetMonday.Before(time.Now().Add(24 * time.Hour)) {
		targetMonday = targetMonday.AddDate(0, 0, 7)
	}
	mon10am := time.Date(targetMonday.Year(), targetMonday.Month(), targetMonday.Day(), 10, 0, 0, 0, time.UTC)
	mon6pm := time.Date(targetMonday.Year(), targetMonday.Month(), targetMonday.Day(), 18, 0, 0, 0, time.UTC)
	targetSunday := targetMonday.AddDate(0, 0, -1)
	sun10am := time.Date(targetSunday.Year(), targetSunday.Month(), targetSunday.Day(), 10, 0, 0, 0, time.UTC)

	// 1. Booking within Monday 09:00-17:00 succeeds
	validBody, _ := json.Marshal(map[string]interface{}{
		"doctor_id":  doctorID.String(),
		"start_time": mon10am.Format(time.RFC3339),
		"end_time":   mon10am.Add(30 * time.Minute).Format(time.RFC3339),
		"type":       "in_person",
	})
	reqValid := httptest.NewRequest(http.MethodPost, "/api/appointments", bytes.NewReader(validBody))
	reqValid.Header.Set("Content-Type", "application/json")
	reqValid.Header.Set("X-User-ID", patientID.String())
	reqValid.Header.Set("X-Role", "patient")
	wValid := httptest.NewRecorder()
	r.ServeHTTP(wValid, reqValid)

	if wValid.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created for appointment inside schedule, got %d: %s", wValid.Code, wValid.Body.String())
	}

	// 2. Booking outside Monday hours (18:00) fails
	outBody, _ := json.Marshal(map[string]interface{}{
		"doctor_id":  doctorID.String(),
		"start_time": mon6pm.Format(time.RFC3339),
		"end_time":   mon6pm.Add(30 * time.Minute).Format(time.RFC3339),
		"type":       "in_person",
	})
	reqOut := httptest.NewRequest(http.MethodPost, "/api/appointments", bytes.NewReader(outBody))
	reqOut.Header.Set("Content-Type", "application/json")
	reqOut.Header.Set("X-User-ID", patientID.String())
	reqOut.Header.Set("X-Role", "patient")
	wOut := httptest.NewRecorder()
	r.ServeHTTP(wOut, reqOut)

	if wOut.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request for appointment outside hours, got %d: %s", wOut.Code, wOut.Body.String())
	}

	// 3. Booking on inactive day (Sunday) fails
	sunBody, _ := json.Marshal(map[string]interface{}{
		"doctor_id":  doctorID.String(),
		"start_time": sun10am.Format(time.RFC3339),
		"end_time":   sun10am.Add(30 * time.Minute).Format(time.RFC3339),
		"type":       "in_person",
	})
	reqSun := httptest.NewRequest(http.MethodPost, "/api/appointments", bytes.NewReader(sunBody))
	reqSun.Header.Set("Content-Type", "application/json")
	reqSun.Header.Set("X-User-ID", patientID.String())
	reqSun.Header.Set("X-Role", "patient")
	wSun := httptest.NewRecorder()
	r.ServeHTTP(wSun, reqSun)

	if wSun.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request for inactive day, got %d: %s", wSun.Code, wSun.Body.String())
	}
}

func TestDeleteDoctorSchedule(t *testing.T) {
	doctorID := uuid.New()
	var deletedDay int

	mockRepo := &repository.MockRepository{
		DeleteDoctorScheduleByDayFunc: func(ctx context.Context, dID uuid.UUID, day int) error {
			deletedDay = day
			return nil
		},
	}

	h := &Handler{
		Repo: mockRepo,
	}
	r := setupPhase4TestRouter(h)

	req := httptest.NewRequest(http.MethodDelete, "/api/doctor/schedules/3", nil)
	req.Header.Set("X-User-ID", doctorID.String())
	req.Header.Set("X-Role", "doctor")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK deleting schedule, got %d", w.Code)
	}
	if deletedDay != 3 {
		t.Errorf("expected deleted day 3, got %d", deletedDay)
	}
}

func TestMedicationAdherence_MetadataAndMultipleDoses(t *testing.T) {
	patientID := uuid.New()
	rxItemID := uuid.New()

	var savedLog models.MedicationLog
	mockRepo := &repository.MockRepository{
		VerifyPrescriptionItemOwnershipFunc: func(ctx context.Context, itemID, pID uuid.UUID) (bool, error) {
			return true, nil
		},
		UpsertMedicationLogFunc: func(ctx context.Context, log models.MedicationLog) (models.MedicationLog, error) {
			log.ID = uuid.New()
			savedLog = log
			return log, nil
		},
	}

	h := &Handler{
		Repo: mockRepo,
	}
	r := setupPhase4TestRouter(h)

	customTime := time.Now().Add(-2 * time.Hour).Truncate(time.Second)
	payload := map[string]interface{}{
		"prescription_item_id": rxItemID.String(),
		"scheduled_date":       time.Now().Format("2006-01-02"),
		"time_of_day":          "as_needed",
		"dose_number":          2,
		"meal_timing":          "after lunch",
		"status":               "taken",
		"taken_at":             customTime.Format(time.RFC3339),
		"notes":                "Second dose taken after lunch for headache",
	}

	b, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/api/patients/medication-schedule/log", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-User-ID", patientID.String())
	req.Header.Set("X-Role", "patient")

	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", w.Code, w.Body.String())
	}

	if savedLog.DoseNumber != 2 {
		t.Errorf("expected dose_number 2, got %d", savedLog.DoseNumber)
	}
	if savedLog.MealTiming != "after lunch" {
		t.Errorf("expected meal_timing after lunch, got %s", savedLog.MealTiming)
	}
	if savedLog.TakenAt == nil || !savedLog.TakenAt.Equal(customTime) {
		t.Errorf("expected custom taken_at %v, got %v", customTime, savedLog.TakenAt)
	}
}

func TestCreateAppointment_CrossMidnightTimezone(t *testing.T) {
	patientID := uuid.New()
	doctorID := uuid.New()

	// Doctor is in Asia/Tokyo (UTC+9).
	// Monday 08:00 - 12:00 in Tokyo is Sunday 23:00 - Monday 03:00 in UTC.
	tokyoLoc, err := time.LoadLocation("Asia/Tokyo")
	if err != nil {
		t.Fatalf("failed to load Asia/Tokyo: %v", err)
	}

	tokyoSchedule := models.DoctorSchedule{
		DoctorID:     doctorID,
		DayOfWeek:    1, // Monday in Tokyo
		StartTime:    "08:00",
		EndTime:      "12:00",
		SlotDuration: 30,
		Timezone:     "Asia/Tokyo",
		IsActive:     true,
	}

	mockRepo := &repository.MockRepository{
		GetDoctorSchedulesFunc: func(ctx context.Context, dID uuid.UUID) ([]models.DoctorSchedule, error) {
			return []models.DoctorSchedule{tokyoSchedule}, nil
		},
		CreateAppointmentFunc: func(ctx context.Context, id, pID, dID uuid.UUID, start, end time.Time, apptType, link, mID string) (uuid.UUID, error) {
			return id, nil
		},
		GetDoctorByIDFunc: func(ctx context.Context, id uuid.UUID) (models.Doctor, error) {
			return models.Doctor{ID: id, Available: true}, nil
		},
	}

	h := &Handler{
		Repo: mockRepo,
	}
	r := setupPhase4TestRouter(h)

	// Booking on a future Monday at 09:00 Tokyo time.
	// 2026-10-05 is a Monday. 09:00 Tokyo time = 2026-10-04 24:00 UTC (Sunday night).
	tokyoMon9am := time.Date(2026, 10, 5, 9, 0, 0, 0, tokyoLoc)

	payload, _ := json.Marshal(map[string]interface{}{
		"doctor_id":  doctorID.String(),
		"start_time": tokyoMon9am.Format(time.RFC3339),
		"end_time":   tokyoMon9am.Add(30 * time.Minute).Format(time.RFC3339),
		"type":       "in_person",
	})

	req := httptest.NewRequest(http.MethodPost, "/api/appointments", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-User-ID", patientID.String())
	req.Header.Set("X-Role", "patient")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created for appointment in doctor timezone, got %d: %s", w.Code, w.Body.String())
	}
}

func TestPhase4_InputValidations(t *testing.T) {
	patientID := uuid.New()
	doctorID := uuid.New()

	mockRepo := &repository.MockRepository{
		HasDoctorPatientRelationshipFunc: func(ctx context.Context, dID, pID uuid.UUID) (bool, error) {
			return true, nil
		},
	}

	h := &Handler{
		Repo: mockRepo,
	}
	r := setupPhase4TestRouter(h)

	// 1. Rejects notes exceeding 2000 chars in CreatePatientVital
	longNotes := string(make([]rune, 2001))
	bp := 120
	dia := 80
	vitalPayload, _ := json.Marshal(map[string]interface{}{
		"systolic_bp":  bp,
		"diastolic_bp": dia,
		"notes":        longNotes,
	})
	reqVital := httptest.NewRequest(http.MethodPost, "/api/vitals", bytes.NewReader(vitalPayload))
	reqVital.Header.Set("Content-Type", "application/json")
	reqVital.Header.Set("X-User-ID", patientID.String())
	reqVital.Header.Set("X-Role", "patient")
	wVital := httptest.NewRecorder()
	r.ServeHTTP(wVital, reqVital)

	if wVital.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for vital notes > 2000 chars, got %d", wVital.Code)
	}

	// 2. Rejects end_date before start_date in GetPatientVitals
	reqDates := httptest.NewRequest(http.MethodGet, "/api/vitals?start_date=2026-09-20&end_date=2026-09-10", nil)
	reqDates.Header.Set("X-User-ID", patientID.String())
	reqDates.Header.Set("X-Role", "patient")
	wDates := httptest.NewRecorder()
	r.ServeHTTP(wDates, reqDates)

	if wDates.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for end_date before start_date, got %d", wDates.Code)
	}

	// 3. Rejects duplicate days in UpsertDoctorSchedule
	dupSched, _ := json.Marshal([]map[string]interface{}{
		{"day_of_week": 1, "start_time": "09:00", "end_time": "12:00"},
		{"day_of_week": 1, "start_time": "13:00", "end_time": "17:00"},
	})
	reqDup := httptest.NewRequest(http.MethodPut, "/api/doctor/schedules", bytes.NewReader(dupSched))
	reqDup.Header.Set("Content-Type", "application/json")
	reqDup.Header.Set("X-User-ID", doctorID.String())
	reqDup.Header.Set("X-Role", "doctor")
	wDup := httptest.NewRecorder()
	r.ServeHTTP(wDup, reqDup)

	if wDup.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for duplicate day_of_week, got %d", wDup.Code)
	}
}

func TestPhase4_ReviewFixes(t *testing.T) {
	patientID := uuid.New()
	doctorID := uuid.New()
	apptID := uuid.New()

	// 1. TokenProvider error returns 502 Bad Gateway
	failingTelehealth := &telehealth.MockProvider{
		CustomID:   "room-failing",
		CustomLink: "https://telehealth.vitalwatch.local/room-failing",
		Err:        errors.New("daily meeting token rate limited"),
	}

	mockRepo := &repository.MockRepository{
		GetAppointmentByIDFunc: func(ctx context.Context, id uuid.UUID) (models.Appointment, error) {
			return models.Appointment{
				ID:          apptID,
				PatientID:   patientID,
				DoctorID:    doctorID,
				Type:        "virtual",
				MeetingLink: "https://telehealth.vitalwatch.local/room-failing",
				MeetingID:   "room-failing",
				Status:      "upcoming",
			}, nil
		},
		CreatePatientVitalFunc: func(ctx context.Context, v models.PatientVital) (uuid.UUID, error) {
			return uuid.New(), nil
		},
		GetPatientVitalsFunc: func(ctx context.Context, pID uuid.UUID, start, end *time.Time, limit, offset int) ([]models.PatientVital, error) {
			return []models.PatientVital{}, nil
		},
	}

	h := &Handler{
		Repo:       mockRepo,
		Telehealth: failingTelehealth,
	}
	r := setupPhase4TestRouter(h)

	reqRoom := httptest.NewRequest(http.MethodGet, "/api/appointments/"+apptID.String()+"/meeting-room", nil)
	reqRoom.Header.Set("X-User-ID", patientID.String())
	wRoom := httptest.NewRecorder()
	r.ServeHTTP(wRoom, reqRoom)

	if wRoom.Code != http.StatusBadGateway {
		t.Errorf("expected 502 Bad Gateway for meeting token generation error, got %d", wRoom.Code)
	}

	// 2. Maximum blood glucose 1000.0 is accepted
	bg := 1000.0
	vitalPayload, _ := json.Marshal(map[string]interface{}{
		"blood_glucose": bg,
	})
	reqVital := httptest.NewRequest(http.MethodPost, "/api/vitals", bytes.NewReader(vitalPayload))
	reqVital.Header.Set("Content-Type", "application/json")
	reqVital.Header.Set("X-User-ID", patientID.String())
	reqVital.Header.Set("X-Role", "patient")
	wVital := httptest.NewRecorder()
	r.ServeHTTP(wVital, reqVital)

	if wVital.Code != http.StatusCreated {
		t.Errorf("expected 201 Created for blood_glucose 1000.0, got %d: %s", wVital.Code, wVital.Body.String())
	}

	// 3. Limit clamped to 100 in GetPatientVitals metadata
	reqClamp := httptest.NewRequest(http.MethodGet, "/api/vitals?limit=500", nil)
	reqClamp.Header.Set("X-User-ID", patientID.String())
	reqClamp.Header.Set("X-Role", "patient")
	wClamp := httptest.NewRecorder()
	r.ServeHTTP(wClamp, reqClamp)

	if wClamp.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", wClamp.Code)
	}
	var respClamp struct {
		Limit int `json:"limit"`
	}
	json.Unmarshal(wClamp.Body.Bytes(), &respClamp)
	if respClamp.Limit != 100 {
		t.Errorf("expected limit clamped to 100, got %d", respClamp.Limit)
	}
}

func TestTelehealth_MeetingRoomMutualExclusion(t *testing.T) {
	patientID := uuid.New()
	doctorID := uuid.New()
	apptID := uuid.New()

	var mu sync.Mutex
	savedLink := ""
	savedID := ""

	mockRepo := &repository.MockRepository{
		GetAppointmentByIDFunc: func(ctx context.Context, id uuid.UUID) (models.Appointment, error) {
			mu.Lock()
			defer mu.Unlock()
			return models.Appointment{
				ID:          apptID,
				PatientID:   patientID,
				DoctorID:    doctorID,
				Type:        "virtual",
				Status:      "upcoming",
				MeetingLink: savedLink,
				MeetingID:   savedID,
			}, nil
		},
		GetOrGenerateAppointmentMeetingRoomFunc: func(ctx context.Context, id uuid.UUID, generator func() (string, string, error)) (models.Appointment, error) {
			mu.Lock()
			defer mu.Unlock()

			appt := models.Appointment{
				ID:          apptID,
				PatientID:   patientID,
				DoctorID:    doctorID,
				Type:        "virtual",
				Status:      "upcoming",
				MeetingLink: savedLink,
				MeetingID:   savedID,
			}
			if appt.MeetingLink == "" && generator != nil {
				link, roomID, err := generator()
				if err != nil {
					return appt, err
				}
				savedLink = link
				savedID = roomID
				appt.MeetingLink = link
				appt.MeetingID = roomID
			}
			return appt, nil
		},
	}

	mockTelehealth := &telehealth.MockProvider{
		CustomLink: "https://telehealth.vitalwatch.local/room-unique-session",
		CustomID:   "room-unique-session",
	}

	h := &Handler{
		Repo:       mockRepo,
		Telehealth: mockTelehealth,
	}

	r := setupPhase4TestRouter(h)

	const concurrency = 6
	var wg sync.WaitGroup
	links := make([]string, concurrency)
	statusCodes := make([]int, concurrency)

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			req := httptest.NewRequest(http.MethodGet, "/api/appointments/"+apptID.String()+"/meeting-room", nil)
			if idx%2 == 0 {
				req.Header.Set("X-User-ID", patientID.String())
				req.Header.Set("X-Role", "patient")
			} else {
				req.Header.Set("X-User-ID", doctorID.String())
				req.Header.Set("X-Role", "doctor")
			}
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			statusCodes[idx] = w.Code

			var resp struct {
				MeetingID   string `json:"meeting_id"`
				MeetingLink string `json:"meeting_link"`
			}
			_ = json.Unmarshal(w.Body.Bytes(), &resp)
			links[idx] = resp.MeetingLink
			if resp.MeetingID != "room-unique-session" {
				t.Errorf("request %d received incorrect meeting ID: %q", idx, resp.MeetingID)
			}
		}(i)
	}

	wg.Wait()

	for i := 0; i < concurrency; i++ {
		if statusCodes[i] != http.StatusOK {
			t.Errorf("request %d returned status %d", i, statusCodes[i])
		}
		if !strings.HasPrefix(links[i], "https://telehealth.vitalwatch.local/room-unique-session") {
			t.Errorf("request %d received divergent meeting base link: %q", i, links[i])
		}
		if i%2 == 0 {
			if !strings.Contains(links[i], "t=mock-patient-token") {
				t.Errorf("patient request %d missing patient token parameter: %q", i, links[i])
			}
		} else {
			if !strings.Contains(links[i], "t=mock-owner-token") {
				t.Errorf("doctor request %d missing owner token parameter: %q", i, links[i])
			}
		}
	}
}

func TestPatientCare_HardeningValidations(t *testing.T) {
	gin.SetMode(gin.TestMode)
	patientID := uuid.New()

	h := &Handler{
		Repo: &repository.MockRepository{
			VerifyPrescriptionItemOwnershipFunc: func(ctx context.Context, itemID, pID uuid.UUID) (bool, error) {
				return true, nil
			},
			GetActivePrescriptionItemsForPatientFunc: func(ctx context.Context, pID uuid.UUID, targetDate time.Time) ([]models.PrescriptionItem, error) {
				return []models.PrescriptionItem{}, nil
			},
			GetMedicationLogsByDateFunc: func(ctx context.Context, pID uuid.UUID, targetDate time.Time) ([]models.MedicationLog, error) {
				return []models.MedicationLog{}, nil
			},
		},
	}

	r := gin.New()
	r.POST("/api/vitals", func(c *gin.Context) {
		c.Set("userID", patientID)
		c.Set("role", "patient")
		h.CreatePatientVital(c)
	})
	r.POST("/api/adherence", func(c *gin.Context) {
		c.Set("userID", patientID)
		c.Set("role", "patient")
		h.LogMedicationAdherence(c)
	})
	r.GET("/api/schedule", func(c *gin.Context) {
		c.Set("userID", patientID)
		c.Set("role", "patient")
		h.GetPatientMedicationSchedule(c)
	})

	t.Run("CreatePatientVital Rejects Timestamps Older Than 5 Years", func(t *testing.T) {
		oldTime := time.Now().AddDate(-6, 0, 0)
		sbp := 120
		body, _ := json.Marshal(VitalInput{
			RecordedAt: &oldTime,
			SystolicBP: &sbp,
		})

		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodPost, "/api/vitals", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 Bad Request for timestamp older than 5 years, got %d: %s", w.Code, w.Body.String())
		}
		if !strings.Contains(w.Body.String(), "older than 5 years") {
			t.Errorf("expected error message to mention 5 years, got: %s", w.Body.String())
		}
	})

	t.Run("LogMedicationAdherence Rejects Dose Number Exceeding 24", func(t *testing.T) {
		body, _ := json.Marshal(MedicationLogInput{
			PrescriptionItemID: uuid.New(),
			ScheduledDate:      time.Now().Format("2006-01-02"),
			TimeOfDay:          "morning",
			Status:             "taken",
			DoseNumber:         25,
		})

		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodPost, "/api/adherence", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 Bad Request for dose_number > 24, got %d: %s", w.Code, w.Body.String())
		}
		if !strings.Contains(w.Body.String(), "cannot exceed 24 doses") {
			t.Errorf("expected error message to mention 24 doses, got: %s", w.Body.String())
		}
	})

	t.Run("GetPatientMedicationSchedule Rejects Dates Outside Query Horizon", func(t *testing.T) {
		// More than 2 years in the past
		pastDate := time.Now().AddDate(-3, 0, 0).Format("2006-01-02")
		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodGet, "/api/schedule?date="+pastDate, nil)
		r.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 Bad Request for date 3 years ago, got %d: %s", w.Code, w.Body.String())
		}
		if !strings.Contains(w.Body.String(), "within the past 2 years and next 1 year") {
			t.Errorf("expected horizon guard error, got: %s", w.Body.String())
		}

		// More than 1 year in the future
		futureDate := time.Now().AddDate(2, 0, 0).Format("2006-01-02")
		w2 := httptest.NewRecorder()
		req2, _ := http.NewRequest(http.MethodGet, "/api/schedule?date="+futureDate, nil)
		r.ServeHTTP(w2, req2)

		if w2.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 Bad Request for date 2 years in future, got %d: %s", w2.Code, w2.Body.String())
		}
	})
}

func TestPatientCare_HIPAAAuditLogging(t *testing.T) {
	gin.SetMode(gin.TestMode)
	doctorID := uuid.New()
	patientID := uuid.New()

	mockAuditor := audit.NewMockAuditor()
	mockRepo := &repository.MockRepository{
		GetDoctorsFunc: func(ctx context.Context, limit, offset int) ([]models.Doctor, error) {
			return []models.Doctor{{ID: doctorID, Specialty: "Cardiology"}}, nil
		},
		GetPatientsByDoctorIDFunc: func(ctx context.Context, dID uuid.UUID, limit, offset int) ([]models.Patient, error) {
			return []models.Patient{{ID: patientID, FirstName: "Jane", LastName: "Doe"}}, nil
		},
		GetAppointmentsForPatientFunc: func(ctx context.Context, dID, pID uuid.UUID, limit, offset int) ([]models.Appointment, error) {
			return []models.Appointment{{ID: uuid.New(), DoctorID: dID, PatientID: pID}}, nil
		},
	}

	h := &Handler{
		Repo:    mockRepo,
		Auditor: mockAuditor,
	}

	r := gin.New()
	r.GET("/doctors", func(c *gin.Context) {
		c.Set("userID", patientID)
		c.Set("role", "patient")
		h.GetDoctors(c)
	})
	r.GET("/doctor/patients", func(c *gin.Context) {
		c.Set("userID", doctorID)
		c.Set("role", "doctor")
		h.GetDoctorPatients(c)
	})
	r.GET("/doctor/patients/:id/appointments", func(c *gin.Context) {
		c.Set("userID", doctorID)
		c.Set("role", "doctor")
		h.GetPatientHistoryAppointments(c)
	})

	// 1. GetDoctors audit
	w1 := httptest.NewRecorder()
	req1, _ := http.NewRequest(http.MethodGet, "/doctors", nil)
	r.ServeHTTP(w1, req1)
	if w1.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for GetDoctors, got %d", w1.Code)
	}

	// 2. GetDoctorPatients audit
	w2 := httptest.NewRecorder()
	req2, _ := http.NewRequest(http.MethodGet, "/doctor/patients", nil)
	r.ServeHTTP(w2, req2)
	if w2.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for GetDoctorPatients, got %d", w2.Code)
	}

	// 3. GetPatientHistoryAppointments audit
	w3 := httptest.NewRecorder()
	req3, _ := http.NewRequest(http.MethodGet, "/doctor/patients/"+patientID.String()+"/appointments", nil)
	r.ServeHTTP(w3, req3)
	if w3.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for GetPatientHistoryAppointments, got %d", w3.Code)
	}

	entries := mockAuditor.GetEntries()
	if len(entries) != 3 {
		t.Fatalf("expected 3 audit log entries, got %d", len(entries))
	}

	actions := map[string]bool{}
	for _, entry := range entries {
		actions[entry.Action] = true
	}

	if !actions[audit.ActionViewDoctorProfile] {
		t.Errorf("missing ActionViewDoctorProfile audit entry")
	}
	if !actions[audit.ActionViewPatientProfile] {
		t.Errorf("missing ActionViewPatientProfile audit entry")
	}
	if !actions[audit.ActionViewAppointments] {
		t.Errorf("missing ActionViewAppointments audit entry")
	}
}

func TestCreatePatientVital_CriticalAlertAndPatientOnlySSE(t *testing.T) {
	gin.SetMode(gin.TestMode)
	patientID := uuid.New()
	doctorID := uuid.New()
	vitalID := uuid.New()

	mockBroker := notifications.NewSSEBroker(nil)
	defer mockBroker.Shutdown()

	patientCh, unsubPatient := mockBroker.Subscribe(patientID)
	defer unsubPatient()

	doctorCh, unsubDoctor := mockBroker.Subscribe(doctorID)
	defer unsubDoctor()

	mockRepo := &repository.MockRepository{
		CreatePatientVitalFunc: func(ctx context.Context, v models.PatientVital) (uuid.UUID, error) {
			return vitalID, nil
		},
	}

	h := &Handler{
		Repo:     mockRepo,
		Notifier: mockBroker,
	}

	r := gin.New()
	r.POST("/api/vitals", func(c *gin.Context) {
		c.Set("userID", patientID)
		c.Set("role", "patient")
		h.CreatePatientVital(c)
	})

	sbp := 195
	dbp := 125
	spo2 := 87.5
	hr := 115
	body, _ := json.Marshal(VitalInput{
		SystolicBP:       &sbp,
		DiastolicBP:      &dbp,
		OxygenSaturation: &spo2,
		HeartRate:        &hr,
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/api/vitals", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp["is_critical"] != true {
		t.Fatalf("expected is_critical to be true for 195/125 mmHg and 87.5%% SpO2, got: %v", resp["is_critical"])
	}

	alerts, ok := resp["critical_alerts"].([]interface{})
	if !ok || len(alerts) < 2 {
		t.Fatalf("expected at least 2 critical alerts (HTN Crisis and Hypoxemia), got: %v", resp["critical_alerts"])
	}

	// Verify Patient received EventVitalAlert via SSE
	select {
	case event := <-patientCh:
		if event.Type != notifications.EventVitalAlert {
			t.Errorf("expected EventVitalAlert event, got %s", event.Type)
		}
		if event.PatientID != patientID {
			t.Errorf("expected patient ID %s, got %s", patientID, event.PatientID)
		}
		if !strings.Contains(event.Message, "Hypertensive Crisis") {
			t.Errorf("expected message to mention Hypertensive Crisis, got: %s", event.Message)
		}
	case <-time.After(1 * time.Second):
		t.Fatalf("timed out waiting for patient EventVitalAlert")
	}

	// Verify Doctor did NOT receive the vital alert (patient-centric privacy)
	select {
	case event := <-doctorCh:
		t.Fatalf("doctor should NOT receive unsolicited vital alert push, but got: %+v", event)
	default:
		// OK - no message received by doctor
	}
}

func TestCreatePatientVital_NormalVitalsNoCriticalAlert(t *testing.T) {
	gin.SetMode(gin.TestMode)
	patientID := uuid.New()
	vitalID := uuid.New()

	mockBroker := notifications.NewSSEBroker(nil)
	defer mockBroker.Shutdown()

	patientCh, unsubPatient := mockBroker.Subscribe(patientID)
	defer unsubPatient()

	mockRepo := &repository.MockRepository{
		CreatePatientVitalFunc: func(ctx context.Context, v models.PatientVital) (uuid.UUID, error) {
			return vitalID, nil
		},
	}

	h := &Handler{
		Repo:     mockRepo,
		Notifier: mockBroker,
	}

	r := gin.New()
	r.POST("/api/vitals", func(c *gin.Context) {
		c.Set("userID", patientID)
		c.Set("role", "patient")
		h.CreatePatientVital(c)
	})

	sbp := 120
	dbp := 80
	spo2 := 99.0
	hr := 72
	body, _ := json.Marshal(VitalInput{
		SystolicBP:       &sbp,
		DiastolicBP:      &dbp,
		OxygenSaturation: &spo2,
		HeartRate:        &hr,
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/api/vitals", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)

	if resp["is_critical"] != false {
		t.Errorf("expected is_critical == false for normal vitals, got: %v", resp["is_critical"])
	}
	if resp["critical_alerts"] != nil {
		t.Errorf("expected nil critical_alerts, got: %v", resp["critical_alerts"])
	}

	// Verify no EventVitalAlert was sent to patient
	select {
	case event := <-patientCh:
		t.Fatalf("expected no vital alert for normal vitals, got: %+v", event)
	default:
		// OK
	}
}

func TestLogMedicationAdherence_TemporalBoundsAndNotification(t *testing.T) {
	gin.SetMode(gin.TestMode)
	patientID := uuid.New()
	itemID := uuid.New()
	logID := uuid.New()

	mockBroker := notifications.NewSSEBroker(nil)
	defer mockBroker.Shutdown()

	patientCh, unsubPatient := mockBroker.Subscribe(patientID)
	defer unsubPatient()

	mockRepo := &repository.MockRepository{
		VerifyPrescriptionItemOwnershipFunc: func(ctx context.Context, pItemID, pID uuid.UUID) (bool, error) {
			return true, nil
		},
		UpsertMedicationLogFunc: func(ctx context.Context, log models.MedicationLog) (models.MedicationLog, error) {
			log.ID = logID
			return log, nil
		},
	}

	h := &Handler{
		Repo:     mockRepo,
		Notifier: mockBroker,
	}

	r := gin.New()
	r.POST("/api/adherence", func(c *gin.Context) {
		c.Set("userID", patientID)
		c.Set("role", "patient")
		h.LogMedicationAdherence(c)
	})

	t.Run("Rejects ScheduledDate Older Than 2 Years", func(t *testing.T) {
		pastDate := time.Now().AddDate(-3, 0, 0).Format("2006-01-02")
		body, _ := json.Marshal(MedicationLogInput{
			PrescriptionItemID: itemID,
			ScheduledDate:      pastDate,
			TimeOfDay:          "morning",
			Status:             "taken",
		})

		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodPost, "/api/adherence", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 Bad Request, got %d: %s", w.Code, w.Body.String())
		}
		if !strings.Contains(w.Body.String(), "older than 2 years") {
			t.Errorf("expected error message to mention 'older than 2 years', got: %s", w.Body.String())
		}
	})

	t.Run("Rejects TakenAt Older Than 2 Years", func(t *testing.T) {
		oldTaken := time.Now().AddDate(-3, 0, 0)
		body, _ := json.Marshal(MedicationLogInput{
			PrescriptionItemID: itemID,
			ScheduledDate:      time.Now().Format("2006-01-02"),
			TimeOfDay:          "morning",
			Status:             "taken",
			TakenAt:            &oldTaken,
		})

		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodPost, "/api/adherence", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 Bad Request, got %d: %s", w.Code, w.Body.String())
		}
		if !strings.Contains(w.Body.String(), "older than 2 years") {
			t.Errorf("expected error message to mention 'older than 2 years', got: %s", w.Body.String())
		}
	})

	t.Run("Publishes EventMedicationLogged on Success", func(t *testing.T) {
		body, _ := json.Marshal(MedicationLogInput{
			PrescriptionItemID: itemID,
			ScheduledDate:      time.Now().Format("2006-01-02"),
			TimeOfDay:          "evening",
			Status:             "taken",
		})

		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodPost, "/api/adherence", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d: %s", w.Code, w.Body.String())
		}

		select {
		case event := <-patientCh:
			if event.Type != notifications.EventMedicationLogged {
				t.Errorf("expected EventMedicationLogged, got %s", event.Type)
			}
			if event.PatientID != patientID {
				t.Errorf("expected patient ID %s, got %s", patientID, event.PatientID)
			}
		case <-time.After(1 * time.Second):
			t.Fatalf("timed out waiting for EventMedicationLogged notification")
		}
	})
}

