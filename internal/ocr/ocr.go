package ocr

import (
	"context"
	"errors"
)

var (
	ErrUnsupportedMIME    = errors.New("unsupported MIME type for OCR provider")
	ErrAllProvidersFailed = errors.New("all configured OCR providers failed or exhausted")
	ErrOCRDisabled        = errors.New("OCR processing is disabled")
	ErrPermanent          = errors.New("permanent unrecoverable OCR failure")
)

// ExtractedMedication represents a single medication item extracted from a prescription slip
type ExtractedMedication struct {
	MedicationName string `json:"medication_name"`
	Dosage         string `json:"dosage,omitempty"`
	Frequency      string `json:"frequency,omitempty"`
	Duration       string `json:"duration,omitempty"`
	Timing         string `json:"timing,omitempty"`
	Instructions   string `json:"instructions,omitempty"`
}

// ExtractedPrescription represents the full clinical extraction result
type ExtractedPrescription struct {
	Medications []ExtractedMedication `json:"medications"`
	Confidence  float64               `json:"confidence,omitempty"`
	Provider    string                `json:"provider,omitempty"`
}

// Provider defines the interface for Vision AI OCR extraction
type Provider interface {
	Name() string
	SupportsMIME(mimeType string) bool
	ExtractPrescription(ctx context.Context, fileBytes []byte, mimeType string) (*ExtractedPrescription, error)
}
