package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/RitwikGupta-0501/vital-watch/internal/models"
	"github.com/RitwikGupta-0501/vital-watch/internal/notifications"
	"github.com/RitwikGupta-0501/vital-watch/internal/pdf"
	"github.com/RitwikGupta-0501/vital-watch/internal/repository"
	"github.com/RitwikGupta-0501/vital-watch/internal/safety"
	"github.com/RitwikGupta-0501/vital-watch/internal/storage"
)

func TestCreatePrescription_UploadFlow(t *testing.T) {
	gin.SetMode(gin.TestMode)
	doctorID := uuid.New()
	patientID := uuid.New()
	validKey := "prescription-" + patientID.String() + "-" + uuid.New().String() + ".pdf"

	mockStorage := storage.NewMockProvider()
	mockStorage.Files[validKey] = []byte("%PDF-1.4 test")

	t.Run("Success with OCR Enabled", func(t *testing.T) {
		expectedID := uuid.New()
		var capturedOCREnabled bool
		mockRepo := &repository.MockRepository{
			GetPatientByIDFunc: func(ctx context.Context, id uuid.UUID) (models.Patient, error) {
				return models.Patient{ID: id}, nil
			},
			GetAppointmentsForPatientFunc: func(ctx context.Context, dID, pID uuid.UUID, limit, offset int) ([]models.Appointment, error) {
				return []models.Appointment{{ID: uuid.New()}}, nil
			},
			CreateUploadedPrescriptionWithJobFunc: func(ctx context.Context, pID, dID uuid.UUID, fn, notes string, ocrEnabled bool) (uuid.UUID, string, error) {
				capturedOCREnabled = ocrEnabled
				return expectedID, "pending_ocr", nil
			},
		}

		h := &Handler{
			Repo:       mockRepo,
			Storage:    mockStorage,
			OCREnabled: true,
		}

		r := gin.New()
		r.POST("/prescriptions", func(c *gin.Context) {
			c.Set("userID", doctorID)
			c.Set("role", "doctor")
			h.CreatePrescription(c)
		})

		body, _ := json.Marshal(map[string]interface{}{
			"patient_id": patientID.String(),
			"file_name":  validKey,
			"notes":      "Check for allergies",
		})

		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodPost, "/prescriptions", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)

		if w.Code != http.StatusCreated {
			t.Fatalf("expected 201 Created, got %d: %s", w.Code, w.Body.String())
		}
		if !capturedOCREnabled {
			t.Errorf("expected ocrEnabled to be true, got false")
		}

		var resp map[string]interface{}
		_ = json.Unmarshal(w.Body.Bytes(), &resp)
		if resp["status"] != "pending_ocr" {
			t.Errorf("expected status pending_ocr, got %v", resp["status"])
		}
	})

	t.Run("Success with OCR Disabled", func(t *testing.T) {
		mockRepo := &repository.MockRepository{
			GetPatientByIDFunc: func(ctx context.Context, id uuid.UUID) (models.Patient, error) {
				return models.Patient{ID: id}, nil
			},
			GetAppointmentsForPatientFunc: func(ctx context.Context, dID, pID uuid.UUID, limit, offset int) ([]models.Appointment, error) {
				return []models.Appointment{{ID: uuid.New()}}, nil
			},
			CreateUploadedPrescriptionWithJobFunc: func(ctx context.Context, pID, dID uuid.UUID, fn, notes string, ocrEnabled bool) (uuid.UUID, string, error) {
				return uuid.New(), "needs_review", nil
			},
		}

		h := &Handler{
			Repo:       mockRepo,
			Storage:    mockStorage,
			OCREnabled: false,
		}

		r := gin.New()
		r.POST("/prescriptions", func(c *gin.Context) {
			c.Set("userID", doctorID)
			c.Set("role", "doctor")
			h.CreatePrescription(c)
		})

		body, _ := json.Marshal(map[string]interface{}{
			"patient_id": patientID.String(),
			"file_name":  validKey,
		})

		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodPost, "/prescriptions", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)

		if w.Code != http.StatusCreated {
			t.Fatalf("expected 201 Created, got %d: %s", w.Code, w.Body.String())
		}

		var resp map[string]interface{}
		_ = json.Unmarshal(w.Body.Bytes(), &resp)
		if resp["status"] != "needs_review" {
			t.Errorf("expected status needs_review, got %v", resp["status"])
		}
	})

	t.Run("Storage Rollback on Database Insertion Failure", func(t *testing.T) {
		rollbackKey := "prescription-" + patientID.String() + "-" + uuid.New().String() + ".pdf"
		mockStorage.Files[rollbackKey] = []byte("%PDF-1.4 test")

		mockRepo := &repository.MockRepository{
			GetPatientByIDFunc: func(ctx context.Context, id uuid.UUID) (models.Patient, error) {
				return models.Patient{ID: id}, nil
			},
			GetAppointmentsForPatientFunc: func(ctx context.Context, dID, pID uuid.UUID, limit, offset int) ([]models.Appointment, error) {
				return []models.Appointment{{ID: uuid.New()}}, nil
			},
			CreateUploadedPrescriptionWithJobFunc: func(ctx context.Context, pID, dID uuid.UUID, fn, notes string, ocrEnabled bool) (uuid.UUID, string, error) {
				return uuid.Nil, "", errors.New("db connection lost")
			},
		}

		h := &Handler{
			Repo:       mockRepo,
			Storage:    mockStorage,
			OCREnabled: true,
		}

		r := gin.New()
		r.POST("/prescriptions", func(c *gin.Context) {
			c.Set("userID", doctorID)
			c.Set("role", "doctor")
			h.CreatePrescription(c)
		})

		body, _ := json.Marshal(map[string]interface{}{
			"patient_id": patientID.String(),
			"file_name":  rollbackKey,
		})

		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodPost, "/prescriptions", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)

		if w.Code != http.StatusInternalServerError {
			t.Fatalf("expected 500 Internal Server Error, got %d", w.Code)
		}
	})

	t.Run("Non-Existent Patient 404", func(t *testing.T) {
		mockRepo := &repository.MockRepository{
			GetPatientByIDFunc: func(ctx context.Context, id uuid.UUID) (models.Patient, error) {
				return models.Patient{}, errors.New("not found")
			},
		}
		h := &Handler{Repo: mockRepo, Storage: mockStorage}
		r := gin.New()
		r.POST("/prescriptions", func(c *gin.Context) {
			c.Set("userID", doctorID)
			h.CreatePrescription(c)
		})

		body, _ := json.Marshal(map[string]interface{}{
			"patient_id": patientID.String(),
			"file_name":  validKey,
		})
		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodPost, "/prescriptions", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)

		if w.Code != http.StatusNotFound {
			t.Fatalf("expected 404 Not Found for non-existent patient, got %d", w.Code)
		}
	})

	t.Run("Unauthorized Doctor (No Appointments) 403", func(t *testing.T) {
		mockRepo := &repository.MockRepository{
			GetPatientByIDFunc: func(ctx context.Context, id uuid.UUID) (models.Patient, error) {
				return models.Patient{ID: id}, nil
			},
			GetAppointmentsForPatientFunc: func(ctx context.Context, dID, pID uuid.UUID, limit, offset int) ([]models.Appointment, error) {
				return []models.Appointment{}, nil
			},
		}
		h := &Handler{Repo: mockRepo, Storage: mockStorage}
		r := gin.New()
		r.POST("/prescriptions", func(c *gin.Context) {
			c.Set("userID", doctorID)
			h.CreatePrescription(c)
		})

		body, _ := json.Marshal(map[string]interface{}{
			"patient_id": patientID.String(),
			"file_name":  validKey,
		})
		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodPost, "/prescriptions", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)

		if w.Code != http.StatusForbidden {
			t.Fatalf("expected 403 Forbidden for unauthorized doctor, got %d", w.Code)
		}
	})
}

