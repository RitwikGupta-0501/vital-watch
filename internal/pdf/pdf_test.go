package pdf

import (
	"bytes"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/RitwikGupta-0501/vital-watch/internal/models"
)

func TestGeneratePrescriptionPDF(t *testing.T) {
	generator := NewStandardPDFGenerator()

	rxID := uuid.New()
	data := PrescriptionData{
		PrescriptionID:  rxID,
		Date:            time.Now(),
		DoctorName:      "Dr. Sarah Jenkins, MD",
		DoctorSpecialty: "Cardiology",
		DoctorEmail:     "sarah.jenkins@hospital.org",
		PatientName:     "John Doe",
		PatientEmail:    "john.doe@gmail.com",
		Notes:           "Take with food. Monitor blood pressure daily in the morning.",
		VerificationURL: "https://vitalwatch.internal/verify/rx/" + rxID.String(),
		Items: []models.PrescriptionItem{
			{
				MedicationName: "Atorvastatin",
				Dosage:         "20mg",
				Frequency:      "OD",
				Duration:       "30 days",
				Timing:         "Night",
				Instructions:   "Take with water before bed",
			},
			{
				MedicationName: "Aspirin",
				Dosage:         "81mg",
				Frequency:      "OD",
				Duration:       "30 days",
				Timing:         "Morning",
				Instructions:   "After breakfast",
			},
		},
	}

	pdfBytes, err := generator.GeneratePrescriptionPDF(data)
	if err != nil {
		t.Fatalf("failed to generate prescription PDF: %v", err)
	}

	if len(pdfBytes) == 0 {
		t.Fatalf("generated PDF byte slice is empty")
	}

	// Verify PDF magic header
	if !bytes.HasPrefix(pdfBytes, []byte("%PDF-")) {
		t.Fatalf("generated data does not have PDF magic header (got prefix %q)", string(pdfBytes[:8]))
	}
}

func TestGeneratePrescriptionPDF_EmptyItems(t *testing.T) {
	generator := NewStandardPDFGenerator()
	data := PrescriptionData{
		PrescriptionID: uuid.New(),
		DoctorName:     "Dr. Alan Turing",
		PatientName:    "Ada Lovelace",
		Notes:          "Rest and hydration",
		Items:          []models.PrescriptionItem{},
	}

	pdfBytes, err := generator.GeneratePrescriptionPDF(data)
	if err != nil {
		t.Fatalf("failed to generate PDF with empty items: %v", err)
	}

	if len(pdfBytes) < 100 {
		t.Fatalf("generated PDF is unexpectedly small (%d bytes)", len(pdfBytes))
	}
}

func TestGeneratePrescriptionPDF_UnicodeAndRuneClamping(t *testing.T) {
	generator := NewStandardPDFGenerator()
	data := PrescriptionData{
		PrescriptionID:  uuid.New(),
		DoctorName:      "Dr. José González Müller",
		DoctorSpecialty: "Endocrinology",
		PatientName:     "François Hélène d'Orléans",
		Notes:           "Monitor blood glucose daily — keep notes.",
		Items: []models.PrescriptionItem{
			{
				MedicationName: "Levothyroxine Sodium Supercalifragilistic",
				Dosage:         "125µg / morning",
				Frequency:      "Once daily at sunrise",
				Duration:       "30 consecutive days",
				Timing:         "Early morning",
				Instructions:   "Take with full glass of water 30 minutes before meal",
			},
		},
	}

	pdfBytes, err := generator.GeneratePrescriptionPDF(data)
	if err != nil {
		t.Fatalf("failed to generate PDF with unicode and clamped items: %v", err)
	}

	if !bytes.HasPrefix(pdfBytes, []byte("%PDF-")) {
		t.Fatalf("expected valid PDF header")
	}
}

func TestGeneratePrescriptionPDF_LongInstructionsAndMultiLine(t *testing.T) {
	generator := NewStandardPDFGenerator()
	data := PrescriptionData{
		PrescriptionID:  uuid.New(),
		DoctorName:      "Dr. Gregory House, MD",
		DoctorSpecialty: "Diagnostics",
		PatientName:     "James Wilson",
		Notes:           "Patient has severe symptoms. Review full instructions carefully.",
		Items: []models.PrescriptionItem{
			{
				MedicationName: "Metoprolol Succinate Extended-Release Tablets USP",
				Dosage:         "100mg once daily in morning",
				Frequency:      "Once daily with morning meal",
				Duration:       "90 consecutive calendar days",
				Timing:         "Morning with food",
				Instructions:   "Take one tablet by mouth every morning with breakfast or immediately following food. Do not crush, chew, or divide the tablets. Drink a full 8 oz glass of water.",
			},
			{
				MedicationName: "Amoxicillin and Clavulanate Potassium Extended Release",
				Dosage:         "1000mg/62.5mg twice daily",
				Frequency:      "Twice daily every 12 hours",
				Duration:       "14 days uninterrupted",
				Timing:         "Every 12 hours with meals",
				Instructions:   "Finish the full course of therapy even if symptoms resolve earlier. Take with meals to minimize gastrointestinal discomfort.",
			},
		},
	}

	pdfBytes, err := generator.GeneratePrescriptionPDF(data)
	if err != nil {
		t.Fatalf("failed to generate PDF with long instructions: %v", err)
	}

	if !bytes.HasPrefix(pdfBytes, []byte("%PDF-")) {
		t.Fatalf("expected valid PDF header")
	}
}

func TestGeneratePrescriptionPDF_WhitespaceNameFallback(t *testing.T) {
	generator := NewStandardPDFGenerator()
	data := PrescriptionData{
		PrescriptionID: uuid.New(),
		DoctorName:     "   ",
		PatientName:    "   ",
		Items: []models.PrescriptionItem{
			{MedicationName: "Ibuprofen", Dosage: "400mg"},
		},
	}

	pdfBytes, err := generator.GeneratePrescriptionPDF(data)
	if err != nil {
		t.Fatalf("failed to generate PDF with whitespace names: %v", err)
	}

	if len(pdfBytes) < 500 {
		t.Fatalf("PDF unexpectedly small")
	}
}
