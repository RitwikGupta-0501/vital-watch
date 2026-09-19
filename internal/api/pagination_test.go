package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/RitwikGupta-0501/vital-watch/internal/models"
	"github.com/RitwikGupta-0501/vital-watch/internal/repository"
	"github.com/RitwikGupta-0501/vital-watch/internal/storage"
)

func TestPagination_ListEndpoints(t *testing.T) {
	var capturedLimit, capturedOffset int

	mockRepo := &repository.MockRepository{
		GetDoctorsFunc: func(ctx context.Context, limit, offset int) ([]models.Doctor, error) {
			capturedLimit = limit
			capturedOffset = offset
			return []models.Doctor{
				{
					ID:         uuid.New(),
					Email:      "dr.smith@example.com",
					Role:       "doctor",
					FirstName:  "Alice",
					LastName:   "Smith",
					Specialty:  "Cardiology",
					Experience: 10,
					Available:  true,
				},
			}, nil
		},
		GetAppointmentsByPatientIDFunc: func(ctx context.Context, patientID uuid.UUID, limit, offset int) ([]models.Appointment, error) {
			capturedLimit = limit
			capturedOffset = offset
			return []models.Appointment{
				{
					ID:        uuid.New(),
					PatientID: patientID,
					DoctorID:  uuid.New(),
					StartTime: time.Now().Add(24 * time.Hour),
					EndTime:   time.Now().Add(25 * time.Hour),
					Status:    "scheduled",
				},
			}, nil
		},
	}

	h := &Handler{
		Repo:    mockRepo,
		Storage: storage.NewMockProvider(),
	}

	patientID := uuid.New()

	r := gin.New()
	r.GET("/doctors", h.GetDoctors)
	r.GET("/patient/appointments", func(c *gin.Context) {
		c.Set("userID", patientID)
		c.Set("role", "patient")
		h.GetPatientAppointments(c)
	})

	// 1. Default pagination (limit=20, offset=0)
	w1 := httptest.NewRecorder()
	req1, _ := http.NewRequest(http.MethodGet, "/doctors", nil)
	r.ServeHTTP(w1, req1)

	if w1.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w1.Code)
	}

	var resp1 struct {
		Data   []models.Doctor `json:"data"`
		Limit  int             `json:"limit"`
		Offset int             `json:"offset"`
	}
	if err := json.Unmarshal(w1.Body.Bytes(), &resp1); err != nil {
		t.Fatalf("failed to unmarshal paginated response: %v", err)
	}

	if resp1.Limit != 20 || resp1.Offset != 0 {
		t.Fatalf("expected default limit=20, offset=0, got limit=%d, offset=%d", resp1.Limit, resp1.Offset)
	}
	if len(resp1.Data) != 1 {
		t.Fatalf("expected 1 doctor, got %d", len(resp1.Data))
	}
	if capturedLimit != 20 || capturedOffset != 0 {
		t.Fatalf("expected repo to receive limit=20, offset=0, got %d, %d", capturedLimit, capturedOffset)
	}

	// 2. Custom pagination parameters (?limit=10&offset=5)
	w2 := httptest.NewRecorder()
	req2, _ := http.NewRequest(http.MethodGet, "/doctors?limit=10&offset=5", nil)
	r.ServeHTTP(w2, req2)

	var resp2 struct {
		Data   []models.Doctor `json:"data"`
		Limit  int             `json:"limit"`
		Offset int             `json:"offset"`
	}
	json.Unmarshal(w2.Body.Bytes(), &resp2)

	if resp2.Limit != 10 || resp2.Offset != 5 {
		t.Fatalf("expected limit=10, offset=5, got limit=%d, offset=%d", resp2.Limit, resp2.Offset)
	}
	if capturedLimit != 10 || capturedOffset != 5 {
		t.Fatalf("expected repo to receive limit=10, offset=5, got %d, %d", capturedLimit, capturedOffset)
	}

	// 3. Clamping limit > 100 to 100
	w3 := httptest.NewRecorder()
	req3, _ := http.NewRequest(http.MethodGet, "/patient/appointments?limit=500&offset=20", nil)
	r.ServeHTTP(w3, req3)

	var resp3 struct {
		Data   []models.Appointment `json:"data"`
		Limit  int                  `json:"limit"`
		Offset int                  `json:"offset"`
	}
	json.Unmarshal(w3.Body.Bytes(), &resp3)

	if resp3.Limit != 100 || resp3.Offset != 20 {
		t.Fatalf("expected clamped limit=100, offset=20, got limit=%d, offset=%d", resp3.Limit, resp3.Offset)
	}
	if capturedLimit != 100 || capturedOffset != 20 {
		t.Fatalf("expected repo to receive clamped limit=100, got %d", capturedLimit)
	}
}