func TestCreateDigitalPrescription(t *testing.T) {
	gin.SetMode(gin.TestMode)
	doctorID := uuid.New()
	patientID := uuid.New()

	t.Run("Valid Digital Prescription", func(t *testing.T) {
		expectedID := uuid.New()
		var capturedItems []models.PrescriptionItem

		mockRepo := &repository.MockRepository{
			GetPatientByIDFunc: func(ctx context.Context, id uuid.UUID) (models.Patient, error) {
				return models.Patient{ID: id}, nil
			},
			GetAppointmentsForPatientFunc: func(ctx context.Context, dID, pID uuid.UUID, limit, offset int) ([]models.Appointment, error) {
				return []models.Appointment{{ID: uuid.New()}}, nil
			},
			CreateDigitalPrescriptionFunc: func(ctx context.Context, pID, dID uuid.UUID, notes string, items []models.PrescriptionItem) (uuid.UUID, error) {
				capturedItems = items
				return expectedID, nil
			},
		}

		h := &Handler{Repo: mockRepo}
		r := gin.New()
		r.POST("/prescriptions/digital", func(c *gin.Context) {
			c.Set("userID", doctorID)
			c.Set("role", "doctor")
			h.CreateDigitalPrescription(c)
		})

		body, _ := json.Marshal(CreateDigitalPrescriptionRequest{
			PatientID: patientID,
			Notes:     "Take with plenty of water",
			Items: []PrescriptionItemInput{
				{
					MedicationName: "Amoxicillin",
					Dosage:         "500mg",
					Frequency:      "TDS",
					Duration:       "7 days",
					Timing:         "After meals",
					Instructions:   "Complete the course",
				},
				{
					MedicationName: "Paracetamol",
					Dosage:         "650mg",
					Frequency:      "SOS",
					Duration:       "3 days",
					Timing:         "Post meals",
					Instructions:   "For fever",
				},
			},
		})

		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodPost, "/prescriptions/digital", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)

		if w.Code != http.StatusCreated {
			t.Fatalf("expected 201 Created, got %d: %s", w.Code, w.Body.String())
		}

		if len(capturedItems) != 2 {
			t.Fatalf("expected 2 items, got %d", len(capturedItems))
		}
		if capturedItems[0].MedicationName != "Amoxicillin" || capturedItems[1].MedicationName != "Paracetamol" {
			t.Errorf("medication names do not match expected: %+v", capturedItems)
		}
	})

	t.Run("Missing Patient ID", func(t *testing.T) {
		h := &Handler{Repo: &repository.MockRepository{}}
		r := gin.New()
		r.POST("/prescriptions/digital", func(c *gin.Context) {
			c.Set("userID", doctorID)
			h.CreateDigitalPrescription(c)
		})

		body, _ := json.Marshal(map[string]interface{}{
			"notes": "some notes",
			"items": []map[string]string{{"medication_name": "Amoxicillin"}},
		})

		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodPost, "/prescriptions/digital", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 Bad Request, got %d", w.Code)
		}
	})

	t.Run("Empty Medication Items List", func(t *testing.T) {
		h := &Handler{Repo: &repository.MockRepository{}}
		r := gin.New()
		r.POST("/prescriptions/digital", func(c *gin.Context) {
			c.Set("userID", doctorID)
			h.CreateDigitalPrescription(c)
		})

		body, _ := json.Marshal(CreateDigitalPrescriptionRequest{
			PatientID: patientID,
			Notes:     "Empty items",
			Items:     []PrescriptionItemInput{},
		})

		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodPost, "/prescriptions/digital", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 Bad Request for empty items, got %d", w.Code)
		}
	})

	t.Run("Item With Empty Medication Name", func(t *testing.T) {
		h := &Handler{Repo: &repository.MockRepository{}}
		r := gin.New()
		r.POST("/prescriptions/digital", func(c *gin.Context) {
			c.Set("userID", doctorID)
			h.CreateDigitalPrescription(c)
		})

		body, _ := json.Marshal(CreateDigitalPrescriptionRequest{
			PatientID: patientID,
			Items: []PrescriptionItemInput{
				{MedicationName: "   ", Dosage: "500mg"},
			},
		})

		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodPost, "/prescriptions/digital", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 Bad Request for whitespace medication name, got %d", w.Code)
		}
	})

	t.Run("Non-Existent Patient", func(t *testing.T) {
		mockRepo := &repository.MockRepository{
			GetPatientByIDFunc: func(ctx context.Context, id uuid.UUID) (models.Patient, error) {
				return models.Patient{}, errors.New("not found")
			},
		}
		h := &Handler{Repo: mockRepo}
		r := gin.New()
		r.POST("/prescriptions/digital", func(c *gin.Context) {
			c.Set("userID", doctorID)
			h.CreateDigitalPrescription(c)
		})

		body, _ := json.Marshal(CreateDigitalPrescriptionRequest{
			PatientID: patientID,
			Items:     []PrescriptionItemInput{{MedicationName: "Amoxicillin"}},
		})

		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodPost, "/prescriptions/digital", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)

		if w.Code != http.StatusNotFound {
			t.Fatalf("expected 404 Not Found for non-existent patient, got %d", w.Code)
		}
	})

	t.Run("Unauthorized Doctor (No Appointments)", func(t *testing.T) {
		mockRepo := &repository.MockRepository{
			GetPatientByIDFunc: func(ctx context.Context, id uuid.UUID) (models.Patient, error) {
				return models.Patient{ID: id}, nil
			},
			GetAppointmentsForPatientFunc: func(ctx context.Context, dID, pID uuid.UUID, limit, offset int) ([]models.Appointment, error) {
				return []models.Appointment{}, nil
			},
		}
		h := &Handler{Repo: mockRepo}
		r := gin.New()
		r.POST("/prescriptions/digital", func(c *gin.Context) {
			c.Set("userID", doctorID)
			h.CreateDigitalPrescription(c)
		})

		body, _ := json.Marshal(CreateDigitalPrescriptionRequest{
			PatientID: patientID,
			Items:     []PrescriptionItemInput{{MedicationName: "Amoxicillin"}},
		})

		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodPost, "/prescriptions/digital", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)

		if w.Code != http.StatusForbidden {
			t.Fatalf("expected 403 Forbidden for unauthorized doctor, got %d", w.Code)
		}
	})
}

