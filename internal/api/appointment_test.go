package api

import (
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
		CreateAppointmentFunc: func(ctx context.Context, patientID, doctorID uuid.UUID, startTime, endTime time.Time, apptType string) (uuid.UUID, error) {
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
