package api

import (
	"database/sql"
	"github.com/RitwikGupta-0501/vital-watch/internal/models"

	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/RitwikGupta-0501/vital-watch/internal/repository"
	"github.com/RitwikGupta-0501/vital-watch/internal/storage"
)

func TestCreateAppointment_InputValidation(t *testing.T) {
	mockRepo := &repository.MockRepository{
		CreateAppointmentFunc: func(ctx context.Context, id, patientID, doctorID uuid.UUID, startTime, endTime time.Time, apptType, meetingLink, meetingID string) (uuid.UUID, error) {
			return uuid.New(), nil
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

	now := time.Now()

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
