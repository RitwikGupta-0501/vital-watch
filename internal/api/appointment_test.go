package api

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/RitwikGupta-0501/vital-watch/internal/models"
	"github.com/RitwikGupta-0501/vital-watch/internal/notifications"
	"github.com/RitwikGupta-0501/vital-watch/internal/repository"
	"github.com/RitwikGupta-0501/vital-watch/internal/storage"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestCreateAppointment_InputValidation(t *testing.T) {
	mockRepo := &repository.MockRepository{
		CreateAppointmentFunc: func(ctx context.Context, id, patientID, doctorID uuid.UUID, startTime, endTime time.Time, apptType, meetingLink, meetingID string) (uuid.UUID, error) {
			return uuid.New(), nil
		},
		GetDoctorByIDFunc: func(ctx context.Context, id uuid.UUID) (models.Doctor, error) {
			return models.Doctor{
				ID:        id,
				Available: true,
			}, nil
		},
		GetDoctorSchedulesFunc: func(ctx context.Context, doctorID uuid.UUID) ([]models.DoctorSchedule, error) {
			var scheds []models.DoctorSchedule
			for day := 0; day <= 6; day++ {
				scheds = append(scheds, models.DoctorSchedule{
					DoctorID:     doctorID,
					DayOfWeek:    day,
					StartTime:    "00:00",
					EndTime:      "23:59",
					SlotDuration: 15,
					IsActive:     true,
					Timezone:     "UTC",
				})
			}
			return scheds, nil
		},
	}
	mockStorage := storage.NewMockProvider()
	h := &Handler{
		Repo:    mockRepo,
		Storage: mockStorage,
	}

	patientID := uuid.New()
	doctorID := uuid.New()

	r := gin.New()
	r.POST("/appointments", func(c *gin.Context) {
		c.Set("userID", patientID)
		c.Set("role", "patient")
		h.CreateAppointment(c)
	})

	now := time.Now().Truncate(time.Hour)

	tests := []struct {
		name       string
		payload    map[string]interface{}
		wantStatus int
		wantError  string
	}{
		{
			name: "Valid in-person appointment",
			payload: map[string]interface{}{
				"doctor_id":  doctorID.String(),
				"start_time": now.Add(2 * time.Hour).Format(time.RFC3339),
				"end_time":   now.Add(2*time.Hour + 30*time.Minute).Format(time.RFC3339),
				"type":       "in_person",
			},
			wantStatus: http.StatusCreated,
		},
		{
			name: "Valid virtual appointment",
			payload: map[string]interface{}{
				"doctor_id":  doctorID.String(),
				"start_time": now.Add(3 * time.Hour).Format(time.RFC3339),
				"end_time":   now.Add(3*time.Hour + 45*time.Minute).Format(time.RFC3339),
				"type":       "virtual",
			},
			wantStatus: http.StatusCreated,
		},
		{
			name: "Default appointment type when empty",
			payload: map[string]interface{}{
				"doctor_id":  doctorID.String(),
				"start_time": now.Add(4 * time.Hour).Format(time.RFC3339),
				"end_time":   now.Add(5 * time.Hour).Format(time.RFC3339),
			},
			wantStatus: http.StatusCreated,
		},
		{
			name: "Past start time rejected (API-04)",
			payload: map[string]interface{}{
				"doctor_id":  doctorID.String(),
				"start_time": now.Add(-1 * time.Hour).Format(time.RFC3339),
				"end_time":   now.Add(1 * time.Hour).Format(time.RFC3339),
				"type":       "in_person",
			},
			wantStatus: http.StatusBadRequest,
			wantError:  "Appointment start time must be in the future",
		},
		{
			name: "End time before start time rejected (API-04)",
			payload: map[string]interface{}{
				"doctor_id":  doctorID.String(),
				"start_time": now.Add(2 * time.Hour).Format(time.RFC3339),
				"end_time":   now.Add(1 * time.Hour).Format(time.RFC3339),
				"type":       "in_person",
			},
			wantStatus: http.StatusBadRequest,
			wantError:  "Appointment end time must be after start time",
		},
		{
			name: "Duration under 15 minutes rejected (API-04)",
			payload: map[string]interface{}{
				"doctor_id":  doctorID.String(),
				"start_time": now.Add(2 * time.Hour).Format(time.RFC3339),
				"end_time":   now.Add(2*time.Hour + 10*time.Minute).Format(time.RFC3339),
				"type":       "in_person",
			},
			wantStatus: http.StatusBadRequest,
			wantError:  "Appointment duration must be at least 15 minutes",
		},
		{
			name: "Duration over 4 hours rejected (API-04)",
			payload: map[string]interface{}{
				"doctor_id":  doctorID.String(),
				"start_time": now.Add(2 * time.Hour).Format(time.RFC3339),
				"end_time":   now.Add(7 * time.Hour).Format(time.RFC3339),
				"type":       "in_person",
			},
			wantStatus: http.StatusBadRequest,
			wantError:  "Appointment duration cannot exceed 4 hours",
		},
		{
			name: "Invalid appointment type rejected (API-04)",
			payload: map[string]interface{}{
				"doctor_id":  doctorID.String(),
				"start_time": now.Add(2 * time.Hour).Format(time.RFC3339),
				"end_time":   now.Add(2*time.Hour + 30*time.Minute).Format(time.RFC3339),
				"type":       "telepathy",
			},
			wantStatus: http.StatusBadRequest,
			wantError:  "Invalid appointment type: must be in_person or virtual",
		},
		{
			name: "Missing Doctor ID rejected",
			payload: map[string]interface{}{
				"doctor_id":  uuid.Nil.String(),
				"start_time": now.Add(2 * time.Hour).Format(time.RFC3339),
				"end_time":   now.Add(2*time.Hour + 30*time.Minute).Format(time.RFC3339),
				"type":       "in_person",
			},
			wantStatus: http.StatusBadRequest,
			wantError:  "Doctor ID is required",
		},
		{
			name: "Advance booking > 365 days rejected",
			payload: map[string]interface{}{
				"doctor_id":  doctorID.String(),
				"start_time": now.Add(400 * 24 * time.Hour).Format(time.RFC3339),
				"end_time":   now.Add(400*24*time.Hour + 30*time.Minute).Format(time.RFC3339),
				"type":       "in_person",
			},
			wantStatus: http.StatusBadRequest,
			wantError:  "Appointment cannot be booked more than 1 year in advance",
		},
		{
			name: "Self-booking with own doctor ID rejected",
			payload: map[string]interface{}{
				"doctor_id":  patientID.String(),
				"start_time": now.Add(2 * time.Hour).Format(time.RFC3339),
				"end_time":   now.Add(2*time.Hour + 30*time.Minute).Format(time.RFC3339),
				"type":       "in_person",
			},
			wantStatus: http.StatusBadRequest,
			wantError:  "Cannot book an appointment with yourself",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(tt.payload)
			w := httptest.NewRecorder()
			req, _ := http.NewRequest(http.MethodPost, "/appointments", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			r.ServeHTTP(w, req)

			if w.Code != tt.wantStatus {
				t.Fatalf("expected status %d, got %d. Body: %s", tt.wantStatus, w.Code, w.Body.String())
			}

			if tt.wantError != "" {
				var resp map[string]string
				json.Unmarshal(w.Body.Bytes(), &resp)
				if resp["error"] != tt.wantError {
					t.Fatalf("expected error %q, got %q", tt.wantError, resp["error"])
				}
			}
		})
	}
}


