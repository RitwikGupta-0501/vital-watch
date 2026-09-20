package models

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestAuthenticatableInterfaces(t *testing.T) {
	testID := uuid.New()
	p := Patient{
		ID:             testID,
		Email:          "patient@example.com",
		HashedPassword: "hashedpass123",
	}

	var auth Authenticatable = p
	if auth.GetID() != testID {
		t.Fatalf("expected patient ID %v, got %v", testID, auth.GetID())
	}
	if auth.GetRole() != "patient" {
		t.Fatalf("expected role patient, got %s", auth.GetRole())
	}

	d := Doctor{
		ID:             testID,
		Email:          "doc@example.com",
		HashedPassword: "hashedpass456",
	}
	auth = d
	if auth.GetID() != testID {
		t.Fatalf("expected doctor ID %v, got %v", testID, auth.GetID())
	}
	if auth.GetRole() != "doctor" {
		t.Fatalf("expected role doctor, got %s", auth.GetRole())
	}
}

func TestModelJSONSnakeCase(t *testing.T) {
	appt := Appointment{
		ID:              uuid.New(),
		DoctorID:        uuid.New(),
		PatientID:       uuid.New(),
		StartTime:       time.Now(),
		EndTime:         time.Now().Add(time.Hour),
		Status:          "upcoming",
		Type:            "in-person",
		DoctorName:      "Dr. Smith",
		DoctorSpecialty: "Cardiology",
		PatientName:     "John Doe",
	}

	data, err := json.Marshal(appt)
	if err != nil {
		t.Fatalf("failed to marshal appointment: %v", err)
	}

	jsonStr := string(data)
	expectedKeys := []string{"doctor_id", "patient_id", "doctor_name", "doctor_specialty", "patient_name"}
	for _, key := range expectedKeys {
		if !strings.Contains(jsonStr, `"`+key+`"`) {
			t.Errorf("expected JSON to contain snake_case key '%s', got: %s", key, jsonStr)
		}
	}
}

func TestPrescriptionJSONSnakeCase(t *testing.T) {
	item := PrescriptionItem{
		ID:             uuid.New(),
		PrescriptionID: uuid.New(),
		MedicationName: "Amoxicillin",
		Dosage:         "500mg",
		Frequency:      "TID",
		CreatedAt:      time.Now(),
	}

	p := Prescription{
		ID:          uuid.New(),
		PatientID:   uuid.New(),
		DoctorID:    uuid.New(),
		Source:      "uploaded",
		Status:      "pending_ocr",
		OCRProvider: "gemini",
		Items:       []PrescriptionItem{item},
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
		DoctorName:  "Dr. Jane",
		PatientName: "John Doe",
	}

	data, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("failed to marshal prescription: %v", err)
	}

	jsonStr := string(data)
	expectedKeys := []string{"patient_id", "doctor_id", "ocr_provider", "items", "medication_name", "doctor_name", "patient_name"}
	for _, key := range expectedKeys {
		if !strings.Contains(jsonStr, `"`+key+`"`) {
			t.Errorf("expected JSON to contain snake_case key '%s', got: %s", key, jsonStr)
		}
	}
}