func TestGetPendingReviewPrescriptions(t *testing.T) {
	gin.SetMode(gin.TestMode)
	doctorID := uuid.New()

	mockRepo := &repository.MockRepository{
		GetPrescriptionsPendingReviewFunc: func(ctx context.Context, dID uuid.UUID, limit, offset int) ([]models.Prescription, error) {
			return []models.Prescription{
				{
					ID:       uuid.New(),
					DoctorID: dID,
					Status:   "needs_review",
					Items: []models.PrescriptionItem{
						{MedicationName: "Metformin", Dosage: "500mg"},
					},
				},
			}, nil
		},
	}

	h := &Handler{Repo: mockRepo}
	r := gin.New()
	r.GET("/prescriptions/pending-review", func(c *gin.Context) {
		c.Set("userID", doctorID)
		c.Set("role", "doctor")
		h.GetPendingReviewPrescriptions(c)
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/prescriptions/pending-review?limit=10&offset=0", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	data, ok := resp["data"].([]interface{})
	if !ok || len(data) != 1 {
		t.Fatalf("expected 1 pending prescription, got: %v", resp["data"])
	}
}

func TestVerifyPrescription(t *testing.T) {
	gin.SetMode(gin.TestMode)
	doctorID := uuid.New()
	prescriptionID := uuid.New()

	t.Run("Approve Prescription with Modified Items", func(t *testing.T) {
		var capturedStatus string
		var capturedItems []models.PrescriptionItem

		mockRepo := &repository.MockRepository{
			GetPrescriptionByIDFunc: func(ctx context.Context, id uuid.UUID) (models.Prescription, error) {
				return models.Prescription{
					ID:    id,
					Items: []models.PrescriptionItem{{MedicationName: "Amoxicillin", Dosage: "500mg"}},
				}, nil
			},
			VerifyPrescriptionFunc: func(ctx context.Context, pID, dID uuid.UUID, status, notes string, items []models.PrescriptionItem) (bool, error) {
				capturedStatus = status
				capturedItems = items
				return true, nil
			},
		}

		h := &Handler{Repo: mockRepo}
		r := gin.New()
		r.PATCH("/prescriptions/:id/verify", func(c *gin.Context) {
			c.Set("userID", doctorID)
			c.Set("role", "doctor")
			h.VerifyPrescription(c)
		})

		body, _ := json.Marshal(VerifyPrescriptionRequest{
			Status: "approved",
			Notes:  "Verified and dosage corrected",
			Items: []PrescriptionItemInput{
				{
					MedicationName: "Azithromycin",
					Dosage:         "500mg",
					Frequency:      "OD",
					Duration:       "3 days",
				},
			},
		})

		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodPatch, "/prescriptions/"+prescriptionID.String()+"/verify", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d: %s", w.Code, w.Body.String())
		}
		if capturedStatus != "approved" {
			t.Errorf("expected status approved, got %s", capturedStatus)
		}
		if len(capturedItems) != 1 || capturedItems[0].MedicationName != "Azithromycin" {
			t.Errorf("expected verified items not captured: %+v", capturedItems)
		}
	})

	t.Run("Reject Prescription", func(t *testing.T) {
		var capturedStatus string
		var capturedItems []models.PrescriptionItem
		mockRepo := &repository.MockRepository{
			GetPrescriptionByIDFunc: func(ctx context.Context, id uuid.UUID) (models.Prescription, error) {
				return models.Prescription{
					ID:    id,
					Items: []models.PrescriptionItem{{MedicationName: "Amoxicillin", Dosage: "500mg"}},
				}, nil
			},
			VerifyPrescriptionFunc: func(ctx context.Context, pID, dID uuid.UUID, status, notes string, items []models.PrescriptionItem) (bool, error) {
				capturedStatus = status
				capturedItems = items
				return true, nil
			},
		}

		h := &Handler{Repo: mockRepo}
		r := gin.New()
		r.PATCH("/prescriptions/:id/verify", func(c *gin.Context) {
			c.Set("userID", doctorID)
			h.VerifyPrescription(c)
		})

		body, _ := json.Marshal(VerifyPrescriptionRequest{
			Status: "rejected",
			Notes:  "Illegible handwriting",
		})

		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodPatch, "/prescriptions/"+prescriptionID.String()+"/verify", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d", w.Code)
		}
		if capturedStatus != "rejected" {
			t.Errorf("expected status rejected, got %s", capturedStatus)
		}
		if capturedItems == nil || len(capturedItems) != 0 {
			t.Errorf("expected rejection to clear items (non-nil empty slice), got: %v", capturedItems)
		}
	})

	t.Run("Approve Prescription with Empty Items Rejected (Requires Medications)", func(t *testing.T) {
		h := &Handler{Repo: &repository.MockRepository{}}
		r := gin.New()
		r.PATCH("/prescriptions/:id/verify", func(c *gin.Context) {
			c.Set("userID", doctorID)
			c.Set("role", "doctor")
			h.VerifyPrescription(c)
		})

		body, _ := json.Marshal(map[string]interface{}{
			"status": "approved",
			"notes":  "Slip approved with no items",
			"items":  []interface{}{},
		})

		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodPatch, "/prescriptions/"+prescriptionID.String()+"/verify", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 Bad Request when approving with empty items list, got %d", w.Code)
		}
	})

	t.Run("Approve Prescription with Nil Items Retains OCR Items", func(t *testing.T) {
		var capturedStatus string
		var capturedItems []models.PrescriptionItem
		mockRepo := &repository.MockRepository{
			GetPrescriptionByIDFunc: func(ctx context.Context, id uuid.UUID) (models.Prescription, error) {
				return models.Prescription{
					ID:    id,
					Items: []models.PrescriptionItem{{MedicationName: "Amoxicillin", Dosage: "500mg"}},
				}, nil
			},
			VerifyPrescriptionFunc: func(ctx context.Context, pID, dID uuid.UUID, status, notes string, items []models.PrescriptionItem) (bool, error) {
				capturedStatus = status
				capturedItems = items
				return true, nil
			},
		}

		h := &Handler{Repo: mockRepo}
		r := gin.New()
		r.PATCH("/prescriptions/:id/verify", func(c *gin.Context) {
			c.Set("userID", doctorID)
			c.Set("role", "doctor")
			h.VerifyPrescription(c)
		})

		body, _ := json.Marshal(map[string]interface{}{
			"status": "approved",
			"notes":  "Accepting existing OCR items",
		})

		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodPatch, "/prescriptions/"+prescriptionID.String()+"/verify", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d", w.Code)
		}
		if capturedStatus != "approved" {
			t.Errorf("expected status approved, got %s", capturedStatus)
		}
		if capturedItems != nil {
			t.Errorf("expected nil items to retain DB OCR items, got: %v", capturedItems)
		}
	})

	t.Run("Invalid Status Rejection", func(t *testing.T) {
		h := &Handler{Repo: &repository.MockRepository{}}
		r := gin.New()
		r.PATCH("/prescriptions/:id/verify", func(c *gin.Context) {
			c.Set("userID", doctorID)
			h.VerifyPrescription(c)
		})

		body, _ := json.Marshal(map[string]string{
			"status": "in_progress",
		})

		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodPatch, "/prescriptions/"+prescriptionID.String()+"/verify", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 Bad Request for invalid status, got %d", w.Code)
		}
	})

	t.Run("Prescription Not Found or Access Denied", func(t *testing.T) {
		mockRepo := &repository.MockRepository{
			VerifyPrescriptionFunc: func(ctx context.Context, pID, dID uuid.UUID, status, notes string, items []models.PrescriptionItem) (bool, error) {
				return false, nil // 0 rows updated
			},
			GetPrescriptionByIDFunc: func(ctx context.Context, id uuid.UUID) (models.Prescription, error) {
				return models.Prescription{}, errors.New("not found")
			},
		}

		h := &Handler{Repo: mockRepo}
		r := gin.New()
		r.PATCH("/prescriptions/:id/verify", func(c *gin.Context) {
			c.Set("userID", doctorID)
			h.VerifyPrescription(c)
		})

		body, _ := json.Marshal(VerifyPrescriptionRequest{
			Status: "approved",
		})

		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodPatch, "/prescriptions/"+prescriptionID.String()+"/verify", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)

		if w.Code != http.StatusNotFound {
			t.Fatalf("expected 404 Not Found, got %d", w.Code)
		}
	})

	t.Run("Verification Conflict on Pending OCR", func(t *testing.T) {
		mockRepo := &repository.MockRepository{
			VerifyPrescriptionFunc: func(ctx context.Context, pID, dID uuid.UUID, status, notes string, items []models.PrescriptionItem) (bool, error) {
				return false, nil // 0 rows updated because status != needs_review
			},
			GetPrescriptionByIDFunc: func(ctx context.Context, id uuid.UUID) (models.Prescription, error) {
				return models.Prescription{
					ID:       id,
					DoctorID: doctorID,
					Status:   "pending_ocr",
				}, nil
			},
		}

		h := &Handler{Repo: mockRepo}
		r := gin.New()
		r.PATCH("/prescriptions/:id/verify", func(c *gin.Context) {
			c.Set("userID", doctorID)
			h.VerifyPrescription(c)
		})

		body, _ := json.Marshal(VerifyPrescriptionRequest{
			Status: "approved",
		})

		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodPatch, "/prescriptions/"+prescriptionID.String()+"/verify", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)

		if w.Code != http.StatusConflict {
			t.Fatalf("expected 409 Conflict when OCR is pending, got %d: %s", w.Code, w.Body.String())
		}
	})

	t.Run("Verification Conflict on Already Approved", func(t *testing.T) {
		mockRepo := &repository.MockRepository{
			VerifyPrescriptionFunc: func(ctx context.Context, pID, dID uuid.UUID, status, notes string, items []models.PrescriptionItem) (bool, error) {
				return false, nil
			},
			GetPrescriptionByIDFunc: func(ctx context.Context, id uuid.UUID) (models.Prescription, error) {
				return models.Prescription{
					ID:       id,
					DoctorID: doctorID,
					Status:   "approved",
				}, nil
			},
		}

		h := &Handler{Repo: mockRepo}
		r := gin.New()
		r.PATCH("/prescriptions/:id/verify", func(c *gin.Context) {
			c.Set("userID", doctorID)
			h.VerifyPrescription(c)
		})

		body, _ := json.Marshal(VerifyPrescriptionRequest{
			Status: "approved",
		})

		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodPatch, "/prescriptions/"+prescriptionID.String()+"/verify", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)

		if w.Code != http.StatusConflict {
			t.Fatalf("expected 409 Conflict when already approved, got %d: %s", w.Code, w.Body.String())
		}
	})

	t.Run("Verification Conflict on Already Rejected", func(t *testing.T) {
		mockRepo := &repository.MockRepository{
			VerifyPrescriptionFunc: func(ctx context.Context, pID, dID uuid.UUID, status, notes string, items []models.PrescriptionItem) (bool, error) {
				return false, nil
			},
			GetPrescriptionByIDFunc: func(ctx context.Context, id uuid.UUID) (models.Prescription, error) {
				return models.Prescription{
					ID:       id,
					DoctorID: doctorID,
					Status:   "rejected",
				}, nil
			},
		}

		h := &Handler{Repo: mockRepo}
		r := gin.New()
		r.PATCH("/prescriptions/:id/verify", func(c *gin.Context) {
			c.Set("userID", doctorID)
			h.VerifyPrescription(c)
		})

		body, _ := json.Marshal(VerifyPrescriptionRequest{
			Status: "approved",
		})

		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodPatch, "/prescriptions/"+prescriptionID.String()+"/verify", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)

		if w.Code != http.StatusConflict {
			t.Fatalf("expected 409 Conflict when already rejected, got %d: %s", w.Code, w.Body.String())
		}
	})

	t.Run("Rejection of Nil Items Approval when OCR Detected Nothing", func(t *testing.T) {
		mockRepo := &repository.MockRepository{
			GetPrescriptionByIDFunc: func(ctx context.Context, id uuid.UUID) (models.Prescription, error) {
				return models.Prescription{
					ID:     id,
					Status: "needs_review",
					Items:  []models.PrescriptionItem{}, // OCR found nothing
				}, nil
			},
		}

		h := &Handler{Repo: mockRepo}
		r := gin.New()
		r.PATCH("/prescriptions/:id/verify", func(c *gin.Context) {
			c.Set("userID", doctorID)
			h.VerifyPrescription(c)
		})

		body, _ := json.Marshal(map[string]interface{}{
			"status": "approved",
		})

		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodPatch, "/prescriptions/"+prescriptionID.String()+"/verify", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 Bad Request when approving 0-item prescription without supplying items, got %d", w.Code)
		}
	})
}