func TestMarkAppointmentAsCompleted_Guards(t *testing.T) {
	doctorID := uuid.New()
	apptIDFuture := uuid.New()
	apptIDPast := uuid.New()

	mockRepo := &repository.MockRepository{
		GetAppointmentByIDFunc: func(ctx context.Context, id uuid.UUID) (models.Appointment, error) {
			if id == apptIDFuture {
				return models.Appointment{
					ID:        apptIDFuture,
					DoctorID:  doctorID,
					StartTime: time.Now().Add(2 * time.Hour),
					EndTime:   time.Now().Add(3 * time.Hour),
					Status:    "upcoming",
				}, nil
			}
			if id == apptIDPast {
				return models.Appointment{
					ID:        apptIDPast,
					DoctorID:  doctorID,
					StartTime: time.Now().Add(-1 * time.Hour),
					EndTime:   time.Now().Add(-30 * time.Minute),
					Status:    "upcoming",
				}, nil
			}
			return models.Appointment{}, sql.ErrNoRows
		},
		UpdateAppointmentAsCompletedForDoctorFunc: func(ctx context.Context, apptID, docID uuid.UUID) (bool, error) {
			return apptID == apptIDPast && docID == doctorID, nil
		},
	}

	h := &Handler{
		Repo: mockRepo,
	}

	r := gin.New()
	r.PATCH("/api/appointments/:id", func(c *gin.Context) {
		c.Set("userID", doctorID)
		c.Set("role", "doctor")
		h.MarkAppointmentAsCompleted(c)
	})

	// 1. Completing future appointment rejected
	w1 := httptest.NewRecorder()
	req1, _ := http.NewRequest(http.MethodPatch, "/api/appointments/"+apptIDFuture.String(), nil)
	r.ServeHTTP(w1, req1)
	if w1.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for completing future appointment, got %d. Body: %s", w1.Code, w1.Body.String())
	}

	// 2. Completing appointment that has already started succeeds
	w2 := httptest.NewRecorder()
	req2, _ := http.NewRequest(http.MethodPatch, "/api/appointments/"+apptIDPast.String(), nil)
	r.ServeHTTP(w2, req2)
	if w2.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for past appointment completion, got %d. Body: %s", w2.Code, w2.Body.String())
	}
}

