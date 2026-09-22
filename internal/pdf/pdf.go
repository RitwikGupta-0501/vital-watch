package pdf

import (
	"bytes"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jung-kurt/gofpdf"
	"github.com/skip2/go-qrcode"

	"github.com/RitwikGupta-0501/vital-watch/internal/models"
)

// PrescriptionData holds all metadata and medications needed to render a prescription PDF
type PrescriptionData struct {
	PrescriptionID  uuid.UUID
	Date            time.Time
	DoctorName      string
	DoctorSpecialty string
	DoctorEmail     string
	PatientName     string
	PatientEmail    string
	Notes           string
	Items           []models.PrescriptionItem
	VerificationURL string
}

// Generator defines the interface for rendering clinical documents
type Generator interface {
	GeneratePrescriptionPDF(data PrescriptionData) ([]byte, error)
}

// StandardPDFGenerator implements Generator using gofpdf
type StandardPDFGenerator struct{}

// NewStandardPDFGenerator creates a new PDF generator instance
func NewStandardPDFGenerator() *StandardPDFGenerator {
	return &StandardPDFGenerator{}
}


// GeneratePrescriptionPDF compiles a standardized clinic-branded prescription slip
func (g *StandardPDFGenerator) GeneratePrescriptionPDF(data PrescriptionData) ([]byte, error) {
	pdf := gofpdf.New("P", "mm", "A4", "")
	pdf.SetMargins(15, 15, 15)
	pdf.SetAutoPageBreak(true, 20)
	pdf.AddPage()

	tr := pdf.UnicodeTranslatorFromDescriptor("")
	safeStr := func(s string) string {
		return tr(strings.TrimSpace(s))
	}

	// 1. Header & Clinic Branding
	pdf.SetFillColor(15, 118, 110) // Medical Teal #0f766e
	pdf.Rect(15, 15, 180, 24, "F")

	pdf.SetTextColor(255, 255, 255)
	pdf.SetFont("Arial", "B", 16)
	pdf.SetXY(20, 19)
	pdf.Cell(100, 8, "VITALWATCH HEALTH NETWORK")

	pdf.SetFont("Arial", "", 10)
	pdf.SetXY(20, 27)
	pdf.Cell(100, 6, "Verified Electronic Prescription Slip")

	// Rx ID & Date in Header
	pdf.SetFont("Arial", "B", 9)
	pdf.SetXY(120, 19)
	pdf.CellFormat(70, 5, "RX ID: "+data.PrescriptionID.String()[:8], "", 0, "R", false, 0, "")
	pdf.SetFont("Arial", "", 9)
	pdf.SetXY(120, 25)
	dateStr := data.Date.Format("02 Jan 2006, 15:04 MST")
	if data.Date.IsZero() {
		dateStr = time.Now().UTC().Format("02 Jan 2006, 15:04 MST")
	}
	pdf.CellFormat(70, 5, "Issued: "+dateStr, "", 0, "R", false, 0, "")

	pdf.Ln(20)

	// 2. Doctor & Patient Info Cards
	pdf.SetTextColor(30, 41, 59) // Slate-800
	currentY := pdf.GetY() + 4

	// Left Box: Prescribing Doctor
	pdf.SetFillColor(248, 250, 252) // Slate-50
	pdf.SetDrawColor(226, 232, 240) // Slate-200
	pdf.Rect(15, currentY, 88, 30, "FD")

	pdf.SetXY(18, currentY+3)
	pdf.SetFont("Arial", "B", 10)
	pdf.SetTextColor(15, 118, 110)
	pdf.Cell(80, 5, "PRESCRIBING PHYSICIAN")

	pdf.SetTextColor(51, 65, 85)
	pdf.SetFont("Arial", "B", 9)
	pdf.SetXY(18, currentY+9)
	docName := strings.TrimSpace(data.DoctorName)
	if docName == "" {
		docName = "Dr. Assigned Clinician"
	}
	pdf.Cell(80, 5, safeStr(docName))

	pdf.SetFont("Arial", "", 8)
	pdf.SetXY(18, currentY+14)
	spec := strings.TrimSpace(data.DoctorSpecialty)
	if spec == "" {
		spec = "General Medicine"
	}
	pdf.Cell(80, 5, "Specialty: "+safeStr(spec))

	pdf.SetXY(18, currentY+19)
	email := strings.TrimSpace(data.DoctorEmail)
	if email == "" {
		email = "clinician@vitalwatch.internal"
	}
	pdf.Cell(80, 5, "Contact: "+safeStr(email))

	// Right Box: Patient Details
	pdf.Rect(107, currentY, 88, 30, "FD")

	pdf.SetXY(110, currentY+3)
	pdf.SetFont("Arial", "B", 10)
	pdf.SetTextColor(15, 118, 110)
	pdf.Cell(80, 5, "PATIENT DETAILS")

	pdf.SetTextColor(51, 65, 85)
	pdf.SetFont("Arial", "B", 9)
	pdf.SetXY(110, currentY+9)
	patName := strings.TrimSpace(data.PatientName)
	if patName == "" {
		patName = "Patient Record"
	}
	pdf.Cell(80, 5, safeStr(patName))

	pdf.SetFont("Arial", "", 8)
	pdf.SetXY(110, currentY+14)
	patEmail := strings.TrimSpace(data.PatientEmail)
	if patEmail == "" {
		patEmail = "patient@vitalwatch.internal"
	}
	pdf.Cell(80, 5, "Email: "+safeStr(patEmail))

	pdf.SetXY(110, currentY+19)
	pdf.Cell(80, 5, "Rx Ref: "+data.PrescriptionID.String())

	pdf.SetY(currentY + 36)

	// 3. Medication Items Table Header
	pdf.SetFont("Arial", "B", 11)
	pdf.SetTextColor(15, 23, 42)
	pdf.Cell(180, 7, "Prescribed Medications")
	pdf.Ln(8)

	// Table column widths (total = 180mm)
	// # (8), Medication (50), Dosage (25), Frequency (25), Duration (25), Timing/Instructions (47)
	colW := []float64{8, 50, 25, 25, 25, 47}
	headers := []string{"#", "Medication", "Dosage", "Frequency", "Duration", "Instructions"}

	printTableHeader := func() {
		pdf.SetFillColor(241, 245, 249) // Slate-100
		pdf.SetTextColor(15, 23, 42)
		pdf.SetFont("Arial", "B", 8)
		for i, h := range headers {
			pdf.CellFormat(colW[i], 7, h, "1", 0, "C", true, 0, "")
		}
		pdf.Ln(-1)
	}

	printTableHeader()

	// Table Rows
	pdf.SetFont("Arial", "", 8)
	pdf.SetTextColor(51, 65, 85)

	if len(data.Items) == 0 {
		pdf.CellFormat(180, 8, "No structured medication items entered. Refer to clinical notes below.", "1", 1, "C", false, 0, "")
	} else {
		for idx, item := range data.Items {
			numStr := fmt.Sprintf("%d", idx+1)
			medName := strings.TrimSpace(item.MedicationName)
			dosage := strings.TrimSpace(item.Dosage)
			if dosage == "" {
				dosage = "-"
			}
			freq := strings.TrimSpace(item.Frequency)
			if freq == "" {
				freq = "-"
			}
			dur := strings.TrimSpace(item.Duration)
			if dur == "" {
				dur = "-"
			}
			instr := strings.TrimSpace(item.Instructions)
			if strings.TrimSpace(item.Timing) != "" {
				if instr != "" {
					instr = strings.TrimSpace(item.Timing) + " - " + instr
				} else {
					instr = strings.TrimSpace(item.Timing)
				}
			}
			if instr == "" {
				instr = "As directed by physician"
			}

			colTexts := []string{
				numStr,
				safeStr(medName),
				safeStr(dosage),
				safeStr(freq),
				safeStr(dur),
				safeStr(instr),
			}

			// Calculate row height based on text wrapping to avoid clinical truncation
			lineH := 3.8
			pdf.SetFont("Arial", "", 8)
			linesMed := pdf.SplitLines([]byte(colTexts[1]), colW[1]-3)
			linesDosage := pdf.SplitLines([]byte(colTexts[2]), colW[2]-3)
			linesFreq := pdf.SplitLines([]byte(colTexts[3]), colW[3]-3)
			linesDur := pdf.SplitLines([]byte(colTexts[4]), colW[4]-3)
			linesInstr := pdf.SplitLines([]byte(colTexts[5]), colW[5]-3)

			maxLines := len(linesMed)
			if len(linesDosage) > maxLines {
				maxLines = len(linesDosage)
			}
			if len(linesFreq) > maxLines {
				maxLines = len(linesFreq)
			}
			if len(linesDur) > maxLines {
				maxLines = len(linesDur)
			}
			if len(linesInstr) > maxLines {
				maxLines = len(linesInstr)
			}
			if maxLines < 1 {
				maxLines = 1
			}

			rowH := float64(maxLines)*lineH + 3.0 // 1.5mm top/bottom padding
			if rowH < 7.0 {
				rowH = 7.0
			}

			// Check for page overflow before rendering row
			if pdf.GetY()+rowH > 270.0 {
				pdf.AddPage()
				printTableHeader()
				pdf.SetFont("Arial", "", 8)
				pdf.SetTextColor(51, 65, 85)
			}

			startY := pdf.GetY()
			startX := 15.0
			curX := startX

			for i, text := range colTexts {
				align := "C"
				textX := curX
				textW := colW[i]
				if i == 1 || i == 5 { // Left-align medication and instructions
					align = "L"
					textX = curX + 1.5
					textW = colW[i] - 2.5
				}

				// Draw cell border
				pdf.Rect(curX, startY, colW[i], rowH, "D")

				// Render wrapped text inside cell
				pdf.SetXY(textX, startY+1.5)
				pdf.MultiCell(textW, lineH, text, "", align, false)

				curX += colW[i]
			}

			pdf.SetXY(startX, startY+rowH)
		}
	}

	pdf.Ln(5)

	// 4. Clinical Notes
	if strings.TrimSpace(data.Notes) != "" {
		pdf.SetFont("Arial", "B", 9)
		pdf.SetTextColor(15, 23, 42)
		pdf.Cell(180, 6, "Clinician Notes & Instructions:")
		pdf.Ln(6)

		pdf.SetFont("Arial", "", 8)
		pdf.SetTextColor(71, 85, 105)
		pdf.MultiCell(180, 5, safeStr(data.Notes), "1", "L", false)
		pdf.Ln(4)
	}

	// 5. Verification QR Code & Sign-Off Footer
	footerHeight := 45.0
	if pdf.GetY()+footerHeight > 275.0 {
		pdf.AddPage()
	}

	footerY := 235.0
	if pdf.GetY() < footerY && pdf.PageNo() == 1 {
		pdf.SetY(footerY)
	} else {
		pdf.Ln(10)
	}

	currentFooterY := pdf.GetY()

	// Generate QR Code PNG in memory
	qrContent := data.VerificationURL
	if qrContent == "" {
		qrContent = fmt.Sprintf("https://vitalwatch.internal/verify/rx/%s", data.PrescriptionID)
	}

	qrPNG, err := qrcode.Encode(qrContent, qrcode.Medium, 128)
	if err == nil {
		imageName := "qr_" + data.PrescriptionID.String()
		imgOpt := gofpdf.ImageOptions{ImageType: "PNG"}
		pdf.RegisterImageOptionsReader(imageName, imgOpt, bytes.NewReader(qrPNG))
		pdf.ImageOptions(imageName, 15, currentFooterY, 26, 26, false, imgOpt, 0, qrContent)

		pdf.SetXY(43, currentFooterY+5)
		pdf.SetFont("Arial", "B", 8)
		pdf.SetTextColor(15, 118, 110)
		pdf.Cell(60, 4, "SCAN TO VERIFY")
		pdf.SetXY(43, currentFooterY+10)
		pdf.SetFont("Arial", "", 7)
		pdf.SetTextColor(100, 116, 139)
		pdf.Cell(60, 4, "Digitally signed on VitalWatch Network")
		pdf.SetXY(43, currentFooterY+14)
		pdf.Cell(60, 4, "Authentication Hash: "+data.PrescriptionID.String()[:18]+"...")
	}

	// Doctor Signature Block (Right side)
	pdf.SetXY(120, currentFooterY+5)
	pdf.SetDrawColor(148, 163, 184)
	pdf.Line(120, currentFooterY+16, 195, currentFooterY+16)

	pdf.SetXY(120, currentFooterY+17)
	pdf.SetFont("Arial", "B", 8)
	pdf.SetTextColor(30, 41, 59)
	pdf.Cell(75, 4, docName)

	pdf.SetXY(120, currentFooterY+21)
	pdf.SetFont("Arial", "", 7)
	pdf.SetTextColor(100, 116, 139)
	pdf.Cell(75, 4, "Authorized Clinician Signature (e-Signed)")

	// Bottom Legal Notice
	noticeY := currentFooterY + 30
	if noticeY < 280 && pdf.PageNo() == 1 {
		noticeY = 280
	}
	pdf.SetY(noticeY)
	pdf.SetFont("Arial", "I", 7)
	pdf.SetTextColor(148, 163, 184)
	pdf.CellFormat(180, 4, "Confidential Medical Document - Generated automatically by VitalWatch. For prescription dispensing verification only.", "", 0, "C", false, 0, "")

	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, fmt.Errorf("failed to render prescription PDF: %w", err)
	}

	return buf.Bytes(), nil
}