func TestDownloadPrescription_UnapprovedBlocked(t *testing.T) {
	gin.SetMode(gin.TestMode)
	patientID := uuid.New()
	filename := "prescription-" + patientID.String() + "-123.pdf"

	mockRepo := &repository.MockRepository{
		GetPrescriptionByFilenameFunc: func(ctx context.Context, pID uuid.UUID, fn string) (models.Prescription, error) {
			// Query returned no rows because status != 'approved'
			return models.Prescription{}, pgx.ErrNoRows
		},
	}

	h := &Handler{
		Repo:    mockRepo,
		Storage: storage.NewMockProvider(),
	}

	r := gin.New()
	r.GET("/prescriptions/:filename/download-url", func(c *gin.Context) {
		c.Set("userID", patientID)
		c.Set("role", "patient")
		h.DownloadPrescription(c)
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/prescriptions/"+filename+"/download-url", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 Not Found for unapproved prescription download, got %d: %s", w.Code, w.Body.String())
	}

}

func TestGetPrescriptionByID_Endpoint(t *testing.T) {
	gin.SetMode(gin.TestMode)
	doctorID := uuid.New()
	patientID := uuid.New()
	prescID := uuid.New()

	samplePrescription := models.Prescription{
		ID:        prescID,
		PatientID: patientID,
		DoctorID:  doctorID,
		Status:    "approved",
		Source:    "digital",
		Items: []models.PrescriptionItem{
			{MedicationName: "Metformin", Dosage: "500mg"},
		},
	}

	t.Run("Success as Authoring Doctor", func(t *testing.T) {
		mockRepo := &repository.MockRepository{
			GetPrescriptionByIDFunc: func(ctx context.Context, id uuid.UUID) (models.Prescription, error) {
				return samplePrescription, nil
			},
		}
		h := &Handler{Repo: mockRepo}
		r := gin.New()
		r.GET("/prescriptions/:id", func(c *gin.Context) {
			c.Set("userID", doctorID)
			c.Set("role", "doctor")
			h.GetPrescriptionByID(c)
		})

		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodGet, "/prescriptions/"+prescID.String(), nil)
		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d: %s", w.Code, w.Body.String())
		}
	})

	t.Run("Success as Patient of Record", func(t *testing.T) {
		mockRepo := &repository.MockRepository{
			GetPrescriptionByIDFunc: func(ctx context.Context, id uuid.UUID) (models.Prescription, error) {
				return samplePrescription, nil
			},
		}
		h := &Handler{Repo: mockRepo}
		r := gin.New()
		r.GET("/prescriptions/:id", func(c *gin.Context) {
			c.Set("userID", patientID)
			c.Set("role", "patient")
			h.GetPrescriptionByID(c)
		})

		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodGet, "/prescriptions/"+prescID.String(), nil)
		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d: %s", w.Code, w.Body.String())
		}
	})

	t.Run("Forbidden / Not Found for Unauthorized Doctor", func(t *testing.T) {
		unrelatedDoctorID := uuid.New()
		mockRepo := &repository.MockRepository{
			GetPrescriptionByIDFunc: func(ctx context.Context, id uuid.UUID) (models.Prescription, error) {
				return samplePrescription, nil
			},
			GetAppointmentsForPatientFunc: func(ctx context.Context, dID, pID uuid.UUID, limit, offset int) ([]models.Appointment, error) {
				return []models.Appointment{}, nil // No appointments with patient
			},
		}
		h := &Handler{Repo: mockRepo}
		r := gin.New()
		r.GET("/prescriptions/:id", func(c *gin.Context) {
			c.Set("userID", unrelatedDoctorID)
			c.Set("role", "doctor")
			h.GetPrescriptionByID(c)
		})

		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodGet, "/prescriptions/"+prescID.String(), nil)
		r.ServeHTTP(w, req)

		if w.Code != http.StatusNotFound {
			t.Fatalf("expected 404 Not Found for unauthorized doctor, got %d", w.Code)
		}
	})

}

func TestGetPrescriptionUploadURL_Authorization(t *testing.T) {
	gin.SetMode(gin.TestMode)
	doctorID := uuid.New()
	patientID := uuid.New()
	mockStorage := storage.NewMockProvider()

	t.Run("Success Authorized Doctor", func(t *testing.T) {
		mockRepo := &repository.MockRepository{
			GetPatientByIDFunc: func(ctx context.Context, id uuid.UUID) (models.Patient, error) {
				return models.Patient{ID: id}, nil
			},
			GetAppointmentsForPatientFunc: func(ctx context.Context, dID, pID uuid.UUID, limit, offset int) ([]models.Appointment, error) {
				return []models.Appointment{{ID: uuid.New()}}, nil
			},
		}
		h := &Handler{Repo: mockRepo, Storage: mockStorage}
		r := gin.New()
		r.POST("/prescriptions/upload-url", func(c *gin.Context) {
			c.Set("userID", doctorID)
			c.Set("role", "doctor")
			h.GetPrescriptionUploadURL(c)
		})

		body, _ := json.Marshal(map[string]interface{}{
			"patient_id":   patientID.String(),
			"filename":     "prescription.pdf",
			"content_type": "application/pdf",
		})
		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodPost, "/prescriptions/upload-url", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d: %s", w.Code, w.Body.String())
		}
	})

	t.Run("Non-existent Patient 404", func(t *testing.T) {
		mockRepo := &repository.MockRepository{
			GetPatientByIDFunc: func(ctx context.Context, id uuid.UUID) (models.Patient, error) {
				return models.Patient{}, errors.New("not found")
			},
		}
		h := &Handler{Repo: mockRepo, Storage: mockStorage}
		r := gin.New()
		r.POST("/prescriptions/upload-url", func(c *gin.Context) {
			c.Set("userID", doctorID)
			h.GetPrescriptionUploadURL(c)
		})

		body, _ := json.Marshal(map[string]interface{}{
			"patient_id":   patientID.String(),
			"filename":     "prescription.pdf",
			"content_type": "application/pdf",
		})
		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodPost, "/prescriptions/upload-url", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)

		if w.Code != http.StatusNotFound {
			t.Fatalf("expected 404 Not Found, got %d", w.Code)
		}
	})

	t.Run("Unauthorized Doctor (No Appointments) 403", func(t *testing.T) {
		mockRepo := &repository.MockRepository{
			GetPatientByIDFunc: func(ctx context.Context, id uuid.UUID) (models.Patient, error) {
				return models.Patient{ID: id}, nil
			},
			GetAppointmentsForPatientFunc: func(ctx context.Context, dID, pID uuid.UUID, limit, offset int) ([]models.Appointment, error) {
				return []models.Appointment{}, nil
			},
		}
		h := &Handler{Repo: mockRepo, Storage: mockStorage}
		r := gin.New()
		r.POST("/prescriptions/upload-url", func(c *gin.Context) {
			c.Set("userID", doctorID)
			h.GetPrescriptionUploadURL(c)
		})

		body, _ := json.Marshal(map[string]interface{}{
			"patient_id":   patientID.String(),
			"filename":     "prescription.pdf",
			"content_type": "application/pdf",
		})
		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodPost, "/prescriptions/upload-url", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)

		if w.Code != http.StatusForbidden {
			t.Fatalf("expected 403 Forbidden, got %d", w.Code)
		}
	})
}