func TestGetAppointmentMeetingRoom_CompletedRejected(t *testing.T) {
	patientID := uuid.New()
	doctorID := uuid.New()
	apptID := uuid.New()

	mockRepo := &repository.MockRepository{
		GetAppointmentByIDFunc: func(ctx context.Context, id uuid.UUID) (models.Appointment, error) {
			return models.Appointment{
				ID:          apptID,
				PatientID:   patientID,
				DoctorID:    doctorID,
				Type:        "virtual",
				MeetingLink: "https://telehealth.vitalwatch.local/room-test",
				MeetingID:   "room-test",
				Status:      "completed",
			}, nil
		},
	}

	h := &Handler{
		Repo: mockRepo,
	}

	r := gin.New()
	r.GET("/api/appointments/:id/meeting-room", func(c *gin.Context) {
		c.Set("userID", patientID)
		c.Set("role", "patient")
		h.GetAppointmentMeetingRoom(c)
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/api/appointments/"+apptID.String()+"/meeting-room", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request for completed appointment meeting room, got %d. Body: %s", w.Code, w.Body.String())
	}
}

func TestDoctorSchedule_WindowValidation(t *testing.T) {
	in := ScheduleInput{
		DayOfWeek:    1,
		StartTime:    "09:00",
		EndTime:      "09:15",
		SlotDuration: 30, // 30 min duration > 15 min window
	}

	_, err := validateScheduleInput(in)
	if err == nil {
		t.Fatalf("expected error when working window is smaller than slot duration, got nil")
	}
}

