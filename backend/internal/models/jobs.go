package models

import "github.com/google/uuid"

// PrescriptionOCRArgs defines the River job payload for asynchronous OCR processing
type PrescriptionOCRArgs struct {
	PrescriptionID uuid.UUID `json:"prescription_id"`
	StorageKey     string    `json:"storage_key"`
}

func (PrescriptionOCRArgs) Kind() string { return "prescription_ocr" }