func TestGetPatientHistoryPrescriptions_StatusFiltering(t *testing.T) {
	gin.SetMode(gin.TestMode)
	doctorID := uuid.New()
	patientID := uuid.New()

	t.Run("Valid Status Filter Passed to Repo", func(t *testing.T) {
		var capturedStatus string
		mockRepo := &repository.MockRepository{
			GetPrescriptionsForPatientFunc: func(ctx context.Context, dID, pID uuid.UUID, status string, limit, offset int) ([]models.Prescription, error) {
				capturedStatus = status
				return []models.Prescription{}, nil
			},
		}
		h := &Handler{Repo: mockRepo}
		r := gin.New()
		r.GET("/doctor/patients/:id/prescriptions", func(c *gin.Context) {
			c.Set("userID", doctorID)
			h.GetPatientHistoryPrescriptions(c)
		})

		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodGet, "/doctor/patients/"+patientID.String()+"/prescriptions?status=approved", nil)
		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d", w.Code)
		}
		if capturedStatus != "approved" {
			t.Errorf("expected status 'approved' captured, got: %s", capturedStatus)
		}
	})

	t.Run("Invalid Status Filter 400", func(t *testing.T) {
		h := &Handler{Repo: &repository.MockRepository{}}
		r := gin.New()
		r.GET("/doctor/patients/:id/prescriptions", func(c *gin.Context) {
			c.Set("userID", doctorID)
			h.GetPatientHistoryPrescriptions(c)
		})

		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodGet, "/doctor/patients/"+patientID.String()+"/prescriptions?status=invalid_status", nil)
		r.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 Bad Request for invalid status filter, got %d", w.Code)
		}
	})
}

func TestDownloadPrescription_ByID_Resolution(t *testing.T) {
	gin.SetMode(gin.TestMode)
	patientID := uuid.New()
	prescID := uuid.New()
	actualFileName := "prescription-" + patientID.String() + "-real.pdf"

	mockStorage := storage.NewMockProvider()
	mockStorage.Files[actualFileName] = []byte("%PDF-1.4 test file")

	t.Run("Success resolving prescription UUID to file download URL", func(t *testing.T) {
		mockRepo := &repository.MockRepository{
			GetPrescriptionByIDFunc: func(ctx context.Context, id uuid.UUID) (models.Prescription, error) {
				if id == prescID {
					return models.Prescription{
						ID:        prescID,
						PatientID: patientID,
						FileName:  actualFileName,
						Status:    "approved",
					}, nil
				}
				return models.Prescription{}, pgx.ErrNoRows
			},
		}

		h := &Handler{
			Repo:    mockRepo,
			Storage: mockStorage,
		}

		r := gin.New()
		r.GET("/prescriptions/:id/download-url", func(c *gin.Context) {
			c.Set("userID", patientID)
			c.Set("role", "patient")
			h.DownloadPrescription(c)
		})

		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodGet, "/prescriptions/"+prescID.String()+"/download-url", nil)
		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d: %s", w.Code, w.Body.String())
		}
		var resp map[string]interface{}
		_ = json.Unmarshal(w.Body.Bytes(), &resp)
		if resp["download_url"] == nil || resp["download_url"] == "" {
			t.Errorf("expected non-empty download_url")
		}
	})

	t.Run("Digital prescription with no file returns 400 Bad Request", func(t *testing.T) {
		digitalPrescID := uuid.New()
		mockRepo := &repository.MockRepository{
			GetPrescriptionByIDFunc: func(ctx context.Context, id uuid.UUID) (models.Prescription, error) {
				return models.Prescription{
					ID:        digitalPrescID,
					PatientID: patientID,
					FileName:  "", // digital prescription has no uploaded file
					Status:    "approved",
				}, nil
			},
		}

		h := &Handler{
			Repo:    mockRepo,
			Storage: mockStorage,
		}

		r := gin.New()
		r.GET("/prescriptions/:id/download-url", func(c *gin.Context) {
			c.Set("userID", patientID)
			c.Set("role", "patient")
			h.DownloadPrescription(c)
		})

		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodGet, "/prescriptions/"+digitalPrescID.String()+"/download-url", nil)
		r.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 Bad Request for digital prescription with no file, got %d: %s", w.Code, w.Body.String())
		}
	})

	t.Run("Unapproved prescription returns 404", func(t *testing.T) {
		unapprovedID := uuid.New()
		mockRepo := &repository.MockRepository{
			GetPrescriptionByIDFunc: func(ctx context.Context, id uuid.UUID) (models.Prescription, error) {
				return models.Prescription{
					ID:        unapprovedID,
					PatientID: patientID,
					FileName:  actualFileName,
					Status:    "needs_review", // unapproved
				}, nil
			},
		}

		h := &Handler{
			Repo:    mockRepo,
			Storage: mockStorage,
		}

		r := gin.New()
		r.GET("/prescriptions/:id/download-url", func(c *gin.Context) {
			c.Set("userID", patientID)
			c.Set("role", "patient")
			h.DownloadPrescription(c)
		})

		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodGet, "/prescriptions/"+unapprovedID.String()+"/download-url", nil)
		r.ServeHTTP(w, req)

		if w.Code != http.StatusNotFound {
			t.Fatalf("expected 404 Not Found for unapproved prescription, got %d: %s", w.Code, w.Body.String())
		}
	})

	t.Run("Prescription belonging to different patient returns 404", func(t *testing.T) {
		diffPatientPrescID := uuid.New()
		mockRepo := &repository.MockRepository{
			GetPrescriptionByIDFunc: func(ctx context.Context, id uuid.UUID) (models.Prescription, error) {
				return models.Prescription{
					ID:        diffPatientPrescID,
					PatientID: uuid.New(), // different patient
					FileName:  actualFileName,
					Status:    "approved",
				}, nil
			},
		}

		h := &Handler{
			Repo:    mockRepo,
			Storage: mockStorage,
		}

		r := gin.New()
		r.GET("/prescriptions/:id/download-url", func(c *gin.Context) {
			c.Set("userID", patientID)
			c.Set("role", "patient")
			h.DownloadPrescription(c)
		})

		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodGet, "/prescriptions/"+diffPatientPrescID.String()+"/download-url", nil)
		r.ServeHTTP(w, req)

		if w.Code != http.StatusNotFound {
			t.Fatalf("expected 404 Not Found for different patient prescription, got %d: %s", w.Code, w.Body.String())
		}
	})
}