func TestMarkAppointmentAsCompleted_BOLA_DoctorIsolation(t *testing.T) {
	doctorA := uuid.New()
	doctorB := uuid.New()
	patientID := uuid.New()
	apptID := uuid.New()

	futureStart := time.Now().Add(2 * time.Hour)
	mockRepo := &repository.MockRepository{
		GetAppointmentByIDFunc: func(ctx context.Context, id uuid.UUID) (models.Appointment, error) {
			if id == apptID {
				return models.Appointment{
					ID:        apptID,
					DoctorID:  doctorA, // Belongs to Doctor A
					PatientID: patientID,
					StartTime: futureStart,
					EndTime:   futureStart.Add(30 * time.Minute),
					Status:    "upcoming",
				}, nil
			}
			return models.Appointment{}, sql.ErrNoRows
		},
	}

	h := &Handler{Repo: mockRepo}
	r := gin.New()
	r.PATCH("/api/appointments/:id", func(c *gin.Context) {
		// Doctor B makes the request
		c.Set("userID", doctorB)
		c.Set("role", "doctor")
		h.MarkAppointmentAsCompleted(c)
	})

	// Doctor B attempts to complete Doctor A's appointment
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPatch, "/api/appointments/"+apptID.String(), nil)
	r.ServeHTTP(w, req)

	// Must return 404 Not Found (BOLA fix), NOT 400 Bad Request
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 Not Found (BOLA defense), got %d. Body: %s", w.Code, w.Body.String())
	}

	// Doctor A attempts to complete a cancelled appointment
	cancelledApptID := uuid.New()
	mockRepo.GetAppointmentByIDFunc = func(ctx context.Context, id uuid.UUID) (models.Appointment, error) {
		return models.Appointment{
			ID:        cancelledApptID,
			DoctorID:  doctorA,
			PatientID: patientID,
			StartTime: time.Now().Add(-1 * time.Hour),
			EndTime:   time.Now().Add(-30 * time.Minute),
			Status:    "cancelled",
		}, nil
	}

	r2 := gin.New()
	r2.PATCH("/api/appointments/:id", func(c *gin.Context) {
		c.Set("userID", doctorA)
		c.Set("role", "doctor")
		h.MarkAppointmentAsCompleted(c)
	})

	w2 := httptest.NewRecorder()
	req2, _ := http.NewRequest(http.MethodPatch, "/api/appointments/"+cancelledApptID.String(), nil)
	r2.ServeHTTP(w2, req2)

	if w2.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request for cancelled appointment, got %d. Body: %s", w2.Code, w2.Body.String())
	}
}

func TestCreateAppointment_DoctorActiveAndAvailabilityGuards(t *testing.T) {
	patientID := uuid.New()
	inactiveDoctorID := uuid.New()
	unavailableDoctorID := uuid.New()
	zeroSchedDoctorID := uuid.New()

	mockRepo := &repository.MockRepository{
		GetDoctorByIDFunc: func(ctx context.Context, id uuid.UUID) (models.Doctor, error) {
			if id == inactiveDoctorID {
				return models.Doctor{}, sql.ErrNoRows // Inactive or non-existent
			}
			if id == unavailableDoctorID {
				return models.Doctor{ID: id, Available: false}, nil // Unavailable
			}
			return models.Doctor{ID: id, Available: true}, nil
		},
		GetDoctorSchedulesFunc: func(ctx context.Context, doctorID uuid.UUID) ([]models.DoctorSchedule, error) {
			if doctorID == zeroSchedDoctorID {
				return []models.DoctorSchedule{}, nil // Zero schedules
			}
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

	h := &Handler{Repo: mockRepo}
	r := gin.New()
	r.POST("/api/appointments", func(c *gin.Context) {
		c.Set("userID", patientID)
		c.Set("role", "patient")
		h.CreateAppointment(c)
	})

	now := time.Now().Truncate(time.Hour)

	// 1. Inactive Doctor
	bodyInactive, _ := json.Marshal(map[string]interface{}{
		"doctor_id":  inactiveDoctorID.String(),
		"start_time": now.Add(2 * time.Hour).Format(time.RFC3339),
		"end_time":   now.Add(2*time.Hour + 30*time.Minute).Format(time.RFC3339),
	})
	w1 := httptest.NewRecorder()
	req1, _ := http.NewRequest(http.MethodPost, "/api/appointments", bytes.NewReader(bodyInactive))
	r.ServeHTTP(w1, req1)
	if w1.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for inactive doctor, got %d. Body: %s", w1.Code, w1.Body.String())
	}

	// 2. Unavailable Doctor
	bodyUnavail, _ := json.Marshal(map[string]interface{}{
		"doctor_id":  unavailableDoctorID.String(),
		"start_time": now.Add(2 * time.Hour).Format(time.RFC3339),
		"end_time":   now.Add(2*time.Hour + 30*time.Minute).Format(time.RFC3339),
	})
	w2 := httptest.NewRecorder()
	req2, _ := http.NewRequest(http.MethodPost, "/api/appointments", bytes.NewReader(bodyUnavail))
	r.ServeHTTP(w2, req2)
	if w2.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for unavailable doctor, got %d. Body: %s", w2.Code, w2.Body.String())
	}

	// 3. Zero-Schedule Doctor
	bodyZeroSched, _ := json.Marshal(map[string]interface{}{
		"doctor_id":  zeroSchedDoctorID.String(),
		"start_time": now.Add(2 * time.Hour).Format(time.RFC3339),
		"end_time":   now.Add(2*time.Hour + 30*time.Minute).Format(time.RFC3339),
	})
	w3 := httptest.NewRecorder()
	req3, _ := http.NewRequest(http.MethodPost, "/api/appointments", bytes.NewReader(bodyZeroSched))
	r.ServeHTTP(w3, req3)
	if w3.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for doctor with no schedules, got %d. Body: %s", w3.Code, w3.Body.String())
	}
}

func TestCreateAppointment_LeadTimeAndSlotAlignment(t *testing.T) {
	patientID := uuid.New()
	doctorID := uuid.New()

	mockRepo := &repository.MockRepository{
		GetDoctorByIDFunc: func(ctx context.Context, id uuid.UUID) (models.Doctor, error) {
			return models.Doctor{ID: id, Available: true}, nil
		},
		GetDoctorSchedulesFunc: func(ctx context.Context, dID uuid.UUID) ([]models.DoctorSchedule, error) {
			return []models.DoctorSchedule{
				{
					DoctorID:     doctorID,
					DayOfWeek:    int(time.Now().Weekday()),
					StartTime:    "08:00",
					EndTime:      "18:00",
					SlotDuration: 30, // 30-minute discrete slots
					IsActive:     true,
					Timezone:     "UTC",
				},
			}, nil
		},
	}

	h := &Handler{Repo: mockRepo}
	r := gin.New()
	r.POST("/api/appointments", func(c *gin.Context) {
		c.Set("userID", patientID)
		c.Set("role", "patient")
		h.CreateAppointment(c)
	})

	// 1. Lead time check: 5 minutes in future (< 15 min requirement)
	now := time.Now()
	bodyShortLead, _ := json.Marshal(map[string]interface{}{
		"doctor_id":  doctorID.String(),
		"start_time": now.Add(5 * time.Minute).Format(time.RFC3339),
		"end_time":   now.Add(35 * time.Minute).Format(time.RFC3339),
	})
	w1 := httptest.NewRecorder()
	req1, _ := http.NewRequest(http.MethodPost, "/api/appointments", bytes.NewReader(bodyShortLead))
	r.ServeHTTP(w1, req1)
	if w1.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for lead time < 15 min, got %d. Body: %s", w1.Code, w1.Body.String())
	}

	// 2. Slot Misalignment check: 10:07 - 10:37 (not multiple of 30 min)
	todayAt1007 := time.Date(now.Year(), now.Month(), now.Day()+1, 10, 7, 0, 0, time.UTC)
	bodyMisaligned, _ := json.Marshal(map[string]interface{}{
		"doctor_id":  doctorID.String(),
		"start_time": todayAt1007.Format(time.RFC3339),
		"end_time":   todayAt1007.Add(30 * time.Minute).Format(time.RFC3339),
	})
	w2 := httptest.NewRecorder()
	req2, _ := http.NewRequest(http.MethodPost, "/api/appointments", bytes.NewReader(bodyMisaligned))
	r.ServeHTTP(w2, req2)
	if w2.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for unaligned slot boundary, got %d. Body: %s", w2.Code, w2.Body.String())
	}
}