func TestPrescription_FieldLengthValidation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	doctorID := uuid.New()
	patientID := uuid.New()

	mockRepo := &repository.MockRepository{
		GetPatientByIDFunc: func(ctx context.Context, id uuid.UUID) (models.Patient, error) {
			return models.Patient{ID: id}, nil
		},
		GetAppointmentsForPatientFunc: func(ctx context.Context, dID, pID uuid.UUID, limit, offset int) ([]models.Appointment, error) {
			return []models.Appointment{{ID: uuid.New()}}, nil
		},
	}

	h := &Handler{
		Repo: mockRepo,
	}

	r := gin.New()
	r.POST("/prescriptions/digital", func(c *gin.Context) {
		c.Set("userID", doctorID)
		c.Set("role", "doctor")
		h.CreateDigitalPrescription(c)
	})
	r.PATCH("/prescriptions/:id/verify", func(c *gin.Context) {
		c.Set("userID", doctorID)
		c.Set("role", "doctor")
		h.VerifyPrescription(c)
	})

	t.Run("Rejects medication name exceeding 255 chars in digital prescription", func(t *testing.T) {
		longMedName := string(make([]rune, 256))
		body, _ := json.Marshal(map[string]interface{}{
			"patient_id": patientID.String(),
			"items": []map[string]interface{}{
				{"medication_name": longMedName, "dosage": "10mg"},
			},
		})
		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodPost, "/prescriptions/digital", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 Bad Request, got %d: %s", w.Code, w.Body.String())
		}
	})

	t.Run("Rejects dosage exceeding 100 chars in digital prescription", func(t *testing.T) {
		longDosage := string(make([]rune, 101))
		body, _ := json.Marshal(map[string]interface{}{
			"patient_id": patientID.String(),
			"items": []map[string]interface{}{
				{"medication_name": "Amoxicillin", "dosage": longDosage},
			},
		})
		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodPost, "/prescriptions/digital", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 Bad Request, got %d: %s", w.Code, w.Body.String())
		}
	})

	t.Run("Rejects frequency exceeding 100 chars in verification", func(t *testing.T) {
		longFreq := string(make([]rune, 101))
		body, _ := json.Marshal(map[string]interface{}{
			"status": "approved",
			"items": []map[string]interface{}{
				{"medication_name": "Amoxicillin", "frequency": longFreq},
			},
		})
		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodPatch, "/prescriptions/"+uuid.New().String()+"/verify", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 Bad Request, got %d: %s", w.Code, w.Body.String())
		}
	})
}

func TestUnifiedDownloadPrescription_DoctorAccess(t *testing.T) {
	gin.SetMode(gin.TestMode)
	doctorID := uuid.New()
	patientID := uuid.New()
	prescID := uuid.New()
	key := "prescription-" + patientID.String() + "-slip.png"

	mockStorage := storage.NewMockProvider()
	mockStorage.Files[key] = []byte("mock-image-bytes")

	t.Run("Doctor authorized via direct authorship can download unapproved slip", func(t *testing.T) {
		mockRepo := &repository.MockRepository{
			GetPrescriptionByIDFunc: func(ctx context.Context, id uuid.UUID) (models.Prescription, error) {
				return models.Prescription{
					ID:        prescID,
					PatientID: patientID,
					DoctorID:  doctorID,
					FileName:  key,
					Status:    "needs_review",
				}, nil
			},
		}

		h := &Handler{
			Repo:    mockRepo,
			Storage: mockStorage,
		}

		r := gin.New()
		r.GET("/prescriptions/:id/download-url", func(c *gin.Context) {
			c.Set("userID", doctorID)
			c.Set("role", "doctor")
			h.DownloadPrescription(c)
		})

		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodGet, "/prescriptions/"+prescID.String()+"/download-url", nil)
		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200 OK for doctor downloading pending review slip, got %d: %s", w.Code, w.Body.String())
		}
	})

	t.Run("Doctor without appointment or authorship is blocked (404)", func(t *testing.T) {
		otherDoctorID := uuid.New()
		mockRepo := &repository.MockRepository{
			GetPrescriptionByIDFunc: func(ctx context.Context, id uuid.UUID) (models.Prescription, error) {
				return models.Prescription{
					ID:        prescID,
					PatientID: patientID,
					DoctorID:  doctorID,
					FileName:  key,
					Status:    "needs_review",
				}, nil
			},
			GetAppointmentsForPatientFunc: func(ctx context.Context, dID, pID uuid.UUID, limit, offset int) ([]models.Appointment, error) {
				return []models.Appointment{}, nil // no appointment
			},
		}

		h := &Handler{
			Repo:    mockRepo,
			Storage: mockStorage,
		}

		r := gin.New()
		r.GET("/prescriptions/:id/download-url", func(c *gin.Context) {
			c.Set("userID", otherDoctorID)
			c.Set("role", "doctor")
			h.DownloadPrescription(c)
		})

		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodGet, "/prescriptions/"+prescID.String()+"/download-url", nil)
		r.ServeHTTP(w, req)

		if w.Code != http.StatusNotFound {
			t.Fatalf("expected 404 Not Found for unauthorized doctor, got %d: %s", w.Code, w.Body.String())
		}
	})
}

type mockSafetyChecker struct {
	hasAlert bool
}

func (m *mockSafetyChecker) CheckPrescriptionSafety(ctx context.Context, newMeds, activeMeds, allergies []string) (*safety.SafetyReport, error) {
	if m.hasAlert {
		return &safety.SafetyReport{
			HasHighSeverityAlerts: true,
			InteractionAlerts: []safety.InteractionAlert{
				{
					DrugA:       "Warfarin",
					DrugB:       "Aspirin",
					Severity:    safety.SeverityHigh,
					Description: "Major bleeding risk",
					Source:      "OpenFDA",
				},
			},
		}, nil
	}
	return &safety.SafetyReport{}, nil
}