func TestCreateAppointment_23P01ConflictMapping(t *testing.T) {
	patientID := uuid.New()
	doctorID := uuid.New()

	var simulateConstraint string
	mockRepo := &repository.MockRepository{
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
		CreateAppointmentFunc: func(ctx context.Context, id, pID, dID uuid.UUID, start, end time.Time, apptType, link, mID string) (uuid.UUID, error) {
			return uuid.Nil, &pgconn.PgError{
				Code:           "23P01",
				ConstraintName: simulateConstraint,
			}
		},
	}

	h := &Handler{Repo: mockRepo}
	r := gin.New()
	r.POST("/api/appointments", func(c *gin.Context) {
		c.Set("userID", patientID)
		c.Set("role", "patient")
		h.CreateAppointment(c)
	})

	now := time.Now().Truncate(time.Hour)
	bodyBytes, _ := json.Marshal(map[string]interface{}{
		"doctor_id":  doctorID.String(),
		"start_time": now.Add(2 * time.Hour).Format(time.RFC3339),
		"end_time":   now.Add(2*time.Hour + 30*time.Minute).Format(time.RFC3339),
	})

	// Doctor double-booking
	simulateConstraint = "appointments_doctor_id_tstzrange_excl"
	w1 := httptest.NewRecorder()
	req1, _ := http.NewRequest(http.MethodPost, "/api/appointments", bytes.NewReader(bodyBytes))
	r.ServeHTTP(w1, req1)
	if w1.Code != http.StatusConflict {
		t.Fatalf("expected 409 Conflict for doctor double-booking, got %d. Body: %s", w1.Code, w1.Body.String())
	}
	var resp1 map[string]string
	json.Unmarshal(w1.Body.Bytes(), &resp1)
	if resp1["error"] != "Doctor is already booked for this time slot" {
		t.Errorf("unexpected error message: %s", resp1["error"])
	}

	// Patient double-booking
	simulateConstraint = "uq_patient_no_overlap"
	w2 := httptest.NewRecorder()
	req2, _ := http.NewRequest(http.MethodPost, "/api/appointments", bytes.NewReader(bodyBytes))
	r.ServeHTTP(w2, req2)
	if w2.Code != http.StatusConflict {
		t.Fatalf("expected 409 Conflict for patient double-booking, got %d. Body: %s", w2.Code, w2.Body.String())
	}
	var resp2 map[string]string
	json.Unmarshal(w2.Body.Bytes(), &resp2)
	if resp2["error"] != "You already have an appointment during this time slot" {
		t.Errorf("unexpected error message: %s", resp2["error"])
	}
}

func TestGetAppointmentMeetingRoom_PreMintingGuard(t *testing.T) {
	patientID := uuid.New()
	doctorID := uuid.New()
	farFutureApptID := uuid.New()
	imminentApptID := uuid.New()

	mockRepo := &repository.MockRepository{
		GetAppointmentByIDFunc: func(ctx context.Context, id uuid.UUID) (models.Appointment, error) {
			if id == farFutureApptID {
				return models.Appointment{
					ID:          farFutureApptID,
					PatientID:   patientID,
					DoctorID:    doctorID,
					Type:        "virtual",
					MeetingLink: "https://telehealth.vitalwatch.local/room-far",
					MeetingID:   "room-far",
					Status:      "upcoming",
					StartTime:   time.Now().Add(2 * time.Hour), // 2 hours away
					EndTime:     time.Now().Add(2*time.Hour + 30*time.Minute),
				}, nil
			}
			return models.Appointment{
				ID:          imminentApptID,
				PatientID:   patientID,
				DoctorID:    doctorID,
				Type:        "virtual",
				MeetingLink: "https://telehealth.vitalwatch.local/room-imminent",
				MeetingID:   "room-imminent",
				Status:      "upcoming",
				StartTime:   time.Now().Add(10 * time.Minute), // 10 minutes away (within 15m window)
				EndTime:     time.Now().Add(40 * time.Minute),
			}, nil
		},
	}

	h := &Handler{Repo: mockRepo}
	r := gin.New()
	r.GET("/api/appointments/:id/meeting-room", func(c *gin.Context) {
		c.Set("userID", patientID)
		c.Set("role", "patient")
		h.GetAppointmentMeetingRoom(c)
	})

	// 1. Far future appointment blocked (403)
	w1 := httptest.NewRecorder()
	req1, _ := http.NewRequest(http.MethodGet, "/api/appointments/"+farFutureApptID.String()+"/meeting-room", nil)
	r.ServeHTTP(w1, req1)
	if w1.Code != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden for appointment > 15m away, got %d. Body: %s", w1.Code, w1.Body.String())
	}

	// 2. Imminent appointment permitted (200)
	w2 := httptest.NewRecorder()
	req2, _ := http.NewRequest(http.MethodGet, "/api/appointments/"+imminentApptID.String()+"/meeting-room", nil)
	r.ServeHTTP(w2, req2)
	if w2.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for imminent appointment, got %d. Body: %s", w2.Code, w2.Body.String())
	}
}