func TestCreateDigitalPrescription_SafetyAndPDF(t *testing.T) {
	gin.SetMode(gin.TestMode)
	doctorID := uuid.New()
	patientID := uuid.New()

	t.Run("Blocked by High Severity Interaction without Override", func(t *testing.T) {
		mockRepo := &repository.MockRepository{
			GetPatientByIDFunc: func(ctx context.Context, id uuid.UUID) (models.Patient, error) {
				return models.Patient{ID: id, FirstName: "Jane", LastName: "Doe"}, nil
			},
			GetAppointmentsForPatientFunc: func(ctx context.Context, dID, pID uuid.UUID, limit, offset int) ([]models.Appointment, error) {
				return []models.Appointment{{ID: uuid.New(), DoctorID: dID, PatientID: pID}}, nil
			},
			GetPrescriptionsByPatientIDFunc: func(ctx context.Context, pID uuid.UUID, limit, offset int) ([]models.Prescription, error) {
				return []models.Prescription{}, nil
			},
		}

		h := &Handler{
			Repo:          mockRepo,
			SafetyChecker: &mockSafetyChecker{hasAlert: true},
		}

		r := gin.New()
		r.POST("/prescriptions/digital", func(c *gin.Context) {
			c.Set("userID", doctorID)
			c.Set("role", "doctor")
			h.CreateDigitalPrescription(c)
		})

		body, _ := json.Marshal(CreateDigitalPrescriptionRequest{
			PatientID: patientID,
			Items: []PrescriptionItemInput{
				{MedicationName: "Warfarin", Dosage: "5mg"},
			},
			OverrideSafety: false,
		})

		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodPost, "/prescriptions/digital", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)

		if w.Code != http.StatusConflict {
			t.Fatalf("expected 409 Conflict, got %d: %s", w.Code, w.Body.String())
		}
	})

	t.Run("Allowed with Doctor Safety Override and PDF Generated", func(t *testing.T) {
		mockStorage := storage.NewMockProvider()
		var linkedFileName string
		mockRepo := &repository.MockRepository{
			UpdatePrescriptionFileNameFunc: func(ctx context.Context, prescriptionID uuid.UUID, fileName string) error {
				linkedFileName = fileName
				return nil
			},
			GetPatientByIDFunc: func(ctx context.Context, id uuid.UUID) (models.Patient, error) {
				return models.Patient{ID: id, FirstName: "Jane", LastName: "Doe"}, nil
			},
			GetDoctorByIDFunc: func(ctx context.Context, id uuid.UUID) (models.Doctor, error) {
				return models.Doctor{ID: id, FirstName: "Gregory", LastName: "House"}, nil
			},
			GetAppointmentsForPatientFunc: func(ctx context.Context, dID, pID uuid.UUID, limit, offset int) ([]models.Appointment, error) {
				return []models.Appointment{{ID: uuid.New(), DoctorID: dID, PatientID: pID}}, nil
			},
			GetPrescriptionsByPatientIDFunc: func(ctx context.Context, pID uuid.UUID, limit, offset int) ([]models.Prescription, error) {
				return []models.Prescription{}, nil
			},
			CreateDigitalPrescriptionFunc: func(ctx context.Context, pID, dID uuid.UUID, notes string, items []models.PrescriptionItem) (uuid.UUID, error) {
				return uuid.New(), nil
			},
		}

		notifier := notifications.NewSSEBroker()
		defer notifier.Shutdown()

		h := &Handler{
			Repo:          mockRepo,
			Storage:       mockStorage,
			SafetyChecker: &mockSafetyChecker{hasAlert: true},
			PDFGenerator:  pdf.NewStandardPDFGenerator(),
			Notifier:      notifier,
		}

		r := gin.New()
		r.POST("/prescriptions/digital", func(c *gin.Context) {
			c.Set("userID", doctorID)
			c.Set("role", "doctor")
			h.CreateDigitalPrescription(c)
		})

		body, _ := json.Marshal(CreateDigitalPrescriptionRequest{
			PatientID: patientID,
			Items: []PrescriptionItemInput{
				{MedicationName: "Warfarin", Dosage: "5mg"},
			},
			OverrideSafety: true,
			OverrideReason: "Benefits outweigh risks; INR will be closely monitored",
		})

		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodPost, "/prescriptions/digital", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)

		if w.Code != http.StatusCreated {
			t.Fatalf("expected 201 Created, got %d: %s", w.Code, w.Body.String())
		}

		var resp map[string]interface{}
		_ = json.Unmarshal(w.Body.Bytes(), &resp)
		fileName, ok := resp["file_name"].(string)
		if !ok || fileName == "" {
			t.Fatalf("expected file_name in response, got %v", resp["file_name"])
		}

		// Verify PDF was saved to storage
		savedBytes, _, err := mockStorage.GetFileBytes(context.Background(), fileName)
		if err != nil || len(savedBytes) == 0 {
			t.Fatalf("expected saved PDF in storage, got err: %v, bytes: %d", err, len(savedBytes))
		}
		if !bytes.HasPrefix(savedBytes, []byte("%PDF-")) {
			t.Fatalf("stored file does not have PDF magic bytes")
		}
		if linkedFileName != fileName {
			t.Fatalf("expected linkedFileName to be %q, got %q", fileName, linkedFileName)
		}
	})

	t.Run("PDF Generation Failure Graceful Handling", func(t *testing.T) {
		mockRepo := &repository.MockRepository{
			GetPatientByIDFunc: func(ctx context.Context, id uuid.UUID) (models.Patient, error) {
				return models.Patient{ID: id, FirstName: "Jane", LastName: "Doe"}, nil
			},
			GetAppointmentsForPatientFunc: func(ctx context.Context, dID, pID uuid.UUID, limit, offset int) ([]models.Appointment, error) {
				return []models.Appointment{{ID: uuid.New(), DoctorID: dID, PatientID: pID}}, nil
			},
			GetPrescriptionsByPatientIDFunc: func(ctx context.Context, pID uuid.UUID, limit, offset int) ([]models.Prescription, error) {
				return []models.Prescription{}, nil
			},
			CreateDigitalPrescriptionFunc: func(ctx context.Context, pID, dID uuid.UUID, notes string, items []models.PrescriptionItem) (uuid.UUID, error) {
				return uuid.New(), nil
			},
		}

		h := &Handler{
			Repo:         mockRepo,
			PDFGenerator: &failingPDFGenerator{},
			Storage:      storage.NewMockProvider(),
		}

		r := gin.New()
		r.POST("/prescriptions/digital", func(c *gin.Context) {
			c.Set("userID", doctorID)
			c.Set("role", "doctor")
			h.CreateDigitalPrescription(c)
		})

		body, _ := json.Marshal(CreateDigitalPrescriptionRequest{
			PatientID: patientID,
			Items: []PrescriptionItemInput{
				{MedicationName: "Atorvastatin", Dosage: "20mg"},
			},
		})

		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodPost, "/prescriptions/digital", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)

		if w.Code != http.StatusCreated {
			t.Fatalf("expected 201 Created even if PDF fails, got %d: %s", w.Code, w.Body.String())
		}

		var resp map[string]interface{}
		_ = json.Unmarshal(w.Body.Bytes(), &resp)
		if fn, exists := resp["file_name"]; exists && fn != nil && fn != "" {
			t.Fatalf("expected no file_name in response when PDF fails, got %v", fn)
		}
	})
}

func TestStreamNotifications(t *testing.T) {
	gin.SetMode(gin.TestMode)
	notifier := notifications.NewSSEBroker()
	defer notifier.Shutdown()

	userID := uuid.New()
	h := &Handler{Notifier: notifier}

	r := gin.New()
	r.GET("/notifications/stream", func(c *gin.Context) {
		c.Set("userID", userID)
		h.StreamNotifications(c)
	})

	w := httptest.NewRecorder()
	ctx, cancel := context.WithCancel(context.Background())
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "/notifications/stream", nil)

	go func() {
		time.Sleep(50 * time.Millisecond)
		notifier.Publish(notifications.NotificationEvent{
			Type:     notifications.EventPrescriptionApproved,
			DoctorID: userID,
			Message:  "Live SSE event test",
		})
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	r.ServeHTTP(w, req)

	if w.Header().Get("Content-Type") != "text/event-stream" {
		t.Fatalf("expected text/event-stream content type, got: %s", w.Header().Get("Content-Type"))
	}
	bodyStr := w.Body.String()
	if !strings.Contains(bodyStr, "prescription.approved") {
		t.Fatalf("expected streamed event in body, got: %s", bodyStr)
	}
}

func TestAuthMiddleware_QueryToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	secret := []byte("test-secret-key-1234567890123456")
	userID := uuid.New()

	// Generate valid token
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub":  userID.String(),
		"role": "doctor",
		"exp":  time.Now().Add(time.Hour).Unix(),
	})
	tokenStr, _ := token.SignedString(secret)

	// 1. Standard AuthMiddleware must strictly REJECT query token
	r1 := gin.New()
	r1.Use(AuthMiddleware(secret))
	r1.GET("/protected", func(c *gin.Context) {
		uid, _ := c.Get("userID")
		c.JSON(http.StatusOK, gin.H{"user_id": uid})
	})

	w1 := httptest.NewRecorder()
	req1, _ := http.NewRequest(http.MethodGet, "/protected?token="+tokenStr, nil)
	r1.ServeHTTP(w1, req1)

	if w1.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized for AuthMiddleware with query token, got %d", w1.Code)
	}

	// 2. SSEAuthMiddleware must ACCEPT query token specifically for EventSource streams
	r2 := gin.New()
	r2.Use(SSEAuthMiddleware(secret))
	r2.GET("/notifications/stream", func(c *gin.Context) {
		uid, _ := c.Get("userID")
		c.JSON(http.StatusOK, gin.H{"user_id": uid})
	})

	w2 := httptest.NewRecorder()
	req2, _ := http.NewRequest(http.MethodGet, "/notifications/stream?token="+tokenStr, nil)
	r2.ServeHTTP(w2, req2)

	if w2.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for SSEAuthMiddleware with query token, got %d: %s", w2.Code, w2.Body.String())
	}
}

func TestCheckSafety_IgnoresRejectedAndNeedsReviewPrescriptions(t *testing.T) {
	patientID := uuid.New()
	var checkedActiveMeds []string

	mockRepo := &repository.MockRepository{
		GetPrescriptionsByPatientIDFunc: func(ctx context.Context, pID uuid.UUID, limit, offset int) ([]models.Prescription, error) {
			return []models.Prescription{
				{
					ID:     uuid.New(),
					Status: "rejected",
					Items:  []models.PrescriptionItem{{MedicationName: "OldRejectedMed"}},
				},
				{
					ID:     uuid.New(),
					Status: "needs_review",
					Items:  []models.PrescriptionItem{{MedicationName: "UnverifiedOCRMed"}},
				},
				{
					ID:     uuid.New(),
					Status: "approved",
					Items:  []models.PrescriptionItem{{MedicationName: "ActiveApprovedMed"}},
				},
			}, nil
		},
	}

	checker := &captureSafetyChecker{
		onCheck: func(active []string) {
			checkedActiveMeds = active
		},
	}

	h := &Handler{
		Repo:          mockRepo,
		SafetyChecker: checker,
	}

	_, _ = h.checkSafety(context.Background(), patientID, []models.PrescriptionItem{{MedicationName: "NewMed"}})

	if len(checkedActiveMeds) != 1 || checkedActiveMeds[0] != "ActiveApprovedMed" {
		t.Fatalf("expected only approved medication in safety check active list, got: %v", checkedActiveMeds)
	}
}

type captureSafetyChecker struct {
	onCheck func(active []string)
}

func (c *captureSafetyChecker) CheckPrescriptionSafety(ctx context.Context, newMeds []string, activeMeds []string, allergies []string) (*safety.SafetyReport, error) {
	if c.onCheck != nil {
		c.onCheck(activeMeds)
	}
	return &safety.SafetyReport{}, nil
}

func TestVerifyPrescription_PreserveUploadedFileNameAndSafetyOverride(t *testing.T) {
	gin.SetMode(gin.TestMode)
	doctorID := uuid.New()
	patientID := uuid.New()
	prescriptionID := uuid.New()

	var capturedNotes string
	var updateFileNameCalled bool

	mockRepo := &repository.MockRepository{
		GetPrescriptionByIDFunc: func(ctx context.Context, id uuid.UUID) (models.Prescription, error) {
			return models.Prescription{
				ID:        prescriptionID,
				DoctorID:  doctorID,
				PatientID: patientID,
				Source:    "uploaded",
				Status:    "needs_review",
				FileName:  "patient-uploaded-scan.png",
				Items: []models.PrescriptionItem{
					{MedicationName: "Warfarin", Dosage: "5mg"},
				},
			}, nil
		},
		VerifyPrescriptionFunc: func(ctx context.Context, pID, dID uuid.UUID, status, notes string, items []models.PrescriptionItem) (bool, error) {
			capturedNotes = notes
			return true, nil
		},
		UpdatePrescriptionFileNameFunc: func(ctx context.Context, pID uuid.UUID, fileName string) error {
			updateFileNameCalled = true
			return nil
		},
	}

	h := &Handler{
		Repo:          mockRepo,
		SafetyChecker: &mockSafetyChecker{hasAlert: true},
	}

	r := gin.New()
	r.PATCH("/prescriptions/:id/verify", func(c *gin.Context) {
		c.Set("userID", doctorID)
		c.Set("role", "doctor")
		h.VerifyPrescription(c)
	})

	body, _ := json.Marshal(VerifyPrescriptionRequest{
		Status:         "approved",
		Notes:          "Initial clinical observation",
		OverrideSafety: true,
		OverrideReason: "INR will be monitored weekly",
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPatch, "/prescriptions/"+prescriptionID.String()+"/verify", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", w.Code, w.Body.String())
	}

	// Verify override reason was appended to notes
	expectedNoteSnippet := "[Safety Override: INR will be monitored weekly]"
	if !strings.Contains(capturedNotes, expectedNoteSnippet) {
		t.Fatalf("expected notes to contain %q, got: %q", expectedNoteSnippet, capturedNotes)
	}

	// Verify UpdatePrescriptionFileName was NOT called, preserving the uploaded scan file
	if updateFileNameCalled {
		t.Fatalf("UpdatePrescriptionFileName was unexpectedly called for an uploaded prescription!")
	}
}

type failingPDFGenerator struct{}

func (f *failingPDFGenerator) GeneratePrescriptionPDF(data pdf.PrescriptionData) ([]byte, error) {
	return nil, errors.New("simulated pdf generator failure")
}

func TestCreateDigitalPrescription_OverrideRequiresReason(t *testing.T) {
	gin.SetMode(gin.TestMode)
	doctorID := uuid.New()
	patientID := uuid.New()

	mockRepo := &repository.MockRepository{
		GetPatientByIDFunc: func(ctx context.Context, id uuid.UUID) (models.Patient, error) {
			return models.Patient{ID: patientID, FirstName: "Alice", LastName: "Smith"}, nil
		},
		GetAppointmentsForPatientFunc: func(ctx context.Context, dID, pID uuid.UUID, limit, offset int) ([]models.Appointment, error) {
			return []models.Appointment{{ID: uuid.New()}}, nil
		},
		CreateDigitalPrescriptionFunc: func(ctx context.Context, pID, dID uuid.UUID, notes string, items []models.PrescriptionItem) (uuid.UUID, error) {
			return uuid.New(), nil
		},
	}

	h := &Handler{
		Repo:          mockRepo,
		SafetyChecker: &mockSafetyChecker{hasAlert: true},
	}

	r := gin.New()
	r.POST("/prescriptions/digital", func(c *gin.Context) {
		c.Set("userID", doctorID)
		c.Set("role", "doctor")
		h.CreateDigitalPrescription(c)
	})

	// 1. Override without reason -> 400 Bad Request
	bodyNoReason, _ := json.Marshal(CreateDigitalPrescriptionRequest{
		PatientID:      patientID,
		Items:          []PrescriptionItemInput{{MedicationName: "Warfarin", Dosage: "5mg"}},
		OverrideSafety: true,
		OverrideReason: "   ",
	})
	w1 := httptest.NewRecorder()
	req1, _ := http.NewRequest(http.MethodPost, "/prescriptions/digital", bytes.NewReader(bodyNoReason))
	req1.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w1, req1)

	if w1.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request when override_reason is whitespace, got %d: %s", w1.Code, w1.Body.String())
	}

	// 2. Override with reason -> 201 Created
	bodyWithReason, _ := json.Marshal(CreateDigitalPrescriptionRequest{
		PatientID:      patientID,
		Items:          []PrescriptionItemInput{{MedicationName: "Warfarin", Dosage: "5mg"}},
		OverrideSafety: true,
		OverrideReason: "Monitored clinic protocol",
	})
	w2 := httptest.NewRecorder()
	req2, _ := http.NewRequest(http.MethodPost, "/prescriptions/digital", bytes.NewReader(bodyWithReason))
	req2.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w2, req2)

	if w2.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created with valid override_reason, got %d: %s", w2.Code, w2.Body.String())
	}
}

func TestVerifyPrescription_OverrideRequiresReason(t *testing.T) {
	gin.SetMode(gin.TestMode)
	doctorID := uuid.New()
	patientID := uuid.New()
	prescriptionID := uuid.New()

	mockRepo := &repository.MockRepository{
		GetPrescriptionByIDFunc: func(ctx context.Context, id uuid.UUID) (models.Prescription, error) {
			return models.Prescription{
				ID:        prescriptionID,
				DoctorID:  doctorID,
				PatientID: patientID,
				Status:    "needs_review",
				Items:     []models.PrescriptionItem{{MedicationName: "Warfarin"}},
			}, nil
		},
	}

	h := &Handler{
		Repo:          mockRepo,
		SafetyChecker: &mockSafetyChecker{hasAlert: true},
	}

	r := gin.New()
	r.PATCH("/prescriptions/:id/verify", func(c *gin.Context) {
		c.Set("userID", doctorID)
		c.Set("role", "doctor")
		h.VerifyPrescription(c)
	})

	bodyNoReason, _ := json.Marshal(VerifyPrescriptionRequest{
		Status:         "approved",
		OverrideSafety: true,
		OverrideReason: "",
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPatch, "/prescriptions/"+prescriptionID.String()+"/verify", bytes.NewReader(bodyNoReason))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request when override_reason is empty on verify, got %d: %s", w.Code, w.Body.String())
	}
}

func TestVerifyPrescription_ApprovedNonExistent_Returns404Fast(t *testing.T) {
	gin.SetMode(gin.TestMode)
	doctorID := uuid.New()
	prescriptionID := uuid.New()

	verifyCalled := false
	mockRepo := &repository.MockRepository{
		GetPrescriptionByIDFunc: func(ctx context.Context, id uuid.UUID) (models.Prescription, error) {
			return models.Prescription{}, errors.New("sql: no rows in result set")
		},
		VerifyPrescriptionFunc: func(ctx context.Context, pID, dID uuid.UUID, status, notes string, items []models.PrescriptionItem) (bool, error) {
			verifyCalled = true
			return false, nil
		},
	}

	h := &Handler{Repo: mockRepo}
	r := gin.New()
	r.PATCH("/prescriptions/:id/verify", func(c *gin.Context) {
		c.Set("userID", doctorID)
		c.Set("role", "doctor")
		h.VerifyPrescription(c)
	})

	body, _ := json.Marshal(VerifyPrescriptionRequest{
		Status: "approved",
		Items: []PrescriptionItemInput{
			{MedicationName: "Amoxicillin", Dosage: "500mg"},
		},
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPatch, "/prescriptions/"+prescriptionID.String()+"/verify", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 Not Found, got %d: %s", w.Code, w.Body.String())
	}
	if verifyCalled {
		t.Fatalf("VerifyPrescription should not have been called when prescription does not exist")
	}
}