func TestCancelAppointment_FlowAndAuthorization(t *testing.T) {
	patientID := uuid.New()
	doctorID := uuid.New()
	intruderID := uuid.New()
	apptID := uuid.New()

	mockBroker := notifications.NewSSEBroker()
	var cancelRepoCalled bool

	mockRepo := &repository.MockRepository{
		GetAppointmentByIDFunc: func(ctx context.Context, id uuid.UUID) (models.Appointment, error) {
			if id == apptID {
				return models.Appointment{
					ID:        apptID,
					PatientID: patientID,
					DoctorID:  doctorID,
					Status:    "upcoming",
				}, nil
			}
			return models.Appointment{}, sql.ErrNoRows
		},
		CancelAppointmentByParticipantFunc: func(ctx context.Context, aID, pID uuid.UUID) (bool, error) {
			cancelRepoCalled = true
			return true, nil
		},
	}

	h := &Handler{
		Repo:     mockRepo,
		Notifier: mockBroker,
	}

	r := gin.New()
	r.PATCH("/api/appointments/:id/cancel", func(c *gin.Context) {
		c.Set("userID", intruderID)
		c.Set("role", "patient")
		h.CancelAppointment(c)
	})

	// 1. Intruder receives 404
	w1 := httptest.NewRecorder()
	req1, _ := http.NewRequest(http.MethodPatch, "/api/appointments/"+apptID.String()+"/cancel", nil)
	r.ServeHTTP(w1, req1)
	if w1.Code != http.StatusNotFound {
		t.Fatalf("expected 404 Not Found for unauthorized cancellation attempt, got %d. Body: %s", w1.Code, w1.Body.String())
	}

	// 2. Legitimate patient can cancel
	r2 := gin.New()
	r2.PATCH("/api/appointments/:id/cancel", func(c *gin.Context) {
		c.Set("userID", patientID)
		c.Set("role", "patient")
		h.CancelAppointment(c)
	})
	w2 := httptest.NewRecorder()
	req2, _ := http.NewRequest(http.MethodPatch, "/api/appointments/"+apptID.String()+"/cancel", nil)
	r2.ServeHTTP(w2, req2)
	if w2.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for patient cancellation, got %d. Body: %s", w2.Code, w2.Body.String())
	}
	if !cancelRepoCalled {
		t.Errorf("expected CancelAppointmentByParticipant to be called")
	}

	// 3. Cancelling a non-upcoming appointment returns 400
	mockRepo.CancelAppointmentByParticipantFunc = func(ctx context.Context, aID, pID uuid.UUID) (bool, error) {
		return false, nil // Not upcoming
	}
	w3 := httptest.NewRecorder()
	req3, _ := http.NewRequest(http.MethodPatch, "/api/appointments/"+apptID.String()+"/cancel", nil)
	r2.ServeHTTP(w3, req3)
	if w3.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request for non-upcoming appointment cancellation, got %d. Body: %s", w3.Code, w3.Body.String())
	}
}

